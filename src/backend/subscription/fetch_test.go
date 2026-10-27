package subscription

import (
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const token = "TOKEN5b1f0c"

// provider serves the answers of a test by path; every route of a test dials it, whatever
// name the request is for, as a resolver of the router or a proxy would reach the provider.
func provider(t *testing.T, tls bool, handler http.HandlerFunc) (address string, reach func(context.Context, string, string) (net.Conn, error)) {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	if tls {
		server.StartTLS()
		roots = server.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	} else {
		server.Start()
	}
	t.Cleanup(server.Close)
	listener := server.Listener.Addr().String()
	_, port, _ := net.SplitHostPort(listener)
	scheme := "http"
	if tls {
		// the name the test certificate is for
		scheme = "https"
	}
	address = scheme + "://example.com:" + port + "/sub/" + token
	reach = func(ctx context.Context, network string, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, listener)
	}
	return address, reach
}

func refused(context.Context, string, string) (net.Conn, error) {
	return nil, &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "no such host", Name: "example.com"}}
}

func TestDownloadTriesTheRoutesInTurn(t *testing.T) {
	var seen []string
	address, reach := provider(t, true, func(writer http.ResponseWriter, request *http.Request) {
		seen = append(seen, request.Header.Get("x-hwid"))
		writer.Header().Set("subscription-userinfo", "upload=1; download=2; total=0; expire=0")
		_, _ = writer.Write([]byte(base64.StdEncoding.EncodeToString([]byte("vless://u@a.example:443#A\n"))))
	})
	header := Device{HWID: "0123456789abcdef"}.Headers("0.8.5")
	routes := []Route{{Name: "direct", Direct: true, Dial: refused}, {Name: "main: NL", Dial: reach}}
	answer, err := Download(context.Background(), address, header, routes, 5*time.Second)
	require.NoError(t, err)
	require.Equal(t, "main: NL", answer.Route)
	require.Equal(t, []string{"0123456789abcdef"}, seen)
	require.Equal(t, "upload=1; download=2; total=0; expire=0", answer.Info()["subscription-userinfo"])
	servers, err := Parse(answer.Body, fakeDecode)
	require.NoError(t, err)
	require.Equal(t, "A", servers[0].Name)
}

func TestDownloadPlainHTTPGoesDirectOnly(t *testing.T) {
	address, reach := provider(t, false, func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("vless://u@a.example:443#A\n"))
	})
	_, err := Download(context.Background(), address, nil, []Route{{Name: "direct", Direct: true, Dial: refused}, {Name: "proxy", Dial: reach}}, 5*time.Second)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "proxy")
	answer, err := Download(context.Background(), address, nil, []Route{{Name: "direct", Direct: true, Dial: reach}}, 5*time.Second)
	require.NoError(t, err)
	require.Equal(t, "direct", answer.Route)
}

func TestDownloadPageGoesOn(t *testing.T) {
	address, reach := provider(t, true, func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("vless://u@a.example:443#A\n"))
	})
	blockPage := func(ctx context.Context, network string, _ string) (net.Conn, error) {
		// a block page served for the provider: the test server of the page
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "text/html")
			_, _ = writer.Write([]byte("<html>blocked</html>"))
		}))
		t.Cleanup(server.Close)
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	answer, err := Download(context.Background(), address, nil, []Route{{Name: "direct", Direct: true, Dial: blockPage}, {Name: "px", Dial: reach}}, 5*time.Second)
	require.NoError(t, err)
	require.Equal(t, "px", answer.Route)
}

func TestDownloadFinalAnswers(t *testing.T) {
	for name, test := range map[string]struct {
		handler http.HandlerFunc
		want    string
	}{
		"no content": {func(writer http.ResponseWriter, request *http.Request) { writer.WriteHeader(http.StatusNoContent) }, "sent nothing"},
		"forbidden":  {func(writer http.ResponseWriter, request *http.Request) { writer.WriteHeader(http.StatusForbidden) }, "403"},
		"placeholder": {func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("x-hwid-not-supported", "true")
			writer.Header().Set("x-hwid-limit", "true")
			writer.Header().Set("announce", "base64:"+base64.StdEncoding.EncodeToString([]byte("App not supported")))
			_, _ = writer.Write([]byte("vless://placeholder@0.0.0.0:1?security=none#App%20not%20supported\n"))
		}, "did not take the identifier of this device (HWID): App not supported"},
		"limit": {func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("x-hwid-max-devices-reached", "true")
			_, _ = writer.Write([]byte("vless://placeholder@0.0.0.0:1#Limit\n"))
		}, "limit of devices"},
	} {
		tries := 0
		address, reach := provider(t, true, func(writer http.ResponseWriter, request *http.Request) {
			tries++
			test.handler(writer, request)
		})
		_, err := Download(context.Background(), address, nil, []Route{{Name: "direct", Direct: true, Dial: reach}, {Name: "px", Dial: reach}}, 5*time.Second)
		require.ErrorContains(t, err, test.want, name)
		require.Equal(t, 1, tries, name)
	}
}

// A failure names neither the address nor the host: the token is the account.
func TestDownloadMessagesKeepTheAddress(t *testing.T) {
	address := "https://sub.example.com/api/sub/" + token
	slow := func(ctx context.Context, _ string, _ string) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	unknown := func(context.Context, string, string) (net.Conn, error) {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "no such host", Name: "sub.example.com"}}
	}
	_, err := Download(context.Background(), address, nil, []Route{{Name: "direct", Direct: true, Dial: unknown}, {Name: "px", Dial: slow}}, 200*time.Millisecond)
	require.Error(t, err)
	for _, secret := range []string{token, "sub.example.com"} {
		require.NotContains(t, err.Error(), secret)
	}
	require.True(t, strings.Contains(err.Error(), "direct: ") && strings.Contains(err.Error(), "px: "), err.Error())
	_, err = Download(context.Background(), "ftp://example.com/"+token, nil, nil, time.Second)
	require.ErrorContains(t, err, "not an address of HTTP")
	require.NotContains(t, err.Error(), token)
}
