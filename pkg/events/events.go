package events

import (
	"github.com/unicast/unicast-mpv/pkg/mpv"
	"github.com/unicast/unicast-mpv/pkg/server"
)

var eventNames = []string{
	"started",
	"stopped",
	"paused",
	"resumed",
	"seek",
	"status",
	"quit",
	"crashed",
}

type Emitter interface {
	RegisterEvent(name string)
	Emit(event string, args ...interface{})
}

func Bridge(m *mpv.MPV, srv *server.Server) {
	BridgeWithServer(m, srv)
}

func BridgeWithServer(m *mpv.MPV, srv Emitter) {
	for _, name := range eventNames {
		srv.RegisterEvent(name)
	}

	m.OnEvent("started", func(args ...interface{}) {
		srv.Emit("started")
	})

	m.OnEvent("stopped", func(args ...interface{}) {
		srv.Emit("stopped")
	})

	m.OnEvent("paused", func(args ...interface{}) {
		srv.Emit("paused")
	})

	m.OnEvent("resumed", func(args ...interface{}) {
		srv.Emit("resumed")
	})

	m.OnEvent("seek", func(args ...interface{}) {
		srv.Emit("seek", args...)
	})

	m.OnEvent("status", func(args ...interface{}) {
		srv.Emit("status", args...)
	})

	m.OnEvent("quit", func(args ...interface{}) {
		srv.Emit("quit", args...)
	})

	m.OnEvent("crashed", func(args ...interface{}) {
		srv.Emit("crashed", args...)
	})
}