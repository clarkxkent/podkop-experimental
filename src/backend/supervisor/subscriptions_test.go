package supervisor

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/podkop-net/podkop-experimental/src/backend/subscription"

	"github.com/stretchr/testify/require"
)

// provider is a subscription of a test: the list it serves now, and the routes it was asked
// by.
type provider struct {
	access sync.Mutex
	body   string
	// refused are the routes that do not reach it
	refused []string
	asked   []string
	headers []http.Header
}

func (p *provider) serve(body string) {
	p.access.Lock()
	p.body = body
	p.access.Unlock()
}

func (p *provider) download(ctx context.Context, address string, header http.Header, routes []subscription.Route, each time.Duration) (*subscription.Answer, error) {
	p.access.Lock()
	defer p.access.Unlock()
	p.headers = append(p.headers, header)
	for _, route := range routes {
		p.asked = append(p.asked, route.Name)
		refused := false
		for _, name := range p.refused {
			refused = refused || name == route.Name
		}
		if !refused {
			return &subscription.Answer{Body: []byte(p.body), Header: http.Header{}, Route: route.Name}, nil
		}
	}
	return nil, os.ErrDeadlineExceeded
}

func subscriptionSettings(directory string, empty string) string {
	return "config settings 'settings'\n\toption dns_type 'udp'\n\toption dns_server '127.0.0.1:9'\n" +
		"\toption cache_path '" + filepath.Join(directory, "cache.db") + "'\n\toption yacd_secret_key 'k'\n" +
		"\nconfig section 'main'\n\toption connection_type 'proxy'\n\toption proxy_config_type 'url'\n\toption proxy_string 'socks5://203.0.113.15:1080'\n" +
		"\toption user_domain_list_type 'dynamic'\n\tlist user_domains 'main.lab.example'\n" +
		"\nconfig subscription 'prov1'\n\toption url 'https://sub.example/api/sub/TOKEN'\n\toption update_interval '1h'\n" +
		"\nconfig group 'nl'\n\tlist subscription 'prov1'\n\tlist include 'NL'\n\toption empty '" + empty + "'\n" +
		"\nconfig section 'youtube'\n\toption connection_type 'group'\n\toption group 'nl'\n" +
		"\toption user_domain_list_type 'dynamic'\n\tlist user_domains 'youtube.lab.example'\n"
}

// download runs a download of every subscription now, as Run would, and takes what it came to.
func downloadNow(t *testing.T, s *Supervisor) {
	t.Helper()
	s.downloadDue(time.Now(), "*")
	select {
	case result := <-s.downloads:
		s.takeList(result)
	case <-time.After(10 * time.Second):
		t.Fatal("no download came back")
	}
}

// The list of a subscription is taken, kept on flash and used at the next start; a list
// that only renamed its servers changes nothing; one with another server makes a new
// configuration.
func TestSubscriptionLists(t *testing.T) {
	directory := t.TempDir()
	fake, system := &fakeEngine{refuse: map[string]error{}}, new(recorder)
	source := &provider{body: "vless://u1@nl1.example:443?security=tls&sni=nl1.example#NL%201\nvless://u2@de1.example:443?security=tls&sni=de1.example#DE%201\n"}
	options := Options{
		SettingsPath: filepath.Join(directory, "podkop"), RuleSetDir: filepath.Join(directory, "rulesets"),
		StagingDir: filepath.Join(directory, "candidate"), StatePath: filepath.Join(directory, "state"),
		ConfigDir: filepath.Join(directory, "run"), ReadyTimeout: time.Second, System: system, Engine: fake,
		SubscriptionDir: filepath.Join(directory, "subscriptions"), Version: "0.8.5",
		Device: subscription.Device{HWID: "0123456789abcdef0123456789abcdef"}, Download: source.download,
	}
	require.NoError(t, os.WriteFile(options.SettingsPath, []byte(subscriptionSettings(directory, "block")), 0o644))
	s := New(options)
	require.True(t, s.Boot().OK)
	// no list yet: the group refuses
	require.NotContains(t, string(s.current.result.Config), "prov1: NL 1")
	require.Equal(t, 1, len(fake.built))

	downloadNow(t, s)
	require.Len(t, fake.built, 2, "the list makes a new configuration")
	config := string(s.current.result.Config)
	require.Contains(t, config, `"prov1: NL 1"`)
	require.NotContains(t, config, `"prov1: DE 1"`, "a server the group does not choose has no outbound")
	require.Equal(t, "0123456789abcdef0123456789abcdef", source.headers[0].Get("x-hwid"))
	require.Equal(t, "podkop/0.8.5", source.headers[0].Get("User-Agent"))
	states := s.subscriptionStates()
	require.Len(t, states, 1)
	require.Equal(t, 2, states[0].Servers)
	require.Equal(t, "direct", states[0].Route)
	file, err := os.Stat(filepath.Join(directory, "subscriptions", "prov1.json"))
	require.NoError(t, err)
	written := file.ModTime()

	// only the names changed: nothing is written, nothing restarts
	source.serve("vless://u2@de1.example:443?security=tls&sni=de1.example#DE%201%20(29%20days)\nvless://u1@nl1.example:443?security=tls&sni=nl1.example#NL%201%20(29%20days)\n")
	time.Sleep(10 * time.Millisecond)
	downloadNow(t, s)
	require.Len(t, fake.built, 2)
	file, err = os.Stat(filepath.Join(directory, "subscriptions", "prov1.json"))
	require.NoError(t, err)
	require.Equal(t, written, file.ModTime())
	require.Contains(t, string(s.current.result.Config), `"prov1: NL 1"`)

	// the same servers, the names swapped: what the group chooses changes, and the list is taken
	source.serve("vless://u1@nl1.example:443?security=tls&sni=nl1.example#DE%201\nvless://u2@de1.example:443?security=tls&sni=de1.example#NL%201\n")
	downloadNow(t, s)
	require.Len(t, fake.built, 3)
	require.Contains(t, string(s.current.result.Config), `"server": "de1.example"`)
	require.NotContains(t, string(s.current.result.Config), `"server": "nl1.example"`)

	// another server: a new configuration
	source.serve("vless://u1@nl1.example:443?security=tls&sni=nl1.example#NL%201\nvless://u3@nl2.example:443?security=tls&sni=nl2.example#NL%202\n")
	downloadNow(t, s)
	require.Len(t, fake.built, 4)
	require.Contains(t, string(s.current.result.Config), `"prov1: NL 2"`)
	s.Stop()

	// the next start has the list before any download
	again := New(options)
	require.True(t, again.Boot().OK)
	require.Contains(t, string(again.current.result.Config), `"prov1: NL 2"`)
	again.Stop()

	// a subscription the settings no longer have is forgotten, its file too
	require.NoError(t, os.WriteFile(options.SettingsPath, []byte(strings.Split(subscriptionSettings(directory, "block"), "\nconfig subscription")[0]), 0o644))
	third := New(options)
	require.True(t, third.Boot().OK)
	_, err = os.Stat(filepath.Join(directory, "subscriptions", "prov1.json"))
	require.True(t, os.IsNotExist(err))
	third.Stop()
}

// The provider is asked directly first, then through the servers of the configuration, the
// fastest first; a list that cannot be downloaded keeps the one there was.
func TestSubscriptionRoutes(t *testing.T) {
	directory := t.TempDir()
	fake, system := &fakeEngine{refuse: map[string]error{}, delays: map[string]uint16{"prov1: NL 2": 90, "prov1: NL 1": 300}}, new(recorder)
	source := &provider{body: "vless://u1@nl1.example:443?security=tls&sni=nl1.example#NL%201\nvless://u2@nl2.example:443?security=tls&sni=nl2.example#NL%202\n"}
	options := Options{
		SettingsPath: filepath.Join(directory, "podkop"), RuleSetDir: filepath.Join(directory, "rulesets"),
		StagingDir: filepath.Join(directory, "candidate"), StatePath: filepath.Join(directory, "state"),
		ConfigDir: filepath.Join(directory, "run"), ReadyTimeout: time.Second, System: system, Engine: fake,
		SubscriptionDir: filepath.Join(directory, "subscriptions"), Download: source.download,
	}
	require.NoError(t, os.WriteFile(options.SettingsPath, []byte(subscriptionSettings(directory, "all")), 0o644))
	s := New(options)
	require.True(t, s.Boot().OK)
	downloadNow(t, s)
	require.Equal(t, []string{"direct"}, source.asked)
	routes := s.routes(3)
	var names []string
	for _, route := range routes {
		names = append(names, route.Name)
	}
	require.Equal(t, []string{"direct", "prov1: NL 2", "prov1: NL 1", "main-out"}, names)
	// direct goes through direct-out of sing-box while it runs
	fake.dial = func(ctx context.Context, tag string, address string) (net.Conn, error) { return nil, os.ErrClosed }
	_, _ = routes[0].Dial(context.Background(), "tcp", "sub.example:443")
	require.Equal(t, []string{"direct-out"}, fake.dialed)

	// blocked directly: through the fastest server
	source.asked = nil
	source.refused = []string{"direct"}
	source.serve("vless://u1@nl1.example:443?security=tls&sni=nl1.example#NL%201\n")
	downloadNow(t, s)
	require.Equal(t, []string{"direct", "prov1: NL 2"}, source.asked)
	require.Equal(t, "prov1: NL 2", s.subscriptionStates()[0].Route)

	// nothing reaches it: the list there was stays, the error is told
	source.refused = []string{"direct", "prov1: NL 1", "prov1: NL 2", "main-out"}
	downloadNow(t, s)
	state := s.subscriptionStates()[0]
	require.NotEmpty(t, state.Error)
	require.Equal(t, 1, state.Servers)
	require.Contains(t, string(s.current.result.Config), `"prov1: NL 1"`)
	s.Stop()
}

// The page sees a list without taking it, and the servers a group of its form would choose.
func TestSubscriptionPreviews(t *testing.T) {
	directory := t.TempDir()
	fake, system := &fakeEngine{refuse: map[string]error{}, unusable: []string{"vmess://"}}, new(recorder)
	source := &provider{body: "vless://u1@nl1.example:443?security=tls&sni=nl1.example#NL%201\nvmess://eyJhZGQiOiJ4In0=#VM\nvless://u2@de1.example:443?security=tls&sni=de1.example#DE%201\n"}
	options := Options{
		SettingsPath: filepath.Join(directory, "podkop"), RuleSetDir: filepath.Join(directory, "rulesets"),
		StagingDir: filepath.Join(directory, "candidate"), StatePath: filepath.Join(directory, "state"),
		ConfigDir: filepath.Join(directory, "run"), ReadyTimeout: time.Second, System: system, Engine: fake,
		SubscriptionDir: filepath.Join(directory, "subscriptions"), Download: source.download,
	}
	require.NoError(t, os.WriteFile(options.SettingsPath, []byte(subscriptionSettings(directory, "all")), 0o644))
	s := New(options)
	require.True(t, s.Boot().OK)
	preview := s.LoadSubscription(context.Background(), "https://other.example/sub/X")
	require.Empty(t, preview.Error)
	require.Len(t, preview.Servers, 3)
	require.Equal(t, "NL 1", preview.Servers[0].Name)
	require.Equal(t, "vless", preview.Servers[0].Scheme)
	require.Equal(t, "nl1.example:443", preview.Servers[0].Address)
	require.NotEmpty(t, preview.Servers[1].Reason)
	require.Len(t, fake.built, 1, "a preview takes nothing")
	_, err := os.Stat(filepath.Join(directory, "subscriptions"))
	require.True(t, os.IsNotExist(err))

	// a subscription of the settings without a list is downloaded for the check, not taken
	check := s.CheckGroup(context.Background(), GroupCheck{Subscriptions: []string{"prov1", "prov1", "gone"}, Include: []string{"nl", "de"}, Exclude: []string{"de 1"}})
	require.Equal(t, []string{"NL 1"}, check.Chosen)
	require.Equal(t, 2, check.Total)
	require.Equal(t, []string{"gone: not in the saved settings"}, check.Missing)
	require.Len(t, fake.built, 1)
	s.Stop()
}

func TestParseInterval(t *testing.T) {
	for text, want := range map[string]time.Duration{"1h": time.Hour, "12h": 12 * time.Hour, "1d": 24 * time.Hour, "30m": 30 * time.Minute, "1m": 10 * time.Minute, "": 12 * time.Hour, "x": 12 * time.Hour, "-1h": 12 * time.Hour} {
		require.Equal(t, want, parseInterval(text), text)
	}
}
