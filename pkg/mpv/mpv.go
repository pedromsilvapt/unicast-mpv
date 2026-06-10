package mpv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unicast/unicast-mpv/pkg/logger"
	"github.com/unicast/unicast-mpv/pkg/mpv/ipc"
	"github.com/unicast/unicast-mpv/pkg/mpv/process"
)

var (
	ErrNotRunning      = errors.New("mpv: not running")
	ErrAlreadyRunning  = errors.New("mpv: already running")
	ErrInvalidMode     = errors.New("mpv: invalid mode")
	ErrInvalidSeekMode = errors.New("mpv: invalid seek mode")
	ErrStartFailed     = errors.New("mpv: start failed")

)

const timePosObserveID = 0

const pendingSeekTimeout = 10 * time.Second

var defaultObservedProperties = []string{
	"mute",
	"pause",
	"duration",
	"volume",
	"filename",
	"path",
	"media-title",
	"playlist-pos",
	"playlist-count",
	"loop",
}

var defaultObservedPropertiesAudioOnly = []string{
	"mute",
	"pause",
	"duration",
	"volume",
	"filename",
	"path",
	"media-title",
	"playlist-pos",
	"playlist-count",
	"loop",
}

var videoObservedProperties = []string{
	"fullscreen",
	"sub-visibility",
}

type MPVOption func(*MPV)

func WithLogger(l *logger.Logger) MPVOption {
	return func(m *MPV) {
		m.log = l
	}
}

type EventHandler func(args ...interface{})

type ExternalInstanceHandler func()

type seekInfo struct {
	startPos float64
}

type MPV struct {
	cfg    process.ProcessConfig
	proc   *process.Process
	ipc    *ipc.IPCClient
	log    *logger.Logger

	running    atomic.Bool
	external   bool
	mu         sync.Mutex

	mpvVersion string

	observedProps   map[string]int64
	observedPropsMu sync.Mutex
	nextObserveID   int64

	currentTimePos  float64
	timePosMu       sync.RWMutex
	hasTimePos      bool

	pendingSeek     *seekInfo
	pendingSeekMu   sync.Mutex
	pendingSeekTimer *time.Timer

	loadConfirmCh   chan error
	loadFileStarted  bool
	loadConfirmMu    sync.Mutex

	eventHandlers   map[string][]EventHandler
	eventHandlersMu sync.RWMutex

	timePosTicker   *time.Ticker
	timePosStop     chan struct{}

	quitCh   chan struct{}
	quitOnce sync.Once

	onExternalInstanceClose ExternalInstanceHandler
}

func NewMPV(cfg process.ProcessConfig, opts ...MPVOption) *MPV {
	m := &MPV{
		cfg:           cfg,
		observedProps: make(map[string]int64),
		nextObserveID: 1,
		eventHandlers: make(map[string][]EventHandler),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func NewExternalMPV(socketPath string, opts ...MPVOption) (*MPV, error) {
	cfg := process.ProcessConfig{
		SocketPath:  socketPath,
		AutoRestart: false,
		TimeUpdate:  1,
	}

	m := &MPV{
		cfg:              cfg,
		observedProps:    make(map[string]int64),
		nextObserveID:    1,
		eventHandlers:    make(map[string][]EventHandler),
		external:         true,
	}
	for _, opt := range opts {
		opt(m)
	}

	ipcClient := ipc.NewIPCClient(socketPath)
	if m.log != nil {
		ipcLog := m.log.Service("mpv-ipc.raw")
		ipcClient.OnMessage = func(data []byte) {
			ipcLog.Debug(string(bytes.TrimRight(data, "\n")))
		}
	}
	if err := ipcClient.Connect(); err != nil {
		return nil, fmt.Errorf("mpv: connect to external instance: %w", err)
	}

	m.mu.Lock()
	m.ipc = ipcClient
	m.mu.Unlock()
	m.running.Store(true)
	m.quitCh = make(chan struct{})

	m.detectAndStoreVersion()

	if err := m.observePropertyWithID("time-pos", timePosObserveID); err != nil {
		ipcClient.Close()
		m.running.Store(false)
		return nil, fmt.Errorf("mpv: observe time-pos on external instance: %w", err)
	}

	props := defaultObservedProperties
	if m.cfg.AudioOnly {
		props = defaultObservedPropertiesAudioOnly
	} else {
		props = append(props, videoObservedProperties...)
	}

	for _, prop := range props {
		if err := m.ObserveProperty(prop); err != nil {
			ipcClient.Close()
			m.running.Store(false)
			return nil, fmt.Errorf("mpv: observe %s on external instance: %w", prop, err)
		}
	}

	go m.externalEventLoop()

	return m, nil
}

func (m *MPV) SetOnExternalInstanceClose(handler ExternalInstanceHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onExternalInstanceClose = handler
}

func (m *MPV) Start() error {
	if m.running.Load() {
		return NewAlreadyRunningError("start()")
	}

	if m.log != nil {
		m.log.Info("starting mpv...")
	}

	if m.log != nil {
		ipcLog := m.log.Service("mpv-ipc.raw")
		m.cfg.OnMessage = func(data []byte) {
			ipcLog.Debug(string(bytes.TrimRight(data, "\n")))
		}
		m.cfg.Log = m.log.Service("mpv-process")
		m.cfg.RootLog = m.log
	}

	m.quitOnce = sync.Once{}
	m.quitCh = make(chan struct{})

	m.mu.Lock()
	if m.proc != nil {
		m.proc.SetOnExit(nil)
		m.proc.Quit()
	}
	proc := process.NewProcess(m.cfg)
	proc.SetOnExit(m.onProcessExit)
	m.proc = proc
	m.mu.Unlock()

	if err := proc.Start(); err != nil {
		if m.log != nil {
			m.log.Errorf("mpv start failed: %v", err)
		}
		if errors.Is(err, process.ErrBinaryNotFound) {
			return NewBinaryNotFoundError(m.cfg.Binary)
		}
		if errors.Is(err, process.ErrIPCBindFailed) {
			return NewIPCBindFailedError(m.cfg.SocketPath)
		}
		return fmt.Errorf("%w: %v", ErrStartFailed, err)
	}

	m.mu.Lock()
	m.ipc = proc.IPC()
	m.mu.Unlock()

	m.running.Store(true)

	m.detectAndStoreVersion()
	if m.log != nil {
		m.log.Infof("mpv version: %s", m.mpvVersion)
	}

	if m.log != nil {
		m.log.Debug("observing time-pos property")
	}
	if err := m.observePropertyWithID("time-pos", timePosObserveID); err != nil {
		if m.log != nil {
			m.log.Errorf("observe time-pos failed: %v", err)
		}
		m.running.Store(false)
		return fmt.Errorf("%w: observe time-pos: %v", ErrStartFailed, err)
	}

	props := defaultObservedProperties
	if m.cfg.AudioOnly {
		props = defaultObservedPropertiesAudioOnly
	} else {
		props = append(props, videoObservedProperties...)
	}

	for _, prop := range props {
		if m.log != nil {
			m.log.Debugf("observing property: %s", prop)
		}
		if err := m.ObserveProperty(prop); err != nil {
			if m.log != nil {
				m.log.Errorf("observe %s failed: %v", prop, err)
			}
			_ = m.Quit()
			return fmt.Errorf("%w: observe %s: %v", ErrStartFailed, prop, err)
		}
	}

	m.startTimePositionTicker()
	go m.eventLoop()

	if m.log != nil {
		m.log.Info("mpv started successfully")
	}

	return nil
}

func (m *MPV) Quit() error {
	if !m.running.Load() {
		return NewNotRunningError("quit()")
	}
	if m.log != nil {
		m.log.Info("quitting mpv")
	}
	m.running.Store(false)
	m.stopTimePositionTicker()
	m.clearPendingSeek()

	m.timePosMu.Lock()
	m.currentTimePos = 0
	m.hasTimePos = false
	m.timePosMu.Unlock()

	m.mu.Lock()
	ipcClient := m.ipc
	m.ipc = nil
	m.mu.Unlock()

	if ipcClient != nil {
		ipcClient.Command("quit")
		ipcClient.Close()
	}

	if m.proc != nil {
		m.proc.Quit()
	}

	m.quitOnce.Do(func() {
		if m.quitCh != nil {
			close(m.quitCh)
		}
	})

	return nil
}

func (m *MPV) Restart() error {
	if m.log != nil {
		m.log.Info("restarting mpv")
	}

	m.stopTimePositionTicker()
	m.clearPendingSeek()

	m.timePosMu.Lock()
	m.currentTimePos = 0
	m.hasTimePos = false
	m.timePosMu.Unlock()

	m.mu.Lock()
	ipcClient := m.ipc
	m.ipc = nil
	oldProc := m.proc
	m.mu.Unlock()

	if ipcClient != nil {
		ipcClient.Command("quit")
		ipcClient.Close()
	}

	m.quitOnce.Do(func() {
		if m.quitCh != nil {
			close(m.quitCh)
		}
	})

	if oldProc != nil {
		oldProc.SetOnExit(nil)
		oldProc.Quit()
	}

	m.running.Store(false)
	return m.Start()
}

func (m *MPV) IsRunning() bool {
	return m.running.Load()
}

func (m *MPV) ReconnectIfNeeded() error {
	if !m.running.Load() {
		return NewNotRunningError("reconnectIfNeeded()")
	}

	m.mu.Lock()
	ipcClient := m.ipc
	m.mu.Unlock()

	if ipcClient == nil || ipcClient.IsClosed() {
		if m.log != nil {
			m.log.Info("mpv ipc connection lost, restarting")
		}
		m.running.Store(false)
		return NewNotRunningError("ipc connection lost")
	}

	_, err := ipcClient.GetProperty("mpv-version")
	if err != nil {
		if m.log != nil {
			m.log.Warnf("mpv ipc health check failed: %v, marking as not running", err)
		}
		ipcClient.Close()
		m.mu.Lock()
		m.ipc = nil
		m.mu.Unlock()
		m.running.Store(false)
		return NewNotRunningError("ipc health check failed")
	}

	return nil
}

func (m *MPV) MpvVersion() string {
	return m.mpvVersion
}

func (m *MPV) VersionAtLeast(minVersion string) bool {
	return m.versionAtLeast(minVersion)
}

func (m *MPV) EmitTestEvent(event string, args ...interface{}) {
	m.emit(event, args...)
}

func (m *MPV) OnEvent(event string, handler EventHandler) {
	m.eventHandlersMu.Lock()
	m.eventHandlers[event] = append(m.eventHandlers[event], handler)
	m.eventHandlersMu.Unlock()
}

func (m *MPV) emit(event string, args ...interface{}) {
	m.eventHandlersMu.RLock()
	handlers := m.eventHandlers[event]
	m.eventHandlersMu.RUnlock()

	for _, h := range handlers {
		h(args...)
	}
}

func (m *MPV) eventLoop() {
	m.mu.Lock()
	ipcClient := m.ipc
	m.mu.Unlock()

	if ipcClient == nil {
		return
	}

	if m.log != nil {
		m.log.Debug("event loop started")
	}

	for {
		select {
		case <-m.quitCh:
			if m.log != nil {
				m.log.Debug("event loop stopped")
			}
			return
		case evt, ok := <-ipcClient.Events():
			if !ok {
				if m.log != nil {
					m.log.Warn("IPC event channel closed")
				}
				m.emit("quit")
				return
			}
			m.handleEvent(evt)
		}
	}
}

func (m *MPV) externalEventLoop() {
	m.mu.Lock()
	ipcClient := m.ipc
	m.mu.Unlock()

	if ipcClient == nil {
		return
	}

	for {
		select {
		case <-m.quitCh:
			return
		case _, ok := <-ipcClient.Events():
			if !ok {
				m.emit("crashed")
				m.emit("quit")
				m.running.Store(false)

				m.mu.Lock()
				handler := m.onExternalInstanceClose
				m.mu.Unlock()

				if handler != nil {
					handler()
				}
				return
			}
		}
	}
}

func (m *MPV) handleEvent(evt ipc.MPVEvent) {
	switch evt.Name {
	case "idle":
		m.emit("stopped")
	case "playback-restart":
		m.pendingSeekMu.Lock()
		if m.pendingSeek != nil {
			if m.pendingSeekTimer != nil {
				m.pendingSeekTimer.Stop()
				m.pendingSeekTimer = nil
			}
			m.timePosMu.RLock()
			endPos := m.currentTimePos
			m.timePosMu.RUnlock()
			m.emit("seek", map[string]interface{}{
				"start": m.pendingSeek.startPos,
				"end":   endPos,
			})
			m.pendingSeek = nil
		}
		m.pendingSeekMu.Unlock()
		m.emit("started")
	case "pause":
		m.emit("paused")
	case "unpause":
		m.emit("resumed")
	case "seek":
		m.handleSeekEvent()
	case "tracks-changed":
		m.clearPendingSeek()
	case "property-change":
		m.handlePropertyChangeEvent(evt)
	case "start-file":
		m.loadConfirmMu.Lock()
		m.loadFileStarted = true
		m.loadConfirmMu.Unlock()
	case "file-loaded":
		m.loadConfirmMu.Lock()
		if m.loadFileStarted && m.loadConfirmCh != nil {
			select {
			case m.loadConfirmCh <- nil:
			default:
			}
		}
		m.loadConfirmMu.Unlock()
	case "end-file":
		m.loadConfirmMu.Lock()
		if m.loadFileStarted && m.loadConfirmCh != nil {
			select {
			case m.loadConfirmCh <- NewLoadFailedError("load()", nil):
			default:
			}
			m.loadFileStarted = false
		}
		m.loadConfirmMu.Unlock()

		var endFileMsg struct {
			Reason string `json:"reason"`
		}
		if json.Unmarshal(evt.Raw, &endFileMsg) == nil && endFileMsg.Reason == "error" {
			m.emit("stopped")
		}
	}
}

type propertyChangeMsg struct {
	Name string      `json:"name"`
	Data interface{} `json:"data"`
	ID   int64       `json:"id"`
}

func (m *MPV) handlePropertyChangeEvent(evt ipc.MPVEvent) {
	var msg propertyChangeMsg
	if err := json.Unmarshal(evt.Raw, &msg); err != nil {
		return
	}

	if msg.Name == "time-pos" {
		m.timePosMu.Lock()
		m.currentTimePos = toFloat64(msg.Data)
		m.hasTimePos = true
		m.timePosMu.Unlock()
	} else {
		m.emit("status", map[string]interface{}{
			"property": msg.Name,
			"value":     msg.Data,
		})
	}
}

func (m *MPV) handleSeekEvent() {
	m.timePosMu.RLock()
	startPos := m.currentTimePos
	m.timePosMu.RUnlock()

	m.pendingSeekMu.Lock()
	if m.pendingSeekTimer != nil {
		m.pendingSeekTimer.Stop()
	}
	m.pendingSeek = &seekInfo{startPos: startPos}
	m.pendingSeekTimer = time.AfterFunc(pendingSeekTimeout, func() {
		m.pendingSeekMu.Lock()
		m.pendingSeek = nil
		m.pendingSeekTimer = nil
		m.pendingSeekMu.Unlock()
	})
	m.pendingSeekMu.Unlock()
}

func (m *MPV) clearPendingSeek() {
	m.pendingSeekMu.Lock()
	if m.pendingSeekTimer != nil {
		m.pendingSeekTimer.Stop()
		m.pendingSeekTimer = nil
	}
	m.pendingSeek = nil
	m.pendingSeekMu.Unlock()
}

func (m *MPV) onProcessExit(exitCode int) {
	if m.log != nil {
		m.log.Warnf("mpv process exited (code %d)", exitCode)
	}

	m.stopTimePositionTicker()
	m.clearPendingSeek()

	m.timePosMu.Lock()
	m.currentTimePos = 0
	m.hasTimePos = false
	m.timePosMu.Unlock()

	m.quitOnce.Do(func() {
		if m.quitCh != nil {
			close(m.quitCh)
		}
	})

	if exitCode == 4 && m.cfg.AutoRestart {
		m.mu.Lock()
		proc := m.proc
		m.mu.Unlock()

		if proc != nil {
			m.mu.Lock()
			m.ipc = proc.IPC()
			m.mu.Unlock()

			m.quitOnce = sync.Once{}
			m.quitCh = make(chan struct{})

			m.observePropertyWithID("time-pos", timePosObserveID)

			props := defaultObservedProperties
			if m.cfg.AudioOnly {
				props = defaultObservedPropertiesAudioOnly
			} else {
				props = append(props, videoObservedProperties...)
			}
			for _, prop := range props {
				m.ObserveProperty(prop)
			}

			m.startTimePositionTicker()
			go m.eventLoop()

			m.emit("crashed")
			return
		}
	}

	m.running.Store(false)

	m.mu.Lock()
	m.ipc = nil
	m.mu.Unlock()

	if exitCode == 4 {
		m.emit("crashed")
	}
	m.emit("quit")
}

func (m *MPV) startTimePositionTicker() {
	interval := time.Duration(m.cfg.TimeUpdate) * time.Second
	if interval <= 0 {
		interval = time.Second
	}
	m.timePosTicker = time.NewTicker(interval)
	m.timePosStop = make(chan struct{})

	go func() {
		for {
			select {
			case <-m.timePosTicker.C:
				if !m.running.Load() {
					continue
				}
				m.timePosMu.RLock()
				pos := m.currentTimePos
				hasTime := m.hasTimePos
				m.timePosMu.RUnlock()

				paused, _ := m.IsPaused()
				if !paused && hasTime {
					m.emit("timeposition", pos)
				}
			case <-m.timePosStop:
				return
			}
		}
	}()
}

func (m *MPV) stopTimePositionTicker() {
	if m.timePosTicker != nil {
		m.timePosTicker.Stop()
	}
	if m.timePosStop != nil {
		close(m.timePosStop)
	}
}

func (m *MPV) getIPC() (*ipc.IPCClient, error) {
	if !m.running.Load() {
		return nil, NewNotRunningError("")
	}
	m.mu.Lock()
	client := m.ipc
	m.mu.Unlock()
	if client == nil || client.IsClosed() {
		return nil, NewNotRunningError("")
	}
	return client, nil
}

func wrapIPCError(method string, args []interface{}, err error) error {
	if err == nil {
		return nil
	}
	if mpvErr, ok := err.(*MPVError); ok {
		return mpvErr
	}
	return NewIPCSendFailedError(method, args, err)
}

func (m *MPV) Load(source string, mode string, options []string, index *int) error {
	if !m.running.Load() {
		return NewNotRunningError("load()")
	}

	if m.log != nil {
		m.log.Infof("load: source=%s mode=%s options=%v", source, mode, options)
	}

	validModes := map[string]bool{"replace": true, "append": true, "append-play": true}
	if !validModes[mode] {
		options := map[string]string{
			"replace":     "Replace the currently playing title",
			"append":      "Append the title to the playlist",
			"append-play": "Append the title and when it is the only title in the list start playback",
		}
		return NewInvalidArgError("load()", []interface{}{source, mode}, fmt.Sprintf("Invalid mode: %s", mode), options)
	}

	if index != nil && !m.versionAtLeast("0.38.0") {
		return NewInvalidArgError("load()", []interface{}{source, mode, *index}, "index parameter requires mpv 0.38.0 or later", nil)
	}

	protocol := extractProtocol(source)
	if protocol != "" && !supportedProtocols[protocol] {
		return NewUnsupportedProtoError("load()", []interface{}{source, mode}, protocol)
	}

	if protocol == "" {
		if !filepath.IsAbs(source) {
			abs, err := filepath.Abs(source)
			if err == nil {
				source = abs
			}
		}
	}

	args := []interface{}{source, mode}
	if index != nil {
		args = append(args, *index)
	} else if m.versionAtLeast("0.38.0") {
		args = append(args, -1)
	}
	if len(options) > 0 {
		optMap := make(map[string]interface{}, len(options))
		for _, opt := range options {
			parts := strings.SplitN(opt, "=", 2)
			if len(parts) == 2 {
				optMap[parts[0]] = parts[1]
			} else {
				optMap[opt] = true
			}
		}
		args = append(args, optMap)
	}

	client, err := m.getIPC()
	if err != nil {
		return NewIPCSendFailedError("load()", []interface{}{source, mode}, err)
	}

	if mode == "append" {
		_, ipcErr := client.Command("loadfile", args...)
		if ipcErr != nil {
			return NewLoadFailedError("load()", []interface{}{source})
		}
		return nil
	}

	if mode == "append-play" {
		playlistSize, sizeErr := client.GetProperty("playlist-count")
		if sizeErr == nil {
			if count, ok := toFloat64Ok(playlistSize); ok && count > 1 {
				_, ipcErr := client.Command("loadfile", args...)
				if ipcErr != nil {
					return NewLoadFailedError("load()", []interface{}{source})
				}
				return nil
			}
		}
	}

	m.loadConfirmMu.Lock()
	m.loadFileStarted = false
	m.loadConfirmCh = make(chan error, 1)
	m.loadConfirmMu.Unlock()

	_, ipcErr := client.Command("loadfile", args...)
	if ipcErr != nil {
		if m.log != nil {
			m.log.Errorf("loadfile command failed: %v", ipcErr)
		}
		m.loadConfirmMu.Lock()
		m.loadConfirmCh = nil
		m.loadConfirmMu.Unlock()
		return NewLoadFailedError("load()", []interface{}{source})
	}

	if m.log != nil {
		m.log.Debug("loadfile sent, waiting for file-loaded or end-file confirmation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	select {
	case loadErr := <-m.loadConfirmCh:
		m.loadConfirmMu.Lock()
		m.loadConfirmCh = nil
		m.loadFileStarted = false
		m.loadConfirmMu.Unlock()
		return loadErr
	case <-ctx.Done():
		if m.log != nil {
			m.log.Errorf("load: timed out waiting for file-loaded/end-file after 30s")
		}
		m.loadConfirmMu.Lock()
		m.loadConfirmCh = nil
		m.loadFileStarted = false
		m.loadConfirmMu.Unlock()
		return NewTimeoutError("load()", []interface{}{source, mode})
	}
}

func (m *MPV) Stop() error {
	client, err := m.getIPC()
	if err != nil {
		return err
	}
	_, err = client.Command("stop")
	return wrapIPCError("stop()", nil, err)
}

func (m *MPV) Pause() error {
	client, err := m.getIPC()
	if err != nil {
		return err
	}
	return wrapIPCError("pause()", nil, client.SetProperty("pause", true))
}

func (m *MPV) Resume() error {
	client, err := m.getIPC()
	if err != nil {
		return err
	}
	return wrapIPCError("resume()", nil, client.SetProperty("pause", false))
}

func (m *MPV) Seek(seconds float64, mode string) error {
	client, err := m.getIPC()
	if err != nil {
		return err
	}

	validModes := map[string]bool{"relative": true, "absolute": true, "relative-percent": true}
	if !validModes[mode] {
		options := map[string]string{
			"relative": "Searches x seconds from the current position",
			"absolute": "Goes to position x seconds of the current track",
		}
		return NewInvalidArgError("seek()", []interface{}{seconds, mode}, fmt.Sprintf("Invalid seek mode: %s", mode), options)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resultCh := make(chan error, 1)
	go func() {
		_, ipcErr := client.Command("seek", seconds, mode, "exact")
		resultCh <- ipcErr
	}()

	select {
	case ipcErr := <-resultCh:
		return wrapIPCError("seek()", []interface{}{seconds, mode}, ipcErr)
	case <-ctx.Done():
		return NewTimeoutError("seek()", []interface{}{seconds, mode})
	}
}

func (m *MPV) GoToPosition(seconds float64) error {
	return m.Seek(seconds, "absolute")
}

func (m *MPV) Mute(set *bool) error {
	client, err := m.getIPC()
	if err != nil {
		return err
	}
	if set != nil {
		return wrapIPCError("mute()", nil, client.SetProperty("mute", *set))
	}
	return wrapIPCError("mute()", nil, client.CycleProperty("mute"))
}

func (m *MPV) Volume(value float64) error {
	client, err := m.getIPC()
	if err != nil {
		return err
	}
	return wrapIPCError("volume()", nil, client.SetProperty("volume", value))
}

func (m *MPV) SetProperty(property string, value interface{}) error {
	client, err := m.getIPC()
	if err != nil {
		return err
	}
	return wrapIPCError("setProperty()", []interface{}{property, value}, client.SetProperty(property, value))
}

func (m *MPV) SetMultipleProperties(properties map[string]interface{}) error {
	if !m.running.Load() {
		return NewNotRunningError("setMultipleProperties()")
	}

	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex

	for prop, val := range properties {
		wg.Add(1)
		go func(p string, v interface{}) {
			defer wg.Done()
			if err := m.SetProperty(p, v); err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
			}
		}(prop, val)
	}
	wg.Wait()
	return firstErr
}

func (m *MPV) GetProperty(property string) (interface{}, error) {
	client, err := m.getIPC()
	if err != nil {
		return nil, err
	}
	val, ipcErr := client.GetProperty(property)
	if ipcErr != nil {
		return nil, NewIPCSendFailedError("getProperty()", []interface{}{property}, ipcErr)
	}
	return val, nil
}

func (m *MPV) AddProperty(property string, value float64) error {
	client, err := m.getIPC()
	if err != nil {
		return err
	}
	return wrapIPCError("addProperty()", []interface{}{property, value}, client.AddProperty(property, value))
}

func (m *MPV) MultiplyProperty(property string, value float64) error {
	client, err := m.getIPC()
	if err != nil {
		return err
	}
	return wrapIPCError("multiplyProperty()", []interface{}{property, value}, client.MultiplyProperty(property, value))
}

func (m *MPV) CycleProperty(property string) error {
	client, err := m.getIPC()
	if err != nil {
		return err
	}
	return wrapIPCError("cycleProperty()", []interface{}{property}, client.CycleProperty(property))
}

func (m *MPV) ObserveProperty(property string) error {
	client, err := m.getIPC()
	if err != nil {
		return err
	}

	m.observedPropsMu.Lock()
	id := m.nextObserveID
	m.nextObserveID++
	m.observedProps[property] = id
	m.observedPropsMu.Unlock()

	_, err = client.Command("observe_property", id, property)
	return wrapIPCError("observeProperty()", []interface{}{property}, err)
}

func (m *MPV) observePropertyWithID(property string, id int64) error {
	client, err := m.getIPC()
	if err != nil {
		return err
	}

	m.observedPropsMu.Lock()
	m.observedProps[property] = id
	m.observedPropsMu.Unlock()

	_, err = client.Command("observe_property", id, property)
	return wrapIPCError("observePropertyWithID()", []interface{}{property, id}, err)
}

func (m *MPV) Command(command string, args []string) error {
	client, err := m.getIPC()
	if err != nil {
		return err
	}
	ifaceArgs := make([]interface{}, len(args))
	for i, a := range args {
		ifaceArgs[i] = a
	}
	_, err = client.Command(command, ifaceArgs...)
	return wrapIPCError("command()", ifaceArgs, err)
}

func (m *MPV) AddSubtitles(file string, flag string, title string, lang string) error {
	client, err := m.getIPC()
	if err != nil {
		return err
	}

	var args []interface{}
	args = append(args, file)
	if flag != "" {
		args = append(args, flag)
	}
	if title != "" {
		args = append(args, title)
	}
	if lang != "" {
		args = append(args, lang)
	}

	_, err = client.Command("sub-add", args...)
	return wrapIPCError("addSubtitles()", args, err)
}

func (m *MPV) AdjustSubtitleTiming(seconds float64) error {
	client, err := m.getIPC()
	if err != nil {
		return err
	}
	return wrapIPCError("adjustSubtitleTiming()", []interface{}{seconds}, client.SetProperty("sub-delay", seconds))
}

func (m *MPV) SubtitleScale(scale float64) error {
	client, err := m.getIPC()
	if err != nil {
		return err
	}
	return wrapIPCError("subtitleScale()", []interface{}{scale}, client.SetProperty("sub-scale", scale))
}

func (m *MPV) HideSubtitles() error {
	client, err := m.getIPC()
	if err != nil {
		return err
	}
	return wrapIPCError("hideSubtitles()", nil, client.SetProperty("sub-visibility", false))
}

func (m *MPV) ShowSubtitles() error {
	client, err := m.getIPC()
	if err != nil {
		return err
	}
	return wrapIPCError("showSubtitles()", nil, client.SetProperty("sub-visibility", true))
}

func (m *MPV) GetTimePosition() (float64, error) {
	client, err := m.getIPC()
	if err != nil {
		return 0, err
	}
	val, ipcErr := client.GetProperty("time-pos")
	if ipcErr != nil {
		return 0, NewIPCSendFailedError("getTimePosition()", nil, ipcErr)
	}
	return toFloat64(val), nil
}

func (m *MPV) IsPaused() (bool, error) {
	client, err := m.getIPC()
	if err != nil {
		return true, err
	}
	val, ipcErr := client.GetProperty("pause")
	if ipcErr != nil {
		return true, NewIPCSendFailedError("isPaused()", nil, ipcErr)
	}
	return toBool(val), nil
}

func extractProtocol(source string) string {
	idx := strings.Index(source, "://")
	if idx == -1 {
		return ""
	}
	return source[:idx]
}

var supportedProtocols = map[string]bool{
	"appending": true, "av": true, "bd": true, "cdda": true,
	"dvb": true, "dvd": true, "edl": true, "fd": true,
	"fdclose": true, "file": true, "hex": true, "http": true,
	"https": true, "lavf": true, "memory": true, "mf": true,
	"null": true, "slice": true, "smb": true, "udp": true,
	"ytdl": true,
}

func toFloat64(v interface{}) float64 {
	val, _ := toFloat64Ok(v)
	return val
}

func toFloat64Ok(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case json.Number:
		if f, err := val.Float64(); err == nil {
			return f, true
		}
		return 0, false
	default:
		return 0, false
	}
}

func (m *MPV) versionAtLeast(minVersion string) bool {
	if m.mpvVersion == "" {
		return true
	}
	return cmpVersion(m.mpvVersion, minVersion)
}

func cmpVersion(mpvVersion string, minVersion string) bool {
	mpvParts := strings.Split(mpvVersion, ".")
	minParts := strings.Split(minVersion, ".")
	for i := 0; i < 3; i++ {
		mpvN := 0
		minN := 0
		if i < len(mpvParts) {
			mpvN, _ = strconv.Atoi(mpvParts[i])
		}
		if i < len(minParts) {
			minN, _ = strconv.Atoi(minParts[i])
		}
		if mpvN > minN {
			return true
		}
		if mpvN < minN {
			return false
		}
	}
	return true
}

func (m *MPV) getMpvVersion(output string) string {
	if output == "" || strings.Contains(output, "UNKNOWN") {
		return "999.999.999"
	}
	re := regexp.MustCompile(`v?(\d+\.\d+\.\d+)`)
	matches := re.FindStringSubmatch(output)
	if matches != nil {
		return matches[1]
	}
	return "999.999.999"
}

func (m *MPV) detectAndStoreVersion() {
	val, err := m.GetProperty("mpv-version")
	if err != nil {
		m.mpvVersion = "999.999.999"
		return
	}
	if str, ok := val.(string); ok {
		m.mpvVersion = m.getMpvVersion(str)
	} else {
		m.mpvVersion = "999.999.999"
	}
}

func toBool(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	default:
		return false
	}
}
