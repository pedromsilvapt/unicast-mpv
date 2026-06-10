package server

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/unicast/unicast-mpv/pkg/config"
	"github.com/unicast/unicast-mpv/pkg/schema"
)

type testLogger struct {
	messages []string
	mu       sync.Mutex
}

func (l *testLogger) Info(msg string) {
	l.mu.Lock()
	l.messages = append(l.messages, "[INFO] "+msg)
	l.mu.Unlock()
}
func (l *testLogger) Debug(msg string) {
	l.mu.Lock()
	l.messages = append(l.messages, "[DEBUG] "+msg)
	l.mu.Unlock()
}
func (l *testLogger) Error(msg string) {
	l.mu.Lock()
	l.messages = append(l.messages, "[ERROR] "+msg)
	l.mu.Unlock()
}
func (l *testLogger) Warn(msg string) {
	l.mu.Lock()
	l.messages = append(l.messages, "[WARN] "+msg)
	l.mu.Unlock()
}
func (l *testLogger) GetMessages() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]string, len(l.messages))
	copy(result, l.messages)
	return result
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

func newTestServer(t *testing.T) (*Server, int) {
	t.Helper()
	port := getFreePort()
	cfg := newConfig(port)
	logger := &testLogger{}
	srv := NewServer(cfg, logger)
	return srv, port
}

func newConfig(port int) *config.Config {
	return config.NewConfig(map[string]interface{}{
		"server": map[string]interface{}{
			"port":    port,
			"address": "127.0.0.1",
		},
	})
}

func startServer(t *testing.T, srv *Server) {
	t.Helper()
	go func() {
		err := srv.Listen()
		if err != nil && !strings.Contains(err.Error(), "use of closed") {
			t.Logf("Server exited with error: %v", err)
		}
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

func TestNewServer(t *testing.T) {
	cfg := newConfig(8080)
	srv := NewServer(cfg, nil)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestNewServerWithLogger(t *testing.T) {
	cfg := newConfig(8080)
	logger := &testLogger{}
	srv := NewServer(cfg, logger)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestRegisterMethod(t *testing.T) {
	cfg := newConfig(8080)
	srv := NewServer(cfg, nil)
	srv.Register("add", schema.Tuple(schema.Number(), schema.Number()), func(args []interface{}) (interface{}, error) {
		a, _ := args[0].(float64)
		b, _ := args[1].(float64)
		return a + b, nil
	})
	if len(srv.methods) != 1 {
		t.Errorf("expected 1 method registered, got %d", len(srv.methods))
	}
}

func TestRegisterEvent(t *testing.T) {
	cfg := newConfig(8080)
	srv := NewServer(cfg, nil)
	srv.RegisterEvent("status")
	if len(srv.events) != 1 {
		t.Errorf("expected 1 event registered, got %d", len(srv.events))
	}
}

func TestRPCMethodCall(t *testing.T) {
	srv, port := newTestServer(t)
	srv.Register("add", schema.Tuple(schema.Number(), schema.Number()), func(args []interface{}) (interface{}, error) {
		a, _ := args[0].(float64)
		b, _ := args[1].(float64)
		return a + b, nil
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	err := sendRequest(conn, 1, "add", []interface{}{3.0, 4.0})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}

	resp, err := readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	if resp["jsonrpc"] != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %v", resp["jsonrpc"])
	}

	result, ok := resp["result"].(float64)
	if !ok {
		t.Fatalf("expected result to be number, got %v", resp["result"])
	}
	if result != 7.0 {
		t.Errorf("expected result 7, got %v", result)
	}

	id, ok := resp["id"].(float64)
	if !ok {
		t.Fatalf("expected id to be number, got %v", resp["id"])
	}
	if id != 1 {
		t.Errorf("expected id 1, got %v", id)
	}
}

func TestRPCMethodWithNoParams(t *testing.T) {
	srv, port := newTestServer(t)
	srv.Register("ping", schema.Array(schema.Any()), func(args []interface{}) (interface{}, error) {
		return "pong", nil
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "ping",
		"id":      2,
	}
	data, _ := json.Marshal(req)
	err := conn.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}

	resp, err := readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	if resp["result"] != "pong" {
		t.Errorf("expected result 'pong', got %v", resp["result"])
	}
}

func TestRPCMethodNotFound(t *testing.T) {
	srv, port := newTestServer(t)

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	err := sendRequest(conn, 1, "nonexistent", nil)
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}

	resp, err := readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object, got %v", resp)
	}

	code, _ := errObj["code"].(float64)
	if code != -32601 {
		t.Errorf("expected error code -32601, got %v", code)
	}
}

func TestRPCInvalidParams(t *testing.T) {
	srv, port := newTestServer(t)
	srv.Register("add", schema.Tuple(schema.Number(), schema.Number()), func(args []interface{}) (interface{}, error) {
		return nil, nil
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	err := sendRequest(conn, 1, "add", []interface{}{"not_a_number"})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}

	resp, err := readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object, got %v", resp)
	}

	code, _ := errObj["code"].(float64)
	if code != -32602 {
		t.Errorf("expected error code -32602, got %v", code)
	}
}

func TestRPCMethodReturnsError(t *testing.T) {
	srv, port := newTestServer(t)
	srv.Register("fail", schema.Array(schema.Any()), func(args []interface{}) (interface{}, error) {
		return nil, NewRPCError(4200, "custom error")
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	err := sendRequest(conn, 1, "fail", []interface{}{})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}

	resp, err := readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object, got %v", resp)
	}

	code, _ := errObj["code"].(float64)
	if code != 4200 {
		t.Errorf("expected custom error code 4200, got %v", code)
	}

	msg, _ := errObj["message"].(string)
	if msg != "custom error" {
		t.Errorf("expected custom error message 'custom error', got %v", msg)
	}
}

func TestRPCPlainError(t *testing.T) {
	srv, port := newTestServer(t)
	srv.Register("plainerr", schema.Array(schema.Any()), func(args []interface{}) (interface{}, error) {
		return nil, fmt.Errorf("plain error message")
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	err := sendRequest(conn, 1, "plainerr", []interface{}{})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}

	resp, err := readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object, got %v", resp)
	}

	code, _ := errObj["code"].(float64)
	if code != -32603 {
		t.Errorf("expected error code -32603 for plain error, got %v", code)
	}

	msg, _ := errObj["message"].(string)
	if msg != "plain error message" {
		t.Errorf("expected 'plain error message', got %v", msg)
	}
}

func TestRPCInvalidJSON(t *testing.T) {
	srv, port := newTestServer(t)

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	err := conn.WriteMessage(websocket.TextMessage, []byte("{invalid json"))
	if err != nil {
		t.Fatalf("write error: %v", err)
	}

	resp, err := readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object, got %v", resp)
	}

	code, _ := errObj["code"].(float64)
	if code != -32700 {
		t.Errorf("expected error code -32700 (parse error), got %v", code)
	}
}

func TestRPCInvalidRequest(t *testing.T) {
	srv, port := newTestServer(t)

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	req := map[string]interface{}{
		"jsonrpc": "1.0",
		"method":  "test",
		"id":      1,
	}
	data, _ := json.Marshal(req)
	err := conn.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		t.Fatalf("write error: %v", err)
	}

	resp, err := readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object, got %v", resp)
	}

	code, _ := errObj["code"].(float64)
	if code != -32600 {
		t.Errorf("expected error code -32600 (invalid request), got %v", code)
	}
}

func TestRPCMissingMethod(t *testing.T) {
	srv, port := newTestServer(t)

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
	}
	data, _ := json.Marshal(req)
	err := conn.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		t.Fatalf("write error: %v", err)
	}

	resp, err := readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object, got %v", resp)
	}

	code, _ := errObj["code"].(float64)
	if code != -32600 {
		t.Errorf("expected error code -32600, got %v", code)
	}
}

func TestEventEmission(t *testing.T) {
	srv, port := newTestServer(t)
	srv.RegisterEvent("status")

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	srv.Emit("status", "playing", 0.5)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read event error: %v", err)
	}

	var notif map[string]interface{}
	if err := json.Unmarshal(message, &notif); err != nil {
		t.Fatalf("unmarshal event error: %v", err)
	}

	if notif["jsonrpc"] != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %v", notif["jsonrpc"])
	}
	if notif["method"] != "status" {
		t.Errorf("expected method 'status', got %v", notif["method"])
	}
	params, ok := notif["params"].([]interface{})
	if !ok {
		t.Fatalf("expected params array, got %v", notif["params"])
	}
	if params[0] != "playing" {
		t.Errorf("expected params[0] 'playing', got %v", params[0])
	}
}

func TestEventEmissionToMultipleClients(t *testing.T) {
	srv, port := newTestServer(t)
	srv.RegisterEvent("update")

	startServer(t, srv)
	defer srv.Close()

	conn1 := dialWS(t, port)
	defer conn1.Close()
	conn2 := dialWS(t, port)
	defer conn2.Close()

	time.Sleep(50 * time.Millisecond)

	srv.Emit("update", "data")

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

	if notif1["method"] != "update" || notif2["method"] != "update" {
		t.Error("both clients should receive the event")
	}
}

func TestGlobalPreHook(t *testing.T) {
	srv, port := newTestServer(t)

	var hookCalled bool
	var hookMethod string
	srv.RegisterGlobalPreHook(func(args []interface{}, method string, ctx map[string]interface{}) {
		hookCalled = true
		hookMethod = method
	})

	srv.Register("test", schema.Array(schema.Any()), func(args []interface{}) (interface{}, error) {
		return "ok", nil
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	err := sendRequest(conn, 1, "test", []interface{}{})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}

	_, err = readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	if !hookCalled {
		t.Error("expected global pre-hook to be called")
	}
	if hookMethod != "test" {
		t.Errorf("expected hook method 'test', got %s", hookMethod)
	}
}

func TestGlobalPostHook(t *testing.T) {
	srv, port := newTestServer(t)

	var hookCalled bool
	var hookResult interface{}
	srv.RegisterGlobalPostHook(func(args []interface{}, method string, rpcErr error, result interface{}, ctx map[string]interface{}) {
		hookCalled = true
		hookResult = result
	})

	srv.Register("test", schema.Array(schema.Any()), func(args []interface{}) (interface{}, error) {
		return 42, nil
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	err := sendRequest(conn, 1, "test", []interface{}{})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}

	_, err = readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	if !hookCalled {
		t.Error("expected global post-hook to be called")
	}
	if hookResult != 42 {
		t.Errorf("expected hook result 42, got %v", hookResult)
	}
}

func TestCommandSpecificPreHook(t *testing.T) {
	srv, port := newTestServer(t)

	var hookCalled bool
	srv.RegisterPreHook("test", func(args []interface{}, method string, ctx map[string]interface{}) {
		hookCalled = true
	})

	srv.Register("test", schema.Array(schema.Any()), func(args []interface{}) (interface{}, error) {
		return "ok", nil
	})
	srv.Register("other", schema.Array(schema.Any()), func(args []interface{}) (interface{}, error) {
		return "ok", nil
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	err := sendRequest(conn, 1, "other", []interface{}{})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}

	_, err = readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	if hookCalled {
		t.Error("pre-hook for 'test' should not be called for 'other'")
	}

	err = sendRequest(conn, 2, "test", []interface{}{})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}

	_, err = readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	if !hookCalled {
		t.Error("expected pre-hook for 'test' to be called")
	}
}

func TestCommandSpecificPostHook(t *testing.T) {
	srv, port := newTestServer(t)

	var hookCalled bool
	var hookErr error
	srv.RegisterPostHook("test", func(args []interface{}, method string, rpcErr error, result interface{}, ctx map[string]interface{}) {
		hookCalled = true
		hookErr = rpcErr
	})

	srv.Register("test", schema.Array(schema.Any()), func(args []interface{}) (interface{}, error) {
		return "ok", nil
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	err := sendRequest(conn, 1, "test", []interface{}{})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}

	_, err = readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	if !hookCalled {
		t.Error("expected post-hook for 'test' to be called")
	}
	if hookErr != nil {
		t.Errorf("expected nil error in post-hook, got %v", hookErr)
	}
}

func TestPostHookWithError(t *testing.T) {
	srv, port := newTestServer(t)

	var hookCalled bool
	var hookErr error
	srv.RegisterGlobalPostHook(func(args []interface{}, method string, rpcErr error, result interface{}, ctx map[string]interface{}) {
		hookCalled = true
		hookErr = rpcErr
	})

	srv.Register("fail", schema.Array(schema.Any()), func(args []interface{}) (interface{}, error) {
		return nil, fmt.Errorf("something went wrong")
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	err := sendRequest(conn, 1, "fail", []interface{}{})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}

	_, err = readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	if !hookCalled {
		t.Error("expected global post-hook to be called on error")
	}
	if hookErr == nil {
		t.Error("expected error in post-hook")
	}
}

func TestEventHook(t *testing.T) {
	srv, port := newTestServer(t)
	srv.RegisterEvent("status")

	var hookCalled bool
	var hookEvent string
	srv.RegisterEventHook("status", func(args []interface{}, event string, ctx map[string]interface{}) {
		hookCalled = true
		hookEvent = event
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	srv.Emit("status", "playing")

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read event error: %v", err)
	}

	if !hookCalled {
		t.Error("expected event hook to be called")
	}
	if hookEvent != "status" {
		t.Errorf("expected event 'status', got %s", hookEvent)
	}
}

func TestGlobalEventHook(t *testing.T) {
	srv, port := newTestServer(t)
	srv.RegisterEvent("status")

	var hookCalled bool
	srv.RegisterGlobalEventHook(func(args []interface{}, event string, ctx map[string]interface{}) {
		hookCalled = true
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	srv.Emit("status", "ok")

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read event error: %v", err)
	}

	if !hookCalled {
		t.Error("expected global event hook to be called")
	}
}

func TestHookContext(t *testing.T) {
	srv, port := newTestServer(t)

	var ctxFromHook map[string]interface{}
	srv.RegisterGlobalPreHook(func(args []interface{}, method string, ctx map[string]interface{}) {
		ctx["pre"] = true
	})
	srv.RegisterGlobalPostHook(func(args []interface{}, method string, rpcErr error, result interface{}, ctx map[string]interface{}) {
		ctxFromHook = ctx
	})

	srv.Register("test", schema.Array(schema.Any()), func(args []interface{}) (interface{}, error) {
		return "ok", nil
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	err := sendRequest(conn, 1, "test", []interface{}{})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}

	_, err = readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	if ctxFromHook == nil {
		t.Fatal("expected context to be passed to post-hook")
	}
	if ctxFromHook["pre"] != true {
		t.Error("expected pre-hook context to be visible in post-hook")
	}
}

func TestMultipleMethods(t *testing.T) {
	srv, port := newTestServer(t)
	srv.Register("add", schema.Tuple(schema.Number(), schema.Number()), func(args []interface{}) (interface{}, error) {
		a, _ := args[0].(float64)
		b, _ := args[1].(float64)
		return a + b, nil
	})
	srv.Register("multiply", schema.Tuple(schema.Number(), schema.Number()), func(args []interface{}) (interface{}, error) {
		a, _ := args[0].(float64)
		b, _ := args[1].(float64)
		return a * b, nil
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	err := sendRequest(conn, 1, "add", []interface{}{3.0, 4.0})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}
	resp, err := readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}
	if resp["result"].(float64) != 7.0 {
		t.Errorf("expected 7.0, got %v", resp["result"])
	}

	err = sendRequest(conn, 2, "multiply", []interface{}{3.0, 4.0})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}
	resp, err = readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}
	if resp["result"].(float64) != 12.0 {
		t.Errorf("expected 12.0, got %v", resp["result"])
	}
}

func TestStringID(t *testing.T) {
	srv, port := newTestServer(t)
	srv.Register("echo", schema.Array(schema.Any()), func(args []interface{}) (interface{}, error) {
		return args, nil
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "echo",
		"params":  []interface{}{"hello"},
		"id":      "abc123",
	}
	data, _ := json.Marshal(req)
	err := conn.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}

	resp, err := readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	if resp["id"] != "abc123" {
		t.Errorf("expected id 'abc123', got %v", resp["id"])
	}
}

func TestNullID(t *testing.T) {
	srv, port := newTestServer(t)
	srv.Register("test", schema.Array(schema.Any()), func(args []interface{}) (interface{}, error) {
		return "ok", nil
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "test",
		"params":  []interface{}{},
	}
	data, _ := json.Marshal(req)
	err := conn.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}

	resp, err := readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	if resp["id"] != nil {
		t.Errorf("expected null id for notification-style request, got %v", resp["id"])
	}
}

func TestServerClose(t *testing.T) {
	srv, port := newTestServer(t)
	srv.Register("test", schema.Array(schema.Any()), func(args []interface{}) (interface{}, error) {
		return "ok", nil
	})

	startServer(t, srv)

	conn := dialWS(t, port)

	err := sendRequest(conn, 1, "test", []interface{}{})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}
	_, err = readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}
	conn.Close()

	srv.Close()
}

func TestRPCErrorType(t *testing.T) {
	rpcErr := NewRPCError(4200, "custom")
	if rpcErr.Error() != "custom" {
		t.Errorf("expected 'custom', got %s", rpcErr.Error())
	}
	if rpcErr.ErrorCode() != 4200 {
		t.Errorf("expected code 4200, got %d", rpcErr.ErrorCode())
	}
	if rpcErr.ErrorMessage() != "custom" {
		t.Errorf("expected 'custom', got %s", rpcErr.ErrorMessage())
	}
}

func TestIsHighFrequency(t *testing.T) {
	patterns := map[string]*hfPattern{
		"status": {pattern: regexp.MustCompile(`^status$`)},
	}
	if !IsHighFrequency("status", patterns) {
		t.Error("expected 'status' to match high frequency pattern")
	}
	if IsHighFrequency("play", patterns) {
		t.Error("expected 'play' to not match high frequency pattern")
	}
}

func TestRegisterHighFrequencyPattern(t *testing.T) {
	srv, port := newTestServer(t)
	srv.RegisterHighFrequencyPattern(regexp.MustCompile(`^status$`), nil, 300)
	if len(srv.hfPatterns) != 1 {
		t.Errorf("expected 1 high frequency pattern, got %d", len(srv.hfPatterns))
	}
	_ = port
}

func TestMultipleHooksOrder(t *testing.T) {
	srv, port := newTestServer(t)

	var order []string
	srv.RegisterGlobalPreHook(func(args []interface{}, method string, ctx map[string]interface{}) {
		order = append(order, "global-pre")
	})
	srv.RegisterPreHook("test", func(args []interface{}, method string, ctx map[string]interface{}) {
		order = append(order, "cmd-pre")
	})
	srv.RegisterGlobalPostHook(func(args []interface{}, method string, rpcErr error, result interface{}, ctx map[string]interface{}) {
		order = append(order, "global-post")
	})
	srv.RegisterPostHook("test", func(args []interface{}, method string, rpcErr error, result interface{}, ctx map[string]interface{}) {
		order = append(order, "cmd-post")
	})

	srv.Register("test", schema.Array(schema.Any()), func(args []interface{}) (interface{}, error) {
		order = append(order, "handler")
		return "ok", nil
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	err := sendRequest(conn, 1, "test", []interface{}{})
	if err != nil {
		t.Fatalf("send request error: %v", err)
	}

	_, err = readResponse(conn)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	expected := []string{"global-pre", "cmd-pre", "handler", "global-post", "cmd-post"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d hook calls, got %d: %v", len(expected), len(order), order)
	}
	for i, exp := range expected {
		if order[i] != exp {
			t.Errorf("hook call %d: expected %s, got %s", i, exp, order[i])
		}
	}
}

func TestEventHooksOrder(t *testing.T) {
	srv, port := newTestServer(t)
	srv.RegisterEvent("status")

	var order []string
	srv.RegisterGlobalEventHook(func(args []interface{}, event string, ctx map[string]interface{}) {
		order = append(order, "global-event")
	})
	srv.RegisterEventHook("status", func(args []interface{}, event string, ctx map[string]interface{}) {
		order = append(order, "event-specific")
	})

	startServer(t, srv)
	defer srv.Close()

	conn := dialWS(t, port)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	srv.Emit("status", "ok")

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read event error: %v", err)
	}

	expected := []string{"global-event", "event-specific"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d event hook calls, got %d: %v", len(expected), len(order), order)
	}
	for i, exp := range expected {
		if order[i] != exp {
			t.Errorf("event hook call %d: expected %s, got %s", i, exp, order[i])
		}
	}
}

func TestConcurrentConnections(t *testing.T) {
	srv, port := newTestServer(t)
	srv.Register("echo", schema.Array(schema.Any()), func(args []interface{}) (interface{}, error) {
		return args, nil
	})

	startServer(t, srv)
	defer srv.Close()

	var wg sync.WaitGroup
	numClients := 5

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			conn := dialWS(t, port)
			defer conn.Close()

			err := sendRequest(conn, id, "echo", []interface{}{strconv.Itoa(id)})
			if err != nil {
				t.Errorf("client %d: send error: %v", id, err)
				return
			}

			resp, err := readResponse(conn)
			if err != nil {
				t.Errorf("client %d: read error: %v", id, err)
				return
			}

			result, ok := resp["result"].([]interface{})
			if !ok || len(result) == 0 {
				t.Errorf("client %d: unexpected result: %v", id, resp["result"])
				return
			}

			if result[0] != strconv.Itoa(id) {
				t.Errorf("client %d: expected echo %d, got %v", id, id, result[0])
			}
		}(i)
	}
	wg.Wait()
}