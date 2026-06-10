package config

import (
	"fmt"

	"github.com/unicast/unicast-mpv/pkg/util/cases"
)

type AppConfig struct {
	Debug  bool         `yaml:"debug"`
	Player PlayerConfig `yaml:"player"`
	Server ServerConfig `yaml:"server"`
}

type PlayerConfig struct {
	Binary        string          `yaml:"binary"`
	SocketPath    string          `yaml:"socketPath"`
	IPCCommand    string          `yaml:"ipcCommand"`
	AutoRestart   bool            `yaml:"autoRestart"`
	Fullscreen    bool            `yaml:"fullscreen"`
	Monitor       int             `yaml:"monitor"`
	OnTop         bool            `yaml:"onTop"`
	QuitOnStop    bool            `yaml:"quitOnStop"`
	RestartOnPlay bool            `yaml:"restartOnPlay"`
	VideoOutput   string          `yaml:"videoOutput"`
	AudioOutput   string          `yaml:"audioOutput"`
	AudioDevice   string          `yaml:"audioDevice"`
	Args          []string        `yaml:"args"`
	Subtitles     SubtitlesConfig `yaml:"subtitles"`
}

type SubtitlesConfig struct {
	FixTiming    bool    `yaml:"fixTiming"`
	Font         string  `yaml:"font"`
	Color        *string `yaml:"color"`
	Bold         bool    `yaml:"bold"`
	Italic       bool    `yaml:"italic"`
	Spacing      *int    `yaml:"spacing"`
	BackColor   *string `yaml:"backColor"`
	BorderColor *string `yaml:"borderColor"`
	BorderSize   *int    `yaml:"borderSize"`
	ShadowColor  *string `yaml:"shadowColor"`
	ShadowOffset *int    `yaml:"shadowOffset"`
	MarginX      *int    `yaml:"marginX"`
	MarginY      *int    `yaml:"marginY"`
}

type ServerConfig struct {
	Port              int    `yaml:"port"`
	Address           string `yaml:"address"`
	Authenticate      string `yaml:"authenticate"`
	DisableRunCommand bool   `yaml:"disableRunCommand"`
}

func valueToMpv(value interface{}) string {
	switch v := value.(type) {
	case bool:
		if v {
			return "yes"
		}
		return "no"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (s SubtitlesConfig) ToMPVArgs() []string {
	var args []string

	m := map[string]interface{}{}
	m["fixTiming"] = s.FixTiming
	m["font"] = s.Font
	m["bold"] = s.Bold
	m["italic"] = s.Italic
	if s.Spacing != nil {
		m["spacing"] = *s.Spacing
	}
	if s.Color != nil {
		m["color"] = *s.Color
	}
	if s.BackColor != nil {
		m["backColor"] = *s.BackColor
	}
	if s.BorderColor != nil {
		m["borderColor"] = *s.BorderColor
	}
	if s.BorderSize != nil {
		m["borderSize"] = *s.BorderSize
	}
	if s.ShadowColor != nil {
		m["shadowColor"] = *s.ShadowColor
	}
	if s.ShadowOffset != nil {
		m["shadowOffset"] = *s.ShadowOffset
	}
	if s.MarginX != nil {
		m["marginX"] = *s.MarginX
	}
	if s.MarginY != nil {
		m["marginY"] = *s.MarginY
	}

	if len(m) == 0 {
		return args
	}

	kebabSubs := cases.Convert(m, cases.Kebab)
	for key, value := range kebabSubs {
		args = append(args, fmt.Sprintf("--sub-%s=%s", key, valueToMpv(value)))
	}

	return args
}

func IntPtr(i int) *int      { return &i }
func StrPtr(s string) *string { return &s }

func DefaultAppConfig() AppConfig {
	return AppConfig{
		Player: PlayerConfig{
			SocketPath:    "/tmp/node-mpv.sock",
			AutoRestart:   true,
			Fullscreen:    true,
			QuitOnStop:    true,
			Monitor:       -1,
			Subtitles: SubtitlesConfig{
				FixTiming:    true,
				Font:         "Droid Sans",
				Bold:         true,
			Spacing:      IntPtr(0),
			ShadowOffset: IntPtr(0),
			MarginX:      IntPtr(25),
			MarginY:      IntPtr(46),
			},
		},
		Server: ServerConfig{
			Port:    2019,
			Address: "0.0.0.0",
		},
	}
}