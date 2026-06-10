package mpv

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unicast/unicast-mpv/pkg/mpv/ipc"
	"github.com/unicast/unicast-mpv/pkg/mpv/process"
)

type testHook struct {
	mpv      *MPV
	ipc      *ipc.IPCClient
	server   net.Conn
	reader   *bufio.Reader
	closeCh  chan struct{}
	closeOnce sync.Once
}

func setupTestHook(t *testing.T) (*testHook, func()) {
	t.Helper()

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "mpv-test.sock")

	l, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	ipcClient := ipc.NewIPCClient(socketPath)
	if err := ipcClient.Connect(); err != nil {
		l.Close()
		t.Fatalf("connect: %v", err)
	}

	conn, err := l.Accept()
	if err != nil {
		l.Close()
		ipcClient.Close()
		t.Fatalf("accept: %v", err)
	}

	cfg := process.ProcessConfig{
		SocketPath: socketPath,
		TimeUpdate: 1,
	}

	mpvInst := NewMPV(cfg)
	mpvInst.running.Store(true)
	mpvInst.mu.Lock()
	mpvInst.ipc = ipcClient
	mpvInst.mu.Unlock()
	mpvInst.quitCh = make(chan struct{})
	go mpvInst.eventLoop()

	hook := &testHook{
		mpv:     mpvInst,
		ipc:     ipcClient,
		server:  conn,
		reader:  bufio.NewReader(conn),
		closeCh: make(chan struct{}),
	}

	cleanup := func() {
		hook.closeOnce.Do(func() {
			close(hook.closeCh)
		})
		mpvInst.running.Store(false)
		mpvInst.stopTimePositionTicker()
		ipcClient.Close()
		conn.Close()
		l.Close()
		mpvInst.quitOnce.Do(func() {
			if mpvInst.quitCh != nil {
				close(mpvInst.quitCh)
			}
		})
	}

	return hook, cleanup
}

func (h *testHook) readRequest(t *testing.T) map[string]interface{} {
	t.Helper()
	h.server.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := h.reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("read request: %v", err)
	}
	h.server.SetReadDeadline(time.Time{})
	var req map[string]interface{}
	if err := json.Unmarshal(line, &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	return req
}

func (h *testHook) respond(t *testing.T, reqID float64, data interface{}, errMsg string) {
	t.Helper()
	resp := map[string]interface{}{
		"request_id": int(reqID),
		"error":       errMsg,
	}
	if data != nil {
		resp["data"] = data
	}
	writeResponse(t, h.server, resp)
}

func writeResponse(t *testing.T, conn net.Conn, v interface{}) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	data = append(data, '\n')
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	conn.SetWriteDeadline(time.Time{})
}

func (h *testHook) sendEvent(t *testing.T, event string, extra map[string]interface{}) {
	t.Helper()
	msg := map[string]interface{}{
		"event": event,
	}
	for k, v := range extra {
		msg[k] = v
	}
	writeResponse(t, h.server, msg)
}

func TestNewMPV(t *testing.T) {
	cfg := process.ProcessConfig{
		SocketPath: "/tmp/test.sock",
	}
	m := NewMPV(cfg)
	if m == nil {
		t.Fatal("expected non-nil MPV")
	}
	if m.IsRunning() {
		t.Error("expected MPV to not be running initially")
	}
	if m.nextObserveID != 1 {
		t.Errorf("expected nextObserveID=1, got %d", m.nextObserveID)
	}
}

func TestMPV_Pause(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.Pause()
	}()

	req := hook.readRequest(t)
	if cmd := req["command"].([]interface{}); cmd[0] != "set_property" {
		t.Fatalf("expected set_property, got %v", cmd[0])
	}
	if req["command"].([]interface{})[1] != "pause" {
		t.Fatalf("expected property pause, got %v", req["command"].([]interface{})[1])
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("Pause returned error: %v", err)
	}
}

func TestMPV_Resume(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.Resume()
	}()

	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "set_property" || cmd[1] != "pause" {
		t.Fatalf("expected set_property pause, got %v %v", cmd[0], cmd[1])
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("Resume returned error: %v", err)
	}
}

func TestMPV_Stop(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.Stop()
	}()

	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "stop" {
		t.Fatalf("expected stop, got %v", cmd[0])
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("Stop returned error: %v", err)
	}
}

func TestMPV_Volume(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.Volume(75.0)
	}()

	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "set_property" || cmd[1] != "volume" {
		t.Fatalf("expected set_property volume, got %v %v", cmd[0], cmd[1])
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("Volume returned error: %v", err)
	}
}

func TestMPV_Mute_SetTrue(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	muteTrue := true
	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.Mute(&muteTrue)
	}()

	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "set_property" || cmd[1] != "mute" {
		t.Fatalf("expected set_property mute, got %v %v", cmd[0], cmd[1])
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("Mute returned error: %v", err)
	}
}

func TestMPV_Mute_Toggle(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.Mute(nil)
	}()

	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "cycle" || cmd[1] != "mute" {
		t.Fatalf("expected cycle mute, got %v %v", cmd[0], cmd[1])
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("Mute toggle returned error: %v", err)
	}
}

func TestMPV_Seek(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.Seek(10.5, "relative")
	}()

	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "seek" {
		t.Fatalf("expected seek, got %v", cmd[0])
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("Seek returned error: %v", err)
	}
}

func TestMPV_GoToPosition(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.GoToPosition(30.0)
	}()

	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "seek" {
		t.Fatalf("expected seek, got %v", cmd[0])
	}
	if cmd[2] != "absolute" {
		t.Errorf("expected mode=absolute, got %v", cmd[2])
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("GoToPosition returned error: %v", err)
	}
}

func TestMPV_SetProperty(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.SetProperty("volume", 80)
	}()

	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "set_property" {
		t.Fatalf("expected set_property, got %v", cmd[0])
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("SetProperty returned error: %v", err)
	}
}

func TestMPV_GetProperty(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan interface{}, 1)
	errDone := make(chan error, 1)
	go func() {
		val, err := hook.mpv.GetProperty("volume")
		if err != nil {
			errDone <- err
		} else {
			done <- val
		}
	}()

	req := hook.readRequest(t)
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, 75.5, "success")

	select {
	case val := <-done:
		if v, ok := val.(float64); !ok || v != 75.5 {
			t.Errorf("expected 75.5, got %v", val)
		}
	case err := <-errDone:
		t.Errorf("GetProperty returned error: %v", err)
	}
}

func TestMPV_SetMultipleProperties(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	props := map[string]interface{}{
		"volume": 80,
		"mute":   true,
	}

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.SetMultipleProperties(props)
	}()

	for i := 0; i < 2; i++ {
		req := hook.readRequest(t)
		reqID := req["request_id"].(float64)
		hook.respond(t, reqID, nil, "success")
	}

	if err := <-done; err != nil {
		t.Errorf("SetMultipleProperties returned error: %v", err)
	}
}

func TestMPV_AddProperty(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.AddProperty("volume", 10.0)
	}()

	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "add" {
		t.Fatalf("expected add, got %v", cmd[0])
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("AddProperty returned error: %v", err)
	}
}

func TestMPV_MultiplyProperty(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.MultiplyProperty("volume", 2.0)
	}()

	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "multiply" {
		t.Fatalf("expected multiply, got %v", cmd[0])
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("MultiplyProperty returned error: %v", err)
	}
}

func TestMPV_CycleProperty(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.CycleProperty("fullscreen")
	}()

	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "cycle" {
		t.Fatalf("expected cycle, got %v", cmd[0])
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("CycleProperty returned error: %v", err)
	}
}

func TestMPV_ObserveProperty(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.ObserveProperty("volume")
	}()

	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "observe_property" {
		t.Fatalf("expected observe_property, got %v", cmd[0])
	}
	observeID := cmd[1].(float64)
	if observeID != 1 {
		t.Errorf("expected observe ID 1, got %v", observeID)
	}
	if cmd[2] != "volume" {
		t.Errorf("expected property volume, got %v", cmd[2])
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("ObserveProperty returned error: %v", err)
	}

	hook.mpv.observedPropsMu.Lock()
	if id, ok := hook.mpv.observedProps["volume"]; !ok || id != 1 {
		t.Errorf("expected observedProps[volume]=1, got %d", id)
	}
	hook.mpv.observedPropsMu.Unlock()
}

func TestMPV_Command(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.Command("playlist-clear", nil)
	}()

	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "playlist-clear" {
		t.Fatalf("expected playlist-clear, got %v", cmd[0])
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("Command returned error: %v", err)
	}
}

func TestMPV_Command_WithArgs(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.Command("loadfile", []string{"/path/to/file.mp4", "replace"})
	}()

	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "loadfile" {
		t.Fatalf("expected loadfile, got %v", cmd[0])
	}
	if len(cmd) != 3 {
		t.Fatalf("expected 3 command elements, got %d", len(cmd))
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("Command with args returned error: %v", err)
	}
}

func TestMPV_AddSubtitles(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.AddSubtitles("/path/to/sub.srt", "select", "English", "en")
	}()

	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "sub-add" {
		t.Fatalf("expected sub-add, got %v", cmd[0])
	}
	if len(cmd) < 5 {
		t.Fatalf("expected at least 5 elements in command, got %d", len(cmd))
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("AddSubtitles returned error: %v", err)
	}
}

func TestMPV_AddSubtitles_FileOnly(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.AddSubtitles("/path/to/sub.srt", "", "", "")
	}()

	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "sub-add" {
		t.Fatalf("expected sub-add, got %v", cmd[0])
	}
	if len(cmd) != 2 {
		t.Fatalf("expected only file in args, got %d elements", len(cmd))
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("AddSubtitles (file only) returned error: %v", err)
	}
}

func TestMPV_AdjustSubtitleTiming(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.AdjustSubtitleTiming(1.5)
	}()

	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "set_property" || cmd[1] != "sub-delay" {
		t.Fatalf("expected set_property sub-delay, got %v %v", cmd[0], cmd[1])
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("AdjustSubtitleTiming returned error: %v", err)
	}
}

func TestMPV_SubtitleScale(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.SubtitleScale(2.0)
	}()

	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "set_property" || cmd[1] != "sub-scale" {
		t.Fatalf("expected set_property sub-scale, got %v %v", cmd[0], cmd[1])
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("SubtitleScale returned error: %v", err)
	}
}

func TestMPV_HideShowSubtitles(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	// HideSubtitles
	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.HideSubtitles()
	}()
	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "set_property" || cmd[1] != "sub-visibility" {
		t.Fatalf("expected set_property sub-visibility, got %v %v", cmd[0], cmd[1])
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")
	if err := <-done; err != nil {
		t.Errorf("HideSubtitles returned error: %v", err)
	}

	// ShowSubtitles
	done2 := make(chan error, 1)
	go func() {
		done2 <- hook.mpv.ShowSubtitles()
	}()
	req = hook.readRequest(t)
	cmd = req["command"].([]interface{})
	if cmd[0] != "set_property" || cmd[1] != "sub-visibility" {
		t.Fatalf("expected set_property sub-visibility, got %v %v", cmd[0], cmd[1])
	}
	reqID = req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")
	if err := <-done2; err != nil {
		t.Errorf("ShowSubtitles returned error: %v", err)
	}
}

func TestMPV_IsPaused(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan bool, 1)
	errDone := make(chan error, 1)
	go func() {
		paused, err := hook.mpv.IsPaused()
		if err != nil {
			errDone <- err
		} else {
			done <- paused
		}
	}()

	req := hook.readRequest(t)
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, true, "success")

	select {
	case paused := <-done:
		if !paused {
			t.Error("expected paused=true")
		}
	case err := <-errDone:
		t.Errorf("IsPaused returned error: %v", err)
	}
}

func TestMPV_GetTimePosition(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan float64, 1)
	errDone := make(chan error, 1)
	go func() {
		pos, err := hook.mpv.GetTimePosition()
		if err != nil {
			errDone <- err
		} else {
			done <- pos
		}
	}()

	req := hook.readRequest(t)
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, 42.5, "success")

	select {
	case pos := <-done:
		if pos != 42.5 {
			t.Errorf("expected 42.5, got %f", pos)
		}
	case err := <-errDone:
		t.Errorf("GetTimePosition returned error: %v", err)
	}
}

func TestMPV_EventHandlers_Stopped(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	received := ""
	hook.mpv.OnEvent("stopped", func(args ...interface{}) {
		received = "stopped"
	})

	msg := map[string]interface{}{"event": "idle"}
	hook.mpv.handleEvent(ipc.MPVEvent{
		Name: "idle",
		Raw:  mustMarshal(msg),
	})

	if received != "stopped" {
		t.Error("expected 'stopped' event to be emitted for 'idle'")
	}
}

func TestMPV_EventHandlers_Started(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	received := ""
	hook.mpv.OnEvent("started", func(args ...interface{}) {
		received = "started"
	})

	msg := map[string]interface{}{"event": "playback-restart"}
	hook.mpv.handleEvent(ipc.MPVEvent{
		Name: "playback-restart",
		Raw:  mustMarshal(msg),
	})

	if received != "started" {
		t.Error("expected 'started' event to be emitted for 'playback-restart'")
	}
}

func TestMPV_EventHandlers_Paused(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	received := ""
	hook.mpv.OnEvent("paused", func(args ...interface{}) {
		received = "paused"
	})

	msg := map[string]interface{}{"event": "pause"}
	hook.mpv.handleEvent(ipc.MPVEvent{
		Name: "pause",
		Raw:  mustMarshal(msg),
	})

	if received != "paused" {
		t.Error("expected 'paused' event to be emitted for 'pause'")
	}
}

func TestMPV_EventHandlers_Resumed(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	received := ""
	hook.mpv.OnEvent("resumed", func(args ...interface{}) {
		received = "resumed"
	})

	msg := map[string]interface{}{"event": "unpause"}
	hook.mpv.handleEvent(ipc.MPVEvent{
		Name: "unpause",
		Raw:  mustMarshal(msg),
	})

	if received != "resumed" {
		t.Error("expected 'resumed' event to be emitted for 'unpause'")
	}
}

func TestMPV_EventHandlers_PropertyChange_Status(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	var received map[string]interface{}
	hook.mpv.OnEvent("status", func(args ...interface{}) {
		if len(args) > 0 {
			if m, ok := args[0].(map[string]interface{}); ok {
				received = m
			}
		}
	})

	msg := map[string]interface{}{
		"event": "property-change",
		"name":  "volume",
		"data":  75,
	}

	hook.mpv.handleEvent(ipc.MPVEvent{
		Name: "property-change",
		Data: 75,
		Raw:  mustMarshal(msg),
	})

	if received == nil {
		t.Fatal("expected 'status' event to be emitted for 'property-change'")
	}
	if received["property"] != "volume" {
		t.Errorf("expected property=volume, got %v", received["property"])
	}
}

func TestMPV_EventHandlers_PropertyChange_TimePos(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	msg := map[string]interface{}{
		"event": "property-change",
		"name":  "time-pos",
		"data":  42.5,
	}

	evt := ipc.MPVEvent{
		Name: "property-change",
		Data: 42.5,
		Raw:  mustMarshal(msg),
	}
	hook.mpv.handleEvent(evt)

	hook.mpv.timePosMu.RLock()
	pos := hook.mpv.currentTimePos
	hook.mpv.timePosMu.RUnlock()

	if pos != 42.5 {
		t.Errorf("expected currentTimePos=42.5, got %f", pos)
	}
}

func TestMPV_MultipleEventHandlers(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	var count int32
	hook.mpv.OnEvent("stopped", func(args ...interface{}) {
		atomic.AddInt32(&count, 1)
	})
	hook.mpv.OnEvent("stopped", func(args ...interface{}) {
		atomic.AddInt32(&count, 1)
	})

	msg := map[string]interface{}{"event": "idle"}
	hook.mpv.handleEvent(ipc.MPVEvent{
		Name: "idle",
		Raw:  mustMarshal(msg),
	})

	if atomic.LoadInt32(&count) != 2 {
		t.Errorf("expected 2 handler calls, got %d", atomic.LoadInt32(&count))
	}
}

func TestExtractProtocol(t *testing.T) {
	tests := []struct {
		source   string
		expected string
	}{
		{"http://example.com/video.mp4", "http"},
		{"https://example.com/video.mp4", "https"},
		{"file:///home/user/video.mp4", "file"},
		{"ytdl://video_id", "ytdl"},
		{"/home/user/video.mp4", ""},
		{"video.mp4", ""},
	}

	for _, tt := range tests {
		result := extractProtocol(tt.source)
		if result != tt.expected {
			t.Errorf("extractProtocol(%q): expected %q, got %q", tt.source, tt.expected, result)
		}
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected float64
	}{
		{float64(42.5), 42.5},
		{float32(3.14), float64(float32(3.14))},
		{int(10), 10.0},
		{int64(100), 100.0},
		{"hello", 0.0},
		{nil, 0.0},
	}

	for _, tt := range tests {
		result := toFloat64(tt.input)
		if result != tt.expected {
			t.Errorf("toFloat64(%v): expected %f, got %f", tt.input, tt.expected, result)
		}
	}
}

func TestToBool(t *testing.T) {
	if !toBool(true) {
		t.Error("toBool(true) should be true")
	}
	if toBool(false) {
		t.Error("toBool(false) should be false")
	}
	if toBool(1) {
		t.Error("toBool(1) should be false")
	}
	if toBool(nil) {
		t.Error("toBool(nil) should be false")
	}
}

func TestSupportedProtocols(t *testing.T) {
	validProtocols := []string{"http", "https", "file", "ytdl", "av", "edl"}
	for _, p := range validProtocols {
		if !supportedProtocols[p] {
			t.Errorf("expected protocol %s to be supported", p)
		}
	}
	if supportedProtocols["ftp"] {
		t.Error("ftp should not be a supported protocol")
	}
}

func TestDefaultObservedProperties(t *testing.T) {
	audioOnly := defaultObservedPropertiesAudioOnly
	full := append([]string{}, defaultObservedProperties...)
	full = append(full, videoObservedProperties...)

	for _, prop := range []string{"mute", "pause", "duration", "volume", "filename", "path", "media-title", "playlist-pos", "playlist-count", "loop"} {
		found := false
		for _, p := range audioOnly {
			if p == prop {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s in audio-only observed properties", prop)
		}
	}

	for _, prop := range []string{"fullscreen", "sub-visibility"} {
		found := false
		for _, p := range full {
			if p == prop {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s in full observed properties", prop)
		}
	}
}

func TestMPV_SeekEvent_PendingSeek(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	var seekData map[string]interface{}
	mu := sync.Mutex{}
	hook.mpv.OnEvent("seek", func(args ...interface{}) {
		if len(args) > 0 {
			if m, ok := args[0].(map[string]interface{}); ok {
				mu.Lock()
				seekData = m
				mu.Unlock()
			}
		}
	})

	hook.mpv.timePosMu.Lock()
	hook.mpv.currentTimePos = 10.0
	hook.mpv.timePosMu.Unlock()

	msg := map[string]interface{}{"event": "seek"}
	hook.mpv.handleEvent(ipc.MPVEvent{
		Name: "seek",
		Raw:  mustMarshal(msg),
	})

	hook.mpv.timePosMu.Lock()
	hook.mpv.currentTimePos = 15.0
	hook.mpv.timePosMu.Unlock()

	msg2 := map[string]interface{}{"event": "playback-restart"}
	hook.mpv.handleEvent(ipc.MPVEvent{
		Name: "playback-restart",
		Raw:  mustMarshal(msg2),
	})

	mu.Lock()
	if seekData == nil {
		t.Fatal("expected seek event data, got nil")
	}
	mu.Unlock()
	if seekData["start"] != 10.0 {
		t.Errorf("expected start=10.0, got %v", seekData["start"])
	}
	if seekData["end"] != 15.0 {
		t.Errorf("expected end=15.0, got %v", seekData["end"])
	}
}

func TestMPV_SeekEvent_NoPendingSeekOnPlaybackRestart(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	seekFired := false
	hook.mpv.OnEvent("seek", func(args ...interface{}) {
		seekFired = true
	})

	msg := map[string]interface{}{"event": "playback-restart"}
	hook.mpv.handleEvent(ipc.MPVEvent{
		Name: "playback-restart",
		Raw:  mustMarshal(msg),
	})

	if seekFired {
		t.Error("expected no seek event without prior seek MPV event")
	}
}

func mustMarshal(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return json.RawMessage(data)
}

func TestIntegration_MPVStartQuit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping integration test on windows")
	}
	mpvPath, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv not found in PATH, skipping integration test")
	}

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "mpv-integ.sock")

	cfg := process.ProcessConfig{
		Binary:      mpvPath,
		SocketPath:  socketPath,
		AutoRestart: false,
		AudioOnly:   true,
		TimeUpdate:  1,
	}

	m := NewMPV(cfg)
	if err := m.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() {
		if m.IsRunning() {
			m.Quit()
		}
	}()

	if !m.IsRunning() {
		t.Fatal("expected MPV to be running after Start()")
	}

	time.Sleep(200 * time.Millisecond)

	if err := m.Quit(); err != nil {
		t.Fatalf("Quit failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if m.IsRunning() {
		t.Error("expected MPV to not be running after Quit()")
	}
}

func TestIntegration_MPVProperties(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping integration test on windows")
	}
	mpvPath, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv not found in PATH, skipping integration test")
	}

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "mpv-props.sock")

	cfg := process.ProcessConfig{
		Binary:      mpvPath,
		SocketPath:  socketPath,
		AutoRestart: false,
		AudioOnly:   true,
		TimeUpdate:  1,
	}

	m := NewMPV(cfg)
	if err := m.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() {
		if m.IsRunning() {
			m.Quit()
		}
	}()

	time.Sleep(100 * time.Millisecond)

	paused, err := m.IsPaused()
	if err != nil {
		t.Errorf("IsPaused failed: %v", err)
	}
	if !paused {
		t.Log("MPV is not paused after starting (expected since idle)")
	}

	volume, err := m.GetProperty("volume")
	if err != nil {
		t.Errorf("GetProperty volume failed: %v", err)
	}
	t.Logf("Volume: %v", volume)

	err = m.Pause()
	if err != nil {
		t.Errorf("Pause failed: %v", err)
	}

	err = m.SetProperty("volume", 50)
	if err != nil {
		t.Errorf("SetProperty volume=50 failed: %v", err)
	}

	volume, err = m.GetProperty("volume")
	if err != nil {
		t.Errorf("GetProperty volume failed after set: %v", err)
	}
	t.Logf("Volume after set: %v", volume)
}

func TestIntegration_MPVEventHandling(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping integration test on windows")
	}
	mpvPath, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv not found in PATH, skipping integration test")
	}

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "mpv-events.sock")

	cfg := process.ProcessConfig{
		Binary:      mpvPath,
		SocketPath:  socketPath,
		AutoRestart: false,
		AudioOnly:   true,
		TimeUpdate:  1,
	}

	m := NewMPV(cfg)
	if err := m.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() {
		if m.IsRunning() {
			m.Quit()
		}
	}()

	var stoppedReceived atomic.Bool
	m.OnEvent("stopped", func(args ...interface{}) {
		stoppedReceived.Store(true)
	})

	time.Sleep(200 * time.Millisecond)

	if stoppedReceived.Load() {
		t.Log("Received 'stopped' event (from idle state)")
	}
}

func TestMPV_NotRunning_ErrCode8(t *testing.T) {
	m := NewMPV(process.ProcessConfig{SocketPath: "/tmp/test.sock"})

	err := m.Pause()
	if mpvErr, ok := err.(*MPVError); !ok {
		t.Errorf("Pause: expected *MPVError, got %T", err)
	} else if mpvErr.Code != ErrCodeNotRunning {
		t.Errorf("Pause: expected errcode 8, got %d", mpvErr.Code)
	}

	err = m.Resume()
	if mpvErr, ok := err.(*MPVError); !ok {
		t.Errorf("Resume: expected *MPVError, got %T", err)
	} else if mpvErr.Code != ErrCodeNotRunning {
		t.Errorf("Resume: expected errcode 8, got %d", mpvErr.Code)
	}

	err = m.Stop()
	if mpvErr, ok := err.(*MPVError); !ok {
		t.Errorf("Stop: expected *MPVError, got %T", err)
	} else if mpvErr.Code != ErrCodeNotRunning {
		t.Errorf("Stop: expected errcode 8, got %d", mpvErr.Code)
	}

	_, err = m.GetProperty("volume")
	if mpvErr, ok := err.(*MPVError); !ok {
		t.Errorf("GetProperty: expected *MPVError, got %T", err)
	} else if mpvErr.Code != ErrCodeNotRunning {
		t.Errorf("GetProperty: expected errcode 8, got %d", mpvErr.Code)
	}

	err = m.SetProperty("volume", 50)
	if mpvErr, ok := err.(*MPVError); !ok {
		t.Errorf("SetProperty: expected *MPVError, got %T", err)
	} else if mpvErr.Code != ErrCodeNotRunning {
		t.Errorf("SetProperty: expected errcode 8, got %d", mpvErr.Code)
	}

	err = m.SetMultipleProperties(map[string]interface{}{"volume": 50})
	if mpvErr, ok := err.(*MPVError); !ok {
		t.Errorf("SetMultipleProperties: expected *MPVError, got %T", err)
	} else if mpvErr.Code != ErrCodeNotRunning {
		t.Errorf("SetMultipleProperties: expected errcode 8, got %d", mpvErr.Code)
	}

	err = m.Load("test.mp4", "replace", nil, nil)
	if err == nil {
		t.Error("Load: expected error when not running")
	}
	if mpvErr, ok := err.(*MPVError); ok {
		if mpvErr.Code != ErrCodeNotRunning {
			t.Errorf("Load: expected errcode NotRunning (8), got %d", mpvErr.Code)
		}
	}

	err = m.Seek(10.0, "relative")
	if mpvErr, ok := err.(*MPVError); !ok {
		t.Errorf("Seek: expected *MPVError, got %T", err)
	} else if mpvErr.Code != ErrCodeNotRunning {
		t.Errorf("Seek: expected errcode 8, got %d", mpvErr.Code)
	}
}

func TestMPV_Load_InvalidMode_ErrCode1(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	err := hook.mpv.Load("test.mp4", "invalid-mode", nil, nil)
	if err == nil {
		t.Error("expected error for invalid mode")
	}
	if mpvErr, ok := err.(*MPVError); ok {
		if mpvErr.Code != ErrCodeInvalidArg {
			t.Errorf("expected ErrCodeInvalidArg (1), got %d", mpvErr.Code)
		}
		if mpvErr.Method != "load()" {
			t.Errorf("expected Method=load(), got %s", mpvErr.Method)
		}
	} else if err != ErrInvalidMode {
		t.Errorf("expected ErrInvalidMode or MPVError with code 1, got %v", err)
	}
}

func TestMPV_Load_UnsupportedProtocol_ErrCode9(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	err := hook.mpv.Load("ftp://example.com/file.mp4", "replace", nil, nil)
	if err == nil {
		t.Error("expected error for unsupported protocol")
	}
	if mpvErr, ok := err.(*MPVError); ok {
		if mpvErr.Code != ErrCodeUnsupportedProto {
			t.Errorf("expected ErrCodeUnsupportedProto (9), got %d", mpvErr.Code)
		}
	}
}

func TestMPV_Seek_InvalidMode_ErrCode1(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	err := hook.mpv.Seek(10.0, "invalid")
	if err == nil {
		t.Error("expected error for invalid seek mode")
	}
	if mpvErr, ok := err.(*MPVError); ok {
		if mpvErr.Code != ErrCodeInvalidArg {
			t.Errorf("expected ErrCodeInvalidArg (1), got %d", mpvErr.Code)
		}
		if mpvErr.Method != "seek()" {
			t.Errorf("expected Method=seek(), got %s", mpvErr.Method)
		}
		if mpvErr.Options == nil {
			t.Error("expected options in error")
		}
	} else if err != ErrInvalidSeekMode {
		t.Errorf("expected ErrInvalidSeekMode or MPVError with code 1, got %v", err)
	}
}

func TestMPV_ObserveProperty_TimePosNoConflict(t *testing.T) {
	cfg := process.ProcessConfig{SocketPath: "/tmp/test.sock"}
	m := NewMPV(cfg)

	if m.nextObserveID != 1 {
		t.Errorf("expected nextObserveID=1 (time-pos uses ID 0), got %d", m.nextObserveID)
	}
}

func TestMPV_ObserveProperty_IDsIncrement(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	props := []string{"volume", "pause", "duration"}
	for i, prop := range props {
		done := make(chan error, 1)
		go func() {
			done <- hook.mpv.ObserveProperty(prop)
		}()

		req := hook.readRequest(t)
		cmd := req["command"].([]interface{})
		observeID := cmd[1].(float64)
		expectedID := float64(i + 1)
		if observeID != expectedID {
			t.Errorf("observe_property %s: expected ID %v, got %v", prop, expectedID, observeID)
		}
		reqID := req["request_id"].(float64)
		hook.respond(t, reqID, nil, "success")

		if err := <-done; err != nil {
			t.Errorf("ObserveProperty(%s) returned error: %v", prop, err)
		}
	}
}

func TestMPV_CrashedEvent_ExitCodes(t *testing.T) {
	tests := []struct {
		exitCode      int
		expectCrashed bool
	}{
		{4, true},
		{0, false},
		{1, false},
		{2, false},
		{139, false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("exit_%d", tt.exitCode), func(t *testing.T) {
			cfg := process.ProcessConfig{SocketPath: "/tmp/test.sock"}
			m := NewMPV(cfg)

			crashed := false
			m.OnEvent("crashed", func(args ...interface{}) {
				crashed = true
			})

			m.onProcessExit(tt.exitCode)
			if crashed != tt.expectCrashed {
				t.Errorf("exit code %d: expected crashed=%v, got crashed=%v", tt.exitCode, tt.expectCrashed, crashed)
			}
		})
	}
}

func TestMPV_ExternalInstanceClose_CrashedAndQuitEvents(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "mpv-ext.sock")

	l, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	serverClosed := make(chan struct{})
	go func() {
		defer close(serverClosed)
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			var req map[string]interface{}
			if err := json.Unmarshal([]byte(scanner.Text()), &req); err != nil {
				break
			}
			reqID, _ := req["request_id"].(float64)
			cmd, ok := req["command"].([]interface{})
			if !ok {
				continue
			}

			cmdName, _ := cmd[0].(string)
			resp := map[string]interface{}{
				"request_id": int(reqID),
				"error":      "success",
			}
			if cmdName == "observe_property" || cmdName == "get_property" {
				resp["data"] = nil
			}
			data, _ := json.Marshal(resp)
			data = append(data, '\n')
			conn.Write(data)
		}
	}()

	m, err := NewExternalMPV(socketPath)
	if err != nil {
		l.Close()
		t.Fatalf("NewExternalMPV: %v", err)
	}

	var crashedFired, quitFired bool
	var eventMu sync.Mutex
	m.OnEvent("crashed", func(args ...interface{}) {
		eventMu.Lock()
		crashedFired = true
		eventMu.Unlock()
	})
	m.OnEvent("quit", func(args ...interface{}) {
		eventMu.Lock()
		quitFired = true
		eventMu.Unlock()
	})

	time.Sleep(100 * time.Millisecond)

	m.mu.Lock()
	ipcClient := m.ipc
	m.mu.Unlock()

	if ipcClient != nil {
		ipcClient.Close()
	}

	time.Sleep(200 * time.Millisecond)

	eventMu.Lock()
	if !crashedFired {
		t.Error("expected 'crashed' event when external instance socket closes")
	}
	if !quitFired {
		t.Error("expected 'quit' event when external instance socket closes")
	}
	eventMu.Unlock()

	m.running.Store(false)
	m.quitOnce.Do(func() {
		if m.quitCh != nil {
			close(m.quitCh)
		}
	})
}

func TestMPV_ErrorTypeAssertion(t *testing.T) {
	m := NewMPV(process.ProcessConfig{SocketPath: "/tmp/test.sock"})

	err := m.Load("test.mp4", "badmode", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}

	if mpvErr, ok := err.(*MPVError); ok {
		t.Logf("Load with invalid mode is MPVError with code %d", mpvErr.ErrorCode())
	}

	hook, cleanup := setupTestHook(t)
	defer cleanup()

	err = hook.mpv.Load("ftp://bad.com/file.mp4", "replace", nil, nil)
	if err == nil {
		t.Fatal("expected error for unsupported protocol")
	}

	if mpvErr2, ok := err.(*MPVError); ok {
		if mpvErr2.Code != ErrCodeUnsupportedProto {
			t.Errorf("expected ErrCodeUnsupportedProto (9), got %d", mpvErr2.Code)
		}
	} else {
		t.Errorf("expected *MPVError for unsupported protocol, got %T", err)
	}
}

func isMPVError(err error) bool {
	_, ok := err.(*MPVError)
	return ok
}

func TestMPV_AlreadyRunning_ErrCode6(t *testing.T) {
	m := NewMPV(process.ProcessConfig{SocketPath: "/tmp/test.sock"})
	m.running.Store(true)

	err := m.Start()
	if err == nil {
		t.Fatal("expected error when already running")
	}
	if mpvErr, ok := err.(*MPVError); ok {
		if mpvErr.Code != ErrCodeAlreadyRunning {
			t.Errorf("expected ErrCodeAlreadyRunning (6), got %d", mpvErr.Code)
		}
	} else if err != ErrAlreadyRunning {
		t.Errorf("expected ErrAlreadyRunning or MPVError, got %v", err)
	}
}

func TestMPV_EndFileEventWithReason(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	stoppedReceived := false
	hook.mpv.OnEvent("stopped", func(args ...interface{}) {
		stoppedReceived = true
	})

	msg := map[string]interface{}{
		"event": "end-file",
		"reason": "error",
	}
	hook.mpv.handleEvent(ipc.MPVEvent{
		Name: "end-file",
		Raw:  mustMarshal(msg),
	})

	if !stoppedReceived {
		t.Error("expected 'stopped' event for end-file with reason=error")
	}
}

func TestMPV_Load_ValidProtocols(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	protocols := []string{"http://example.com/file.mp4", "https://example.com/file.mp4", "ytdl://video_id"}
	for _, source := range protocols {
		done := make(chan error, 1)
		go func(s string) {
			done <- hook.mpv.Load(s, "replace", nil, nil)
		}(source)

		req := hook.readRequest(t)
		reqID := req["request_id"].(float64)
		hook.respond(t, reqID, nil, "success")

		hook.sendEvent(t, "start-file", nil)
		hook.sendEvent(t, "file-loaded", nil)

		if err := <-done; err != nil {
			t.Errorf("Load(%s): unexpected error: %v", source, err)
		}
	}
}

func TestMPV_Load_Replace_FileLoadedConfirm(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.Load("test.mp4", "replace", nil, nil)
	}()

	req := hook.readRequest(t)
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	hook.sendEvent(t, "start-file", nil)
	hook.sendEvent(t, "file-loaded", nil)

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Load timed out waiting for file-loaded confirmation")
	}
}

func TestMPV_Load_Replace_FileLoadFailed(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.Load("nonexistent.mp4", "replace", nil, nil)
	}()

	req := hook.readRequest(t)
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	hook.sendEvent(t, "start-file", nil)
	hook.sendEvent(t, "end-file", map[string]interface{}{
		"reason": "error",
	})

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected error when file fails to load")
		}
		if mpvErr, ok := err.(*MPVError); ok {
			if mpvErr.Code != ErrCodeLoadFailed {
				t.Errorf("expected ErrCodeLoadFailed (0), got %d", mpvErr.Code)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Load timed out waiting for end-file")
	}
}

func TestMPV_Load_Append_NoConfirmation(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.Load("test.mp4", "append", nil, nil)
	}()

	req := hook.readRequest(t)
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("append mode: expected no error, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("append mode should resolve immediately without file-loaded confirmation")
	}
}

func TestMPV_Load_AppendPlay_WithExistingPlaylist(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.Load("test.mp4", "append-play", nil, nil)
	}()

	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "get_property" || cmd[1] != "playlist-count" {
		t.Fatalf("expected get_property playlist-count, got %v %v", cmd[0], cmd[1])
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, 3.0, "success")

	req2 := hook.readRequest(t)
	reqID2 := req2["request_id"].(float64)
	hook.respond(t, reqID2, nil, "success")

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("append-play with playlist > 1: expected no error, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("append-play mode with playlist > 1 should resolve immediately")
	}
}

func TestMPV_Load_AppendPlay_EmptyPlaylist(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.Load("test.mp4", "append-play", nil, nil)
	}()

	req := hook.readRequest(t)
	cmd := req["command"].([]interface{})
	if cmd[0] != "get_property" || cmd[1] != "playlist-count" {
		t.Fatalf("expected get_property playlist-count, got %v %v", cmd[0], cmd[1])
	}
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, 1.0, "success")

	req2 := hook.readRequest(t)
	reqID2 := req2["request_id"].(float64)
	hook.respond(t, reqID2, nil, "success")

	hook.sendEvent(t, "start-file", nil)
	hook.sendEvent(t, "file-loaded", nil)

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("append-play with playlist == 1: expected no error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("append-play with empty playlist timed out waiting for file-loaded")
	}
}

func TestMPV_Load_StartFileSetsLoadFileStarted(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	hook.mpv.loadConfirmMu.Lock()
	hook.mpv.loadFileStarted = false
	hook.mpv.loadConfirmCh = make(chan error, 1)
	hook.mpv.loadConfirmMu.Unlock()

	msg := map[string]interface{}{"event": "start-file"}
	hook.mpv.handleEvent(ipc.MPVEvent{
		Name: "start-file",
		Raw:  mustMarshal(msg),
	})

	hook.mpv.loadConfirmMu.Lock()
	started := hook.mpv.loadFileStarted
	ch := hook.mpv.loadConfirmCh
	hook.mpv.loadConfirmMu.Unlock()

	if !started {
		t.Error("expected loadFileStarted=true after start-file event")
	}
	if ch == nil {
		t.Error("expected loadConfirmCh to be non-nil while Load is waiting")
	}
}

func TestMPV_Load_FileLoadedWithoutStartFile_Ignored(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.Load("test.mp4", "replace", nil, nil)
	}()

	req := hook.readRequest(t)
	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	hook.sendEvent(t, "file-loaded", nil)
	hook.sendEvent(t, "start-file", nil)
	hook.sendEvent(t, "file-loaded", nil)

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected no error when file-loaded arrives after start-file, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Load timed out")
	}
}

func TestMPV_Quit_NotRunning_ErrCode8(t *testing.T) {
	m := NewMPV(process.ProcessConfig{SocketPath: "/tmp/test.sock"})
	err := m.Quit()
	if mpvErr, ok := err.(*MPVError); !ok {
		t.Errorf("expected *MPVError, got %T: %v", err, err)
	} else if mpvErr.Code != ErrCodeNotRunning {
		t.Errorf("expected ErrCodeNotRunning (8), got %d", mpvErr.Code)
	}
}

func TestMPV_ErrorCodeMapping_NodeCompatibility(t *testing.T) {
	nodeMappings := []struct {
		code    ErrorCode
		message string
	}{
		{ErrCodeLoadFailed, "Unable to load file or stream"},
		{ErrCodeInvalidArg, "Invalid argument"},
		{ErrCodeBinaryNotFound, "Binary not found"},
		{ErrCodeIPCCommand, "ipcCommand invalid"},
		{ErrCodeIPCBindFailed, "Unable to bind IPC socket"},
		{ErrCodeTimeout, "Timeout"},
		{ErrCodeAlreadyRunning, "MPV is already running"},
		{ErrCodeIPCSendFailed, "Could not send IPC message"},
		{ErrCodeNotRunning, "MPV is not running"},
		{ErrCodeUnsupportedProto, "Unsupported protocol"},
	}

	for _, m := range nodeMappings {
		err := NewError(m.code, "test()", nil, "", nil)
		if err.Error() != m.message {
			t.Errorf("error code %d: expected message %q, got %q (should match NodeJS)", m.code, m.message, err.Error())
		}
	}
}

func TestMPV_IPCSendFailure_ErrCode7(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	done := make(chan error, 1)
	go func() {
		_, err := hook.mpv.GetProperty("volume")
		done <- err
	}()

	req := hook.readRequest(t)
	reqID := req["request_id"].(float64)

	hook.respond(t, reqID, nil, "property not found")

	err := <-done
	if err == nil {
		t.Error("expected error for failed IPC command")
	}
}

func TestMPV_PropertyObsIncrement_NoConflictWithTimePos(t *testing.T) {
	cfg := process.ProcessConfig{SocketPath: "/tmp/test.sock"}
	m := NewMPV(cfg)

	if m.nextObserveID != 1 {
		t.Errorf("expected nextObserveID to start at 1 (since time-pos=0), got %d", m.nextObserveID)
	}

	m.observedPropsMu.Lock()
	id1 := m.nextObserveID
	m.nextObserveID++
	m.observedPropsMu.Unlock()

	if id1 != 1 {
		t.Errorf("expected first observe ID to be 1, got %d", id1)
	}
}

func TestGetMpvVersion(t *testing.T) {
	m := NewMPV(process.ProcessConfig{SocketPath: "/tmp/test.sock"})

	tests := []struct {
		input    string
		expected string
	}{
		{"mpv 0.38.0", "0.38.0"},
		{"mpv 0.17.0", "0.17.0"},
		{"mpv 999.999.999", "999.999.999"},
		{"UNKNOWN", "999.999.999"},
		{"", "999.999.999"},
		{"mpv abcdef", "999.999.999"},
		{"mpv 1.2.3 Copyright", "1.2.3"},
	}

	for _, tc := range tests {
		result := m.getMpvVersion(tc.input)
		if result != tc.expected {
			t.Errorf("getMpvVersion(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestVersionAtLeast(t *testing.T) {
	tests := []struct {
		version   string
		minVersion string
		expected   bool
	}{
		{"0.38.0", "0.38.0", true},
		{"0.39.0", "0.38.0", true},
		{"0.38.1", "0.38.0", true},
		{"0.37.0", "0.38.0", false},
		{"1.0.0", "0.38.0", true},
		{"0.17.0", "0.38.0", false},
		{"0.38.0", "0.17.0", true},
		{"999.999.999", "0.38.0", true},
	}

	for _, tc := range tests {
		m := NewMPV(process.ProcessConfig{SocketPath: "/tmp/test.sock"})
		m.mpvVersion = tc.version
		result := m.versionAtLeast(tc.minVersion)
		if result != tc.expected {
			t.Errorf("versionAtLeast(%q, %q) = %v, want %v", tc.version, tc.minVersion, result, tc.expected)
		}
	}

	m := NewMPV(process.ProcessConfig{SocketPath: "/tmp/test.sock"})
	if !m.versionAtLeast("0.38.0") {
		t.Error("versionAtLeast should return true when mpvVersion is empty")
	}
}

func TestCmpVersion(t *testing.T) {
	tests := []struct {
		v1, v2   string
		expected bool
	}{
		{"0.38.0", "0.38.0", true},
		{"0.39.0", "0.38.0", true},
		{"0.38.1", "0.38.0", true},
		{"0.37.99", "0.38.0", false},
		{"1.0.0", "0.38.0", true},
		{"0.38.0", "1.0.0", false},
	}

	for _, tc := range tests {
		result := cmpVersion(tc.v1, tc.v2)
		if result != tc.expected {
			t.Errorf("cmpVersion(%q, %q) = %v, want %v", tc.v1, tc.v2, result, tc.expected)
		}
	}
}

func TestLoad_IndexParameter_OldVersion(t *testing.T) {
	cfg := process.ProcessConfig{SocketPath: "/tmp/test.sock"}
	m := NewMPV(cfg)
	m.running.Store(true)
	m.mpvVersion = "0.37.0"

	idx := 2
	err := m.Load("test.mp4", "replace", nil, &idx)
	if err == nil {
		t.Fatal("expected error when index provided with old mpv version")
	}
	if mpvErr, ok := err.(*MPVError); ok {
		if mpvErr.Code != ErrCodeInvalidArg {
			t.Errorf("expected ErrCodeInvalidArg (1), got %d", mpvErr.Code)
		}
	} else {
		t.Errorf("expected *MPVError, got %T", err)
	}
}

func TestLoad_IndexParameter_NewVersion(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	hook.mpv.mpvVersion = "0.38.0"

	done := make(chan error, 1)
	idx := 5
	go func() {
		done <- hook.mpv.Load("test.mp4", "append", nil, &idx)
	}()

	req := hook.readRequest(t)
	cmd, ok := req["command"].([]interface{})
	if !ok {
		t.Fatal("expected command in request")
	}
	if len(cmd) < 4 {
		t.Fatalf("expected at least 4 command args (loadfile source mode index), got %d", len(cmd))
	}
	if cmd[0] != "loadfile" {
		t.Fatalf("expected loadfile command, got %v", cmd[0])
	}
	if idxVal, ok := cmd[3].(float64); !ok || idxVal != 5 {
		t.Errorf("expected index 5, got %v", cmd[3])
	}

	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("expected no error with index on mpv >= 0.38.0, got %v", err)
	}
}

func TestLoad_DefaultIndex_NewVersion(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	hook.mpv.mpvVersion = "0.38.0"

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.Load("test.mp4", "append", nil, nil)
	}()

	req := hook.readRequest(t)
	cmd, ok := req["command"].([]interface{})
	if !ok {
		t.Fatal("expected command in request")
	}
	if len(cmd) < 4 {
		t.Fatalf("expected at least 4 command args (loadfile source mode index) for mpv >= 0.38.0, got %d", len(cmd))
	}
	if idxVal, ok := cmd[3].(float64); !ok || idxVal != -1 {
		t.Errorf("expected default index -1, got %v", cmd[3])
	}

	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("expected no error with default index on mpv >= 0.38.0, got %v", err)
	}
}

func TestLoad_NoDefaultIndex_OldVersion(t *testing.T) {
	hook, cleanup := setupTestHook(t)
	defer cleanup()

	hook.mpv.mpvVersion = "0.37.0"

	done := make(chan error, 1)
	go func() {
		done <- hook.mpv.Load("test.mp4", "append", nil, nil)
	}()

	req := hook.readRequest(t)
	cmd, ok := req["command"].([]interface{})
	if !ok {
		t.Fatal("expected command in request")
	}
	if len(cmd) != 3 {
		t.Fatalf("expected exactly 3 command args (loadfile source mode) for old mpv, got %d: %v", len(cmd), cmd)
	}

	reqID := req["request_id"].(float64)
	hook.respond(t, reqID, nil, "success")

	if err := <-done; err != nil {
		t.Errorf("expected no error without index on old mpv, got %v", err)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}