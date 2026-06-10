package commands

import (
	"github.com/unicast/unicast-mpv/pkg/logger"
	"github.com/unicast/unicast-mpv/pkg/player"
	"github.com/unicast/unicast-mpv/pkg/schema"
)

type StatusCommand struct {
	*CommandRegistry
	status *player.PlayerStatus
	log    *logger.Logger
}

func NewStatusCommand(registry *CommandRegistry, status *player.PlayerStatus, log *logger.Logger) *StatusCommand {
	sc := &StatusCommand{
		CommandRegistry: registry,
		status:           status,
		log:              log,
	}

	sc.Register("status", schema.Tuple(), sc.statusHandler)

	sc.server.RegisterPostHook("quit", func(args []interface{}, method string, err error, result interface{}, ctx map[string]interface{}) {
		if sc.log != nil {
			sc.log.Debug("status: post-hook 'quit' → status.Stop()")
		}
		sc.status.Stop()
	})

	sc.server.RegisterPostHook("stop", func(args []interface{}, method string, err error, result interface{}, ctx map[string]interface{}) {
		if sc.log != nil {
			sc.log.Debug("status: post-hook 'stop' → status.Stop()")
		}
		sc.status.Stop()
	})

	sc.server.RegisterPreHook("play", func(args []interface{}, method string, ctx map[string]interface{}) {
		if sc.log != nil {
			sc.log.Debug("status: pre-hook 'play' → status.Play()")
		}
		sc.status.Play()
	})

	sc.mpv.OnEvent("status", func(args ...interface{}) {
		if len(args) > 0 {
			if m, ok := args[0].(map[string]interface{}); ok {
				prop, _ := m["property"].(string)
				value := m["value"]
				sc.status.Update(prop, value)
			}
		}
	})

	sc.mpv.OnEvent("timeposition", func(args ...interface{}) {
		if len(args) > 0 {
			sc.status.Update("position", args[0])
		}
	})

	return sc
}

func (sc *StatusCommand) statusHandler(args []interface{}) (interface{}, error) {
	status, err := sc.status.Get()
	if sc.log != nil {
		if err != nil {
			sc.log.Warnf("status: RPC status returned error: %v", err)
		} else {
			sc.log.Debugf("status: RPC status returned path=%v pause=%v", status.Path, status.Pause)
		}
	}
	return status, err
}