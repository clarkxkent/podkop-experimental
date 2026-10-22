//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
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

// check_links with the engine of this build: a link it takes, one whose transport it does not
// have, and the answer of the build for VMess (the slim one has no VMess outbound)
func TestCheckLinksWithTheEngine(t *testing.T) {
	var output, errorOutput bytes.Buffer
	env := ctl.Environment{CheckLink: engine.CheckLink}
	status := ctl.Run(env, []string{"check_links",
		"socks5://192.0.2.1:1080",
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@192.0.2.1:443?type=kcp",
		"vmess://b831381d-6324-4d53-ad4f-8cda48b30811@192.0.2.1:443?type=ws",
	}, &output, &errorOutput)
	require.Zero(t, status)
	var results []ctl.LinkCheck
	require.NoError(t, json.Unmarshal(output.Bytes(), &results))
	require.Len(t, results, 3)
	require.True(t, results[0].OK)
	require.False(t, results[1].OK)
	require.Contains(t, results[1].Error, "mKCP")
	// the slim build says what it has not, without the advice of the engine's own packages
	require.False(t, results[2].OK)
	require.Contains(t, results[2].Error, "not included in this build")
	require.NotContains(t, results[2].Error, "podkop-engine-full")
}
