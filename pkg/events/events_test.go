package events

import (
	"sync"
	"testing"

	"github.com/unicast/unicast-mpv/pkg/config"
	"github.com/unicast/unicast-mpv/pkg/mpv"
	"github.com/unicast/unicast-mpv/pkg/mpv/process"
	"github.com/unicast/unicast-mpv/pkg/server"
)

func newTestServerConfig() *config.ServerConfig {
	return &config.ServerConfig{
		Port:    0,
		Address: "127.0.0.1",
	}
}

func newTestProcessConfig() process.ProcessConfig {
	return process.ProcessConfig{
		SocketPath: "/tmp/test-events.sock",
	}
}

func TestBridge_RegistersAllEvents(t *testing.T) {
	srvCfg := newTestServerConfig()
	srv := server.NewServer(srvCfg, nil)
	m := mpv.NewMPV(newTestProcessConfig())

	Bridge(m, srv)

	registered := srv.RegisteredEvents()
	for _, name := range eventNames {
		if !registered[name] {
			t.Errorf("expected event %q to be registered", name)
		}
	}
}

func TestBridge_RegistersExactlyEightEvents(t *testing.T) {
	srvCfg := newTestServerConfig()
	srv := server.NewServer(srvCfg, nil)
	m := mpv.NewMPV(newTestProcessConfig())

	Bridge(m, srv)

	registered := srv.RegisteredEvents()
	if len(registered) != 8 {
		t.Errorf("expected exactly 8 events registered, got %d", len(registered))
	}
}

func TestBridge_EventNames(t *testing.T) {
	expected := map[string]bool{
		"started": true,
		"stopped": true,
		"paused":  true,
		"resumed": true,
		"seek":    true,
		"status":  true,
		"quit":    true,
		"crashed": true,
	}

	for _, name := range eventNames {
		if !expected[name] {
			t.Errorf("unexpected event name: %s", name)
		}
	}
	if len(eventNames) != len(expected) {
		t.Errorf("expected %d event names, got %d", len(expected), len(eventNames))
	}
}

type emittedEvent struct {
	name string
	args []interface{}
}

type capturingServer struct {
	events  map[string]bool
	mu      sync.Mutex
	emitted []emittedEvent
}

func newCapturingServer() *capturingServer {
	return &capturingServer{
		events: make(map[string]bool),
	}
}

func (s *capturingServer) RegisterEvent(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[name] = true
}

func (s *capturingServer) Emit(event string, args ...interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emitted = append(s.emitted, emittedEvent{name: event, args: args})
}

func (s *capturingServer) getEmitted() []emittedEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]emittedEvent, len(s.emitted))
	copy(result, s.emitted)
	return result
}

func (s *capturingServer) hasEvent(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.events[name]
}

func (s *capturingServer) eventCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

func TestBridge_NoExtraEventsWithCapturingServer(t *testing.T) {
	srv := newCapturingServer()
	m := mpv.NewMPV(newTestProcessConfig())

	BridgeWithServer(m, srv)

	if srv.eventCount() != len(eventNames) {
		t.Errorf("expected %d events registered, got %d", len(eventNames), srv.eventCount())
	}
}

func TestBridge_SimpleEventForwarding(t *testing.T) {
	srv := newCapturingServer()
	m := mpv.NewMPV(newTestProcessConfig())

	BridgeWithServer(m, srv)

	simpleEvents := []string{"started", "stopped", "paused", "resumed", "quit", "crashed"}
	for _, evtName := range simpleEvents {
		m.EmitTestEvent(evtName)
	}

	emitted := srv.getEmitted()
	if len(emitted) != len(simpleEvents) {
		t.Fatalf("expected %d emitted events, got %d", len(simpleEvents), len(emitted))
	}

	for i, e := range emitted {
		if e.name != simpleEvents[i] {
			t.Errorf("event %d: expected %q, got %q", i, simpleEvents[i], e.name)
		}
	}
}

func TestBridge_SeekEventForwardsData(t *testing.T) {
	srv := newCapturingServer()
	m := mpv.NewMPV(newTestProcessConfig())

	BridgeWithServer(m, srv)

	seekData := map[string]interface{}{
		"start": 10.0,
		"end":   15.0,
	}
	m.EmitTestEvent("seek", seekData)

	emitted := srv.getEmitted()
	if len(emitted) != 1 {
		t.Fatalf("expected 1 emitted event, got %d", len(emitted))
	}
	if emitted[0].name != "seek" {
		t.Errorf("expected event 'seek', got %q", emitted[0].name)
	}
	if len(emitted[0].args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(emitted[0].args))
	}
	data, ok := emitted[0].args[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{} arg, got %T", emitted[0].args[0])
	}
	if data["start"] != 10.0 {
		t.Errorf("expected start=10.0, got %v", data["start"])
	}
	if data["end"] != 15.0 {
		t.Errorf("expected end=15.0, got %v", data["end"])
	}
}

func TestBridge_StatusEventForwardsData(t *testing.T) {
	srv := newCapturingServer()
	m := mpv.NewMPV(newTestProcessConfig())

	BridgeWithServer(m, srv)

	statusData := map[string]interface{}{
		"property": "volume",
		"value":    75,
	}
	m.EmitTestEvent("status", statusData)

	emitted := srv.getEmitted()
	if len(emitted) != 1 {
		t.Fatalf("expected 1 emitted event, got %d", len(emitted))
	}
	if emitted[0].name != "status" {
		t.Errorf("expected event 'status', got %q", emitted[0].name)
	}
	data, ok := emitted[0].args[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{} arg, got %T", emitted[0].args[0])
	}
	if data["property"] != "volume" {
		t.Errorf("expected property=volume, got %v", data["property"])
	}
}

func TestBridge_QuitEventForwardsData(t *testing.T) {
	srv := newCapturingServer()
	m := mpv.NewMPV(newTestProcessConfig())

	BridgeWithServer(m, srv)

	m.EmitTestEvent("quit", "exit-data")

	emitted := srv.getEmitted()
	if len(emitted) != 1 {
		t.Fatalf("expected 1 emitted event, got %d", len(emitted))
	}
	if emitted[0].name != "quit" {
		t.Errorf("expected event 'quit', got %q", emitted[0].name)
	}
}

func TestBridge_CrashedEventForwardsData(t *testing.T) {
	srv := newCapturingServer()
	m := mpv.NewMPV(newTestProcessConfig())

	BridgeWithServer(m, srv)

	m.EmitTestEvent("crashed", "error-info")

	emitted := srv.getEmitted()
	if len(emitted) != 1 {
		t.Fatalf("expected 1 emitted event, got %d", len(emitted))
	}
	if emitted[0].name != "crashed" {
		t.Errorf("expected event 'crashed', got %q", emitted[0].name)
	}
}

func TestBridge_MultipleEventsInOrder(t *testing.T) {
	srv := newCapturingServer()
	m := mpv.NewMPV(newTestProcessConfig())

	BridgeWithServer(m, srv)

	m.EmitTestEvent("started")
	m.EmitTestEvent("paused")
	m.EmitTestEvent("resumed")
	m.EmitTestEvent("stopped")

	emitted := srv.getEmitted()
	if len(emitted) != 4 {
		t.Fatalf("expected 4 emitted events, got %d", len(emitted))
	}

	expected := []string{"started", "paused", "resumed", "stopped"}
	for i, e := range emitted {
		if e.name != expected[i] {
			t.Errorf("event %d: expected %q, got %q", i, expected[i], e.name)
		}
	}
}

func TestBridge_SimpleEventsHaveNoArgs(t *testing.T) {
	srv := newCapturingServer()
	m := mpv.NewMPV(newTestProcessConfig())

	BridgeWithServer(m, srv)

	m.EmitTestEvent("started")
	m.EmitTestEvent("stopped")
	m.EmitTestEvent("paused")
	m.EmitTestEvent("resumed")

	emitted := srv.getEmitted()
	for _, e := range emitted {
		if len(e.args) != 0 {
			t.Errorf("event %q: expected 0 args, got %d", e.name, len(e.args))
		}
	}
}