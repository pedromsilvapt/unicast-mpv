package player

import (
	"fmt"

	"github.com/unicast/unicast-mpv/pkg/config"
	"github.com/unicast/unicast-mpv/pkg/logger"
	"github.com/unicast/unicast-mpv/pkg/mpv"
	"github.com/unicast/unicast-mpv/pkg/util/cases"
)

type LoadFlags string

const (
	LoadFlagsReplace    LoadFlags = "replace"
	LoadFlagsAppend     LoadFlags = "append"
	LoadFlagsAppendPlay LoadFlags = "append-play"
)

func ValueToMpv(value interface{}) string {
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

func BuildMPVArgs(cfg *config.Config) []string {
	args := []string{
		"--player-operation-mode=pseudo-gui",
		"--force-window",
		"--terminal",
	}

	if cfg.GetBool("quitOnStop", true) {
		args = append(args, "--idle=once")
	} else {
		args = append(args, "--idle=yes")
	}

	monitor := cfg.GetInt("monitor", -1)
	if monitor >= 0 {
		args = append(args, fmt.Sprintf("--screen=%d", monitor))
		args = append(args, fmt.Sprintf("--fs-screen=%d", monitor))
	}

	if cfg.GetBool("onTop", false) {
		args = append(args, "--ontop")
	}

	if cfg.GetBool("fullscreen", false) {
		args = append(args, "--fs")
	}

	if videoOutput := cfg.Get("videoOutput"); videoOutput != nil {
		args = append(args, fmt.Sprintf("--vo=%v", videoOutput))
	}

	if audioOutput := cfg.Get("audioOutput"); audioOutput != nil {
		args = append(args, fmt.Sprintf("--ao=%v", audioOutput))
	}

	if audioDevice := cfg.Get("audioDevice"); audioDevice != nil {
		args = append(args, fmt.Sprintf("--audio-device=%v", audioDevice))
	}

	if customArgs := cfg.Get("args"); customArgs != nil {
		if argsSlice, ok := customArgs.([]interface{}); ok {
			for _, a := range argsSlice {
				if s, ok := a.(string); ok {
					args = append(args, s)
				}
			}
		}
	}

	subtitlesConfig := cfg.Slice("subtitles")
	subtitlesData := subtitlesConfig.Data()
	if subtitlesData != nil {
		kebabSubs := cases.Convert(subtitlesData, cases.Kebab)
		for key, value := range kebabSubs {
			if value == nil {
				continue
			}
			args = append(args, fmt.Sprintf("--sub-%s=%s", key, ValueToMpv(value)))
		}
	}

	return args
}

type Player struct {
	Config *config.Config
	MPV    *mpv.MPV
	Status *PlayerStatus
	log    *logger.Logger

	observedProperties []string
}

func NewPlayer(cfg *config.Config, mpvInst *mpv.MPV, log *logger.Logger) *Player {
	var statusLog *logger.Logger
	if log != nil {
		statusLog = log.Service("status")
	}

	p := &Player{
		Config: cfg,
		MPV:    mpvInst,
		Status: NewPlayerStatus(mpvInst, statusLog),
		log:    log,
	}

	return p
}

func (p *Player) ObserveProperty(property string) {
	p.observedProperties = append(p.observedProperties, property)

	if p.MPV.IsRunning() {
		_ = p.MPV.ObserveProperty(property)
	}
}

func (p *Player) Start() error {
	if p.log != nil {
		p.log.Info("starting player")
	}
	if err := p.MPV.Start(); err != nil {
		if p.log != nil {
			p.log.Errorf("player start failed: %v", err)
		}
		return err
	}

	for _, property := range p.observedProperties {
		if err := p.MPV.ObserveProperty(property); err != nil {
			if p.log != nil {
				p.log.Errorf("observe property %s failed: %v", property, err)
			}
			return fmt.Errorf("observe property %s: %w", property, err)
		}
	}

	if err := p.MPV.ObserveProperty("sub-scale"); err != nil {
		if p.log != nil {
			p.log.Errorf("observe sub-scale failed: %v", err)
		}
		return fmt.Errorf("observe sub-scale: %w", err)
	}

	if p.log != nil {
		p.log.Info("player started")
	}
	return nil
}

func (p *Player) Load(file string, flags LoadFlags, options map[string]interface{}, index *int) error {
	if p.log != nil {
		p.log.Infof("load: file=%s flags=%s", file, flags)
	}
	kebabOpts := cases.Convert(options, cases.Kebab)

	if nested, ok := kebabOpts["options"]; ok {
		if inner, ok := nested.(map[string]interface{}); ok {
			for k, v := range inner {
				kebabOpts[k] = v
			}
		}
		delete(kebabOpts, "options")
	}

	if _, ok := kebabOpts["sub-delay"]; !ok {
		kebabOpts["sub-delay"] = 0
	}

	var optionsStrings []string
	for key, value := range kebabOpts {
		optionsStrings = append(optionsStrings, fmt.Sprintf("%s=%s", key, ValueToMpv(value)))
	}

	if err := p.MPV.Load(file, string(flags), optionsStrings, index); err != nil {
		return err
	}

	return p.MPV.AdjustSubtitleTiming(0)
}

func (p *Player) Stop() error {
	status, err := p.Status.Get()
	if err != nil {
		return err
	}

	if status.Path != nil {
		return p.MPV.Stop()
	}

	return nil
}

func (p *Player) SetMultipleProperties(properties map[string]interface{}) error {
	kebabProps := cases.Convert(properties, cases.Kebab)
	return p.MPV.SetMultipleProperties(kebabProps)
}