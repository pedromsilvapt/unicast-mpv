package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"strings"
	"syscall"

	"github.com/unicast/unicast-mpv/pkg/commands"
	"github.com/unicast/unicast-mpv/pkg/config"
	"github.com/unicast/unicast-mpv/pkg/events"
	"github.com/unicast/unicast-mpv/pkg/logger"
	"github.com/unicast/unicast-mpv/pkg/mpv"
	"github.com/unicast/unicast-mpv/pkg/mpv/process"
	"github.com/unicast/unicast-mpv/pkg/player"
	"github.com/unicast/unicast-mpv/pkg/server"
)

//go:embed config
var embeddedConfig embed.FS

func main() {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			fmt.Fprintf(os.Stderr, "panic recovered: %v\n%s\n", r, buf[:n])
			os.Exit(1)
		}
	}()

	cfg, err := loadConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	minLevel := logger.InfoLevel
	if cfg.GetBool("debug", false) {
		minLevel = logger.DebugLevel
	}
	log := logger.New(logger.WithMinLevel(minLevel))

	rpcLogger := log.Service("rpc")

	ignoredEvents := map[string]bool{"status": true}

	srv := server.NewServer(cfg, rpcLogger)

	mpvInst := createMPV(cfg, log)

	playerInst := player.NewPlayer(cfg.Slice("player"), mpvInst, log)

	registry := commands.NewCommandRegistry(srv, playerInst, mpvInst)

	commands.NewNativeCommands(registry)
	commands.NewStatusCommand(registry, playerInst.Status, log.Service("status"))
	commands.NewQuitCommand(registry)
	commands.NewPlayCommand(registry, cfg.Slice("player"), log)

	events.Bridge(mpvInst, srv)

	srv.RegisterGlobalPreHook(func(args []interface{}, method string, ctx map[string]interface{}) {
		if !ignoredEvents[method] {
			argsJSON, _ := json.Marshal(args)
			srvLogger(log, method).Info(fmt.Sprintf("\033[36memit\033[0m %s", string(argsJSON)))
		}
	})

	srv.RegisterGlobalPostHook(func(args []interface{}, method string, err error, result interface{}, ctx map[string]interface{}) {
		if err != nil {
			log.Service(method).Error(err.Error())
		}
	})

	srv.RegisterHighFrequencyPattern(regexp.MustCompile(`status`), nil, 300)

	setupSignalHandler(srv, mpvInst, log)

	log.Infof("Starting unicast-mpv on %s:%d", cfg.GetString("server.address", "0.0.0.0"), cfg.GetInt("server.port", 2019))

	if err := srv.Listen(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func loadConfig(args []string) (*config.Config, error) {
	baseCfg, err := loadBaseConfig()
	if err != nil {
		return nil, fmt.Errorf("load base config: %w", err)
	}

	var overrideCfg *config.Config
	if len(args) > 0 && args[0] != "" {
		overrideCfg, err = config.LoadFile(args[0])
		if err != nil {
			return nil, fmt.Errorf("load config from %s: %w", args[0], err)
		}
	} else {
		overrideCfg, err = config.LoadOptional("")
		if err != nil {
			overrideCfg = config.NewConfig(nil)
		}
	}

	return config.Merge(baseCfg, overrideCfg), nil
}

func loadBaseConfig() (*config.Config, error) {
	tempDir, err := os.MkdirTemp("", "unicast-mpv-config-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	entries, err := embeddedConfig.ReadDir("config")
	if err != nil {
		return nil, fmt.Errorf("read embedded config: %w", err)
	}

	for _, entry := range entries {
		data, err := embeddedConfig.ReadFile("config/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read embedded config %s: %w", entry.Name(), err)
		}
		if err := os.WriteFile(tempDir+"/"+entry.Name(), data, 0644); err != nil {
			return nil, fmt.Errorf("write config %s: %w", entry.Name(), err)
		}
	}

	return config.Load(tempDir)
}

func createMPV(cfg *config.Config, log *logger.Logger) *mpv.MPV {
	playerCfg := cfg.Slice("player")

	binary := ""
	if b := playerCfg.Get("binary"); b != nil {
		if s, ok := b.(string); ok && s != "" {
			binary = s
		}
	}

	ipcCommand := ""
	if ic := playerCfg.Get("ipcCommand"); ic != nil {
		if s, ok := ic.(string); ok && s != "" {
			ipcCommand = s
		}
	}

	autoRestart := true
	if ar := playerCfg.Get("autoRestart"); ar != nil {
		if b, ok := ar.(bool); ok {
			autoRestart = b
		}
	}

	socketPath := "/tmp/node-mpv.sock"
	if sp := playerCfg.Get("socketPath"); sp != nil {
		if s, ok := sp.(string); ok && s != "" {
			socketPath = s
		}
	}

	args := player.BuildMPVArgs(playerCfg)

	procCfg := process.ProcessConfig{
		Binary:      binary,
		SocketPath:  socketPath,
		AutoRestart: autoRestart,
		IPCCommand:  ipcCommand,
		MPVArgs:     args,
	}

	return mpv.NewMPV(procCfg, mpv.WithLogger(log))
}

func setupSignalHandler(srv *server.Server, mpvInst *mpv.MPV, log *logger.Logger) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 4096)
				n := runtime.Stack(buf, false)
				log.Errorf("panic in signal handler: %v\n%s", r, string(buf[:n]))
			}
		}()

		sig := <-sigCh
		log.Infof("received signal %s, shutting down...", sig)

		if mpvInst.IsRunning() {
			_ = mpvInst.Quit()
		}

		_ = srv.Close()
		os.Exit(0)
	}()
}

func srvLogger(log *logger.Logger, method string) *logger.Logger {
	return log.Service(method)
}

func formatArgsForLog(args []interface{}) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		b, _ := json.Marshal(arg)
		parts[i] = string(b)
	}
	return strings.Join(parts, " ")
}