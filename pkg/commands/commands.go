package commands

import (
	"fmt"
	"reflect"

	"github.com/unicast/unicast-mpv/pkg/mpv"
	"github.com/unicast/unicast-mpv/pkg/player"
	"github.com/unicast/unicast-mpv/pkg/schema"
	"github.com/unicast/unicast-mpv/pkg/server"
)

var rpcToGoName = map[string]string{
	"load":                "Load",
	"stop":                "Stop",
	"pause":               "Pause",
	"resume":              "Resume",
	"seek":                "Seek",
	"goToPosition":        "GoToPosition",
	"mute":                "Mute",
	"volume":              "Volume",
	"setProperty":         "SetProperty",
	"setMultipleProperties": "SetMultipleProperties",
	"getProperty":         "GetProperty",
	"addProperty":         "AddProperty",
	"multiplyProperty":    "MultiplyProperty",
	"cycleProperty":       "CycleProperty",
	"subtitleScale":       "SubtitleScale",
	"adjustSubtitleTiming": "AdjustSubtitleTiming",
	"hideSubtitles":       "HideSubtitles",
	"showSubtitles":       "ShowSubtitles",
}

type CommandRegistry struct {
	server *server.Server
	player *player.Player
	mpv    *mpv.MPV
}

func NewCommandRegistry(srv *server.Server, p *player.Player, m *mpv.MPV) *CommandRegistry {
	return &CommandRegistry{
		server: srv,
		player: p,
		mpv:    m,
	}
}

func (c *CommandRegistry) RegisterNative(rpcName string, sch schema.Schema) {
	goName := rpcToGoName[rpcName]
	if goName == "" {
		goName = rpcName
	}
	handler := c.lookupNativeMethod(goName)
	c.Register(rpcName, sch, handler)
}

func (c *CommandRegistry) Register(name string, sch schema.Schema, handler func(args []interface{}) (interface{}, error)) {
	c.server.Register(name, sch, handler)
}

func (c *CommandRegistry) lookupNativeMethod(name string) func(args []interface{}) (interface{}, error) {
	pVal := reflect.ValueOf(c.player)
	method := pVal.MethodByName(name)
	if method.IsValid() {
		return c.methodToHandler(method)
	}

	mVal := reflect.ValueOf(c.mpv)
	method = mVal.MethodByName(name)
	if method.IsValid() {
		return c.methodToHandler(method)
	}

	return func(args []interface{}) (interface{}, error) {
		return nil, fmt.Errorf("commands: no method %s found on player or mpv", name)
	}
}

func (c *CommandRegistry) methodToHandler(method reflect.Value) func(args []interface{}) (interface{}, error) {
	mType := method.Type()
	numIn := mType.NumIn()

	return func(args []interface{}) (interface{}, error) {
		callArgs := make([]reflect.Value, numIn)
		for i := 0; i < numIn; i++ {
			paramType := mType.In(i)
			if i < len(args) {
				converted, err := convertArg(args[i], paramType)
				if err != nil {
					return nil, fmt.Errorf("commands: parameter %d: %w", i, err)
				}
				callArgs[i] = converted
			} else {
				callArgs[i] = reflect.Zero(paramType)
			}
		}

		results := method.Call(callArgs)

		switch len(results) {
		case 0:
			return nil, nil
		case 1:
			if err, ok := results[0].Interface().(error); ok {
				return nil, err
			}
			return results[0].Interface(), nil
		case 2:
			var result interface{}
			if !results[0].IsNil() {
				result = results[0].Interface()
			}
			if !results[1].IsNil() {
				return result, results[1].Interface().(error)
			}
			return result, nil
		default:
			return nil, fmt.Errorf("commands: unexpected number of return values: %d", len(results))
		}
	}
}

func convertArg(val interface{}, targetType reflect.Type) (reflect.Value, error) {
	if val == nil {
		return reflect.Zero(targetType), nil
	}

	srcVal := reflect.ValueOf(val)
	srcType := srcVal.Type()

	if srcType.AssignableTo(targetType) {
		return srcVal, nil
	}

	if srcType.ConvertibleTo(targetType) {
		return srcVal.Convert(targetType), nil
	}

	if targetType.Kind() == reflect.Ptr {
		elemType := targetType.Elem()
		converted, err := convertArg(val, elemType)
		if err != nil {
			return reflect.Value{}, err
		}
		ptr := reflect.New(elemType)
		ptr.Elem().Set(converted)
		return ptr, nil
	}

	if targetType.Kind() == reflect.Interface {
		return srcVal, nil
	}

	return reflect.Value{}, fmt.Errorf("cannot convert %v to %v", srcType, targetType)
}