package mpv

import (
	"encoding/json"
	"testing"
)

func TestMPVError_ErrorCodes(t *testing.T) {
	tests := []struct {
		code     ErrorCode
		expected int
		message  string
	}{
		{ErrCodeLoadFailed, 0, "Unable to load file or stream"},
		{ErrCodeInvalidArg, 1, "Invalid argument"},
		{ErrCodeBinaryNotFound, 2, "Binary not found"},
		{ErrCodeIPCCommand, 3, "ipcCommand invalid"},
		{ErrCodeIPCBindFailed, 4, "Unable to bind IPC socket"},
		{ErrCodeTimeout, 5, "Timeout"},
		{ErrCodeAlreadyRunning, 6, "MPV is already running"},
		{ErrCodeIPCSendFailed, 7, "Could not send IPC message"},
		{ErrCodeNotRunning, 8, "MPV is not running"},
		{ErrCodeUnsupportedProto, 9, "Unsupported protocol"},
	}

	for _, tt := range tests {
		err := NewError(tt.code, "test()", nil, "", nil)
		if err.ErrorCode() != tt.expected {
			t.Errorf("ErrorCode(): expected %d, got %d", tt.expected, err.ErrorCode())
		}
		if err.ErrorMessage() != tt.message {
			t.Errorf("ErrorMessage(): expected %q, got %q", tt.message, err.ErrorMessage())
		}
		if err.Error() != tt.message {
			t.Errorf("Error(): expected %q, got %q", tt.message, err.Error())
		}
		if err.Code != tt.code {
			t.Errorf("Code: expected %d, got %d", tt.code, err.Code)
		}
	}
}

func TestNewError_WithAllFields(t *testing.T) {
	options := map[string]string{
		"replace":     "Replace the currently playing title",
		"append":      "Append the title to the playlist",
		"append-play": "Append the title and start playback",
	}
	err := NewError(ErrCodeInvalidArg, "seek()", []interface{}{"10", "badmode"}, "Invalid seek mode: badmode", options)

	if err.Code != ErrCodeInvalidArg {
		t.Errorf("expected Code=%d, got %d", ErrCodeInvalidArg, err.Code)
	}
	if err.Method != "seek()" {
		t.Errorf("expected Method=seek(), got %s", err.Method)
	}
	if len(err.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(err.Args))
	}
	if err.Message != "Invalid seek mode: badmode" {
		t.Errorf("expected Message='Invalid seek mode: badmode', got %q", err.Message)
	}
	if len(err.Options) != 3 {
		t.Errorf("expected 3 options, got %d", len(err.Options))
	}
	if err.Verbose != "Invalid argument" {
		t.Errorf("expected Verbose='Invalid argument', got %q", err.Verbose)
	}
}

func TestNewLoadFailedError(t *testing.T) {
	err := NewLoadFailedError("load()", []interface{}{"/path/to/file.mp4"})
	if err.Code != ErrCodeLoadFailed {
		t.Errorf("expected Code=%d, got %d", ErrCodeLoadFailed, err.Code)
	}
	if err.ErrorCode() != 0 {
		t.Errorf("expected ErrorCode=0, got %d", err.ErrorCode())
	}
	if err.Method != "load()" {
		t.Errorf("expected Method=load(), got %s", err.Method)
	}
	if len(err.Args) != 1 || err.Args[0] != "/path/to/file.mp4" {
		t.Errorf("expected args=[/path/to/file.mp4], got %v", err.Args)
	}
}

func TestNewInvalidArgError(t *testing.T) {
	options := map[string]string{
		"replace": "Replace current",
		"append":  "Append to playlist",
	}
	err := NewInvalidArgError("load()", []interface{}{"test.mp4", "badmode"}, "Invalid mode: badmode", options)
	if err.Code != ErrCodeInvalidArg {
		t.Errorf("expected Code=%d, got %d", ErrCodeInvalidArg, err.Code)
	}
	if err.ErrorCode() != 1 {
		t.Errorf("expected ErrorCode=1, got %d", err.ErrorCode())
	}
}

func TestNewNotRunningError(t *testing.T) {
	err := NewNotRunningError("pause()")
	if err.Code != ErrCodeNotRunning {
		t.Errorf("expected Code=%d, got %d", ErrCodeNotRunning, err.Code)
	}
	if err.ErrorCode() != 8 {
		t.Errorf("expected ErrorCode=8, got %d", err.ErrorCode())
	}
	if err.Method != "pause()" {
		t.Errorf("expected Method=pause(), got %s", err.Method)
	}
	if err.Error() != "MPV is not running" {
		t.Errorf("expected 'MPV is not running', got %q", err.Error())
	}
}

func TestNewUnsupportedProtoError(t *testing.T) {
	err := NewUnsupportedProtoError("load()", []interface{}{"ftp://example.com/file.mp4", "replace"}, "ftp")
	if err.Code != ErrCodeUnsupportedProto {
		t.Errorf("expected Code=%d, got %d", ErrCodeUnsupportedProto, err.Code)
	}
	if err.ErrorCode() != 9 {
		t.Errorf("expected ErrorCode=9, got %d", err.ErrorCode())
	}
	if err.Message != "Unsupported protocol: ftp" {
		t.Errorf("expected 'Unsupported protocol: ftp', got %q", err.Message)
	}
	if err.Options == nil || err.Options["reference"] == "" {
		t.Error("expected reference option")
	}
}

func TestNewIPCSendFailedError(t *testing.T) {
	err := NewIPCSendFailedError("getProperty()", []interface{}{"volume"}, nil)
	if err.Code != ErrCodeIPCSendFailed {
		t.Errorf("expected Code=%d, got %d", ErrCodeIPCSendFailed, err.Code)
	}
	if err.ErrorCode() != 7 {
		t.Errorf("expected ErrorCode=7, got %d", err.ErrorCode())
	}
}

func TestNewIPCSendFailedError_WithInner(t *testing.T) {
	inner := &json.SyntaxError{Offset: 10}
	err := NewIPCSendFailedError("command()", []interface{}{}, inner)
	if err.Message != inner.Error() {
		t.Errorf("expected inner error message, got %q", err.Message)
	}
}

func TestNewTimeoutError(t *testing.T) {
	err := NewTimeoutError("load()", []interface{}{"file.mp4"})
	if err.Code != ErrCodeTimeout {
		t.Errorf("expected Code=%d, got %d", ErrCodeTimeout, err.Code)
	}
	if err.ErrorCode() != 5 {
		t.Errorf("expected ErrorCode=5, got %d", err.ErrorCode())
	}
}

func TestNewAlreadyRunningError(t *testing.T) {
	err := NewAlreadyRunningError("start()")
	if err.Code != ErrCodeAlreadyRunning {
		t.Errorf("expected Code=%d, got %d", ErrCodeAlreadyRunning, err.Code)
	}
	if err.ErrorCode() != 6 {
		t.Errorf("expected ErrorCode=6, got %d", err.ErrorCode())
	}
}

func TestNewBinaryNotFoundError(t *testing.T) {
	err := NewBinaryNotFoundError("/usr/local/bin/mpv")
	if err.Code != ErrCodeBinaryNotFound {
		t.Errorf("expected Code=%d, got %d", ErrCodeBinaryNotFound, err.Code)
	}
	if err.ErrorCode() != 2 {
		t.Errorf("expected ErrorCode=2, got %d", err.ErrorCode())
	}
	if err.Method != "start()" {
		t.Errorf("expected Method=start(), got %s", err.Method)
	}
}

func TestNewIPCBindFailedError(t *testing.T) {
	err := NewIPCBindFailedError("/tmp/mpv.sock")
	if err.Code != ErrCodeIPCBindFailed {
		t.Errorf("expected Code=%d, got %d", ErrCodeIPCBindFailed, err.Code)
	}
	if err.ErrorCode() != 4 {
		t.Errorf("expected ErrorCode=4, got %d", err.ErrorCode())
	}
	if err.Method != "start()" {
		t.Errorf("expected Method=start(), got %s", err.Method)
	}
}

func TestMPVError_Interfaces(t *testing.T) {
	err := NewNotRunningError("play()")

	var _ error = err

	if err.Error() != "MPV is not running" {
		t.Errorf("Error() should return verbose message, got %q", err.Error())
	}

	type errorCodeInterface interface {
		ErrorCode() int
		ErrorMessage() string
	}

	var ec errorCodeInterface = err
	if ec.ErrorCode() != 8 {
		t.Errorf("expected ErrorCode=8, got %d", ec.ErrorCode())
	}
	if ec.ErrorMessage() != "MPV is not running" {
		t.Errorf("expected 'MPV is not running', got %q", ec.ErrorMessage())
	}
}

func TestErrorCodeMapping(t *testing.T) {
	mappings := []struct {
		code            ErrorCode
		expectedInt     int
		expectedMessage string
	}{
		{ErrCodeLoadFailed, 0, "Unable to load file or stream"},
		{ErrCodeInvalidArg, 1, "Invalid argument"},
		{ErrCodeBinaryNotFound, 2, "Binary not found"},
		{ErrCodeIPCCommand, 3, "ipcCommand invalid"},
		{ErrCodeIPCBindFailed, 4, "Unable to bind IPC socket"},
		{ErrCodeTimeout, 5, "Timeout"},
		{ErrCodeAlreadyRunning, 6, "MPV is already running"},
		{ErrCodeIPCSendFailed, 7, "Could not send IPC message"},
		{ErrCodeNotRunning, 8, "MPV is not running"},
		{ErrCodeUnsupportedProto, 9, "Unsupported protocol"},
	}

	for _, m := range mappings {
		err := NewError(m.code, "test()", nil, "", nil)
		if err.ErrorCode() != m.expectedInt {
			t.Errorf("error code %d: expected int %d, got %d", m.code, m.expectedInt, err.ErrorCode())
		}
		if err.ErrorMessage() != m.expectedMessage {
			t.Errorf("error code %d: expected message %q, got %q", m.code, m.expectedMessage, err.ErrorMessage())
		}
	}
}

func TestErrorCodeConstantValues(t *testing.T) {
	if ErrCodeLoadFailed != 0 {
		t.Errorf("expected ErrCodeLoadFailed=0, got %d", ErrCodeLoadFailed)
	}
	if ErrCodeInvalidArg != 1 {
		t.Errorf("expected ErrCodeInvalidArg=1, got %d", ErrCodeInvalidArg)
	}
	if ErrCodeBinaryNotFound != 2 {
		t.Errorf("expected ErrCodeBinaryNotFound=2, got %d", ErrCodeBinaryNotFound)
	}
	if ErrCodeIPCCommand != 3 {
		t.Errorf("expected ErrCodeIPCCommand=3, got %d", ErrCodeIPCCommand)
	}
	if ErrCodeIPCBindFailed != 4 {
		t.Errorf("expected ErrCodeIPCBindFailed=4, got %d", ErrCodeIPCBindFailed)
	}
	if ErrCodeTimeout != 5 {
		t.Errorf("expected ErrCodeTimeout=5, got %d", ErrCodeTimeout)
	}
	if ErrCodeAlreadyRunning != 6 {
		t.Errorf("expected ErrCodeAlreadyRunning=6, got %d", ErrCodeAlreadyRunning)
	}
	if ErrCodeIPCSendFailed != 7 {
		t.Errorf("expected ErrCodeIPCSendFailed=7, got %d", ErrCodeIPCSendFailed)
	}
	if ErrCodeNotRunning != 8 {
		t.Errorf("expected ErrCodeNotRunning=8, got %d", ErrCodeNotRunning)
	}
	if ErrCodeUnsupportedProto != 9 {
		t.Errorf("expected ErrCodeUnsupportedProto=9, got %d", ErrCodeUnsupportedProto)
	}
}