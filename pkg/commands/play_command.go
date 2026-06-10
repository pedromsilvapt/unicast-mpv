package commands

import (
	"fmt"

	"github.com/unicast/unicast-mpv/pkg/config"
	"github.com/unicast/unicast-mpv/pkg/logger"
	"github.com/unicast/unicast-mpv/pkg/player"
	"github.com/unicast/unicast-mpv/pkg/schema"
)

type PlayCommand struct {
	*CommandRegistry
	config *config.Config
	log    *logger.Logger
}

func NewPlayCommand(registry *CommandRegistry, cfg *config.Config, log *logger.Logger) *PlayCommand {
	pc := &PlayCommand{
		CommandRegistry: registry,
		config:           cfg,
		log:               log,
	}

	pc.Register("play", schema.Tuple(schema.String(), schema.Optional(schema.String()), schema.Any()), pc.play)

	return pc
}

func (pc *PlayCommand) play(args []interface{}) (interface{}, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("play: file argument required")
	}

	file, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("play: file must be a string")
	}

	var subtitles string
	if len(args) >= 2 {
		if s, ok := args[1].(string); ok {
			subtitles = s
		}
	}

	var options map[string]interface{}
	if len(args) >= 3 {
		if m, ok := args[2].(map[string]interface{}); ok {
			options = m
		}
	}

	if options == nil {
		options = make(map[string]interface{})
	}

	if pc.log != nil {
		pc.log.Infof("play: file=%s subtitles=%s", file, subtitles)
	}

	if pc.config.GetBool("restartOnPlay", false) {
		if pc.mpv.IsRunning() {
			if pc.log != nil {
				pc.log.Debug("play: restarting mpv (restartOnPlay=true)")
			}
			_ = pc.mpv.Quit()
		}
	}

	if !pc.mpv.IsRunning() {
		if pc.log != nil {
			pc.log.Debug("play: mpv not running, starting player")
		}
		if err := pc.player.Start(); err != nil {
			if pc.log != nil {
				pc.log.Errorf("play: failed to start player: %v", err)
			}
			return nil, fmt.Errorf("play: failed to start player: %w", err)
		}
	} else if err := pc.mpv.ReconnectIfNeeded(); err != nil {
		if pc.log != nil {
			pc.log.Infof("play: mpv ipc disconnected, restarting player")
		}
		_ = pc.mpv.Quit()
		if err := pc.player.Start(); err != nil {
			if pc.log != nil {
				pc.log.Errorf("play: failed to start player: %v", err)
			}
			return nil, fmt.Errorf("play: failed to start player: %w", err)
		}
	}

	if pc.log != nil {
		pc.log.Debugf("play: loading file=%s", file)
	}
	if err := pc.player.Load(file, player.LoadFlagsReplace, options, nil); err != nil {
		if pc.log != nil {
			pc.log.Errorf("play: failed to load file: %v", err)
		}
		return nil, fmt.Errorf("play: failed to load file: %w", err)
	}

	if subtitles != "" {
		if pc.log != nil {
			pc.log.Debugf("play: adding subtitles=%s", subtitles)
		}
		if err := pc.mpv.AddSubtitles(subtitles, "", "", ""); err != nil {
			if pc.log != nil {
				pc.log.Errorf("play: failed to add subtitles: %v", err)
			}
			return nil, fmt.Errorf("play: failed to add subtitles: %w", err)
		}
	}

	status, err := pc.player.Status.Get()
	if err != nil {
		return nil, fmt.Errorf("play: status wait failed: %w", err)
	}
	return status, nil
}
