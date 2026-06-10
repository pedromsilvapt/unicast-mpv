package commands

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/unicast/unicast-mpv/pkg/config"
	"github.com/unicast/unicast-mpv/pkg/mpv"
	"github.com/unicast/unicast-mpv/pkg/mpv/process"
	"github.com/unicast/unicast-mpv/pkg/player"
	"github.com/unicast/unicast-mpv/pkg/schema"
	"github.com/unicast/unicast-mpv/pkg/server"
)

func newTestServer(t *testing.T) (*server.Server, *config.Config) {
	t.Helper()
	cfg := config.NewConfig(map[string]interface{}{
		"server": map[string]interface{}{
			"port":    0,
			"address": "127.0.0.1",
		},
	})
	logger := &testCmdLogger{}
	srv := server.NewServer(cfg, logger)
	return srv, cfg
}

func newTestCommandRegistry(t *testing.T) (*CommandRegistry, *server.Server, *config.Config, *player.Player, *mpv.MPV) {
	t.Helper()
	srv, cfg := newTestServer(t)
	mpvInst := mpv.NewMPV(process.ProcessConfig{SocketPath: "/tmp/test-cmd.sock"})
	p := player.NewPlayer(cfg, mpvInst, nil)
	reg := NewCommandRegistry(srv, p, mpvInst)
	return reg, srv, cfg, p, mpvInst
}

type testCmdLogger struct{}

func (l *testCmdLogger) Info(msg string)  {}
func (l *testCmdLogger) Debug(msg string) {}
func (l *testCmdLogger) Error(msg string) {}
func (l *testCmdLogger) Warn(msg string)  {}

func TestNewCommandRegistry(t *testing.T) {
	srv, _ := newTestServer(t)
	cfg := config.NewConfig(map[string]interface{}{})
	mpvInst := mpv.NewMPV(process.ProcessConfig{SocketPath: "/tmp/test-cmd.sock"})
	p := player.NewPlayer(cfg, mpvInst, nil)

	reg := NewCommandRegistry(srv, p, mpvInst)
	if reg == nil {
		t.Fatal("expected non-nil CommandRegistry")
	}
}

func TestCommandRegistry_Register(t *testing.T) {
	srv, _ := newTestServer(t)
	cfg := config.NewConfig(map[string]interface{}{})
	mpvInst := mpv.NewMPV(process.ProcessConfig{SocketPath: "/tmp/test-cmd.sock"})
	p := player.NewPlayer(cfg, mpvInst, nil)

	reg := NewCommandRegistry(srv, p, mpvInst)

	called := false
	reg.Register("testMethod", schema.Tuple(schema.Number()), func(args []interface{}) (interface{}, error) {
		called = true
		return args[0], nil
	})

	entry := srv.MethodEntry("testMethod")
	if entry == nil {
		t.Fatal("expected testMethod to be registered")
	}

	result, err := entry.Handler([]interface{}{float64(42)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected handler to be called")
	}
	if result.(float64) != 42 {
		t.Errorf("expected 42, got %v", result)
	}
}

func TestCommandRegistry_RegisterNative_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	cfg := config.NewConfig(map[string]interface{}{})
	mpvInst := mpv.NewMPV(process.ProcessConfig{SocketPath: "/tmp/test-cmd.sock"})
	p := player.NewPlayer(cfg, mpvInst, nil)
	reg := NewCommandRegistry(srv, p, mpvInst)

	reg.RegisterNative("NonExistentMethod", schema.Tuple())

	entry := srv.MethodEntry("NonExistentMethod")
	if entry == nil {
		t.Fatal("expected method to be registered even if native method not found")
	}

	_, err := entry.Handler([]interface{}{})
	if err == nil {
		t.Error("expected error for non-existent method")
	}
}

func TestNativeCommands_RegistersAllCommands(t *testing.T) {
	reg, srv, _, _, _ := newTestCommandRegistry(t)

	NewNativeCommands(reg)

	expectedCommands := []string{
		"load", "stop", "pause", "resume",
		"seek", "goToPosition",
		"mute", "volume",
		"setProperty", "setMultipleProperties", "getProperty",
		"addProperty", "multiplyProperty", "cycleProperty",
		"subtitleScale", "adjustSubtitleTiming",
		"hideSubtitles", "showSubtitles",
		"showProgress",
	}

	methods := srv.RegisteredMethods()
	for _, cmd := range expectedCommands {
		if _, ok := methods[cmd]; !ok {
			t.Errorf("expected command %q to be registered", cmd)
		}
	}
}

func TestNativeCommands_CamelCaseNamingConvention(t *testing.T) {
	reg, srv, _, _, _ := newTestCommandRegistry(t)

	NewNativeCommands(reg)

	pascalCaseNames := []string{
		"Load", "Stop", "Pause", "Resume",
		"Seek", "GoToPosition",
		"Mute", "Volume",
		"SetProperty", "SetMultipleProperties", "GetProperty",
		"AddProperty", "MultiplyProperty", "CycleProperty",
		"SubtitleScale", "AdjustSubtitleTiming",
		"HideSubtitles", "ShowSubtitles",
	}

	methods := srv.RegisteredMethods()
	for _, cmd := range pascalCaseNames {
		if _, ok := methods[cmd]; ok {
			t.Errorf("PascalCase command %q should NOT be registered (must be camelCase)", cmd)
		}
	}

	camelCaseExpected := map[string]bool{
		"load": true, "stop": true, "pause": true, "resume": true,
		"seek": true, "goToPosition": true,
		"mute": true, "volume": true,
		"setProperty": true, "setMultipleProperties": true, "getProperty": true,
		"addProperty": true, "multiplyProperty": true, "cycleProperty": true,
		"subtitleScale": true, "adjustSubtitleTiming": true,
		"hideSubtitles": true, "showSubtitles": true,
		"showProgress": true,
	}

	for cmd := range camelCaseExpected {
		if _, ok := methods[cmd]; !ok {
			t.Errorf("expected camelCase command %q to be registered", cmd)
		}
	}
}

func TestPlayCommand_SchemaValidation(t *testing.T) {
	reg, srv, cfg, _, _ := newTestCommandRegistry(t)

	NewPlayCommand(reg, cfg, nil)

	entry := srv.MethodEntry("play")
	if entry == nil {
		t.Fatal("expected 'play' to be registered")
	}

	err := entry.Schema.Validate([]interface{}{"test.mp4"})
	if err != nil {
		t.Errorf("expected valid for play with file only: %v", err)
	}

	err = entry.Schema.Validate([]interface{}{"test.mp4", "sub.srt"})
	if err != nil {
		t.Errorf("expected valid for play with file+subs: %v", err)
	}

	err = entry.Schema.Validate([]interface{}{"test.mp4", "sub.srt", map[string]interface{}{}})
	if err != nil {
		t.Errorf("expected valid for play with file+subs+options: %v", err)
	}

	err = entry.Schema.Validate([]interface{}{})
	if err == nil {
		t.Error("expected validation error for play with no args")
	}

	err = entry.Schema.Validate([]interface{}{42})
	if err == nil {
		t.Error("expected validation error for play with non-string file")
	}
}

func TestQuitCommand_SchemaValidation(t *testing.T) {
	reg, srv, _, _, _ := newTestCommandRegistry(t)

	NewQuitCommand(reg)

	entry := srv.MethodEntry("quit")
	if entry == nil {
		t.Fatal("expected 'quit' to be registered")
	}

	err := entry.Schema.Validate([]interface{}{})
	if err != nil {
		t.Errorf("expected valid for quit with no args: %v", err)
	}

	err = entry.Schema.Validate([]interface{}{"extra"})
	if err == nil {
		t.Error("expected validation error for quit with extra args")
	}
}

func TestQuitCommand_WhenNotRunning(t *testing.T) {
	srv, _ := newTestServer(t)
	cfg := config.NewConfig(map[string]interface{}{})
	mpvInst := mpv.NewMPV(process.ProcessConfig{SocketPath: "/tmp/test-cmd.sock"})
	p := player.NewPlayer(cfg, mpvInst, nil)

	reg := NewCommandRegistry(srv, p, mpvInst)
	NewQuitCommand(reg)

	entry := srv.MethodEntry("quit")
	if entry == nil {
		t.Fatal("expected quit to be registered")
	}

	result, err := entry.Handler([]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}

func TestNativeCommands_Schemas(t *testing.T) {
	reg, srv, _, _, _ := newTestCommandRegistry(t)

	NewNativeCommands(reg)

	tests := []struct {
		name    string
		method  string
		args    []interface{}
		wantErr bool
	}{
		{"Stop with no args", "stop", []interface{}{}, false},
		{"Pause with no args", "pause", []interface{}{}, false},
		{"Resume with no args", "resume", []interface{}{}, false},
		{"Seek with number", "seek", []interface{}{float64(10)}, false},
		{"Seek without args", "seek", []interface{}{}, true},
		{"GoToPosition with number", "goToPosition", []interface{}{float64(30)}, false},
		{"Volume with number", "volume", []interface{}{float64(75)}, false},
		{"Mute with boolean", "mute", []interface{}{true}, false},
		{"SetProperty with string and any", "setProperty", []interface{}{"volume", float64(50)}, false},
		{"GetProperty with string", "getProperty", []interface{}{"volume"}, false},
		{"AddProperty with string and number", "addProperty", []interface{}{"volume", float64(10)}, false},
		{"MultiplyProperty with string and number", "multiplyProperty", []interface{}{"volume", float64(2)}, false},
		{"CycleProperty with string", "cycleProperty", []interface{}{"fullscreen"}, false},
		{"SubtitleScale with number", "subtitleScale", []interface{}{float64(2)}, false},
		{"AdjustSubtitleTiming with number", "adjustSubtitleTiming", []interface{}{float64(1.5)}, false},
		{"HideSubtitles with no args", "hideSubtitles", []interface{}{}, false},
		{"ShowSubtitles with no args", "showSubtitles", []interface{}{}, false},
		{"showProgress with no args", "showProgress", []interface{}{}, false},
		{"showProgress with extra args", "showProgress", []interface{}{"extra"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := srv.MethodEntry(tt.method)
			if entry == nil {
				t.Fatalf("method %s not registered", tt.method)
			}
			err := entry.Schema.Validate(tt.args)
			if tt.wantErr && err == nil {
				t.Errorf("expected validation error for %s", tt.name)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected validation error for %s: %v", tt.name, err)
			}
		})
	}
}

func TestLoadSchema(t *testing.T) {
	reg, srv, _, _, _ := newTestCommandRegistry(t)
	NewNativeCommands(reg)

	entry := srv.MethodEntry("load")

	err := entry.Schema.Validate([]interface{}{"test.mp4"})
	if err != nil {
		t.Errorf("expected valid for Load with one arg: %v", err)
	}

	err = entry.Schema.Validate([]interface{}{"test.mp4", "replace"})
	if err != nil {
		t.Errorf("expected valid for Load with two args: %v", err)
	}

	err = entry.Schema.Validate([]interface{}{})
	if err == nil {
		t.Error("expected validation error for Load with no args")
	}

	err = entry.Schema.Validate([]interface{}{42})
	if err == nil {
		t.Error("expected validation error for Load with non-string first arg")
	}
}

func TestSetMultiplePropertiesSchema(t *testing.T) {
	reg, srv, _, _, _ := newTestCommandRegistry(t)
	NewNativeCommands(reg)

	entry := srv.MethodEntry("setMultipleProperties")

	err := entry.Schema.Validate([]interface{}{map[string]interface{}{"volume": 50}})
	if err != nil {
		t.Errorf("expected valid for SetMultipleProperties: %v", err)
	}

	err = entry.Schema.Validate([]interface{}{})
	if err == nil {
		t.Error("expected validation error for SetMultipleProperties with no args")
	}
}

func TestRegisterAllCommands(t *testing.T) {
	reg, srv, cfg, _, mpvInst := newTestCommandRegistry(t)
	status := player.NewPlayerStatus(mpvInst, nil)

	NewNativeCommands(reg)
	NewPlayCommand(reg, cfg, nil)
	NewStatusCommand(reg, status, nil)
	NewQuitCommand(reg)

	expectedCommands := []string{
		"load", "stop", "pause", "resume",
		"seek", "goToPosition",
		"mute", "volume",
		"setProperty", "setMultipleProperties", "getProperty",
		"addProperty", "multiplyProperty", "cycleProperty",
		"subtitleScale", "adjustSubtitleTiming",
		"hideSubtitles", "showSubtitles",
		"showProgress",
		"play", "status", "quit",
	}

	methods := srv.RegisteredMethods()
	for _, cmd := range expectedCommands {
		if _, ok := methods[cmd]; !ok {
			t.Errorf("expected command %q to be registered", cmd)
		}
	}
}

func TestStatusCommand_RegistersHooksAndHandler(t *testing.T) {
	reg, srv, _, _, mpvInst := newTestCommandRegistry(t)
	status := player.NewPlayerStatus(mpvInst, nil)

	NewStatusCommand(reg, status, nil)

	if entry := srv.MethodEntry("status"); entry == nil {
		t.Error("expected 'status' command to be registered")
	}
}

func TestStatusCommand_StatusHandlerReturnsInfo(t *testing.T) {
	srv, _ := newTestServer(t)
	cfg := config.NewConfig(map[string]interface{}{})
	mpvInst := mpv.NewMPV(process.ProcessConfig{SocketPath: "/tmp/test-cmd.sock"})
	p := player.NewPlayer(cfg, mpvInst, nil)

	reg := NewCommandRegistry(srv, p, mpvInst)
	status := player.NewPlayerStatus(mpvInst, nil)

	NewStatusCommand(reg, status, nil)

	entry := srv.MethodEntry("status")
	if entry == nil {
		t.Fatal("expected status to be registered")
	}

	status.Update("path", "/test.mp4")
	status.Update("filename", "test.mp4")
	status.Update("duration", 120.0)
	status.Update("position", 30.0)
	status.Update("mediaTitle", "Test")
	status.Update("playlistPos", 1.0)
	status.Update("playlistCount", 5.0)

	result, err := entry.Handler([]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, ok := result.(player.StatusInfo)
	if !ok {
		t.Fatalf("expected StatusInfo, got %T", result)
	}
	if info.Path == nil || *info.Path != "/test.mp4" {
		t.Errorf("expected Path=/test.mp4, got %v", info.Path)
	}
}

func TestPlayCommand_WithRestartOnPlayConfig(t *testing.T) {
	srv, _ := newTestServer(t)
	cfg := config.NewConfig(map[string]interface{}{
		"restartOnPlay": true,
	})
	mpvInst := mpv.NewMPV(process.ProcessConfig{SocketPath: "/tmp/test-cmd.sock"})
	p := player.NewPlayer(cfg, mpvInst, nil)

	reg := NewCommandRegistry(srv, p, mpvInst)
	NewPlayCommand(reg, cfg, nil)

	entry := srv.MethodEntry("play")
	if entry == nil {
		t.Fatal("expected play to be registered")
	}

	_, err := entry.Handler([]interface{}{"test.mp4"})
	if err == nil {
		t.Log("play handler called (mpv not running, expected to fail starting)")
	}
}

func TestConvertArg_BasicTypes(t *testing.T) {
	tests := []struct {
		name   string
		val    interface{}
		target reflect.Type
		ok     bool
	}{
		{"string to string", "hello", reflect.TypeOf(""), true},
		{"int to float64", 42, reflect.TypeOf(float64(0)), true},
		{"float64 to float64", 3.14, reflect.TypeOf(float64(0)), true},
		{"bool to bool", true, reflect.TypeOf(false), true},
		{"string to float64 fails", "hello", reflect.TypeOf(float64(0)), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := convertArg(tt.val, tt.target)
			if tt.ok && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
			if !tt.ok && err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestConvertArg_NilValue(t *testing.T) {
	val, err := convertArg(nil, reflect.TypeOf(""))
	if err != nil {
		t.Fatalf("expected no error for nil, got %v", err)
	}
	if val.String() != "" {
		t.Errorf("expected zero value for string, got %v", val)
	}
}

func TestCommandRegistry_MethodToHandler_CallWithArgs(t *testing.T) {
	srv, _ := newTestServer(t)
	cfg := config.NewConfig(map[string]interface{}{})
	mpvInst := mpv.NewMPV(process.ProcessConfig{SocketPath: "/tmp/test-cmd.sock"})
	p := player.NewPlayer(cfg, mpvInst, nil)
	reg := NewCommandRegistry(srv, p, mpvInst)

	reg.Register("echo", schema.Tuple(schema.String()), func(args []interface{}) (interface{}, error) {
		return args[0].(string), nil
	})

	entry := srv.MethodEntry("echo")
	result, err := entry.Handler([]interface{}{"hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello" {
		t.Errorf("expected 'hello', got %v", result)
	}
}

func TestCommandRegistry_MethodToHandler_WithError(t *testing.T) {
	srv, _ := newTestServer(t)
	cfg := config.NewConfig(map[string]interface{}{})
	mpvInst := mpv.NewMPV(process.ProcessConfig{SocketPath: "/tmp/test-cmd.sock"})
	p := player.NewPlayer(cfg, mpvInst, nil)
	reg := NewCommandRegistry(srv, p, mpvInst)

	testErr := fmt.Errorf("test error")
	reg.Register("fail", schema.Tuple(), func(args []interface{}) (interface{}, error) {
		return nil, testErr
	})

	entry := srv.MethodEntry("fail")
	result, err := entry.Handler([]interface{}{})
	if err != testErr {
		t.Errorf("expected test error, got %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}

func TestPlayCommand_PlayHandlerArgs(t *testing.T) {
	srv, _ := newTestServer(t)
	cfg := config.NewConfig(map[string]interface{}{})
	mpvInst := mpv.NewMPV(process.ProcessConfig{SocketPath: "/tmp/test-cmd.sock"})
	p := player.NewPlayer(cfg, mpvInst, nil)

	reg := NewCommandRegistry(srv, p, mpvInst)
	NewPlayCommand(reg, cfg, nil)

	entry := srv.MethodEntry("play")

	_, err := entry.Handler([]interface{}{"test.mp4"})
	if err == nil {
		t.Log("play called with test.mp4 (mpv not running)")
	}
}

func TestStatusCommand_MpvEventUpdates(t *testing.T) {
	srv, _ := newTestServer(t)
	cfg := config.NewConfig(map[string]interface{}{})
	mpvInst := mpv.NewMPV(process.ProcessConfig{SocketPath: "/tmp/test-cmd.sock"})
	p := player.NewPlayer(cfg, mpvInst, nil)

	reg := NewCommandRegistry(srv, p, mpvInst)
	status := player.NewPlayerStatus(mpvInst, nil)

	NewStatusCommand(reg, status, nil)

	mpvInst.EmitTestEvent("status", map[string]interface{}{
		"property": "volume",
		"value":    75.0,
	})

	result := status.GetSync()
	if result.Volume != 75.0 {
		t.Errorf("expected Volume=75.0 after status event, got %f", result.Volume)
	}
}

func TestStatusCommand_TimePositionEvent(t *testing.T) {
	srv, _ := newTestServer(t)
	cfg := config.NewConfig(map[string]interface{}{})
	mpvInst := mpv.NewMPV(process.ProcessConfig{SocketPath: "/tmp/test-cmd.sock"})
	p := player.NewPlayer(cfg, mpvInst, nil)

	reg := NewCommandRegistry(srv, p, mpvInst)
	status := player.NewPlayerStatus(mpvInst, nil)

	NewStatusCommand(reg, status, nil)

	mpvInst.EmitTestEvent("timeposition", 42.5)

	result := status.GetSync()
	if result.Position == nil || *result.Position != 42.5 {
		t.Errorf("expected Position=42.5 after timeposition event, got %v", result.Position)
	}
}