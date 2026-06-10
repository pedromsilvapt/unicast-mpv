package mpv

import (
	"fmt"
)

type ErrorCode int

const (
	ErrCodeLoadFailed       ErrorCode = 0
	ErrCodeInvalidArg       ErrorCode = 1
	ErrCodeBinaryNotFound   ErrorCode = 2
	ErrCodeIPCCommand       ErrorCode = 3
	ErrCodeIPCBindFailed    ErrorCode = 4
	ErrCodeTimeout          ErrorCode = 5
	ErrCodeAlreadyRunning   ErrorCode = 6
	ErrCodeIPCSendFailed    ErrorCode = 7
	ErrCodeNotRunning       ErrorCode = 8
	ErrCodeUnsupportedProto ErrorCode = 9
)

var errorCodeMessages = map[ErrorCode]string{
	ErrCodeLoadFailed:       "Unable to load file or stream",
	ErrCodeInvalidArg:       "Invalid argument",
	ErrCodeBinaryNotFound:   "Binary not found",
	ErrCodeIPCCommand:       "ipcCommand invalid",
	ErrCodeIPCBindFailed:    "Unable to bind IPC socket",
	ErrCodeTimeout:          "Timeout",
	ErrCodeAlreadyRunning:   "MPV is already running",
	ErrCodeIPCSendFailed:    "Could not send IPC message",
	ErrCodeNotRunning:       "MPV is not running",
	ErrCodeUnsupportedProto: "Unsupported protocol",
}

type MPVError struct {
	Code    ErrorCode
	Verbose string
	Method  string
	Args    []interface{}
	Message string
	Options map[string]string
}

func (e *MPVError) Error() string {
	return e.Verbose
}

func (e *MPVError) ErrorCode() int {
	return int(e.Code)
}

func (e *MPVError) ErrorMessage() string {
	return e.Verbose
}

func NewError(code ErrorCode, method string, args []interface{}, errMsg string, options map[string]string) *MPVError {
	verbose, ok := errorCodeMessages[code]
	if !ok {
		verbose = fmt.Sprintf("Unknown error code %d", code)
	}
	return &MPVError{
		Code:    code,
		Verbose: verbose,
		Method:  method,
		Args:    args,
		Message: errMsg,
		Options: options,
	}
}

func NewLoadFailedError(method string, args []interface{}) *MPVError {
	return NewError(ErrCodeLoadFailed, method, args, "", nil)
}

func NewInvalidArgError(method string, args []interface{}, errMsg string, options map[string]string) *MPVError {
	return NewError(ErrCodeInvalidArg, method, args, errMsg, options)
}

func NewNotRunningError(method string) *MPVError {
	return NewError(ErrCodeNotRunning, method, nil, "", nil)
}

func NewUnsupportedProtoError(method string, args []interface{}, proto string) *MPVError {
	return NewError(ErrCodeUnsupportedProto, method, args,
		fmt.Sprintf("Unsupported protocol: %s", proto), map[string]string{
			"reference": "See https://mpv.io/manual/stable/#protocols for supported protocols",
		})
}

func NewIPCSendFailedError(method string, args []interface{}, inner error) *MPVError {
	msg := ""
	if inner != nil {
		msg = inner.Error()
	}
	return NewError(ErrCodeIPCSendFailed, method, args, msg, nil)
}

func NewTimeoutError(method string, args []interface{}) *MPVError {
	return NewError(ErrCodeTimeout, method, args, "", nil)
}

func NewAlreadyRunningError(method string) *MPVError {
	return NewError(ErrCodeAlreadyRunning, method, nil, "", nil)
}

func NewBinaryNotFoundError(binary string) *MPVError {
	return NewError(ErrCodeBinaryNotFound, "start()", []interface{}{binary},
		fmt.Sprintf("Binary not found: %s", binary), nil)
}

func NewIPCBindFailedError(socketPath string) *MPVError {
	return NewError(ErrCodeIPCBindFailed, "start()", []interface{}{socketPath},
		fmt.Sprintf("Could not bind IPC socket: %s", socketPath), nil)
}