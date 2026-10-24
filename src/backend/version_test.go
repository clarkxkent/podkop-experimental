package podkop

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEngine(t *testing.T) {
	saved := EngineRelease
	defer func() { EngineRelease = saved }()
	EngineRelease = "r16"
	require.Equal(t, "sing-box 1.14.3, podkop-engine r16, "+Build+" (ae09bd696dff)", Engine("1.14.3-pdk-ae09bd696dff"))
	EngineRelease = ""
	require.Equal(t, "sing-box 1.14.3, "+Build, Engine("1.14.3"))
}
