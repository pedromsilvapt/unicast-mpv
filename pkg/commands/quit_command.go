package commands

import (
	"github.com/unicast/unicast-mpv/pkg/schema"
)

type QuitCommand struct {
	*CommandRegistry
}

func NewQuitCommand(registry *CommandRegistry) *QuitCommand {
	qc := &QuitCommand{CommandRegistry: registry}

	qc.Register("quit", schema.Tuple(), qc.quit)

	return qc
}

func (qc *QuitCommand) quit(args []interface{}) (interface{}, error) {
	if qc.mpv.IsRunning() {
		if err := qc.mpv.Quit(); err != nil {
			return nil, err
		}
	}
	return nil, nil
}