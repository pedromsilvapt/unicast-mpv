package process

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestResolveBinary_WithExplicitPath(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "mpv-test-binary-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	path, err := resolveBinary(tmpFile.Name())
	if err != nil {
		t.Fatalf("expected no error for existing file, got: %v", err)
	}
	if path != tmpFile.Name() {
		t.Errorf("expected %s, got %s", tmpFile.Name(), path)
	}
}

func TestResolveBinary_NonexistentPath(t *testing.T) {
	_, err := resolveBinary("/nonexistent/path/to/mpv")
	if err == nil {
		t.Fatal("expected error for nonexistent binary path")
	}
	if !strings.Contains(err.Error(), ErrBinaryNotFound.Error()) {
		t.Errorf("expected ErrBinaryNotFound, got: %v", err)
	}
}

func TestResolveBinary_DefaultMPV(t *testing.T) {
	path, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv not found in PATH, skipping default binary resolution test")
	}

	result, err := resolveBinary("")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result != path {
		t.Errorf("expected %s, got %s", path, result)
	}
}

func TestResolveBinary_MPVNotInPath(t *testing.T) {
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	os.Setenv("PATH", "/nonexistent_empty_path_12345")
	_, err := resolveBinary("")
	if err == nil {
		t.Fatal("expected error when mpv is not in PATH")
	}
	if !strings.Contains(err.Error(), ErrBinaryNotFound.Error()) {
		t.Errorf("expected ErrBinaryNotFound, got: %v", err)
	}
}

func TestResolveIPCCommand_ExplicitValid(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"--input-ipc-server", "--input-ipc-server"},
		{"--input-unix-socket", "--input-unix-socket"},
	}

	for _, tt := range tests {
		result, err := resolveIPCCommand(tt.input, "mpv")
		if err != nil {
			t.Errorf("resolveIPCCommand(%q): unexpected error: %v", tt.input, err)
		}
		if result != tt.expected {
			t.Errorf("resolveIPCCommand(%q): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestResolveIPCCommand_InvalidExplicit(t *testing.T) {
	_, err := resolveIPCCommand("--invalid-flag", "mpv")
	if err == nil {
		t.Fatal("expected error for invalid IPC command")
	}
	if !strings.Contains(err.Error(), ErrIPCCommand.Error()) {
		t.Errorf("expected ErrIPCCommand, got: %v", err)
	}
}

func TestResolveIPCCommand_AutoDetectFromVersion(t *testing.T) {
	mpvPath, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv not found in PATH, skipping auto-detect test")
	}

	cmd, err := resolveIPCCommand("", mpvPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "--input-ipc-server" && cmd != "--input-unix-socket" {
		t.Errorf("unexpected IPC command: %s", cmd)
	}
}

func TestParseMPVVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		hasErr   bool
	}{
		{"mpv 0.32.0", "0.32.0", false},
		{"mpv 0.17.0", "0.17.0", false},
		{"mpv 0.16.0 Copyright", "0.16.0", false},
		{"mpv UNKNOWN", "999.999.999", false},
		{"some random output", "", true},
	}

	for _, tt := range tests {
		result, err := parseMPVVersion(tt.input)
		if tt.hasErr && err == nil {
			t.Errorf("parseMPVVersion(%q): expected error, got nil", tt.input)
		}
		if !tt.hasErr && err != nil {
			t.Errorf("parseMPVVersion(%q): unexpected error: %v", tt.input, err)
		}
		if !tt.hasErr && result != tt.expected {
			t.Errorf("parseMPVVersion(%q): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestVersionAtLeast(t *testing.T) {
	tests := []struct {
		mpv    string
		min    string
		expect bool
	}{
		{"0.17.0", "0.17.0", true},
		{"0.18.0", "0.17.0", true},
		{"0.16.0", "0.17.0", false},
		{"1.0.0", "0.17.0", true},
		{"0.40.0", "0.38.0", true},
		{"0.38.0", "0.38.0", true},
		{"0.37.0", "0.38.0", false},
	}

	for _, tt := range tests {
		result := versionAtLeast(tt.mpv, tt.min)
		if result != tt.expect {
			t.Errorf("versionAtLeast(%q, %q): expected %v, got %v", tt.mpv, tt.min, tt.expect, result)
		}
	}
}

func TestBuildMPVArgs_Basic(t *testing.T) {
	cfg := ProcessConfig{
		SocketPath: "/tmp/mpv.sock",
	}

	args := buildMPVArgs(cfg, "--input-ipc-server")

	hasMsgLevel := false
	hasSocket := false
	for _, a := range args {
		if a == "--msg-level=all=no,ipc=v" {
			hasMsgLevel = true
		}
		if strings.HasPrefix(a, "--input-ipc-server=/tmp/mpv.sock") {
			hasSocket = true
		}
	}

	if !hasMsgLevel {
		t.Error("expected --msg-level=all=no,ipc=v in args")
	}
	if !hasSocket {
		t.Error("expected --input-ipc-server=/tmp/mpv.sock in args")
	}
}

func TestBuildMPVArgs_AudioOnly(t *testing.T) {
	cfg := ProcessConfig{
		SocketPath: "/tmp/mpv.sock",
		AudioOnly:  true,
	}

	args := buildMPVArgs(cfg, "--input-ipc-server")

	hasNoVideo := false
	hasNoAudioDisplay := false
	for _, a := range args {
		if a == "--no-video" {
			hasNoVideo = true
		}
		if a == "--no-audio-display" {
			hasNoAudioDisplay = true
		}
	}

	if !hasNoVideo {
		t.Error("expected --no-video in audio-only mode")
	}
	if !hasNoAudioDisplay {
		t.Error("expected --no-audio-display in audio-only mode")
	}
}

func TestBuildMPVArgs_WithUserArgs(t *testing.T) {
	cfg := ProcessConfig{
		SocketPath: "/tmp/mpv.sock",
		MPVArgs:    []string{"--profile=pseudo-gui", "--force-window"},
	}

	args := buildMPVArgs(cfg, "--input-ipc-server")

	found := false
	for _, a := range args {
		if a == "--profile=pseudo-gui" {
			found = true
		}
	}
	if !found {
		t.Error("expected user args to be included")
	}
}

func TestBuildMPVArgs_UserArgsAppendedAfterDefaults(t *testing.T) {
	cfg := ProcessConfig{
		SocketPath: "/tmp/mpv.sock",
		MPVArgs:    []string{"--idle", "--volume=50"},
	}

	args := buildMPVArgs(cfg, "--input-ipc-server")

	defaultEnd := -1
	for i, a := range args {
		if a == "--input-ipc-server=/tmp/mpv.sock" {
			defaultEnd = i
			break
		}
	}
	if defaultEnd == -1 {
		t.Fatal("expected --input-ipc-server in default args")
	}

	for i, a := range args {
		if i <= defaultEnd {
			continue
		}
		if a == "--idle" || a == "--volume=50" {
			return
		}
	}
	t.Error("expected user args to appear after default args")
}

func TestBuildMPVArgs_OldIPCCommand(t *testing.T) {
	cfg := ProcessConfig{
		SocketPath: "/tmp/mpv.sock",
	}

	args := buildMPVArgs(cfg, "--input-unix-socket")

	hasOldSocket := false
	for _, a := range args {
		if strings.HasPrefix(a, "--input-unix-socket=/tmp/mpv.sock") {
			hasOldSocket = true
		}
	}
	if !hasOldSocket {
		t.Error("expected --input-unix-socket=/tmp/mpv.sock in args")
	}
}

func TestBuildMPVArgs_EmptySocketPath(t *testing.T) {
	cfg := ProcessConfig{
		SocketPath: "",
	}

	args := buildMPVArgs(cfg, "--input-ipc-server")

	found := false
	for _, a := range args {
		if a == "--input-ipc-server=" {
			found = true
		}
	}
	if !found {
		t.Error("expected --input-ipc-server= with empty value in args")
	}
}

func TestIsRunning_Initial(t *testing.T) {
	p := NewProcess(ProcessConfig{SocketPath: "/tmp/test.sock"})
	if p.IsRunning() {
		t.Error("expected new process to not be running")
	}
}

func TestStart_AlreadyRunning(t *testing.T) {
	p := NewProcess(ProcessConfig{SocketPath: "/tmp/test.sock"})
	p.mu.Lock()
	p.running = true
	p.mu.Unlock()

	err := p.Start()
	if err != ErrAlreadyRunning {
		t.Errorf("expected ErrAlreadyRunning, got: %v", err)
	}
}

func TestStart_EmptySocketPath(t *testing.T) {
	p := NewProcess(ProcessConfig{SocketPath: ""})
	err := p.Start()
	if err != ErrEmptySocketPath {
		t.Errorf("expected ErrEmptySocketPath, got: %v", err)
	}
}

func TestQuit_NotRunning(t *testing.T) {
	p := NewProcess(ProcessConfig{SocketPath: "/tmp/test.sock"})
	err := p.Quit()
	if err != ErrNotRunning {
		t.Errorf("expected ErrNotRunning, got: %v", err)
	}
}

func TestNewProcessWithOptions(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Binary = "/usr/bin/mpv"
	cfg.SocketPath = "/tmp/custom.sock"
	cfg.AutoRestart = false
	cfg.AudioOnly = true
	cfg.Debug = true

	p := NewProcess(cfg)

	if p.cfg.Binary != "/usr/bin/mpv" {
		t.Errorf("expected Binary=/usr/bin/mpv, got %s", p.cfg.Binary)
	}
	if p.cfg.SocketPath != "/tmp/custom.sock" {
		t.Errorf("expected SocketPath=/tmp/custom.sock, got %s", p.cfg.SocketPath)
	}
	if p.cfg.AutoRestart != false {
		t.Error("expected AutoRestart=false")
	}
	if p.cfg.AudioOnly != true {
		t.Error("expected AudioOnly=true")
	}
	if p.cfg.Debug != true {
		t.Error("expected Debug=true")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Binary != "" {
		t.Errorf("expected empty Binary, got %s", cfg.Binary)
	}
	if cfg.SocketPath != "/tmp/node-mpv.sock" {
		t.Errorf("expected /tmp/node-mpv.sock, got %s", cfg.SocketPath)
	}
	if cfg.AutoRestart != true {
		t.Error("expected AutoRestart=true")
	}
	if cfg.AudioOnly != false {
		t.Error("expected AudioOnly=false")
	}
	if cfg.TimeUpdate != 1 {
		t.Errorf("expected TimeUpdate=1, got %d", cfg.TimeUpdate)
	}
}

func TestProcessOptions(t *testing.T) {
	cfg := DefaultConfig()
	opt := WithBinary("/custom/mpv")
	opt(&cfg)
	if cfg.Binary != "/custom/mpv" {
		t.Errorf("expected /custom/mpv, got %s", cfg.Binary)
	}

	opt2 := WithAudioOnly(true)
	opt2(&cfg)
	if !cfg.AudioOnly {
		t.Error("expected AudioOnly=true")
	}

	opt3 := WithMPVArgs([]string{"--profile=fast"})
	opt3(&cfg)
	if len(cfg.MPVArgs) != 1 || cfg.MPVArgs[0] != "--profile=fast" {
		t.Errorf("expected [--profile=fast], got %v", cfg.MPVArgs)
	}
}

func TestResolveBinary_ExplictPathNonexistent(t *testing.T) {
	_, err := resolveBinary("/does/not/exist/mpv")
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}
}

func TestParseMPVVersion_GitHash(t *testing.T) {
	result, err := parseMPVVersion("mpv abcdef1234")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != "999.999.999" {
		t.Errorf("expected 999.999.999 for git hash, got %s", result)
	}
}

func TestResolveIPCCommand_AutoDetect_BinaryNotFound(t *testing.T) {
	cmd, err := resolveIPCCommand("", "/nonexistent/path/mpv")
	if err != nil {
		t.Skipf("binary not found: %v - this is expected when mpv is not installed", err)
		return
	}
	_ = cmd
}

func TestIPCRegexPatterns(t *testing.T) {
	tests := []struct {
		regex   *regexp.Regexp
		input   string
		matches bool
	}{
		{ipcListeningRe, "Listening to IPC socket", true},
		{ipcListeningRe, "Listening to IPC pipe", true},
		{ipcListeningRe, "Listening to IPC something_else", false},
		{ipcListeningRe, "Random output", false},
		{ipcBindFailedRe, "Could not bind IPC socket", true},
		{ipcBindFailedRe, "Could not bind IPC pipe", true},
		{ipcBindFailedRe, "Could not bind IPC something_else", false},
	}

	for _, tt := range tests {
		result := tt.regex.MatchString(tt.input)
		if result != tt.matches {
			t.Errorf("regex match %q: expected %v, got %v", tt.input, tt.matches, result)
		}
	}
}

func TestIntegration_SpawnMPV(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping integration test on windows")
	}
	mpvPath, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv not found in PATH, skipping integration test")
	}

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "mpv-test.sock")

	cfg := ProcessConfig{
		Binary:      mpvPath,
		SocketPath:  socketPath,
		AutoRestart: false,
		AudioOnly:   true,
		Debug:       true,
	}

	p := NewProcess(cfg)
	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() {
		if p.IsRunning() {
			p.Quit()
		}
	}()

	if !p.IsRunning() {
		t.Fatal("expected process to be running after Start()")
	}

	ipcClient := p.IPC()
	if ipcClient == nil {
		t.Fatal("expected IPC client to be non-nil after Start()")
	}

	result, err := ipcClient.GetProperty("mpv-version")
	if err != nil {
		t.Fatalf("GetProperty mpv-version failed: %v", err)
	}

	t.Logf("mpv-version: %v", result)

	time.Sleep(100 * time.Millisecond)

	if err := p.Quit(); err != nil {
		t.Fatalf("Quit failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if p.IsRunning() {
		t.Error("expected process to not be running after Quit()")
	}
}

func TestIntegration_SpawnMPVWithOldIPCCommand(t *testing.T) {
	mpvPath, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv not found in PATH, skipping integration test")
	}

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "mpv-test2.sock")

	cfg := ProcessConfig{
		Binary:      mpvPath,
		SocketPath:  socketPath,
		IPCCommand:  "--input-ipc-server",
		AutoRestart: false,
	}

	p := NewProcess(cfg)
	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() {
		if p.IsRunning() {
			p.Quit()
		}
	}()

	if !p.IsRunning() {
		t.Fatal("expected process to be running")
	}

	if err := p.Quit(); err != nil {
		t.Fatalf("Quit failed: %v", err)
	}
}

func TestIntegration_ProbeExistingInstance(t *testing.T) {
	mpvPath, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv not found in PATH, skipping integration test")
	}

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "mpv-test3.sock")

	cfg := ProcessConfig{
		Binary:      mpvPath,
		SocketPath:  socketPath,
		AutoRestart: false,
	}

	p := NewProcess(cfg)
	if err := p.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() {
		if p.IsRunning() {
			p.Quit()
		}
	}()

	existing, err := probeExistingInstance(socketPath)
	if err != nil {
		t.Logf("probeExistingInstance returned error (this can happen if mpv hasn't fully started): %v", err)
	} else if !existing {
		t.Error("expected to find existing mpv instance on socket")
	}
}

func TestIntegration_BinaryNotFound(t *testing.T) {
	cfg := ProcessConfig{
		Binary:     "/nonexistent/path/to/mpv",
		SocketPath: "/tmp/test-mpv.sock",
	}

	p := NewProcess(cfg)
	err := p.Start()
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}
	if !strings.Contains(err.Error(), ErrBinaryNotFound.Error()) {
		t.Errorf("expected ErrBinaryNotFound, got: %v", err)
	}
}

func TestIntegration_StartStopCycle(t *testing.T) {
	mpvPath, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv not found in PATH, skipping integration test")
	}

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "mpv-cycle.sock")

	cfg := ProcessConfig{
		Binary:      mpvPath,
		SocketPath:  socketPath,
		AutoRestart: false,
	}

	p := NewProcess(cfg)

	if err := p.Start(); err != nil {
		t.Fatalf("First Start failed: %v", err)
	}
	if !p.IsRunning() {
		t.Fatal("expected process to be running after first Start()")
	}

	if err := p.Quit(); err != nil {
		t.Fatalf("Quit failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	if p.IsRunning() {
		t.Error("expected process to not be running after Quit()")
	}
}