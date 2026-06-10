package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGetContextDefaults(t *testing.T) {
	os.Unsetenv("HOST")
	os.Unsetenv("HOSTNAME")
	os.Unsetenv("NODE_ENV")
	os.Unsetenv("NODE_APP_INSTANCE")

	ctx := GetContext()

	if ctx.Deployment != "development" {
		t.Errorf("expected Deployment=development, got %s", ctx.Deployment)
	}
	if ctx.Platform != runtime.GOOS {
		t.Errorf("expected Platform=%s, got %s", runtime.GOOS, ctx.Platform)
	}
}

func TestGetContextEnvVars(t *testing.T) {
	os.Setenv("HOST", "myhost.example.com")
	os.Setenv("NODE_ENV", "production")
	os.Setenv("NODE_APP_INSTANCE", "worker1")
	defer os.Unsetenv("HOST")
	defer os.Unsetenv("NODE_ENV")
	defer os.Unsetenv("NODE_APP_INSTANCE")

	ctx := GetContext()

	if ctx.FullHost != "myhost.example.com" {
		t.Errorf("expected FullHost=myhost.example.com, got %s", ctx.FullHost)
	}
	if ctx.ShortHost != "myhost" {
		t.Errorf("expected ShortHost=myhost, got %s", ctx.ShortHost)
	}
	if ctx.Deployment != "production" {
		t.Errorf("expected Deployment=production, got %s", ctx.Deployment)
	}
	if ctx.Instance != "worker1" {
		t.Errorf("expected Instance=worker1, got %s", ctx.Instance)
	}
}

func TestGetContextHostnameFallback(t *testing.T) {
	os.Unsetenv("HOST")
	os.Unsetenv("HOSTNAME")
	hostname, _ := os.Hostname()

	ctx := GetContext()

	if ctx.FullHost != hostname {
		t.Errorf("expected FullHost=%s, got %s", hostname, ctx.FullHost)
	}
}

func TestGetFileNamesBasic(t *testing.T) {
	ctx := Context{
		Instance:   "worker1",
		ShortHost:  "myhost",
		FullHost:   "myhost.example.com",
		Deployment: "production",
		Platform:   "linux",
	}

	names := GetFileNames(ctx)

	expected := []string{
		"default.yaml",
		"default-linux.yaml",
		"default-worker1.yaml",
		"default-worker1-linux.yaml",
		"default-production.yaml",
		"default-production-linux.yaml",
		"default-production-worker1.yaml",
		"default-production-worker1-linux.yaml",
		"myhost.yaml",
		"myhost-linux.yaml",
		"myhost-worker1.yaml",
		"myhost-worker1-linux.yaml",
		"myhost-production.yaml",
		"myhost-production-linux.yaml",
		"myhost-production-worker1.yaml",
		"myhost-production-worker1-linux.yaml",
		"myhost.example.com.yaml",
		"myhost.example.com-linux.yaml",
		"myhost.example.com-worker1.yaml",
		"myhost.example.com-worker1-linux.yaml",
		"myhost.example.com-production.yaml",
		"myhost.example.com-production-linux.yaml",
		"myhost.example.com-production-worker1.yaml",
		"myhost.example.com-production-worker1-linux.yaml",
		"local.yaml",
		"local-linux.yaml",
		"local-worker1.yaml",
		"local-worker1-linux.yaml",
		"local-production.yaml",
		"local-production-linux.yaml",
		"local-production-worker1.yaml",
		"local-production-worker1-linux.yaml",
	}

	if len(names) != len(expected) {
		t.Fatalf("expected %d filenames, got %d", len(expected), len(names))
	}

	for i, exp := range expected {
		if names[i] != exp {
			t.Errorf("name[%d]: expected %s, got %s", i, exp, names[i])
		}
	}
}

func TestGetFileNamesMissingInstance(t *testing.T) {
	ctx := Context{
		Instance:   "",
		ShortHost: "myhost",
		FullHost:  "myhost.example.com",
		Deployment: "production",
		Platform:  "linux",
	}

	names := GetFileNames(ctx)

	for _, name := range names {
		if strings.Contains(name, "production-") && strings.Contains(name, "-linux") {
			if strings.Contains(name, "production--") {
				t.Errorf("should not have empty segment in name: %s", name)
			}
		}
	}

	for _, name := range names {
		if strings.Contains(name, "--") {
			t.Errorf("should not have double dash in name: %s", name)
		}
	}
}

func TestGetFileNamesNoInstanceOrDeployment(t *testing.T) {
	ctx := Context{
		Instance:   "",
		ShortHost: "myhost",
		FullHost:  "myhost.example.com",
		Deployment: "development",
		Platform:  "linux",
	}

	names := GetFileNames(ctx)

	instanceNames := []string{}
	for _, name := range names {
		if strings.Contains(name, "-worker") || name == "default-worker1.yaml" {
			instanceNames = append(instanceNames, name)
		}
	}

	if len(instanceNames) > 0 {
		t.Errorf("expected no instance-specific names when instance is empty, got %v", instanceNames)
	}

	hasDefaultYaml := false
	hasDefaultPlatformYaml := false
	for _, name := range names {
		if name == "default.yaml" {
			hasDefaultYaml = true
		}
		if name == "default-linux.yaml" {
			hasDefaultPlatformYaml = true
		}
	}
	if !hasDefaultYaml {
		t.Error("expected default.yaml in filenames")
	}
	if !hasDefaultPlatformYaml {
		t.Error("expected default-linux.yaml in filenames")
	}
}

func TestLoadAppConfigBasic(t *testing.T) {
	yaml := "server:\n  port: 2019\n  address: 0.0.0.0\nplayer:\n  fullscreen: true\n  subtitles:\n    font: Droid Sans\n"
	cfg, err := LoadAppConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadAppConfig error: %v", err)
	}

	if cfg.Server.Port != 2019 {
		t.Errorf("expected server.port=2019, got %d", cfg.Server.Port)
	}
	if cfg.Server.Address != "0.0.0.0" {
		t.Errorf("expected server.address=0.0.0.0, got %s", cfg.Server.Address)
	}
	if !cfg.Player.Fullscreen {
		t.Error("expected player.fullscreen=true")
	}
	if cfg.Player.Subtitles.Font != "Droid Sans" {
		t.Errorf("expected player.subtitles.font=Droid Sans, got %s", cfg.Player.Subtitles.Font)
	}
}

func TestLoadAppConfigDefaults(t *testing.T) {
	cfg, err := LoadAppConfig()
	if err != nil {
		t.Fatalf("LoadAppConfig error: %v", err)
	}

	if cfg.Server.Port != 2019 {
		t.Errorf("expected default server.port=2019, got %d", cfg.Server.Port)
	}
	if cfg.Server.Address != "0.0.0.0" {
		t.Errorf("expected default server.address=0.0.0.0, got %s", cfg.Server.Address)
	}
	if !cfg.Player.QuitOnStop {
		t.Error("expected default player.quitOnStop=true")
	}
}

func TestLoadAppConfigLayering(t *testing.T) {
	base := "server:\n  port: 3000\n  address: 0.0.0.0\n"
	override := "server:\n  port: 2019\n"

	cfg, err := LoadAppConfig([]byte(base), []byte(override))
	if err != nil {
		t.Fatalf("LoadAppConfig error: %v", err)
	}

	if cfg.Server.Port != 2019 {
		t.Errorf("expected server.port=2019 (overridden), got %d", cfg.Server.Port)
	}
	if cfg.Server.Address != "0.0.0.0" {
		t.Errorf("expected server.address=0.0.0.0 (from base), got %s", cfg.Server.Address)
	}
}

func TestLoadAppConfigNullPointerClears(t *testing.T) {
	base := "player:\n  subtitles:\n    marginX: 25\n    marginY: 46\n"
	override := "player:\n  subtitles:\n    marginX: null\n"

	cfg, err := LoadAppConfig([]byte(base), []byte(override))
	if err != nil {
		t.Fatalf("LoadAppConfig error: %v", err)
	}

	if cfg.Player.Subtitles.MarginX != nil {
		t.Errorf("expected MarginX to be nil after null override, got %v", cfg.Player.Subtitles.MarginX)
	}
	if cfg.Player.Subtitles.MarginY == nil || *cfg.Player.Subtitles.MarginY != 46 {
		t.Errorf("expected MarginY=46 (preserved), got %v", cfg.Player.Subtitles.MarginY)
	}
}

func TestLoadAppConfigDeepNested(t *testing.T) {
	layer1 := "player:\n  fullscreen: false\n  monitor: 1\nserver:\n  port: 3000\n"
	layer2 := "player:\n  binary: /usr/bin/mpv\nserver:\n  port: 2019\n"

	cfg, err := LoadAppConfig([]byte(layer1), []byte(layer2))
	if err != nil {
		t.Fatalf("LoadAppConfig error: %v", err)
	}

	if cfg.Player.Fullscreen {
		t.Error("expected player.fullscreen=false from layer1")
	}
	if cfg.Player.Monitor != 1 {
		t.Errorf("expected player.monitor=1 from layer1, got %d", cfg.Player.Monitor)
	}
	if cfg.Player.Binary != "/usr/bin/mpv" {
		t.Errorf("expected player.binary=/usr/bin/mpv from layer2, got %s", cfg.Player.Binary)
	}
	if cfg.Server.Port != 2019 {
		t.Errorf("expected server.port=2019 from layer2, got %d", cfg.Server.Port)
	}
}

func TestLoadLayersFromDir(t *testing.T) {
	dir := t.TempDir()

	defaultYAML := "server:\n  port: 3000\n  address: 0.0.0.0\n"
	if err := os.WriteFile(filepath.Join(dir, "default.yaml"), []byte(defaultYAML), 0644); err != nil {
		t.Fatal(err)
	}

	localYAML := "server:\n  port: 2019\n"
	if err := os.WriteFile(filepath.Join(dir, "local.yaml"), []byte(localYAML), 0644); err != nil {
		t.Fatal(err)
	}

	layers, err := LoadLayersFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadAppConfig(layers...)
	if err != nil {
		t.Fatalf("LoadAppConfig error: %v", err)
	}

	if cfg.Server.Port != 2019 {
		t.Errorf("expected server.port=2019 (local override), got %d", cfg.Server.Port)
	}
	if cfg.Server.Address != "0.0.0.0" {
		t.Errorf("expected server.address=0.0.0.0 (from default), got %s", cfg.Server.Address)
	}
}

func TestLoadLayersFromDirLayering(t *testing.T) {
	dir := t.TempDir()

	defaultYAML := "player:\n  fullscreen: false\n  monitor: -1\nserver:\n  port: 3000\n"
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

	layers, err := LoadLayersFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadAppConfig(layers...)
	if err != nil {
		t.Fatalf("LoadAppConfig error: %v", err)
	}

	if cfg.Player.Fullscreen {
		t.Error("expected player.fullscreen=false from default")
	}
	if cfg.Player.Binary != "/usr/bin/mpv" {
		t.Errorf("expected player.binary=/usr/bin/mpv from platform, got %s", cfg.Player.Binary)
	}
	if cfg.Server.Port != 2019 {
		t.Errorf("expected server.port=2019 from local, got %d", cfg.Server.Port)
	}
}

func TestLoadLayersFromDirNonexistent(t *testing.T) {
	layers, err := LoadLayersFromDir("/nonexistent/directory")
	if err != nil {
		t.Errorf("expected no error for nonexistent directory, got %v", err)
	}
	if len(layers) != 0 {
		t.Error("expected no layers for nonexistent directory")
	}
}

func TestLoadLayersFromDirAsFile(t *testing.T) {
	dir := t.TempDir()
	yamlContent := "debug: true\n"
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	layers, err := LoadLayersFromDir(path)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadAppConfig(layers...)
	if err != nil {
		t.Fatalf("LoadAppConfig error: %v", err)
	}

	if !cfg.Debug {
		t.Errorf("expected debug=true, got %v", cfg.Debug)
	}
}

func TestLoadAppConfigEmptyLayers(t *testing.T) {
	cfg, err := LoadAppConfig([]byte{})
	if err != nil {
		t.Fatalf("LoadAppConfig error: %v", err)
	}

	if cfg.Server.Port != 2019 {
		t.Errorf("expected default server.port=2019, got %d", cfg.Server.Port)
	}
}

func TestDefaultAppConfig(t *testing.T) {
	defaults := DefaultAppConfig()

	if defaults.Server.Port != 2019 {
		t.Errorf("expected default server.port=2019, got %d", defaults.Server.Port)
	}
	if defaults.Server.Address != "0.0.0.0" {
		t.Errorf("expected default server.address=0.0.0.0, got %s", defaults.Server.Address)
	}
	if !defaults.Player.QuitOnStop {
		t.Error("expected default player.quitOnStop=true")
	}
	if defaults.Player.SocketPath != "/tmp/node-mpv.sock" {
		t.Errorf("expected default player.socketPath=/tmp/node-mpv.sock, got %s", defaults.Player.SocketPath)
	}
	if defaults.Player.Subtitles.Font != "Droid Sans" {
		t.Errorf("expected default player.subtitles.font=Droid Sans, got %s", defaults.Player.Subtitles.Font)
	}
}

func TestLoadAppConfigWithMonitor(t *testing.T) {
	yaml := "player:\n  monitor: 2\nserver:\n  port: 2019\n"
	cfg, err := LoadAppConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadAppConfig error: %v", err)
	}

	if cfg.Player.Monitor != 2 {
		t.Errorf("expected monitor=2, got %d", cfg.Player.Monitor)
	}
}

func TestLoadAppConfigMonitorSentinel(t *testing.T) {
	cfg, err := LoadAppConfig()
	if err != nil {
		t.Fatalf("LoadAppConfig error: %v", err)
	}

	if cfg.Player.Monitor != -1 {
		t.Errorf("expected default monitor=-1, got %d", cfg.Player.Monitor)
	}
}

func TestLoadAppConfigNullPointerClearsSubtitle(t *testing.T) {
	yaml := "player:\n  subtitles:\n    color: null\n"
	cfg, err := LoadAppConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadAppConfig error: %v", err)
	}

	if cfg.Player.Subtitles.Color != nil {
		t.Errorf("expected Color=nil after null override, got %v", *cfg.Player.Subtitles.Color)
	}
}