package podkop

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEngine(t *testing.T) {
	saved := EngineRelease
	defer func() { EngineRelease = saved }()
	EngineRelease = "r15"
	require.Equal(t, "sing-box 1.14.3, podkop-engine r15, "+Build+" (1d47804dc783)", Engine("1.14.3-pdk-1d47804dc783"))
	EngineRelease = ""
	require.Equal(t, "sing-box 1.14.3, "+Build, Engine("1.14.3"))
}
