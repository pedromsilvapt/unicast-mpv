package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type testServer struct {
	conn   net.Conn
	reader *bufio.Reader
}

func setupSocketPair(t *testing.T) (*IPCClient, *testServer, func()) {
	t.Helper()

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "mpv.sock")

	l, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	client := NewIPCClient(socketPath)
	if err := client.Connect(); err != nil {
		l.Close()
		t.Fatalf("connect: %v", err)
	}

	conn, err := l.Accept()
	if err != nil {
		l.Close()
		client.Close()
		t.Fatalf("accept: %v", err)
	}

	cleanup := func() {
		conn.Close()
		l.Close()
	}

	ts := &testServer{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}

	return client, ts, cleanup
}

func (ts *testServer) readRequest(t *testing.T) map[string]interface{} {
	t.Helper()
	ts.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := ts.reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("read request: %v", err)
	}
	ts.conn.SetReadDeadline(time.Time{})
	var req map[string]interface{}
	if err := json.Unmarshal(line, &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	return req
}

func writeJSONLn(t *testing.T, conn net.Conn, v interface{}) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	data = append(data, '\n')
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write(data); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func TestNewIPCClient(t *testing.T) {
	client := NewIPCClient("/tmp/test.sock")
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.socketPath != "/tmp/test.sock" {
		t.Errorf("expected socketPath=/tmp/test.sock, got %s", client.socketPath)
	}
	if client.nextID.Load() != 1 {
		t.Errorf("expected nextID=1, got %d", client.nextID.Load())
	}
}

func TestConnectFail(t *testing.T) {
	client := NewIPCClient("/tmp/nonexistent_socket_12345.sock")
	err := client.Connect()
	if err == nil {
		t.Fatal("expected error connecting to nonexistent socket")
	}
}

func TestCommandSuccess(t *testing.T) {
	client, ts, cleanup := setupSocketPair(t)
	defer cleanup()
	defer client.Close()

	done := make(chan interface{})
	go func() {
		result, err := client.GetProperty("volume")
		if err != nil {
			done <- err
		} else {
			done <- result
		}
	}()

	req := ts.readRequest(t)

	if cmd, ok := req["command"].([]interface{}); !ok || len(cmd) < 1 || cmd[0] != "get_property" {
		t.Fatalf("expected command get_property, got %v", req["command"])
	}

	reqID := int(req["request_id"].(float64))

	writeJSONLn(t, ts.conn, map[string]interface{}{
		"request_id": reqID,
		"error":      "success",
		"data":        75.5,
	})

	result := <-done
	switch v := result.(type) {
	case error:
		t.Fatalf("command returned error: %v", v)
	default:
		if v != 75.5 {
			t.Errorf("expected 75.5, got %v", v)
		}
	}
}

func TestCommandError(t *testing.T) {
	client, ts, cleanup := setupSocketPair(t)
	defer cleanup()
	defer client.Close()

	done := make(chan error, 1)
	go func() {
		_, err := client.GetProperty("nonexistent")
		done <- err
	}()

	req := ts.readRequest(t)
	reqID := int(req["request_id"].(float64))

	writeJSONLn(t, ts.conn, map[string]interface{}{
		"request_id": reqID,
		"error":      "property not found",
		"data":        nil,
	})

	err := <-done
	if err == nil {
		t.Fatal("expected error for failed command")
	}
}

func TestSetProperty(t *testing.T) {
	client, ts, cleanup := setupSocketPair(t)
	defer cleanup()
	defer client.Close()

	done := make(chan error, 1)
	go func() {
		done <- client.SetProperty("volume", 80)
	}()

	req := ts.readRequest(t)
	reqID := int(req["request_id"].(float64))

	cmd := req["command"].([]interface{})
	if cmd[0] != "set_property" {
		t.Fatalf("expected set_property, got %v", cmd[0])
	}

	writeJSONLn(t, ts.conn, map[string]interface{}{
		"request_id": reqID,
		"error":      "success",
		"data":        nil,
	})

	if err := <-done; err != nil {
		t.Fatalf("SetProperty returned error: %v", err)
	}
}

func TestAddProperty(t *testing.T) {
	client, ts, cleanup := setupSocketPair(t)
	defer cleanup()
	defer client.Close()

	done := make(chan error, 1)
	go func() {
		done <- client.AddProperty("volume", 10.0)
	}()

	req := ts.readRequest(t)
	reqID := int(req["request_id"].(float64))

	cmd := req["command"].([]interface{})
	if cmd[0] != "add" {
		t.Fatalf("expected add, got %v", cmd[0])
	}

	writeJSONLn(t, ts.conn, map[string]interface{}{
		"request_id": reqID,
		"error":      "success",
		"data":        nil,
	})

	if err := <-done; err != nil {
		t.Fatalf("AddProperty returned error: %v", err)
	}
}

func TestMultiplyProperty(t *testing.T) {
	client, ts, cleanup := setupSocketPair(t)
	defer cleanup()
	defer client.Close()

	done := make(chan error, 1)
	go func() {
		done <- client.MultiplyProperty("volume", 2.0)
	}()

	req := ts.readRequest(t)
	reqID := int(req["request_id"].(float64))

	cmd := req["command"].([]interface{})
	if cmd[0] != "multiply" {
		t.Fatalf("expected multiply, got %v", cmd[0])
	}

	writeJSONLn(t, ts.conn, map[string]interface{}{
		"request_id": reqID,
		"error":      "success",
		"data":        nil,
	})

	if err := <-done; err != nil {
		t.Fatalf("MultiplyProperty returned error: %v", err)
	}
}

func TestCycleProperty(t *testing.T) {
	client, ts, cleanup := setupSocketPair(t)
	defer cleanup()
	defer client.Close()

	done := make(chan error, 1)
	go func() {
		done <- client.CycleProperty("fullscreen")
	}()

	req := ts.readRequest(t)
	reqID := int(req["request_id"].(float64))

	cmd := req["command"].([]interface{})
	if cmd[0] != "cycle" {
		t.Fatalf("expected cycle, got %v", cmd[0])
	}

	writeJSONLn(t, ts.conn, map[string]interface{}{
		"request_id": reqID,
		"error":      "success",
		"data":        nil,
	})

	if err := <-done; err != nil {
		t.Fatalf("CycleProperty returned error: %v", err)
	}
}

func TestFreeCommand(t *testing.T) {
	client, ts, cleanup := setupSocketPair(t)
	defer cleanup()
	defer client.Close()

	done := make(chan error, 1)
	go func() {
		done <- client.FreeCommand("show-text hello")
	}()

	ts.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	freeReader := bufio.NewReader(ts.reader)
	line, err := freeReader.ReadString('\n')
	if err != nil {
		t.Fatalf("read free command: %v", err)
	}

	expected := "show-text hello\n"
	if line != expected {
		t.Errorf("expected %q, got %q", expected, line)
	}

	if err := <-done; err != nil {
		t.Fatalf("FreeCommand returned error: %v", err)
	}
}

func TestEventForwarding(t *testing.T) {
	client, ts, cleanup := setupSocketPair(t)
	defer cleanup()

	writeJSONLn(t, ts.conn, map[string]interface{}{
		"event":     "property-change",
		"name":      "volume",
		"data":      50,
		"request_id": 0,
	})

	evt := <-client.Events()
	if evt.Name != "property-change" {
		t.Errorf("expected event name 'property-change', got %q", evt.Name)
	}

	client.Close()
}

func TestEventWithoutRequestID(t *testing.T) {
	client, ts, cleanup := setupSocketPair(t)
	defer cleanup()

	writeJSONLn(t, ts.conn, map[string]interface{}{
		"event": "idle",
	})

	evt := <-client.Events()
	if evt.Name != "idle" {
		t.Errorf("expected event name 'idle', got %q", evt.Name)
	}

	client.Close()
}

func TestConcurrentSends(t *testing.T) {
	client, ts, cleanup := setupSocketPair(t)
	defer cleanup()
	defer client.Close()

	const numConcurrent = 5

	var wg sync.WaitGroup
	results := make([]chan interface{}, numConcurrent)
	for i := range results {
		results[i] = make(chan interface{}, 1)
	}

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			result, err := client.Command("get_property", fmt.Sprintf("prop%d", idx))
			if err != nil {
				results[idx] <- err
			} else {
				results[idx] <- result
			}
		}(i)
	}

	for i := 0; i < numConcurrent; i++ {
		req := ts.readRequest(t)
		reqID := int(req["request_id"].(float64))

		writeJSONLn(t, ts.conn, map[string]interface{}{
			"request_id": reqID,
			"error":      "success",
			"data":        reqID,
		})
	}

	wg.Wait()

	for i := range results {
		select {
		case r := <-results[i]:
			switch v := r.(type) {
			case error:
				t.Errorf("concurrent send %d returned error: %v", i, v)
			}
		case <-time.After(3 * time.Second):
			t.Errorf("timeout waiting for result %d", i)
		}
	}
}

func TestIncrementingRequestID(t *testing.T) {
	client, ts, cleanup := setupSocketPair(t)
	defer cleanup()
	defer client.Close()

	var id1, id2 int

	done := make(chan struct{})
	go func() {
		defer close(done)
		// First call blocks until response, so we verify IDs are incrementing
		// by sending two separate requests sequentially
		result1, err := client.GetProperty("prop1")
		if err != nil {
			t.Errorf("GetProperty prop1 error: %v", err)
			return
		}
		_ = result1
		result2, err := client.GetProperty("prop2")
		if err != nil {
			t.Errorf("GetProperty prop2 error: %v", err)
			return
		}
		_ = result2
	}()

	// Read first request and respond
	req1 := ts.readRequest(t)
	id1 = int(req1["request_id"].(float64))
	writeJSONLn(t, ts.conn, map[string]interface{}{
		"request_id": id1, "error": "success", "data": "val1",
	})

	// Read second request and respond
	req2 := ts.readRequest(t)
	id2 = int(req2["request_id"].(float64))
	writeJSONLn(t, ts.conn, map[string]interface{}{
		"request_id": id2, "error": "success", "data": "val2",
	})

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for goroutine to finish")
	}

	if id1 >= id2 {
		t.Errorf("expected incrementing request IDs, got %d then %d", id1, id2)
	}
}

func TestClose(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "mpv.sock")

	l, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	client := NewIPCClient(socketPath)
	if err := client.Connect(); err != nil {
		l.Close()
		t.Fatalf("connect: %v", err)
	}

	serverConn, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	serverConn.Close()
	l.Close()

	if err := client.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	_, err = client.GetProperty("volume")
	if err == nil {
		t.Fatal("expected error after close")
	}
}

func TestCloseThenConnectFails(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "mpv.sock")

	client := NewIPCClient(socketPath)
	if err := client.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	err := client.Connect()
	if err == nil {
		t.Fatal("expected error connecting after close")
	}
}

func TestCommandWithArgs(t *testing.T) {
	client, ts, cleanup := setupSocketPair(t)
	defer cleanup()
	defer client.Close()

	done := make(chan error, 1)
	go func() {
		_, err := client.Command("seek", 10.0, "relative")
		done <- err
	}()

	req := ts.readRequest(t)
	reqID := int(req["request_id"].(float64))

	cmd := req["command"].([]interface{})
	if cmd[0] != "seek" {
		t.Fatalf("expected seek, got %v", cmd[0])
	}
	if len(cmd) != 3 {
		t.Fatalf("expected 3 elements in command, got %d", len(cmd))
	}

	writeJSONLn(t, ts.conn, map[string]interface{}{
		"request_id": reqID,
		"error":      "success",
		"data":        nil,
	})

	if err := <-done; err != nil {
		t.Fatalf("Command with args returned error: %v", err)
	}
}

func TestSocketDisconnection(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "mpv.sock")

	l, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	client := NewIPCClient(socketPath)
	if err := client.Connect(); err != nil {
		l.Close()
		t.Fatalf("connect: %v", err)
	}

	serverConn, err := l.Accept()
	if err != nil {
		l.Close()
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := client.GetProperty("volume")
		errCh <- err
	}()

	serverConn.Close()
	l.Close()

	err = <-errCh
	if err == nil {
		t.Fatal("expected error when server disconnects")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}