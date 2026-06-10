package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

type Context struct {
	Instance   string
	ShortHost  string
	FullHost   string
	Deployment string
	Platform   string
}

func GetContext() Context {
	fullHost, _ := os.Hostname()
	if h := os.Getenv("HOSTNAME"); h != "" {
		fullHost = h
	}
	if h := os.Getenv("HOST"); h != "" {
		fullHost = h
	}
	shortHost := fullHost
	if idx := strings.Index(fullHost, "."); idx > 0 {
		shortHost = fullHost[:idx]
	}
	deployment := os.Getenv("NODE_ENV")
	if deployment == "" {
		deployment = "development"
	}
	platform := runtime.GOOS
	instance := os.Getenv("NODE_APP_INSTANCE")

	return Context{
		Instance:   instance,
		ShortHost:  shortHost,
		FullHost:   fullHost,
		Deployment: deployment,
		Platform:   platform,
	}
}

type fileCombo struct {
	instance   bool
	platform   bool
	deployment bool
	shortHost  bool
	fullHost   bool
}

func GetFileNames(ctx Context) []string {
	prefixes := []struct {
		name string
		c    fileCombo
	}{
		{"default", fileCombo{}},
		{"default", fileCombo{platform: true}},
		{"default", fileCombo{instance: true}},
		{"default", fileCombo{instance: true, platform: true}},
		{"default", fileCombo{deployment: true}},
		{"default", fileCombo{deployment: true, platform: true}},
		{"default", fileCombo{deployment: true, instance: true}},
		{"default", fileCombo{deployment: true, instance: true, platform: true}},
		{"", fileCombo{shortHost: true}},
		{"", fileCombo{shortHost: true, platform: true}},
		{"", fileCombo{shortHost: true, instance: true}},
		{"", fileCombo{shortHost: true, instance: true, platform: true}},
		{"", fileCombo{shortHost: true, deployment: true}},
		{"", fileCombo{shortHost: true, deployment: true, platform: true}},
		{"", fileCombo{shortHost: true, deployment: true, instance: true}},
		{"", fileCombo{shortHost: true, deployment: true, instance: true, platform: true}},
		{"", fileCombo{fullHost: true}},
		{"", fileCombo{fullHost: true, platform: true}},
		{"", fileCombo{fullHost: true, instance: true}},
		{"", fileCombo{fullHost: true, instance: true, platform: true}},
		{"", fileCombo{fullHost: true, deployment: true}},
		{"", fileCombo{fullHost: true, deployment: true, platform: true}},
		{"", fileCombo{fullHost: true, deployment: true, instance: true}},
		{"", fileCombo{fullHost: true, deployment: true, instance: true, platform: true}},
		{"local", fileCombo{}},
		{"local", fileCombo{platform: true}},
		{"local", fileCombo{instance: true}},
		{"local", fileCombo{instance: true, platform: true}},
		{"local", fileCombo{deployment: true}},
		{"local", fileCombo{deployment: true, platform: true}},
		{"local", fileCombo{deployment: true, instance: true}},
		{"local", fileCombo{deployment: true, instance: true, platform: true}},
	}

	var result []string
	for _, p := range prefixes {
		name := buildFileName(p.name, p.c, ctx)
		if name != "" {
			result = append(result, name)
		}
	}
	return result
}

func buildFileName(prefix string, c fileCombo, ctx Context) string {
	if c.shortHost || c.fullHost {
		host := ""
		if c.shortHost {
			host = ctx.ShortHost
		} else if c.fullHost {
			host = ctx.FullHost
		}
		if host == "" {
			return ""
		}
		parts := []string{host}
		if c.deployment {
			parts = append(parts, ctx.Deployment)
		}
		if c.instance {
			parts = append(parts, ctx.Instance)
		}
		if c.platform {
			parts = append(parts, ctx.Platform)
		}
		for _, p := range parts[1:] {
			if p == "" {
				return ""
			}
		}
		return strings.Join(parts, "-") + ".yaml"
	}

	parts := []string{}
	if prefix != "" {
		parts = append(parts, prefix)
	}
	if c.deployment {
		parts = append(parts, ctx.Deployment)
	}
	if c.instance {
		parts = append(parts, ctx.Instance)
	}
	if c.platform {
		parts = append(parts, ctx.Platform)
	}
	for _, p := range parts[1:] {
		if p == "" {
			return ""
		}
	}
	return strings.Join(parts, "-") + ".yaml"
}

func LoadAppConfig(layers ...[]byte) (*AppConfig, error) {
	cfg := DefaultAppConfig()
	for _, data := range layers {
		if len(data) == 0 {
			continue
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
	}
	return &cfg, nil
}

func LoadLayersFromDir(dir string) ([][]byte, error) {
	dir = filepath.Clean(dir)

	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("config: stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		data, err := os.ReadFile(dir)
		if err != nil {
			return nil, fmt.Errorf("config: read %s: %w", dir, err)
		}
		return [][]byte{data}, nil
	}

	ctx := GetContext()
	names := GetFileNames(ctx)

	var layers [][]byte
	for _, name := range names {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("config: read %s: %w", path, err)
		}
		layers = append(layers, data)
	}
	return layers, nil
}