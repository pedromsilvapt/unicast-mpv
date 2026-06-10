package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/unicast/unicast-mpv/pkg/config"
	"github.com/unicast/unicast-mpv/pkg/schema"
)

type Logger interface {
	Info(msg string)
	Debug(msg string)
	Error(msg string)
	Warn(msg string)
}

type defaultLogger struct{}

func (d *defaultLogger) Info(msg string)  { fmt.Println("[INFO] " + msg) }
func (d *defaultLogger) Debug(msg string) { fmt.Println("[DEBUG] " + msg) }
func (d *defaultLogger) Error(msg string) { fmt.Println("[ERROR] " + msg) }
func (d *defaultLogger) Warn(msg string)  { fmt.Println("[WARN] " + msg) }

type PreHook func(args []interface{}, method string, ctx map[string]interface{})
type PostHook func(args []interface{}, method string, err error, result interface{}, ctx map[string]interface{})
type EventHook func(args []interface{}, event string, ctx map[string]interface{})

type MethodEntry struct {
	Schema  schema.Schema
	Handler func(args []interface{}) (interface{}, error)
}

type hfPattern struct {
	pattern   *regexp.Regexp
	suppressMs int64
	lastEmit  int64
	suppressed int64
}

type clientConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

type Server struct {
	cfg    *config.Config
	logger Logger

	methods   map[string]*MethodEntry
	events    map[string]bool
	connSet   map[*clientConn]bool
	connMutex sync.RWMutex

	preHooks         map[string][]PreHook
	postHooks        map[string][]PostHook
	globalPreHooks   []PreHook
	globalPostHooks  []PostHook
	eventHooks       map[string][]EventHook
	globalEventHooks []EventHook

	hookMutex sync.RWMutex

	upgrader websocket.Upgrader

	hfPatterns map[string]*hfPattern
	hfMutex    sync.Mutex
	listener   net.Listener
}

func NewServer(cfg *config.Config, logger Logger) *Server {
	if logger == nil {
		logger = &defaultLogger{}
	}
	return &Server{
		cfg:       cfg,
		logger:    logger,
		methods:   make(map[string]*MethodEntry),
		events:    make(map[string]bool),
		connSet:   make(map[*clientConn]bool),
		preHooks:  make(map[string][]PreHook),
		postHooks: make(map[string][]PostHook),
		eventHooks: make(map[string][]EventHook),
		hfPatterns: make(map[string]*hfPattern),
		upgrader:  websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
	}
}

func (s *Server) Register(method string, sch schema.Schema, handler func(args []interface{}) (interface{}, error)) {
	s.methods[method] = &MethodEntry{
		Schema:  sch,
		Handler: handler,
	}
}

func (s *Server) MethodEntry(name string) *MethodEntry {
	return s.methods[name]
}

func (s *Server) RegisteredMethods() map[string]*MethodEntry {
	result := make(map[string]*MethodEntry)
	for k, v := range s.methods {
		result[k] = v
	}
	return result
}

func (s *Server) RegisterEvent(name string) {
	s.events[name] = true
}

func (s *Server) RegisteredEvents() map[string]bool {
	result := make(map[string]bool)
	for k, v := range s.events {
		result[k] = v
	}
	return result
}

func (s *Server) RegisterPreHook(method string, hook PreHook) {
	s.hookMutex.Lock()
	defer s.hookMutex.Unlock()
	s.preHooks[method] = append(s.preHooks[method], hook)
}

func (s *Server) RegisterPostHook(method string, hook PostHook) {
	s.hookMutex.Lock()
	defer s.hookMutex.Unlock()
	s.postHooks[method] = append(s.postHooks[method], hook)
}

func (s *Server) RegisterGlobalPreHook(hook PreHook) {
	s.hookMutex.Lock()
	defer s.hookMutex.Unlock()
	s.globalPreHooks = append(s.globalPreHooks, hook)
}

func (s *Server) RegisterGlobalPostHook(hook PostHook) {
	s.hookMutex.Lock()
	defer s.hookMutex.Unlock()
	s.globalPostHooks = append(s.globalPostHooks, hook)
}

func (s *Server) RegisterEventHook(event string, hook EventHook) {
	s.hookMutex.Lock()
	defer s.hookMutex.Unlock()
	s.eventHooks[event] = append(s.eventHooks[event], hook)
}

func (s *Server) RegisterGlobalEventHook(hook EventHook) {
	s.hookMutex.Lock()
	defer s.hookMutex.Unlock()
	s.globalEventHooks = append(s.globalEventHooks, hook)
}

func (s *Server) RegisterHighFrequencyPattern(pattern *regexp.Regexp, _ interface{}, suppressMs int64) {
	s.hfMutex.Lock()
	defer s.hfMutex.Unlock()
	s.hfPatterns[pattern.String()] = &hfPattern{
		pattern:    pattern,
		suppressMs: suppressMs,
	}
}

func (s *Server) addConn(cc *clientConn) {
	s.connMutex.Lock()
	s.connSet[cc] = true
	s.connMutex.Unlock()
}

func (s *Server) removeConn(cc *clientConn) {
	s.connMutex.Lock()
	delete(s.connSet, cc)
	s.connMutex.Unlock()
}

func (s *Server) Emit(event string, args ...interface{}) {
	s.hookMutex.RLock()
	globalHooks := s.globalEventHooks
	eventHooks := s.eventHooks[event]
	s.hookMutex.RUnlock()

	ctx := make(map[string]interface{})

	for _, hook := range globalHooks {
		hook(args, event, ctx)
	}
	for _, hook := range eventHooks {
		hook(args, event, ctx)
	}

	msg := jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  event,
		Params:  args,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		s.logger.Error(fmt.Sprintf("failed to marshal event %s: %v", event, err))
		return
	}

	s.connMutex.RLock()
	conns := make([]*clientConn, 0, len(s.connSet))
	for cc := range s.connSet {
		conns = append(conns, cc)
	}
	s.connMutex.RUnlock()

	for _, cc := range conns {
		cc.mu.Lock()
		err := cc.conn.WriteMessage(websocket.TextMessage, data)
		cc.mu.Unlock()
		if err != nil {
			s.logger.Error(fmt.Sprintf("failed to emit event %s to client: %v", event, err))
		}
	}
}

func (s *Server) Listen() error {
	host := s.cfg.GetString("server.address", "0.0.0.0")
	port := s.cfg.GetInt("server.port", 8080)
	addr := fmt.Sprintf("%s:%d", host, port)

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWS)
	mux.HandleFunc("/rpc", s.handleHTTP)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("server: listen %s: %w", addr, err)
	}
	s.listener = listener

	s.logger.Info(fmt.Sprintf("Server listening on %s", addr))

	return http.Serve(listener, mux)
}

func (s *Server) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

func (s *Server) handleHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	resp, _ := s.processMessage(body)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error(fmt.Sprintf("websocket upgrade error: %v", err))
		return
	}

	cc := &clientConn{conn: conn}

	s.addConn(cc)
	defer func() {
		s.removeConn(cc)
		cc.conn.Close()
	}()

	for {
		_, message, err := cc.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				s.logger.Error(fmt.Sprintf("websocket read error: %v", err))
			}
			return
		}

		s.handleMessage(cc, message)
	}
}

type jsonRPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      *json.RawMessage `json:"id"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *jsonRPCError `json:"error,omitempty"`
	ID      *json.RawMessage `json:"id"`
}

type jsonRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type jsonRPCNotification struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

func (s *Server) processMessage(message []byte) (jsonRPCResponse, bool) {
	var req jsonRPCRequest
	if err := json.Unmarshal(message, &req); err != nil {
		return jsonRPCResponse{
			JSONRPC: "2.0",
			Error:   &jsonRPCError{Code: -32700, Message: "Parse error"},
			ID:      nil,
		}, false
	}

	if req.JSONRPC != "2.0" {
		return jsonRPCResponse{
			JSONRPC: "2.0",
			Error:   &jsonRPCError{Code: -32600, Message: "Invalid Request: jsonrpc must be 2.0"},
			ID:      req.ID,
		}, false
	}

	if req.Method == "" {
		return jsonRPCResponse{
			JSONRPC: "2.0",
			Error:   &jsonRPCError{Code: -32600, Message: "Invalid Request: method is required"},
			ID:      req.ID,
		}, false
	}

	entry, ok := s.methods[req.Method]
	if !ok {
		return jsonRPCResponse{
			JSONRPC: "2.0",
			Error:   &jsonRPCError{Code: -32601, Message: fmt.Sprintf("Method not found: %s", req.Method)},
			ID:      req.ID,
		}, false
	}

	args := req.Params
	if args == nil {
		args = []interface{}{}
	}

	if entry.Schema != nil {
		if err := entry.Schema.Validate(args); err != nil {
			return jsonRPCResponse{
				JSONRPC: "2.0",
				Error:   &jsonRPCError{Code: -32602, Message: fmt.Sprintf("Invalid params: %s", err.Error())},
				ID:      req.ID,
			}, false
		}
	}

	ctx := make(map[string]interface{})

	s.hookMutex.RLock()
	globalPre := make([]PreHook, len(s.globalPreHooks))
	copy(globalPre, s.globalPreHooks)
	cmdPre := make([]PreHook, len(s.preHooks[req.Method]))
	copy(cmdPre, s.preHooks[req.Method])
	s.hookMutex.RUnlock()

	for _, hook := range globalPre {
		hook(args, req.Method, ctx)
	}
	for _, hook := range cmdPre {
		hook(args, req.Method, ctx)
	}

	result, rpcErr := entry.Handler(args)

	s.hookMutex.RLock()
	globalPost := make([]PostHook, len(s.globalPostHooks))
	copy(globalPost, s.globalPostHooks)
	cmdPost := make([]PostHook, len(s.postHooks[req.Method]))
	copy(cmdPost, s.postHooks[req.Method])
	s.hookMutex.RUnlock()

	for _, hook := range globalPost {
		hook(args, req.Method, rpcErr, result, ctx)
	}
	for _, hook := range cmdPost {
		hook(args, req.Method, rpcErr, result, ctx)
	}

	if rpcErr != nil {
		errCode := -32603
		errMsg := rpcErr.Error()
		if rpcRPCError, ok := rpcErr.(*RPCError); ok {
			errCode = rpcRPCError.ErrorCode()
			errMsg = rpcRPCError.ErrorMessage()
		} else if errMsgProvider, ok := rpcErr.(interface {
			ErrorMessage() string
		}); ok {
			errMsg = errMsgProvider.ErrorMessage()
		}
		return jsonRPCResponse{
			JSONRPC: "2.0",
			Error:   &jsonRPCError{Code: errCode, Message: errMsg},
			ID:      req.ID,
		}, false
	}

	return jsonRPCResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}, true
}

func (s *Server) handleMessage(cc *clientConn, message []byte) {
	resp, _ := s.processMessage(message)
	data, err := json.Marshal(resp)
	if err != nil {
		s.logger.Error(fmt.Sprintf("failed to marshal response: %v", err))
		return
	}
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if err := cc.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		s.logger.Error(fmt.Sprintf("failed to send response: %v", err))
	}
}

type RPCError struct {
	Code    int
	Message string
}

func (e *RPCError) Error() string      { return e.Message }
func (e *RPCError) ErrorCode() int      { return e.Code }
func (e *RPCError) ErrorMessage() string { return e.Message }

func NewRPCError(code int, message string) *RPCError {
	return &RPCError{Code: code, Message: message}
}

func IsHighFrequency(method string, patterns map[string]*hfPattern) bool {
	for _, p := range patterns {
		if p.pattern.MatchString(method) {
			return true
		}
	}
	return false
}