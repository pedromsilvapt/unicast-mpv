package ipc

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/unicast/unicast-mpv/pkg/logger"
)

var (
	ErrNotConnected    = errors.New("ipc: not connected")
	ErrClosed          = errors.New("ipc: client closed")
	ErrCommandFailed   = errors.New("ipc: command failed")
	ErrRequestTimeout  = errors.New("ipc: request timed out")
)

type MPVEvent struct {
	Name string
	Data interface{}
	Raw  json.RawMessage
}

type mpvResponse struct {
	Error      string          `json:"error"`
	Data       json.RawMessage `json:"data"`
	RequestID  int             `json:"request_id"`
}

type mpvRequest struct {
	Command   []interface{} `json:"command"`
	RequestID int           `json:"request_id"`
}

type pendingRequest struct {
	resolve chan<- json.RawMessage
	reject  chan<- error
}

type IPCClient struct {
	socketPath string
	conn       net.Conn

	nextID    atomic.Int64
	mu       sync.Mutex
	pending  map[int64]*pendingRequest

	eventCh  chan MPVEvent
	closeCh  chan struct{}
	closeOnce sync.Once
	closed   atomic.Bool

	writeMu   sync.Mutex
	OnMessage func([]byte)
	log       *logger.Logger
}

func WithLogger(l *logger.Logger) IPCClientOption {
	return func(c *IPCClient) { c.log = l }
}

type IPCClientOption func(*IPCClient)

func NewIPCClient(socketPath string, opts ...IPCClientOption) *IPCClient {
	c := &IPCClient{
		socketPath: socketPath,
		pending:    make(map[int64]*pendingRequest),
		eventCh:    make(chan MPVEvent, 256),
		closeCh:     make(chan struct{}),
	}
	c.nextID.Store(1)
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *IPCClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case <-c.closeCh:
		return ErrClosed
	default:
	}

	if c.log != nil {
		c.log.Debugf("connecting to %s", c.socketPath)
	}
	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		if c.log != nil {
			c.log.Errorf("connect to %s failed: %v", c.socketPath, err)
		}
		return fmt.Errorf("ipc: connect to %s: %w", c.socketPath, err)
	}
	c.conn = conn
	if c.log != nil {
		c.log.Infof("connected to %s", c.socketPath)
	}

	go c.readLoop()
	return nil
}

func (c *IPCClient) Close() error {
	var err error
	c.closeOnce.Do(func() {
		if c.log != nil {
			c.log.Debug("closing ipc client")
		}
		c.closed.Store(true)
		close(c.closeCh)
		c.mu.Lock()
		if c.conn != nil {
			err = c.conn.Close()
		}
		c.mu.Unlock()

		c.drainPending(ErrClosed)
		close(c.eventCh)
	})
	return err
}

func (c *IPCClient) IsClosed() bool {
	return c.closed.Load()
}

func (c *IPCClient) Events() <-chan MPVEvent {
	return c.eventCh
}

func (c *IPCClient) Command(cmd string, args ...interface{}) (interface{}, error) {
	command := make([]interface{}, 0, 1+len(args))
	command = append(command, cmd)
	command = append(command, args...)
	return c.sendCommand(command)
}

func (c *IPCClient) SetProperty(property string, value interface{}) error {
	_, err := c.sendCommand([]interface{}{"set_property", property, value})
	return err
}

func (c *IPCClient) GetProperty(property string) (interface{}, error) {
	return c.sendCommand([]interface{}{"get_property", property})
}

func (c *IPCClient) AddProperty(property string, value float64) error {
	_, err := c.sendCommand([]interface{}{"add", property, value})
	return err
}

func (c *IPCClient) MultiplyProperty(property string, value float64) error {
	_, err := c.sendCommand([]interface{}{"multiply", property, value})
	return err
}

func (c *IPCClient) CycleProperty(property string) error {
	_, err := c.sendCommand([]interface{}{"cycle", property})
	return err
}

func (c *IPCClient) FreeCommand(cmd string) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.mu.Lock()
	if c.conn == nil {
		c.mu.Unlock()
		return ErrNotConnected
	}
	conn := c.conn
	c.mu.Unlock()

	_, err := fmt.Fprintf(conn, "%s\n", cmd)
	if err != nil {
		return fmt.Errorf("ipc: free command write: %w", err)
	}
	return nil
}

func (c *IPCClient) sendCommand(command []interface{}) (interface{}, error) {
	c.mu.Lock()
	select {
	case <-c.closeCh:
		c.mu.Unlock()
		return nil, ErrClosed
	default:
	}
	if c.conn == nil {
		c.mu.Unlock()
		return nil, ErrNotConnected
	}

	id := c.nextID.Add(1) - 1
	req := mpvRequest{
		Command:   command,
		RequestID: int(id),
	}

	resolveCh := make(chan json.RawMessage, 1)
	rejectCh := make(chan error, 1)
	pr := &pendingRequest{
		resolve: resolveCh,
		reject:  rejectCh,
	}
	c.pending[id] = pr

	msg, err := json.Marshal(req)
	if err != nil {
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("ipc: marshal request: %w", err)
	}
	msg = append(msg, '\n')

	conn := c.conn
	c.mu.Unlock()

	if c.log != nil {
		cmdName := ""
		if len(command) > 0 {
			if s, ok := command[0].(string); ok {
				cmdName = s
			}
		}
		c.log.Debugf("send [%d] %s", id, cmdName)
	}

	c.writeMu.Lock()
	_, err = conn.Write(msg)
	c.writeMu.Unlock()

	if err != nil {
		if c.log != nil {
			c.log.Errorf("send [%d] write failed: %v", id, err)
		}
		c.removePending(id)
		return nil, fmt.Errorf("ipc: write: %w", err)
	}

	select {
	case data := <-resolveCh:
		if c.log != nil {
			c.log.Debugf("recv [%d] success", id)
		}
		if len(data) == 0 {
			return nil, nil
		}
		var result interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("ipc: unmarshal response data: %w", err)
		}
		return result, nil
	case err := <-rejectCh:
		if c.log != nil {
			c.log.Debugf("recv [%d] error: %v", id, err)
		}
		return nil, fmt.Errorf("%w: %s", ErrCommandFailed, err)
	case <-c.closeCh:
		if c.log != nil {
			c.log.Debugf("recv [%d] cancelled (client closed)", id)
		}
		return nil, ErrClosed
	}
}

func (c *IPCClient) readLoop() {
	reader := bufio.NewReader(c.conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				if c.log != nil {
					c.log.Debug("read loop: eof, closing")
				}
				c.Close()
				return
			}
			select {
			case <-c.closeCh:
				return
			default:
			}
			if c.log != nil {
				c.log.Errorf("read loop: read error: %v", err)
			}
			c.Close()
			return
		}

		if len(line) == 0 {
			continue
		}

		if c.OnMessage != nil {
			c.OnMessage(line)
		}

		var resp mpvResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}

		if resp.RequestID != 0 {
			c.handleResponse(resp)
		} else {
			c.handleEvent(line)
		}
	}
}

func (c *IPCClient) handleResponse(resp mpvResponse) {
	id := int64(resp.RequestID)
	c.mu.Lock()
	pr, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.mu.Unlock()

	if !ok {
		if c.log != nil {
			c.log.Warnf("recv [%d]: no pending request found", id)
		}
		return
	}

	if resp.Error == "success" {
		pr.resolve <- resp.Data
	} else {
		pr.reject <- errors.New(resp.Error)
	}
}

func (c *IPCClient) handleEvent(raw json.RawMessage) {
	if c.closed.Load() {
		return
	}
	var event struct {
		Name string          `json:"event"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return
	}

	mpvEvt := MPVEvent{
		Name: event.Name,
		Raw:  raw,
	}
	if event.Data != nil {
		var data interface{}
		if err := json.Unmarshal(event.Data, &data); err == nil {
			mpvEvt.Data = data
		}
	}

	select {
	case c.eventCh <- mpvEvt:
	default:
	}
}

func (c *IPCClient) removePending(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *IPCClient) drainPending(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.log != nil && len(c.pending) > 0 {
		c.log.Debugf("draining %d pending requests: %v", len(c.pending), err)
	}
	for id, pr := range c.pending {
		pr.reject <- err
		delete(c.pending, id)
	}
}