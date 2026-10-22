//go:build linux

package main

import (
	"context"
	"testing"

	"github.com/podkop-net/podkop-experimental/src/backend/ctl"
	"github.com/podkop-net/podkop-experimental/src/backend/engine"
	"github.com/podkop-net/podkop-experimental/src/backend/ubus"
	"github.com/stretchr/testify/require"
)

type fixedTraffic struct {
	traffic engine.Traffic
	running bool
}

func (f fixedTraffic) Traffic() (engine.Traffic, bool) { return f.traffic, f.running }

func TestBusStats(t *testing.T) {
	object := busObject(ctl.Environment{}, fixedTraffic{engine.Traffic{Up: 10, Down: 20, Connections: 3}, true})
	require.Contains(t, object.Methods, "run")
	answer, status := object.Methods["stats"].Call(context.Background(), nil)
	require.Equal(t, ubus.StatusOK, status)
	stats := answer.(Stats)
	require.True(t, stats.Running)
	require.Equal(t, int64(10), stats.Up)
	require.Equal(t, int64(20), stats.Down)
	require.Equal(t, 3, stats.Connections)
	require.NotZero(t, stats.Memory)
	later, _ := object.Methods["stats"].Call(context.Background(), nil)
	require.GreaterOrEqual(t, later.(Stats).At, stats.At)

	answer, _ = busObject(ctl.Environment{}, fixedTraffic{}).Methods["stats"].Call(context.Background(), nil)
	require.False(t, answer.(Stats).Running)
}
