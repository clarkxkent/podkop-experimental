//go:build linux

package main

import (
	"context"
	"runtime"
	"time"

	"github.com/podkop-net/podkop-experimental/src/backend/ctl"
	"github.com/podkop-net/podkop-experimental/src/backend/engine"
	"github.com/podkop-net/podkop-experimental/src/backend/ubus"
)

// Stats is what the dashboard of the page shows of the backend, asked every few seconds
// while it is open, where it used to read two websockets of the Clash API (one of them a
// whole list of the connections every second): the bytes up and down since the sing-box
// that runs started (the page makes rates of two answers and their At), the connections
// open now, the memory of the process as the Clash API tells it.
type Stats struct {
	Running bool `json:"running"`
	// At is the time of the answer in milliseconds of a clock of the process
	At          int64  `json:"at"`
	Up          int64  `json:"up"`
	Down        int64  `json:"down"`
	Connections int    `json:"connections"`
	Memory      uint64 `json:"memory"`
}

type trafficSource interface {
	Traffic() (engine.Traffic, bool)
}

var processStart = time.Now()

// busObject is the object "podkop" of the backend on ubus: the commands of the page
// (ctl.BusObject) and the method stats.
func busObject(env ctl.Environment, source trafficSource) ubus.Object {
	object := ctl.BusObject(env)
	object.Methods["stats"] = ubus.Method{
		Call: func(ctx context.Context, arguments map[string]any) (any, int) {
			traffic, running := source.Traffic()
			var memory runtime.MemStats
			runtime.ReadMemStats(&memory)
			return Stats{
				Running:     running,
				At:          time.Since(processStart).Milliseconds(),
				Up:          traffic.Up,
				Down:        traffic.Down,
				Connections: traffic.Connections,
				Memory:      memory.StackInuse + memory.HeapInuse + memory.HeapIdle - memory.HeapReleased,
			}, ubus.StatusOK
		},
	}
	return object
}
