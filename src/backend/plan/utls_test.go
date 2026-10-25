package plan

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// The uTLS fingerprint fallback of the engine is on without the option and with 'enabled',
// off with 'disabled'; the configuration says it either way, so the engine does not read
// its environment. The flag of earlier versions is not read.
func TestUTLSFallback(t *testing.T) {
	const head = "config settings 'settings'\n\toption dns_type 'udp'\n\toption dns_server '8.8.8.8'\n"
	const proxy = "\nconfig section 'main'\n\toption connection_type 'proxy'\n\toption proxy_config_type 'url'\n\toption proxy_string 'socks5://203.0.113.15:1080'\n"
	for option, enabled := range map[string]bool{
		"":                                     true,
		"\toption utls_fallback 'enabled'\n":   true,
		"\toption utls_fallback 'disabled'\n":  false,
		"\toption utls_fallback '0'\n":         false,
		"\toption enable_utls_fallback '0'\n":  true,
		"\toption enable_utls_fallback '1'\n":  true,
		"\toption utls_fallback 'something'\n": true,
	} {
		result := buildText(t, head+option+proxy, Facts{})
		var document struct {
			Experimental struct {
				UTLSFallback map[string]any `json:"utls_fallback"`
			} `json:"experimental"`
		}
		require.NoError(t, json.Unmarshal(result.Config, &document), option)
		require.Equal(t, map[string]any{"enabled": enabled}, document.Experimental.UTLSFallback, option)
	}
}
