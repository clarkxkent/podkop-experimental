package firewall

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The trace table as nft reads it back: the rule of the connection of a check and the one
// of its DNS questions; another check replaces it, and it is gone after.
func TestArmTrace(t *testing.T) {
	needNft(t)
	require.NoError(t, ArmTrace(40001, "6e1d0c3b2a19f8e7d6c5"))
	defer DisarmTraces()
	listed := nft(t, "list", "table", "inet", "PodkopTrace40001")
	require.Contains(t, listed, "hook prerouting priority -400")
	require.Regexp(t, `ip daddr 198\.18\.0\.0/15 .*tcp dport 40001 meta nftrace set 1`, listed)
	// the length of the label and its first 15 letters
	require.Regexp(t, `udp dport 53 @th,160,128 0x14366531643063336232613139663865 meta nftrace set 1`, listed)
	// a check of its own port, at the same time: a table of its own
	require.NoError(t, ArmTrace(40002, "aaaaaaaa"))
	listed = nft(t, "list", "table", "inet", "PodkopTrace40002")
	require.Equal(t, 2, strings.Count(listed, "meta nftrace set 1"), listed)
	require.Contains(t, listed, "tcp dport 40002")
	require.Contains(t, listed, "@th,160,72 0x86161616161616161")
	require.Contains(t, nft(t, "list", "tables"), "PodkopTrace40001")
	// the same port again replaces the table
	require.NoError(t, ArmTrace(40002, "bbbbbbbb"))
	require.NotContains(t, nft(t, "list", "table", "inet", "PodkopTrace40002"), "0x86161616161616161")
	require.NoError(t, DisarmTrace("PodkopTrace40002"))
	require.NoError(t, DisarmTrace("PodkopTrace40002"))
	require.Contains(t, nft(t, "list", "tables"), "PodkopTrace40001")
	require.NoError(t, DisarmTraces())
	require.NotContains(t, nft(t, "list", "tables"), TraceTable)
}
