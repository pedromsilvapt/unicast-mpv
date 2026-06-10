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

func TestConfigGetDotPath(t *testing.T) {
	cfg := newConfig(map[string]interface{}{
		"server": map[string]interface{}{
			"port":    2019,
			"address": "0.0.0.0",
		},
		"player": map[string]interface{}{
			"fullscreen": true,
			"subtitles": map[string]interface{}{
				"font": "Droid Sans",
			},
		},
	})

	if !cfg.Has("server.port") {
		t.Error("expected Has(server.port) to be true")
	}
	if cfg.Has("server.nonexistent") {
		t.Error("expected Has(server.nonexistent) to be false")
	}

	port := cfg.Get("server.port")
	if port != 2019 {
		t.Errorf("expected server.port=2019, got %v", port)
	}

	addr := cfg.Get("server.address")
	if addr != "0.0.0.0" {
		t.Errorf("expected server.address=0.0.0.0, got %v", addr)
	}

	font := cfg.Get("player.subtitles.font")
	if font != "Droid Sans" {
		t.Errorf("expected player.subtitles.font=Droid Sans, got %v", font)
	}

	def := cfg.Get("server.missing", "fallback")
	if def != "fallback" {
		t.Errorf("expected default value 'fallback', got %v", def)
	}

	nilVal := cfg.Get("server.missing")
	if nilVal != nil {
		t.Errorf("expected nil for missing path, got %v", nilVal)
	}
}

func TestConfigGetTypedMethods(t *testing.T) {
	cfg := newConfig(map[string]interface{}{
		"port":    2019,
		"host":    "localhost",
		"enabled": true,
	})

	if cfg.GetString("host") != "localhost" {
		t.Errorf("expected localhost, got %s", cfg.GetString("host"))
	}
	if cfg.GetString("missing", "default") != "default" {
		t.Errorf("expected default, got %s", cfg.GetString("missing", "default"))
	}

	if cfg.GetInt("port") != 2019 {
		t.Errorf("expected 2019, got %d", cfg.GetInt("port"))
	}
	if cfg.GetInt("missing", 8080) != 8080 {
		t.Errorf("expected 8080, got %d", cfg.GetInt("missing", 8080))
	}

	if !cfg.GetBool("enabled") {
		t.Error("expected enabled=true")
	}
	if cfg.GetBool("missing", true) != true {
		t.Error("expected default bool=true")
	}
}

func TestConfigSlice(t *testing.T) {
	cfg := newConfig(map[string]interface{}{
		"server": map[string]interface{}{
			"port":    2019,
			"address": "0.0.0.0",
			"sub": map[string]interface{}{
				"key": "value",
			},
		},
	})

	sub := cfg.Slice("server")
	if !sub.Has("port") {
		t.Error("expected slice to have 'port'")
	}
	if sub.GetInt("port") != 2019 {
		t.Errorf("expected port=2019 in slice, got %d", sub.GetInt("port"))
	}

	sub2 := sub.Slice("sub")
	if sub2.GetString("key") != "value" {
		t.Errorf("expected key=value in nested slice, got %s", sub2.GetString("key"))
	}

	empty := cfg.Slice("nonexistent")
	if len(empty.Data()) != 0 {
		t.Error("expected empty config for nonexistent path")
	}
}

func TestConfigClone(t *testing.T) {
	original := newConfig(map[string]interface{}{
		"key": "value",
		"nested": map[string]interface{}{
			"inner": "data",
		},
	})

	clone := original.Clone()
	clone.data["key"] = "changed"
	clone.data["nested"].(map[string]interface{})["inner"] = "modified"

	if original.GetString("key") != "value" {
		t.Error("clone should not affect original")
	}
	if original.GetString("nested.inner") != "data" {
		t.Error("clone deep changes should not affect original")
	}
}

func TestMerge(t *testing.T) {
	base := newConfig(map[string]interface{}{
		"server": map[string]interface{}{
			"port":    3000,
			"address": "localhost",
		},
		"debug": true,
	})

	overlay := newConfig(map[string]interface{}{
		"server": map[string]interface{}{
			"port": 2019,
		},
		"debug": false,
	})

	merged := Merge(base, overlay)

	if merged.GetInt("server.port") != 2019 {
		t.Errorf("expected merged server.port=2019, got %d", merged.GetInt("server.port"))
	}
	if merged.GetString("server.address") != "localhost" {
		t.Errorf("expected merged server.address=localhost, got %s", merged.GetString("server.address"))
	}
	if merged.GetBool("debug") != false {
		t.Error("expected merged debug=false")
	}
}

func TestMergeDeepNested(t *testing.T) {
	c1 := newConfig(map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": 1,
				"d": 2,
			},
		},
	})

	c2 := newConfig(map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": 10,
				"e": 3,
			},
		},
	})

	merged := Merge(c1, c2)

	if merged.GetInt("a.b.c") != 10 {
		t.Errorf("expected a.b.c=10, got %d", merged.GetInt("a.b.c"))
	}
	if merged.GetInt("a.b.d") != 2 {
		t.Errorf("expected a.b.d=2, got %d", merged.GetInt("a.b.d"))
	}
	if merged.GetInt("a.b.e") != 3 {
		t.Errorf("expected a.b.e=3, got %d", merged.GetInt("a.b.e"))
	}
}

func TestMergeSingle(t *testing.T) {
	cfg := newConfig(map[string]interface{}{"key": "value"})
	merged := Merge(cfg)
	if merged != cfg {
		t.Error("Merge with single config should return same config")
	}
}

func TestMergeEmpty(t *testing.T) {
	merged := Merge()
	if merged == nil {
		t.Error("Merge with no configs should return non-nil config")
	}
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	yamlContent := "server:\n  port: 2019\n  address: 0.0.0.0\n"
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.GetInt("server.port") != 2019 {
		t.Errorf("expected server.port=2019, got %d", cfg.GetInt("server.port"))
	}
	if cfg.GetString("server.address") != "0.0.0.0" {
		t.Errorf("expected server.address=0.0.0.0, got %s", cfg.GetString("server.address"))
	}
}

func TestLoadFileNotFound(t *testing.T) {
	cfg, err := LoadFile("/nonexistent/path/test.yaml")
	if err != nil {
		t.Errorf("expected no error for nonexistent file (graceful), got %v", err)
	}
	if len(cfg.Data()) != 0 {
		t.Error("expected empty config for nonexistent file")
	}
}

func TestLoadDirectory(t *testing.T) {
	dir := t.TempDir()

	defaultYAML := "server:\n  port: 3000\n  address: 0.0.0.0\n"
	if err := os.WriteFile(filepath.Join(dir, "default.yaml"), []byte(defaultYAML), 0644); err != nil {
		t.Fatal(err)
	}

	localYAML := "server:\n  port: 2019\n"
	if err := os.WriteFile(filepath.Join(dir, "local.yaml"), []byte(localYAML), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.GetInt("server.port") != 2019 {
		t.Errorf("expected server.port=2019 (local override), got %d", cfg.GetInt("server.port"))
	}
	if cfg.GetString("server.address") != "0.0.0.0" {
		t.Errorf("expected server.address=0.0.0.0 (from default), got %s", cfg.GetString("server.address"))
	}
}

func TestLoadDirectoryLayering(t *testing.T) {
	dir := t.TempDir()

	defaultYAML := "player:\n  fullscreen: false\n  monitor: null\nserver:\n  port: 3000\n"
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

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.GetBool("player.fullscreen") != false {
		t.Error("expected player.fullscreen=false from default")
	}
	if cfg.GetString("player.binary") != "/usr/bin/mpv" {
		t.Errorf("expected player.binary=/usr/bin/mpv from platform, got %s", cfg.GetString("player.binary"))
	}
	if cfg.GetInt("server.port") != 2019 {
		t.Errorf("expected server.port=2019 from local, got %d", cfg.GetInt("server.port"))
	}
}

func TestLoadNonexistentDirectory(t *testing.T) {
	cfg, err := Load("/nonexistent/directory")
	if err != nil {
		t.Errorf("expected no error for nonexistent directory, got %v", err)
	}
	if len(cfg.Data()) != 0 {
		t.Error("expected empty config for nonexistent directory")
	}
}

func TestLoadFileAsDirectory(t *testing.T) {
	dir := t.TempDir()
	yamlContent := "key: value\n"
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GetString("key") != "value" {
		t.Errorf("expected key=value, got %s", cfg.GetString("key"))
	}
}

func TestLoadOptionalExplicit(t *testing.T) {
	dir := t.TempDir()
	yamlContent := "custom: true\n"
	path := filepath.Join(dir, "unicast-mpv.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadOptional(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.GetBool("custom") {
		t.Error("expected custom=true")
	}
}

func TestLoadOptionalNonexistent(t *testing.T) {
	cfg, err := LoadOptional("/nonexistent/file.yaml")
	if err != nil {
		t.Errorf("expected no error for nonexistent optional file, got %v", err)
	}
	if len(cfg.Data()) != 0 {
		t.Error("expected empty config for nonexistent optional file")
	}
}

func TestDeepMergeArray(t *testing.T) {
	base := newConfig(map[string]interface{}{
		"items": []interface{}{"a", "b"},
	})
	overlay := newConfig(map[string]interface{}{
		"items": []interface{}{"c", "d"},
	})

	merged := Merge(base, overlay)

	items, ok := merged.Get("items").([]interface{})
	if !ok {
		t.Fatal("expected items to be a slice")
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
	if items[0] != "c" || items[1] != "d" {
		t.Errorf("expected items [c d], got %v", items)
	}
}

func TestDeepMergeNilOverlay(t *testing.T) {
	cfg := newConfig(map[string]interface{}{"key": "value"})

	merged := Merge(cfg, nil)
	if merged.GetString("key") != "value" {
		t.Error("expected key=value after merging nil")
	}
}

func TestConfigGetNestedPath(t *testing.T) {
	cfg := newConfig(map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"level3": map[string]interface{}{
					"value": "deep",
				},
			},
		},
	})

	if cfg.GetString("level1.level2.level3.value") != "deep" {
		t.Errorf("expected deep nested value, got %s", cfg.GetString("level1.level2.level3.value"))
	}
}

func TestConfigHasMissingNestedPath(t *testing.T) {
	cfg := newConfig(map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": "flat",
		},
	})

	if cfg.Has("level1.level2.level3") {
		t.Error("expected Has(level1.level2.level3) to be false since level2 is not a map")
	}
}

func TestConfigNullValues(t *testing.T) {
	dir := t.TempDir()
	yamlContent := "key: null\nother: value\n"
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Has("key") {
		val := cfg.Get("key")
		if val != nil {
			t.Errorf("expected nil for null yaml value, got %v", val)
		}
	}
}