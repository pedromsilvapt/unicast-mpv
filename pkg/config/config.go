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
	Instance     string
	ShortHost    string
	FullHost     string
	Deployment   string
	Platform     string
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

func GetFileNames(ctx Context) []string {
	type combo struct {
		instance   bool
		platform   bool
		deployment bool
		shortHost  bool
		fullHost   bool
	}

	prefixes := []struct {
		name string
		c    combo
	}{
		{"default", combo{}},
		{"default", combo{platform: true}},
		{"default", combo{instance: true}},
		{"default", combo{instance: true, platform: true}},
		{"default", combo{deployment: true}},
		{"default", combo{deployment: true, platform: true}},
		{"default", combo{deployment: true, instance: true}},
		{"default", combo{deployment: true, instance: true, platform: true}},
		{"", combo{shortHost: true}},
		{"", combo{shortHost: true, platform: true}},
		{"", combo{shortHost: true, instance: true}},
		{"", combo{shortHost: true, instance: true, platform: true}},
		{"", combo{shortHost: true, deployment: true}},
		{"", combo{shortHost: true, deployment: true, platform: true}},
		{"", combo{shortHost: true, deployment: true, instance: true}},
		{"", combo{shortHost: true, deployment: true, instance: true, platform: true}},
		{"", combo{fullHost: true}},
		{"", combo{fullHost: true, platform: true}},
		{"", combo{fullHost: true, instance: true}},
		{"", combo{fullHost: true, instance: true, platform: true}},
		{"", combo{fullHost: true, deployment: true}},
		{"", combo{fullHost: true, deployment: true, platform: true}},
		{"", combo{fullHost: true, deployment: true, instance: true}},
		{"", combo{fullHost: true, deployment: true, instance: true, platform: true}},
		{"local", combo{}},
		{"local", combo{platform: true}},
		{"local", combo{instance: true}},
		{"local", combo{instance: true, platform: true}},
		{"local", combo{deployment: true}},
		{"local", combo{deployment: true, platform: true}},
		{"local", combo{deployment: true, instance: true}},
		{"local", combo{deployment: true, instance: true, platform: true}},
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

func buildFileName(prefix string, c combo, ctx Context) string {
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

type combo = struct {
	instance   bool
	platform   bool
	deployment bool
	shortHost  bool
	fullHost   bool
}

type Config struct {
	data map[string]interface{}
}

func NewConfig(data map[string]interface{}) *Config {
	return newConfig(data)
}

func newConfig(data map[string]interface{}) *Config {
	if data == nil {
		data = make(map[string]interface{})
	}
	return &Config{data: data}
}

func Load(dir string) (*Config, error) {
	dir = filepath.Clean(dir)

	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return newConfig(nil), nil
		}
		return nil, fmt.Errorf("config: stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return LoadFile(dir)
	}

	ctx := GetContext()
	names := GetFileNames(ctx)

	merged := make(map[string]interface{})
	for _, name := range names {
		path := filepath.Join(dir, name)
		data, err := loadYAMLFile(path)
		if err != nil {
			return nil, fmt.Errorf("config: load %s: %w", path, err)
		}
		if data != nil {
			merged = deepMerge(merged, data)
		}
	}

	return newConfig(merged), nil
}

func LoadFile(path string) (*Config, error) {
	data, err := loadYAMLFile(path)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return newConfig(nil), nil
	}
	return newConfig(data), nil
}

func LoadOptional(path string) (*Config, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return newConfig(nil), nil
		}
		path = filepath.Join(home, "unicast-mpv.yaml")
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return newConfig(nil), nil
	}

	return LoadFile(path)
}

func Merge(configs ...*Config) *Config {
	if len(configs) == 0 {
		return newConfig(nil)
	}
	if len(configs) == 1 {
		return configs[0]
	}

	merged := make(map[string]interface{})
	for _, c := range configs {
		if c == nil || c.data == nil {
			continue
		}
		merged = deepMerge(merged, c.data)
	}
	return newConfig(merged)
}

func (c *Config) Get(path string, defaultVal ...interface{}) interface{} {
	val, ok := c.getByPath(path)
	if !ok {
		if len(defaultVal) > 0 {
			return defaultVal[0]
		}
		return nil
	}
	return val
}

func (c *Config) GetString(path string, defaultVal ...string) string {
	val := c.Get(path)
	if val == nil {
		if len(defaultVal) > 0 {
			return defaultVal[0]
		}
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", val)
}

func (c *Config) GetInt(path string, defaultVal ...int) int {
	val := c.Get(path)
	if val == nil {
		if len(defaultVal) > 0 {
			return defaultVal[0]
		}
		return 0
	}
	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

func (c *Config) GetBool(path string, defaultVal ...bool) bool {
	val := c.Get(path)
	if val == nil {
		if len(defaultVal) > 0 {
			return defaultVal[0]
		}
		return false
	}
	if b, ok := val.(bool); ok {
		return b
	}
	return false
}

func (c *Config) Has(path string) bool {
	_, ok := c.getByPath(path)
	return ok
}

func (c *Config) Slice(path string) *Config {
	val := c.Get(path)
	if m, ok := val.(map[string]interface{}); ok {
		return newConfig(m)
	}
	return newConfig(nil)
}

func (c *Config) Data() map[string]interface{} {
	return c.data
}

func (c *Config) Clone() *Config {
	return newConfig(deepCopyMap(c.data))
}

func (c *Config) getByPath(path string) (interface{}, bool) {
	parts := strings.Split(path, ".")
 current := interface{}(c.data)

	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		current, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func loadYAMLFile(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	var result map[string]interface{}
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	return result, nil
}

func deepMerge(base, overlay map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range base {
		result[k] = deepCopyValue(v)
	}
	for k, v := range overlay {
		if existing, ok := result[k]; ok {
			if baseMap, ok := existing.(map[string]interface{}); ok {
				if overlayMap, ok := v.(map[string]interface{}); ok {
					result[k] = deepMerge(baseMap, overlayMap)
					continue
				}
			}
		}
		result[k] = deepCopyValue(v)
	}
	return result
}

func deepCopyValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		return deepCopyMap(val)
	case []interface{}:
		cp := make([]interface{}, len(val))
		for i, elem := range val {
			cp[i] = deepCopyValue(elem)
		}
		return cp
	default:
		return v
	}
}

func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = deepCopyValue(v)
	}
	return result
}