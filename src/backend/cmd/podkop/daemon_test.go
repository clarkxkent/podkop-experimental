//go:build linux

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The name of the process that started the test is read from the kernel: the daemon refuses
// to run when it is rpcd.
func TestParentName(t *testing.T) {
	require.NotEmpty(t, parentName())
	require.NotEqual(t, "rpcd", parentName())
}
