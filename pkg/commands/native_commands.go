package commands

import (
	"fmt"

	"github.com/unicast/unicast-mpv/pkg/player"
	"github.com/unicast/unicast-mpv/pkg/schema"
)

type NativeCommands struct {
	*CommandRegistry
}

func NewNativeCommands(registry *CommandRegistry) *NativeCommands {
	nc := &NativeCommands{CommandRegistry: registry}

	nc.Register("load", schema.Tuple(schema.String(), schema.Optional(schema.String())), nc.loadHandler)
	nc.RegisterNative("stop", schema.Tuple())
	nc.RegisterNative("pause", schema.Tuple())
	nc.RegisterNative("resume", schema.Tuple())
	nc.RegisterNative("seek", schema.Tuple(schema.Number()))
	nc.RegisterNative("goToPosition", schema.Tuple(schema.Number()))
	nc.RegisterNative("mute", schema.Tuple(schema.Boolean()))

	nc.RegisterNative("volume", schema.Tuple(schema.Number()))

	nc.RegisterNative("setProperty", schema.Tuple(schema.String(), schema.Any()))
	nc.RegisterNative("setMultipleProperties", schema.Tuple(schema.Object(map[string]schema.Schema{})))
	nc.RegisterNative("getProperty", schema.Tuple(schema.String()))
	nc.RegisterNative("addProperty", schema.Tuple(schema.String(), schema.Number()))
	nc.RegisterNative("multiplyProperty", schema.Tuple(schema.String(), schema.Number()))
	nc.RegisterNative("cycleProperty", schema.Tuple(schema.String()))

	nc.RegisterNative("subtitleScale", schema.Tuple(schema.Number()))
	nc.RegisterNative("adjustSubtitleTiming", schema.Tuple(schema.Number()))
	nc.RegisterNative("hideSubtitles", schema.Tuple())
	nc.RegisterNative("showSubtitles", schema.Tuple())

	nc.Register("showProgress", schema.Tuple(), func(args []interface{}) (interface{}, error) {
		return nil, nc.mpv.Command("show-progress", nil)
	})

	return nc
}

func (nc *NativeCommands) loadHandler(args []interface{}) (interface{}, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("load: file argument required")
	}

	file, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("load: file must be a string")
	}

	flags := player.LoadFlagsReplace
	if len(args) >= 2 {
		if f, ok := args[1].(string); ok && f != "" {
			flags = player.LoadFlags(f)
		}
	}

	options := make(map[string]interface{})

	return nil, nc.player.Load(file, flags, options, nil)
}