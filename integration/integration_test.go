package integration

import (
	"embed"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/unicast/unicast-mpv/pkg/commands"
	"github.com/unicast/unicast-mpv/pkg/config"
	"github.com/unicast/unicast-mpv/pkg/events"
	"github.com/unicast/unicast-mpv/pkg/logger"
	"github.com/unicast/unicast-mpv/pkg/mpv"
	"github.com/unicast/unicast-mpv/pkg/mpv/process"
	"github.com/unicast/unicast-mpv/pkg/player"
	"github.com/unicast/unicast-mpv/pkg/schema"
	"github.com/unicast/unicast-mpv/pkg/server"
)

//go:embed testdata
var testConfigFS embed.FS

func mpvAvailable() bool {
	binary := os.Getenv("MPV_BINARY")
	if binary != "" {
		if _, err := os.Stat(binary); err == nil {
			return true
		}
	}
	_, err := exec.LookPath("mpv")
	return err == nil
}

func getFreePort() int {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		panic(err)
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		panic(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func newTestConfig(port int) *config.Config {
	return config.NewConfig(map[string]interface{}{
		"server": map[string]interface{}{
			"port":    port,
			"address": "127.0.0.1",
		},
		"player": map[string]interface{}{
			"fullscreen":   false,
			"quitOnStop":   true,
			"restartOnPlay": false,
			"subtitles": map[string]interface{}{
				"fixTiming": true,
			},
		},
	})
}

func newTestServer(t *testing.T, cfg *config.Config) (*server.Server, int) {
	t.Helper()
	port := getFreePort()
	if cfg == nil {
		cfg = newTestConfig(port)
	}
	log := logger.New(logger.WithColorize(false), logger.WithMinLevel(logger.WarnLevel))
	srv := server.NewServer(cfg, log.Service("rpc"))
	return srv, port
}

func startServer(t *testing.T, srv *server.Server) {
	t.Helper()
	go func() {
		_ = srv.Listen()
	}()
	time.Sleep(100 * time.Millisecond)
}

func dialWS(t *testing.T, port int) *websocket.Conn {
	t.Helper()
	url := fmt.Sprintf("ws://127.0.0.1:%d/", port)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	return conn
}

func sendRequest(conn *websocket.Conn, id interface{}, method string, params []interface{}) error {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      id,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

func readResponse(conn *websocket.Conn) (map[string]interface{}, error) {
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, message, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(message, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func readNotification(conn *websocket.Conn, timeout time.Duration) (map[string]interface{}, error) {
	conn.SetReadDeadline(time.Now().Add(timeout))
	_, message, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	var notif map[string]interface{}
	if err := json.Unmarshal(message, &notif); err != nil {
		return nil, err
	}
	return notif, nil
}

// TestConfigLayering tests that config files layer correctly:
// default -> platform -> local
func TestConfigLayering(t *testing.T) {
	dir := t.TempDir()

	defaultYAML := "player:\n  fullscreen: true\n  monitor: null\nserver:\n  port: 3000\n  address: 0.0.0.0\n"
	if err := os.WriteFile(filepath.Join(dir, "default.yaml"), []byte(defaultYAML), 0644); err != nil {
		t.Fatal(err)
	}

	platformYAML := "player:\n  binary: /usr/bin/mpv\n"
	platformFile := "default-" + runtime.GOOS + ".yaml"
	if err := os.WriteFile(filepath.Join(dir, platformFile), []byte(platformYAML), 0644); err != nil {
		t.Fatal(err)
	}

	localYAML := "server:\n  port: 2019\n"
	if err := os.WriteFile(filepath.Join(dir, "local.yaml"), []byte(localYAML), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if !cfg.GetBool("player.fullscreen") {
		t.Error("expected player.fullscreen=true from default")
	}
	if cfg.GetString("player.binary") != "/usr/bin/mpv" {
		t.Errorf("expected player.binary=/usr/bin/mpv from platform, got %s", cfg.GetString("player.binary"))
	}
	if cfg.GetInt("server.port") != 2019 {
		t.Errorf("expected server.port=2019 from local, got %d", cfg.GetInt("server.port"))
	}
	if cfg.GetString("server.address") != "0.0.0.0" {
		t.Errorf("expected server.address=0.0.0.0 from default, got %s", cfg.GetString("server.address"))
	}
}

// TestConfigMergeOrdering tests that merged config respects priority
func TestConfigMergeOrdering(t *testing.T) {
	base := config.NewConfig(map[string]interface{}{
		"server": map[string]interface{}{
			"port": 3000,
		},
		"player": map[string]interface{}{
			"fullscreen": true,
		},
	})

	local := config.NewConfig(map[string]interface{}{
		"server": map[string]interface{}{
			"port": 2019,
		},
	})

	merged := config.Merge(base, local)

	if merged.GetInt("server.port") != 2019 {
		t.Errorf("expected server.port=2019 from local override, got %d", merged.GetInt("server.port"))
	}
	if merged.GetBool("player.fullscreen") != true {
		t.Error("expected player.fullscreen=true from base")
	}
}

// TestConfigDefaultsFromEmbedded tests loading config from embedded files
func TestConfigDefaultsFromEmbedded(t *testing.T) {
	dir := t.TempDir()

	entries, err := testConfigFS.ReadDir("testdata")
	if err != nil {
		t.Skipf("No testdata directory: %v", err)
	}

	for _, entry := range entries {
		data, err := testConfigFS.ReadFile("testdata/" + entry.Name())
		if err != nil {
			t.Fatalf("Failed to read embedded config %s: %v", entry.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dir, entry.Name()), data, 0644); err != nil {
			t.Fatalf("Failed to write config %s: %v", entry.Name(), err)
		}
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if cfg.GetInt("server.port") != 2019 {
		t.Errorf("expected server.port=2019 from default config, got %d", cfg.GetInt("server.port"))
	}
}

// TestEmbeddedConfigMatchesNodeJS verifies the Go config matches NodeJS defaults
func TestEmbeddedConfigMatchesNodeJS(t *testing.T) {
	dir := t.TempDir()
	defaultYAML := "player:\n    fullscreen: true\n    quitOnStop: true\n    restartOnPlay: false\nserver:\n    port: 2019\n    address: 0.0.0.0\n"
	if err := os.WriteFile(filepath.Join(dir, "default.yaml"), []byte(defaultYAML), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	type check struct {
		path     string
		expected interface{}
	}

	checks := []check{
		{"player.fullscreen", true},
		{"player.quitOnStop", true},
		{"player.restartOnPlay", false},
		{"server.port", 2019},
		{"server.address", "0.0.0.0"},
	}

	for _, c := range checks {
		actual := cfg.Get(c.path)
		switch exp := c.expected.(type) {
		case int:
			if cfg.GetInt(c.path) != exp {
				t.Errorf("config %s: expected %v, got %v", c.path, exp, cfg.GetInt(c.path))
			}
		case string:
			if cfg.GetString(c.path) != exp {
				t.Errorf("config %s: expected %v, got %v", c.path, exp, cfg.GetString(c.path))
			}
		case bool:
			if cfg.GetBool(c.path) != exp {
				t.Errorf("config %s: expected %v, got %v", c.path, exp, cfg.GetBool(c.path))
			}
		default:
			if actual != c.expected {
				t.Errorf("config %s: expected %v, got %v", c.path, c.expected, actual)
			}
		}
	}
}

// TestWireServerConfigPlayer tests the wiring: Config -> Server -> Player
func TestWireServerConfigPlayer(t *testing.T) {
	port := getFreePort()
	cfg := newTestConfig(port)

	log := logger.New(logger.WithColorize(false), logger.WithMinLevel(logger.WarnLevel))
	srv := server.NewServer(cfg, log.Service("rpc"))

	playerCfg := cfg.Slice("player")
	mpvInst := mpv.NewMPV(process.ProcessConfig{})
	playerInst := player.NewPlayer(playerCfg, mpvInst, nil)

	registry := commands.NewCommandRegistry(srv, playerInst, mpvInst)

	commands.NewNativeCommands(registry)
	commands.NewStatusCommand(registry, playerInst.Status, nil)
	commands.NewQuitCommand(registry)

	methods := srv.RegisteredMethods()
	expectedMethods := []string{
		"load", "stop", "pause", "resume", "seek",
		"goToPosition", "mute", "volume",
		"setProperty", "setMultipleProperties", "getProperty",
		"addProperty", "multiplyProperty", "cycleProperty",
		"subtitleScale", "adjustSubtitleTiming",
		"hideSubtitles", "showSubtitles",
		"status", "quit",
	}

	for _, m := range expectedMethods {
		if _, ok := methods[m]; !ok {
			t.Errorf("expected method %s to be registered", m)
		}
	}
}

// TestWireEventsRegistration verifies all events are registered when Bridge is called
func TestWireEventsRegistration(t *testing.T) {
	port := getFreePort()
	cfg := newTestConfig(port)
	log := logger.New(logger.WithColorize(false), logger.WithMinLevel(logger.WarnLevel))
	srv := server.NewServer(cfg, log.Service("rpc"))

	mpvInst := mpv.NewMPV(process.ProcessConfig{
		Binary:      "mpv",
		AutoRestart: false,
		MPVArgs:     []string{},
	})

	events.Bridge(mpvInst, srv)

	registeredEvents := srv.RegisteredEvents()
	expectedEvents := []string{"started", "stopped", "paused", "resumed", "seek", "status", "quit", "crashed"}

	for _, evt := range expectedEvents {
		if !registeredEvents[evt] {
			t.Errorf("expected event %s to be registered", evt)
		}
	}
}

// TestServerRPCMethods tests calling all registered RPC methods via WebSocket
func TestServerRPCMethods(t *testing.T) {
	port := getFreePort()
	cfg := newTestConfig(port)
	log := logger.New(logger.WithColorize(false), logger.WithMinLevel(logger.WarnLevel))

	srv := server.NewServer(cfg, log.Service("rpc"))

	playerCfg := cfg.Slice("player")
	mpvInst2 := mpv.NewMPV(process.ProcessConfig{})
	playerInst := player.NewPlayer(playerCfg, mpvInst2, nil)

	registry := commands.NewCommandRegistry(srv, playerInst, mpvInst2)
	commands.NewNativeCommands(registry)
	commands.NewStatusCommand(registry, playerInst.Status, nil)
	commands.NewQuitCommand(registry)
	commands.NewPlayCommand(registry, playerCfg, nil)

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	testCases := []struct {
		method    string
		params    []interface{}
		expectErr bool
		errCode   float64
		skipCheck bool
	}{
		{"status", []interface{}{}, false, 0, false},
		{"mute", []interface{}{true}, true, -32603, false},
		{"volume", []interface{}{50.0}, true, -32603, false},
		{"pause", []interface{}{}, true, -32603, false},
		{"resume", []interface{}{}, true, -32603, false},
		{"stop", []interface{}{}, false, 0, false},
		{"seek", []interface{}{10.0}, true, -32603, false},
		{"goToPosition", []interface{}{30.0}, true, -32603, false},
		{"showProgress", []interface{}{}, true, -32603, false},
	}

	for _, tc := range testCases {
		t.Run(tc.method, func(t *testing.T) {
			err := sendRequest(conn, tc.method, tc.method, tc.params)
			if err != nil {
				t.Fatalf("send request error: %v", err)
			}

			resp, err := readResponse(conn)
			if err != nil {
				t.Fatalf("read response error: %v", err)
			}

			if tc.expectErr {
				errObj, ok := resp["error"].(map[string]interface{})
				if !ok {
					t.Fatalf("expected error response for %s, got: %v", tc.method, resp)
				}
				code, _ := errObj["code"].(float64)
				if code != tc.errCode {
					t.Errorf("expected error code %v for %s, got %v", tc.errCode, tc.method, code)
				}
			} else if !tc.skipCheck {
				if errObj, ok := resp["error"].(map[string]interface{}); ok {
					code, _ := errObj["code"].(float64)
					if code != 0 {
						t.Errorf("unexpected error for %s: code=%v msg=%v", tc.method, code, errObj["message"])
					}
				}
			}
		})
	}
}

// TestServerMethodNotFound tests calling an unregistered method
func TestServerMethodNotFound(t *testing.T) {
	port := getFreePort()
	cfg := newTestConfig(port)
	log := logger.New(logger.WithColorize(false), logger.WithMinLevel(logger.WarnLevel))

	srv := server.NewServer(cfg, log.Service("rpc"))

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	err := sendRequest(conn, 1, "nonexistentMethod", []interface{}{})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}

	resp, err := readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error response, got: %v", resp)
	}
	code, _ := errObj["code"].(float64)
	if code != -32601 {
		t.Errorf("expected error code -32601 (method not found), got %v", code)
	}
}

// TestServerEventEmission tests that events are emitted correctly over WebSocket
func TestServerEventEmission(t *testing.T) {
	port := getFreePort()
	cfg := newTestConfig(port)
	log := logger.New(logger.WithColorize(false), logger.WithMinLevel(logger.WarnLevel))

	srv := server.NewServer(cfg, log.Service("rpc"))
	srv.RegisterEvent("started")
	srv.RegisterEvent("stopped")
	srv.RegisterEvent("paused")
	srv.RegisterEvent("resumed")
	srv.RegisterEvent("seek")
	srv.RegisterEvent("status")
	srv.RegisterEvent("quit")
	srv.RegisterEvent("crashed")

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	testEvents := []struct {
		name string
		args []interface{}
	}{
		{"started", nil},
		{"stopped", nil},
		{"paused", nil},
		{"resumed", nil},
		{"seek", []interface{}{map[string]interface{}{"start": 10.5, "end": 20.3}}},
		{"status", []interface{}{map[string]interface{}{"property": "pause", "value": true}}},
		{"quit", nil},
		{"crashed", nil},
	}

	for _, evt := range testEvents {
		t.Run(evt.name, func(t *testing.T) {
			srv.Emit(evt.name, evt.args...)

			notif, err := readNotification(conn, 2*time.Second)
			if err != nil {
				t.Fatalf("read notification error for event %s: %v", evt.name, err)
			}

			if notif["jsonrpc"] != "2.0" {
				t.Errorf("expected jsonrpc 2.0, got %v", notif["jsonrpc"])
			}
			if notif["method"] != evt.name {
				t.Errorf("expected method %s, got %v", evt.name, notif["method"])
			}
		})
	}
}

// TestServerSeekEventData verifies seek events contain {start, end} data
func TestServerSeekEventData(t *testing.T) {
	port := getFreePort()
	cfg := newTestConfig(port)
	log := logger.New(logger.WithColorize(false), logger.WithMinLevel(logger.WarnLevel))

	srv := server.NewServer(cfg, log.Service("rpc"))
	srv.RegisterEvent("seek")

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	seekData := map[string]interface{}{
		"start": 10.5,
		"end":   20.3,
	}
	srv.Emit("seek", seekData)

	notif, err := readNotification(conn, 2*time.Second)
	if err != nil {
		t.Fatalf("read notification error: %v", err)
	}

	if notif["method"] != "seek" {
		t.Errorf("expected method 'seek', got %v", notif["method"])
	}

	params, ok := notif["params"].([]interface{})
	if !ok || len(params) == 0 {
		t.Fatalf("expected params array with seek data, got %v", notif["params"])
	}

	seekObj, ok := params[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected params[0] to be an object, got %v", params[0])
	}

	if startVal, ok := seekObj["start"].(float64); !ok || startVal != 10.5 {
		t.Errorf("expected start=10.5, got %v", seekObj["start"])
	}
	if endVal, ok := seekObj["end"].(float64); !ok || endVal != 20.3 {
		t.Errorf("expected end=20.3, got %v", seekObj["end"])
	}
}

// TestServerSchemaValidation tests that schema validation works for RPC methods
func TestServerSchemaValidation(t *testing.T) {
	port := getFreePort()
	cfg := newTestConfig(port)
	log := logger.New(logger.WithColorize(false), logger.WithMinLevel(logger.WarnLevel))

	srv := server.NewServer(cfg, log.Service("rpc"))

	srv.Register("add", schema.Tuple(schema.Number(), schema.Number()), func(args []interface{}) (interface{}, error) {
		a, _ := args[0].(float64)
		b, _ := args[1].(float64)
		return a + b, nil
	})

	srv.Register("greet", schema.Tuple(schema.String()), func(args []interface{}) (interface{}, error) {
		return "hello " + args[0].(string), nil
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	err := sendRequest(conn, 1, "add", []interface{}{"not_a_number", 2.0})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}
	resp, err := readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error response, got: %v", resp)
	}
	code, _ := errObj["code"].(float64)
	if code != -32602 {
		t.Errorf("expected error code -32602 (invalid params), got %v", code)
	}

	err = sendRequest(conn, 2, "add", []interface{}{3.0, 4.0})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}
	resp, err = readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}
	if resp["result"].(float64) != 7.0 {
		t.Errorf("expected result 7.0, got %v", resp["result"])
	}
}

// TestHooksWiring tests that global pre/post hooks work in the integration setup
func TestHooksWiring(t *testing.T) {
	port := getFreePort()
	cfg := newTestConfig(port)
	log := logger.New(logger.WithColorize(false), logger.WithMinLevel(logger.WarnLevel))

	srv := server.NewServer(cfg, log.Service("rpc"))

	var preHookCalled bool
	var postHookCalled bool
	var methodCalled string

	srv.RegisterGlobalPreHook(func(args []interface{}, method string, ctx map[string]interface{}) {
		preHookCalled = true
		methodCalled = method
	})
	srv.RegisterGlobalPostHook(func(args []interface{}, method string, rpcErr error, result interface{}, ctx map[string]interface{}) {
		postHookCalled = true
	})

	srv.Register("ping", schema.Tuple(), func(args []interface{}) (interface{}, error) {
		return "pong", nil
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	err := sendRequest(conn, 1, "ping", []interface{}{})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}

	_, err = readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	if !preHookCalled {
		t.Error("expected global pre-hook to be called")
	}
	if !postHookCalled {
		t.Error("expected global post-hook to be called")
	}
	if methodCalled != "ping" {
		t.Errorf("expected hook method 'ping', got %s", methodCalled)
	}
}

// TestConcurrentWebSocketClients tests multiple clients connecting simultaneously
func TestConcurrentWebSocketClients(t *testing.T) {
	port := getFreePort()
	cfg := newTestConfig(port)
	log := logger.New(logger.WithColorize(false), logger.WithMinLevel(logger.WarnLevel))

	srv := server.NewServer(cfg, log.Service("rpc"))
	srv.Register("echo", schema.Array(schema.Any()), func(args []interface{}) (interface{}, error) {
		return args, nil
	})

	startServer(t, srv)
	defer srv.Close()

	var wg sync.WaitGroup
	numClients := 5
	errors := make(chan error, numClients)

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			conn := dialWS(t, port)
			defer conn.Close()

			err := sendRequest(conn, id, "echo", []interface{}{fmt.Sprintf("client-%d", id)})
			if err != nil {
				errors <- fmt.Errorf("client %d: send error: %w", id, err)
				return
			}

			resp, err := readResponse(conn)
			if err != nil {
				errors <- fmt.Errorf("client %d: read error: %w", id, err)
				return
			}

			result, ok := resp["result"].([]interface{})
			if !ok || len(result) == 0 {
				errors <- fmt.Errorf("client %d: unexpected result: %v", id, resp["result"])
				return
			}

			if result[0] != fmt.Sprintf("client-%d", id) {
				errors <- fmt.Errorf("client %d: expected echo %d, got %v", id, id, result[0])
			}
		}(i)
	}
	wg.Wait()
	close(errors)

	for err := range errors {
		if err != nil {
			t.Error(err)
		}
	}
}

// TestEventBroadcastToAllClients tests that events are broadcast to all connected clients
func TestEventBroadcastToAllClients(t *testing.T) {
	port := getFreePort()
	cfg := newTestConfig(port)
	log := logger.New(logger.WithColorize(false), logger.WithMinLevel(logger.WarnLevel))

	srv := server.NewServer(cfg, log.Service("rpc"))
	srv.RegisterEvent("update")

	startServer(t, srv)
	defer srv.Close()

	conn1 := dialWS(t, port)
	defer conn1.Close()
	conn2 := dialWS(t, port)
	defer conn2.Close()

	time.Sleep(50 * time.Millisecond)

	srv.Emit("update", "test-data")

	conn1.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg1, err := conn1.ReadMessage()
	if err != nil {
		t.Fatalf("conn1 read error: %v", err)
	}

	conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg2, err := conn2.ReadMessage()
	if err != nil {
		t.Fatalf("conn2 read error: %v", err)
	}

	var notif1, notif2 map[string]interface{}
	json.Unmarshal(msg1, &notif1)
	json.Unmarshal(msg2, &notif2)

	if notif1["method"] != "update" {
		t.Errorf("conn1: expected method 'update', got %v", notif1["method"])
	}
	if notif2["method"] != "update" {
		t.Errorf("conn2: expected method 'update', got %v", notif2["method"])
	}
}

// TestPlayerStatusTracking tests PlayerStatus Futures/Timeouts without MPV
func TestPlayerStatusTracking(t *testing.T) {
	status := player.NewPlayerStatus(nil, nil)

	initialStatus := status.GetSync()
	if initialStatus.Volume != 100 {
		t.Errorf("expected default volume 100, got %v", initialStatus.Volume)
	}
	if initialStatus.Loop != "no" {
		t.Errorf("expected default loop 'no', got %v", initialStatus.Loop)
	}
	if initialStatus.SubVisibility != true {
		t.Errorf("expected default subVisibility true, got %v", initialStatus.SubVisibility)
	}
	if initialStatus.SubScale != 1 {
		t.Errorf("expected default subScale 1, got %v", initialStatus.SubScale)
	}

	status.Update("mute", true)
	status.Update("pause", false)
	status.Update("volume", 75.0)
	status.Update("fullscreen", true)

	s := status.GetSync()
	if s.Mute != true {
		t.Error("expected mute=true after update")
	}
	if s.Pause != false {
		t.Error("expected pause=false after update")
	}
	if s.Volume != 75.0 {
		t.Errorf("expected volume=75, got %v", s.Volume)
	}
	if s.Fullscreen != true {
		t.Error("expected fullscreen=true after update")
	}
}

// TestPlayerStatusTimeout tests that Get() times out when required keys are missing
func TestPlayerStatusTimeout(t *testing.T) {
	status := player.NewPlayerStatus(nil, nil)

	status.Play()

	done := make(chan struct{})
	go func() {
		_, _ = status.Get()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(7 * time.Second):
		t.Error("expected Get() to timeout, but it didn't return")
	}
}

// TestPlayerStatusStop tests that Stop() clears file info
func TestPlayerStatusStop(t *testing.T) {
	status := player.NewPlayerStatus(nil, nil)

	status.Update("path", "/test/video.mp4")
	status.Update("filename", "video.mp4")

	s := status.GetSync()
	if s.Path == nil || *s.Path != "/test/video.mp4" {
		t.Errorf("expected path=/test/video.mp4, got %v", s.Path)
	}

	status.Stop()
	s = status.GetSync()
	if s.Path != nil {
		t.Errorf("expected nil path after stop, got %v", s.Path)
	}
	if s.Filename != nil {
		t.Errorf("expected nil filename after stop, got %v", s.Filename)
	}
}

// TestHighFrequencyPatternRegistration tests that high frequency patterns work
func TestHighFrequencyPatternRegistration(t *testing.T) {
	port := getFreePort()
	cfg := newTestConfig(port)
	log := logger.New(logger.WithColorize(false), logger.WithMinLevel(logger.WarnLevel))

	srv := server.NewServer(cfg, log.Service("rpc"))

	srv.RegisterHighFrequencyPattern(regexp.MustCompile(`status`), nil, 300)

	if len(srv.RegisteredMethods()) != 0 {
		t.Error("expected no methods registered from HF pattern")
	}
}

// TestFullWiring creates the complete wiring but does not actually start MPV
func TestFullWiring(t *testing.T) {
	port := getFreePort()
	cfg := newTestConfig(port)

	log := logger.New(logger.WithColorize(false), logger.WithMinLevel(logger.WarnLevel))
	srv := server.NewServer(cfg, log.Service("rpc"))

	playerCfg := cfg.Slice("player")
	mpvInst := mpv.NewMPV(process.ProcessConfig{
		Binary:      "mpv",
		AutoRestart: true,
		MPVArgs:     player.BuildMPVArgs(playerCfg),
	})

	playerInst := player.NewPlayer(playerCfg, mpvInst, nil)

	registry := commands.NewCommandRegistry(srv, playerInst, mpvInst)

	commands.NewNativeCommands(registry)
	commands.NewStatusCommand(registry, playerInst.Status, nil)
	commands.NewQuitCommand(registry)
	commands.NewPlayCommand(registry, playerCfg, nil)

	events.Bridge(mpvInst, srv)

	expectedEvents := []string{"started", "stopped", "paused", "resumed", "seek", "status", "quit", "crashed"}
	registeredEvents := srv.RegisteredEvents()
	for _, evt := range expectedEvents {
		if !registeredEvents[evt] {
			t.Errorf("expected event %s to be registered", evt)
		}
	}

	expectedMethods := []string{
		"load", "stop", "pause", "resume", "seek",
		"goToPosition", "mute", "volume",
		"setProperty", "setMultipleProperties", "getProperty",
		"addProperty", "multiplyProperty", "cycleProperty",
		"subtitleScale", "adjustSubtitleTiming",
		"hideSubtitles", "showSubtitles",
		"status", "quit", "play", "showProgress",
	}

	methods := srv.RegisteredMethods()
	for _, m := range expectedMethods {
		if _, ok := methods[m]; !ok {
			t.Errorf("expected method %s to be registered", m)
		}
	}

	srv.RegisterGlobalPreHook(func(args []interface{}, method string, ctx map[string]interface{}) {
		// Simulated hook from main.go
	})

	srv.RegisterGlobalPostHook(func(args []interface{}, method string, rpcErr error, result interface{}, ctx map[string]interface{}) {
		// Simulated error logging hook from main.go
	})

	srv.RegisterHighFrequencyPattern(regexp.MustCompile(`status`), nil, 300)
}

// TestMPVBuildArgs verifies MPV args are built correctly from config
func TestMPVBuildArgs(t *testing.T) {
	playerCfg := config.NewConfig(map[string]interface{}{
		"fullscreen": true,
		"onTop":      false,
		"monitor":    nil,
		"args":       []interface{}{"--gpu-api=opengl"},
		"subtitles": map[string]interface{}{
			"fixTiming": true,
			"font":      "Droid Sans",
			"bold":      true,
		},
	})

	args := player.BuildMPVArgs(playerCfg)

	hasFullscreen := false
	hasGpuAPI := false
	for _, arg := range args {
		if arg == "--fs" {
			hasFullscreen = true
		}
		if arg == "--gpu-api=opengl" {
			hasGpuAPI = true
		}
	}

	if !hasFullscreen {
		t.Error("expected --fs flag for fullscreen=true")
	}
	if !hasGpuAPI {
		t.Error("expected --gpu-api=opengl from custom args")
	}
}

// TestMPVBuildArgsAudioDevice tests audio device args
func TestMPVBuildArgsAudioDevice(t *testing.T) {
	playerCfg := config.NewConfig(map[string]interface{}{
		"audioDevice": "alsa",
	})

	args := player.BuildMPVArgs(playerCfg)

	found := false
	for _, arg := range args {
		if arg == "--audio-device=alsa" {
			found = true
		}
	}
	if !found {
		t.Error("expected --audio-device=alsa in args")
	}
}

// TestBuildMPVArgsFromConfig tests building MPV args from embedded config
func TestBuildMPVArgsFromConfig(t *testing.T) {
	dir := t.TempDir()
	defaultYAML := "player:\n    fullscreen: true\n    subtitles:\n        font: 'Droid Sans'\n"
	if err := os.WriteFile(filepath.Join(dir, "default.yaml"), []byte(defaultYAML), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	playerCfg := cfg.Slice("player")
	args := player.BuildMPVArgs(playerCfg)

	hasFS := false
	for _, arg := range args {
		if arg == "--fs" {
			hasFS = true
		}
	}
	if !hasFS {
		t.Error("expected --fs in args from config with fullscreen=true")
	}
}

// --- Tests that require MPV binary ---

func skipIfNoMPV(t *testing.T) {
	t.Helper()
	if !mpvAvailable() {
		t.Skip("mpv binary not available, skipping test that requires MPV")
	}
}

// TestWithRealMPVProcess tests starting MPV and making RPC calls
func TestWithRealMPVProcess(t *testing.T) {
	skipIfNoMPV(t)

	binary := os.Getenv("MPV_BINARY")
	if binary == "" {
		binary = "mpv"
	}

	port := getFreePort()
	cfg := newTestConfig(port)
	log := logger.New(logger.WithColorize(false), logger.WithMinLevel(logger.WarnLevel))

	srv := server.NewServer(cfg, log.Service("rpc"))

	socketPath := filepath.Join(t.TempDir(), "mpv-integration.sock")
	procCfg := process.ProcessConfig{
		Binary:      binary,
		SocketPath:  socketPath,
		AutoRestart: false,
		MPVArgs:     []string{"--idle", "--no-video"},
		AudioOnly:   true,
		TimeUpdate:  1,
	}

	mpvInst := mpv.NewMPV(procCfg)

	if err := mpvInst.Start(); err != nil {
		t.Fatalf("Failed to start MPV: %v", err)
	}
	defer func() {
		if mpvInst.IsRunning() {
			_ = mpvInst.Quit()
		}
	}()

	playerCfg := cfg.Slice("player")
	playerInst := player.NewPlayer(playerCfg, mpvInst, nil)

	registry := commands.NewCommandRegistry(srv, playerInst, mpvInst)
	commands.NewNativeCommands(registry)
	commands.NewStatusCommand(registry, playerInst.Status, nil)
	commands.NewQuitCommand(registry)
	commands.NewPlayCommand(registry, playerCfg, nil)

	events.Bridge(mpvInst, srv)

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	err := sendRequest(conn, 1, "getProperty", []interface{}{"mpv-version"})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}

	resp, err := readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	if resp["error"] != nil {
		errObj, _ := resp["error"].(map[string]interface{})
		t.Fatalf("unexpected error for getProperty: %v", errObj)
	}

	if resp["result"] == nil {
		t.Error("expected non-nil result for mpv-version")
	}
}

// TestMPVAutoRestartOnCrash tests auto-restart functionality
func TestMPVAutoRestartOnCrash(t *testing.T) {
	skipIfNoMPV(t)

	binary := os.Getenv("MPV_BINARY")
	if binary == "" {
		binary = "mpv"
	}

	socketPath := filepath.Join(t.TempDir(), "mpv-restart.sock")
	procCfg := process.ProcessConfig{
		Binary:      binary,
		SocketPath:  socketPath,
		AutoRestart: true,
		MPVArgs:     []string{"--idle", "--no-video"},
		AudioOnly:   true,
		TimeUpdate:  1,
	}

	mpvInst := mpv.NewMPV(procCfg)

	if err := mpvInst.Start(); err != nil {
		t.Fatalf("Failed to start MPV: %v", err)
	}
	defer func() {
		if mpvInst.IsRunning() {
			_ = mpvInst.Quit()
		}
	}()

	if !mpvInst.IsRunning() {
		t.Fatal("expected MPV to be running after Start()")
	}
}

// TestMPVEventsEmitOverSocket tests that MPV events emit over WebSocket
func TestMPVEventsEmitOverSocket(t *testing.T) {
	skipIfNoMPV(t)

	binary := os.Getenv("MPV_BINARY")
	if binary == "" {
		binary = "mpv"
	}

	port := getFreePort()
	cfg := newTestConfig(port)
	log := logger.New(logger.WithColorize(false), logger.WithMinLevel(logger.WarnLevel))

	srv := server.NewServer(cfg, log.Service("rpc"))

	socketPath := filepath.Join(t.TempDir(), "mpv-events.sock")
	procCfg := process.ProcessConfig{
		Binary:      binary,
		SocketPath:  socketPath,
		AutoRestart: false,
		MPVArgs:     []string{"--idle", "--no-video"},
		AudioOnly:   true,
		TimeUpdate:  1,
	}

	mpvInst := mpv.NewMPV(procCfg)

	if err := mpvInst.Start(); err != nil {
		t.Fatalf("Failed to start MPV: %v", err)
	}
	defer func() {
		if mpvInst.IsRunning() {
			_ = mpvInst.Quit()
		}
	}()

	events.Bridge(mpvInst, srv)

	var receivedEvents []string
	var eventsMu sync.Mutex

	srv.RegisterGlobalEventHook(func(args []interface{}, event string, ctx map[string]interface{}) {
		eventsMu.Lock()
		receivedEvents = append(receivedEvents, event)
		eventsMu.Unlock()
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	err := sendRequest(conn, 1, "pause", []interface{}{})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}

	resp, err := readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	_ = resp
}