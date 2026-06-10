package process

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/unicast/unicast-mpv/pkg/logger"
	"github.com/unicast/unicast-mpv/pkg/mpv/ipc"
)

var (
	ErrAlreadyRunning     = errors.New("mpv process: already running")
	ErrNotRunning         = errors.New("mpv process: not running")
	ErrBinaryNotFound     = errors.New("mpv process: binary not found")
	ErrEmptySocketPath    = errors.New("mpv process: socket path is empty")
	ErrIPCBindFailed      = errors.New("mpv process: could not bind IPC socket")
	ErrIPCCommand         = errors.New("mpv process: invalid IPC command")
	ErrStartTimeout       = errors.New("mpv process: start timed out")
	ErrInstanceOnSocket   = errors.New("mpv process: another instance already running on socket")
)

var ipcListeningRe = regexp.MustCompile(`Listening to IPC (socket|pipe)`)
var ipcBindFailedRe = regexp.MustCompile(`Could not bind IPC (socket|pipe)`)
var mpvVersionRe = regexp.MustCompile(`mpv (\d+)\.(\d+)\.(\d+)`)

const (
	defaultIPCCommand   = "--input-ipc-server"
	oldIPCCommand       = "--input-unix-socket"
	startTimeout        = 10
	idleActiveTimeout   = 5
)

type ProcessConfig struct {
	Binary      string
	SocketPath  string
	IPCCommand  string
	AutoRestart bool
	AudioOnly   bool
	TimeUpdate  int
	Debug       bool
	Verbose     bool
	MPVArgs     []string
	OnMessage   func([]byte)
	Log         *logger.Logger
	RootLog     *logger.Logger
}

type ExitCallback func(exitCode int)

type Process struct {
	cfg       ProcessConfig
	cmd       *exec.Cmd
	running   bool
	stopped   atomic.Bool
	mu        sync.Mutex
	cancel    context.CancelFunc
	ipc       *ipc.IPCClient
	stdoutW   io.Writer
	stderrW   io.Writer
	onExit   ExitCallback
}

type ProcessOption func(*ProcessConfig)

func WithBinary(bin string) ProcessOption {
	return func(cfg *ProcessConfig) { cfg.Binary = bin }
}

func WithSocketPath(path string) ProcessOption {
	return func(cfg *ProcessConfig) { cfg.SocketPath = path }
}

func WithAutoRestart(auto bool) ProcessOption {
	return func(cfg *ProcessConfig) { cfg.AutoRestart = auto }
}

func WithAudioOnly(audioOnly bool) ProcessOption {
	return func(cfg *ProcessConfig) { cfg.AudioOnly = audioOnly }
}

func WithDebug(debug bool) ProcessOption {
	return func(cfg *ProcessConfig) { cfg.Debug = debug }
}

func WithVerbose(verbose bool) ProcessOption {
	return func(cfg *ProcessConfig) { cfg.Verbose = verbose }
}

func WithMPVArgs(args []string) ProcessOption {
	return func(cfg *ProcessConfig) { cfg.MPVArgs = args }
}

func WithIPCCommand(cmd string) ProcessOption {
	return func(cfg *ProcessConfig) { cfg.IPCCommand = cmd }
}

func WithProcessLogger(l *logger.Logger) ProcessOption {
	return func(cfg *ProcessConfig) { cfg.Log = l }
}

func DefaultConfig() ProcessConfig {
	return ProcessConfig{
		Binary:      "",
		SocketPath:  "/tmp/node-mpv.sock",
		IPCCommand:  "",
		AutoRestart: true,
		AudioOnly:   false,
		TimeUpdate:  1,
		Debug:       false,
		Verbose:     false,
		MPVArgs:     nil,
	}
}

func NewProcess(cfg ProcessConfig) *Process {
	return &Process{
		cfg: cfg,
	}
}

func (p *Process) logDebugf(format string, args ...interface{}) {
	if p.cfg.Log != nil {
		p.cfg.Log.Debugf(format, args...)
	}
}

func (p *Process) logInfof(format string, args ...interface{}) {
	if p.cfg.Log != nil {
		p.cfg.Log.Infof(format, args...)
	}
}

func (p *Process) logWarnf(format string, args ...interface{}) {
	if p.cfg.Log != nil {
		p.cfg.Log.Warnf(format, args...)
	}
}

func (p *Process) logErrorf(format string, args ...interface{}) {
	if p.cfg.Log != nil {
		p.cfg.Log.Errorf(format, args...)
	}
}

func (p *Process) SetOnExit(cb ExitCallback) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onExit = cb
}

func (p *Process) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

func (p *Process) Start() error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return ErrAlreadyRunning
	}
	p.mu.Unlock()

	if p.stopped.Load() {
		return ErrNotRunning
	}

	p.mu.Lock()
	if p.ipc != nil {
		p.ipc.Close()
		p.ipc = nil
	}
	p.mu.Unlock()

	if p.cfg.SocketPath == "" {
		p.logErrorf("socket path is empty")
		return ErrEmptySocketPath
	}

	binary, err := resolveBinary(p.cfg.Binary)
	if err != nil {
		p.logErrorf("resolve binary: %v", err)
		return err
	}
	p.logInfof("binary found: %s", binary)

	ipcCmd, err := resolveIPCCommand(p.cfg.IPCCommand, binary)
	if err != nil {
		p.logErrorf("resolve ipc command: %v", err)
		return err
	}
	p.logDebugf("ipc command: %s", ipcCmd)

	p.logDebugf("probing existing instance on socket %s", p.cfg.SocketPath)
	existing, err := probeExistingInstance(p.cfg.SocketPath)
	if err == nil && existing {
		p.logInfof("found existing mpv instance on socket %s, reusing", p.cfg.SocketPath)
		p.mu.Lock()
		p.running = true
		p.mu.Unlock()
		return nil
	}
	p.logDebugf("no existing instance on socket (probe err: %v)", err)

	args := buildMPVArgs(p.cfg, ipcCmd)
	p.logDebugf("mpv args: %v", args)

	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, binary, args...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		p.logErrorf("stdout pipe: %v", err)
		return fmt.Errorf("mpv process: stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		p.logErrorf("stderr pipe: %v", err)
		return fmt.Errorf("mpv process: stderr pipe: %w", err)
	}

	p.logInfof("spawning mpv process...")
	if err := cmd.Start(); err != nil {
		cancel()
		p.logErrorf("process start: %v", err)
		return fmt.Errorf("mpv process: start: %w", err)
	}

	p.mu.Lock()
	p.cmd = cmd
	p.running = true
	p.cancel = cancel
	p.mu.Unlock()

	p.logDebugf("mpv process started (pid %d), waiting for IPC socket on %s", cmd.Process.Pid, p.cfg.SocketPath)

	listeningCh := make(chan error, 1)
	scanOutput := func(pipe io.Reader, label string) {
		var writers []io.Writer
		if p.cfg.Debug || p.cfg.Verbose {
			writers = append(writers, os.Stderr)
		}
		if label == "stderr" && p.stderrW != nil {
			writers = append(writers, p.stderrW)
		}
		if label == "stdout" && p.stdoutW != nil {
			writers = append(writers, p.stdoutW)
		}
		var multiW io.Writer
		if len(writers) > 0 {
			multiW = io.MultiWriter(writers...)
		}

		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			line := scanner.Text()
			if multiW != nil {
				fmt.Fprintf(multiW, "[mpv-process] %s: %s\n", label, line)
			}
			p.logDebugf("mpv %s: %s", label, line)
			if ipcListeningRe.MatchString(line) {
				p.logInfof("mpv %s: IPC socket ready", label)
				listeningCh <- nil
				return
			}
			if ipcBindFailedRe.MatchString(line) {
				p.logErrorf("mpv %s: IPC bind failed", label)
				listeningCh <- ErrIPCBindFailed
				return
			}
		}
		select {
		case listeningCh <- fmt.Errorf("mpv process: %s closed before IPC socket was ready", label):
		default:
		}
	}
	go scanOutput(stdoutPipe, "stdout")
	go scanOutput(stderrPipe, "stderr")

	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, time.Duration(startTimeout)*time.Second)
	defer timeoutCancel()

	p.logDebugf("waiting up to %ds for IPC socket readiness", startTimeout)
	select {
	case err := <-listeningCh:
		if err != nil {
			p.logErrorf("IPC socket readiness failed: %v", err)
			p.killProcess()
			return err
		}
	case <-timeoutCtx.Done():
		p.logErrorf("IPC socket readiness timed out after %ds", startTimeout)
		p.killProcess()
		return ErrStartTimeout
	}

	p.logInfof("connecting IPC client to %s", p.cfg.SocketPath)
	var ipcOpts []ipc.IPCClientOption
	if p.cfg.RootLog != nil {
		ipcOpts = append(ipcOpts, ipc.WithLogger(p.cfg.RootLog.Service("mpv-ipc")))
	} else if p.cfg.Log != nil {
		ipcOpts = append(ipcOpts, ipc.WithLogger(p.cfg.Log.Service("mpv-ipc")))
	}
	ipcClient := ipc.NewIPCClient(p.cfg.SocketPath, ipcOpts...)
	ipcClient.OnMessage = p.cfg.OnMessage
	if err := ipcClient.Connect(); err != nil {
		p.logErrorf("IPC connect failed: %v", err)
		p.killProcess()
		return fmt.Errorf("mpv process: ipc connect: %w", err)
	}
	p.logInfof("IPC client connected")

	stimulusCh := make(chan struct{}, 1)
	go func() {
		defer close(stimulusCh)
		_, err := ipcClient.GetProperty("idle-active")
		if err == nil {
			p.logDebugf("idle-active property retrieved")
			select {
			case stimulusCh <- struct{}{}:
			default:
			}
		} else {
			p.logDebugf("idle-active property query failed: %v", err)
		}
	}()

	p.logDebugf("waiting up to %ds for idle event", idleActiveTimeout)
	idleCtx, idleCancel := context.WithTimeout(context.Background(), time.Duration(idleActiveTimeout)*time.Second)
	defer idleCancel()

waitIdle:
	for {
		select {
		case evt, ok := <-ipcClient.Events():
			if !ok {
				p.logErrorf("IPC closed before idle event")
				p.mu.Lock()
				p.ipc = nil
				p.mu.Unlock()
				ipcClient.Close()
				p.killProcess()
				return fmt.Errorf("mpv process: ipc closed before idle event")
			}
			p.logDebugf("received event while waiting for idle: %s", evt.Name)
			if evt.Name == "idle" || evt.Name == "idle-active" || evt.Name == "file-loaded" {
				p.logInfof("received idle event: %s", evt.Name)
				break waitIdle
			}
		case _, ok := <-stimulusCh:
			if ok {
				p.logInfof("idle-active stimulus received")
				break waitIdle
			}
		case <-idleCtx.Done():
			p.logErrorf("timed out waiting for idle event after %ds", idleActiveTimeout)
			p.mu.Lock()
			p.ipc = nil
			p.mu.Unlock()
			ipcClient.Close()
			p.killProcess()
			return fmt.Errorf("mpv process: timed out waiting for idle event: %w", ErrStartTimeout)
		}
	}

	p.mu.Lock()
	p.ipc = ipcClient
	p.mu.Unlock()

	p.logInfof("mpv process started successfully")
	go p.watchProcess()

	return nil
}

func (p *Process) Restart() error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return ErrAlreadyRunning
	}
	if p.stopped.Load() {
		p.mu.Unlock()
		return ErrNotRunning
	}

	if p.ipc != nil {
		p.ipc.Close()
		p.ipc = nil
	}
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.cmd = nil
	p.mu.Unlock()

	return p.Start()
}

func (p *Process) Quit() error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return ErrNotRunning
	}
	p.logInfof("quitting mpv process")
	p.stopped.Store(true)
	if p.ipc != nil {
		p.ipc.Command("quit")
		p.ipc.Close()
		p.ipc = nil
	}
	p.running = false
	if p.cancel != nil {
		p.cancel()
	}
	p.mu.Unlock()

	return nil
}

func (p *Process) IPC() *ipc.IPCClient {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ipc
}

func (p *Process) watchProcess() {
	p.mu.Lock()
	cmd := p.cmd
	p.mu.Unlock()

	if cmd == nil {
		return
	}
	err := cmd.Wait()
	if err != nil {
		p.logDebugf("process exited with error: %v", err)
	} else {
		p.logDebugf("process exited normally")
	}

	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	p.logInfof("mpv process exited with code %d", exitCode)

	p.mu.Lock()
	wasRunning := p.running
	p.running = false
	ipcClient := p.ipc
	p.ipc = nil
	p.mu.Unlock()

	if ipcClient != nil {
		ipcClient.Close()
	}

	p.mu.Lock()
	onExit := p.onExit
	p.mu.Unlock()

	if !wasRunning || p.stopped.Load() {
		if onExit != nil {
			onExit(exitCode)
		}
		return
	}

	if exitCode == 4 && p.cfg.AutoRestart {
		p.logWarnf("mpv crashed (exit code 4), auto-restarting")
		if err := p.Start(); err != nil {
			p.logErrorf("auto-restart failed: %v", err)
		}
	}

	if onExit != nil {
		onExit(exitCode)
	}
}

func (p *Process) killProcess() {
	p.mu.Lock()
	p.running = false
	if p.cancel != nil {
		p.cancel()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Signal(syscall.SIGKILL)
	}
	p.mu.Unlock()
}

func resolveBinary(binary string) (string, error) {
	if binary != "" {
		if _, err := os.Stat(binary); err != nil {
			return "", fmt.Errorf("%w: %s", ErrBinaryNotFound, binary)
		}
		return binary, nil
	}
	path, err := exec.LookPath("mpv")
	if err != nil {
		return "", fmt.Errorf("%w: mpv not found in PATH", ErrBinaryNotFound)
	}
	return path, nil
}

func resolveIPCCommand(ipcCommand string, binary string) (string, error) {
	if ipcCommand != "" {
		switch ipcCommand {
		case "--input-ipc-server", "--input-unix-socket":
			return ipcCommand, nil
		default:
			return "", fmt.Errorf("%w: %q (valid: --input-ipc-server, --input-unix-socket)", ErrIPCCommand, ipcCommand)
		}
	}

	cmd := exec.Command(binary, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return defaultIPCCommand, nil
	}

	version, err := parseMPVVersion(string(output))
	if err != nil || version == "999.999.999" {
		return defaultIPCCommand, nil
	}

	if versionAtLeast(version, "0.17.0") {
		return "--input-ipc-server", nil
	}
	return "--input-unix-socket", nil
}

func parseMPVVersion(output string) (string, error) {
	if strings.Contains(output, "UNKNOWN") {
		return "999.999.999", nil
	}
	matches := mpvVersionRe.FindStringSubmatch(output)
	if matches == nil {
		regexGitHash := regexp.MustCompile(`mpv\s+[a-f0-9]+`)
		if regexGitHash.MatchString(output) {
			return "999.999.999", nil
		}
		return "", fmt.Errorf("could not parse mpv version from: %s", output)
	}
	return matches[1] + "." + matches[2] + "." + matches[3], nil
}

func versionAtLeast(mpvVersion string, minVersion string) bool {
	mpvParts := strings.Split(mpvVersion, ".")
	minParts := strings.Split(minVersion, ".")

	for i := 0; i < 3; i++ {
		mpvN, _ := strconv.Atoi(mpvParts[i])
		minN, _ := strconv.Atoi(minParts[i])
		if mpvN > minN {
			return true
		}
		if mpvN < minN {
			return false
		}
	}
	return true
}

func buildMPVArgs(cfg ProcessConfig, ipcCommand string) []string {
	args := []string{"--msg-level=all=no,ipc=v"}

	if cfg.AudioOnly {
		args = append(args, "--no-video", "--no-audio-display")
	}

	args = append(args, ipcCommand+"="+cfg.SocketPath)

	if cfg.MPVArgs != nil {
		args = append(args, cfg.MPVArgs...)
	}

	return args
}

func probeExistingInstance(socketPath string) (bool, error) {
	client := ipc.NewIPCClient(socketPath)
	if err := client.Connect(); err != nil {
		return false, err
	}
	defer client.Close()

	_, err := client.GetProperty("mpv-version")
	if err != nil {
		return false, err
	}
	return true, nil
}