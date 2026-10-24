package podkop

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEngine(t *testing.T) {
	saved := EngineRelease
	defer func() { EngineRelease = saved }()
	EngineRelease = "r16"
	require.Equal(t, "sing-box 1.14.3, podkop-engine r16, "+Build+" (5d99dc7986cb)", Engine("1.14.3-pdk-5d99dc7986cb"))
	EngineRelease = ""
	require.Equal(t, "sing-box 1.14.3, "+Build, Engine("1.14.3"))
}
