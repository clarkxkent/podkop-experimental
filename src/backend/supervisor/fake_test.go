package supervisor

import (
	"context"
	"crypto/sha256"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/podkop-net/podkop-experimental/src/backend/engine"
	"github.com/podkop-net/podkop-experimental/src/backend/plan"

	"github.com/stretchr/testify/require"
)

// fakeEngine is an Engine without sing-box: what it builds is told by the test, and it
// remembers what it was asked. The tests with it bind no port and touch no network.
type fakeEngine struct {
	// refuse is the error of Check for a configuration that has it in its text
	refuse map[string]error
	// broken are the texts of configurations whose build fails, silent those whose
	// instance never answers, stuck those whose instance does not close
	broken, silent, stuck []string
	built                 []*fakeInstance
	// checked counts the checks of whole configurations
	checked int
}

type fakeInstance struct {
	config          []byte
	started, closed bool
	silent, stuck   bool
}

func has(config []byte, texts []string) bool {
	for _, text := range texts {
		if strings.Contains(string(config), text) {
			return true
		}
	}
	return false
}

func (e *fakeEngine) Check(config []byte, links []plan.Link) error {
	e.checked++
	for text, err := range e.refuse {
		if strings.Contains(string(config), text) {
			return err
		}
	}
	return nil
}

func (e *fakeEngine) Build(config []byte, sink engine.LogSink, hooks ...engine.ConnectionHook) (Instance, error) {
	if has(config, e.broken) {
		return nil, os.ErrInvalid
	}
	instance := &fakeInstance{config: config, silent: has(config, e.silent), stuck: has(config, e.stuck)}
	e.built = append(e.built, instance)
	return instance, nil
}

func (e *fakeEngine) Ready(ctx context.Context) error {
	last := e.built[len(e.built)-1]
	if last.silent {
		return os.ErrDeadlineExceeded
	}
	return nil
}

func (e *fakeEngine) Notes() []string { return nil }

func (i *fakeInstance) OnRuleSetUpdate(tags []string, updated func(tag string)) {}
func (i *fakeInstance) Start() error                                            { i.started = true; return nil }
func (i *fakeInstance) Close(time.Duration) error {
	if i.stuck {
		return engine.ErrStuck
	}
	i.closed = true
	return nil
}
func (i *fakeInstance) RuleSetContent(string) ([]byte, bool)    { return nil, false }
func (i *fakeInstance) RuleSetUpdated(string) (time.Time, bool) { return time.Time{}, false }
func (i *fakeInstance) FakeIPAddress(string) (netip.Addr, bool, error) {
	return netip.Addr{}, false, nil
}
func (i *fakeInstance) UpdateRuleSets(context.Context) (engine.RuleSetUpdate, error) {
	return engine.RuleSetUpdate{}, nil
}
func (i *fakeInstance) Traffic() (engine.Traffic, error) {
	if !i.started || i.closed {
		return engine.Traffic{}, engine.ErrClosed
	}
	return engine.Traffic{Up: int64(len(i.config)), Down: 2, Connections: 3}, nil
}

// fakeFixture is a supervisor of a fake engine and a recorder: no root is needed.
func fakeFixture(t *testing.T) (*Supervisor, *fakeEngine, *recorder, func(name string, ttl string)) {
	directory := t.TempDir()
	fake, system := &fakeEngine{refuse: map[string]error{}}, new(recorder)
	s := New(Options{
		SettingsPath: filepath.Join(directory, "podkop"), RuleSetDir: filepath.Join(directory, "rulesets"),
		StagingDir: filepath.Join(directory, "candidate"), StatePath: filepath.Join(directory, "state"),
		ConfigDir: filepath.Join(directory, "run"), ReadyTimeout: time.Second, System: system, Engine: fake,
	})
	settings := func(name string, ttl string) {
		text := "config settings 'settings'\n\toption dns_type 'udp'\n\toption dns_server '127.0.0.1:9'\n\toption dns_rewrite_ttl '" + ttl + "'\n" +
			"\toption cache_path '" + filepath.Join(directory, "cache.db") + "'\n" +
			"\toption yacd_secret_key 'k'\n" +
			"\nconfig section 'main'\n\toption connection_type 'proxy'\n\toption proxy_config_type 'url'\n\toption proxy_string 'socks5://203.0.113.15:1080'\n" +
			"\toption user_domain_list_type 'dynamic'\n\tlist user_domains '" + name + "'\n"
		require.NoError(t, os.WriteFile(filepath.Join(directory, "podkop"), []byte(text), 0o644))
	}
	return s, fake, system, settings
}

func state(t *testing.T, s *Supervisor) string {
	content, err := os.ReadFile(s.options.StatePath)
	require.NoError(t, err)
	return strings.Fields(string(content))[0]
}

// Settings sing-box refuses at boot: the router keeps its own DNS, nothing is taken.
func TestFakeBootRefused(t *testing.T) {
	s, fake, system, settings := fakeFixture(t)
	settings("one.lab.example", "60")
	// the names of a list are in a rule-set file: a configuration is told by its TTL
	fake.refuse[`"rewrite_ttl": 60`] = os.ErrInvalid
	result := s.Boot()
	require.False(t, result.OK)
	require.False(t, s.Running())
	require.Empty(t, fake.built)
	require.Equal(t, []string{"no dns", "no firewall", "no routing"}, system.take())
	require.Equal(t, stateReleased, state(t, s))
}

// A boot whose sing-box does not answer is closed, and the router given back.
func TestFakeBootSilent(t *testing.T) {
	s, fake, system, settings := fakeFixture(t)
	settings("one.lab.example", "60")
	fake.silent = []string{`"rewrite_ttl": 60`}
	result := s.Boot()
	require.False(t, result.OK)
	require.False(t, s.Running())
	require.Len(t, fake.built, 1)
	require.True(t, fake.built[0].closed)
	require.Equal(t, []string{"no dns", "no firewall", "no routing"}, system.take())
}

// The routing and the firewall come before the DNS of the LAN; the same configuration
// again leaves sing-box alone; another one replaces it; one that does not come up gives
// way to the previous one, started again.
func TestFakeApply(t *testing.T) {
	s, fake, system, settings := fakeFixture(t)
	settings("one.lab.example", "60")
	require.True(t, s.Boot().OK)
	require.Equal(t, []string{"routing", "firewall", "dns"}, system.take())
	first := s.current.instance
	traffic, running := s.Traffic()
	require.True(t, running)
	require.Equal(t, engine.Traffic{Up: int64(len(fake.built[0].config)), Down: 2, Connections: 3}, traffic)

	result := s.Apply()
	require.True(t, result.OK, result.Message)
	require.Same(t, first, s.current.instance)
	require.Len(t, fake.built, 1)

	settings("one.lab.example", "61")
	result = s.Apply()
	require.True(t, result.OK, result.Message)
	require.Len(t, fake.built, 2)
	require.True(t, fake.built[0].closed)
	require.True(t, fake.built[1].started)

	settings("two.lab.example", "62")
	fake.silent = []string{`"rewrite_ttl": 62`}
	result = s.Apply()
	require.False(t, result.OK)
	require.Contains(t, result.Message, "the previous one runs again")
	require.Len(t, fake.built, 4)
	require.True(t, fake.built[2].closed, "the one that did not come up is closed")
	require.Equal(t, string(fake.built[1].config), string(fake.built[3].config), "the previous configuration is started again")
	require.Same(t, fake.built[3], s.current.instance)
	require.Equal(t, stateRunning, state(t, s))
	// what goes through the instance that runs now, not one closed before
	traffic, running = s.Traffic()
	require.True(t, running)
	require.Equal(t, int64(len(fake.built[3].config)), traffic.Up)
	s.Stop()
	_, running = s.Traffic()
	require.False(t, running)
}

// An instance that does not close leaves the process nothing to go on with: the router is
// given back, and Run would end with engine.ErrStuck.
func TestFakeStuck(t *testing.T) {
	s, fake, system, settings := fakeFixture(t)
	settings("one.lab.example", "60")
	fake.stuck = []string{`"rewrite_ttl": 60`}
	require.True(t, s.Boot().OK)
	system.take()
	settings("one.lab.example", "61")
	result := s.Apply()
	require.False(t, result.OK)
	require.False(t, s.Running())
	require.Equal(t, []string{"no dns", "no firewall", "no routing"}, system.take())
	require.ErrorIs(t, s.stuckError(), engine.ErrStuck)
}

// An apply of the settings and the local lists the running configuration was checked
// with does not build that configuration again; a local list that changed, or another
// configuration, is checked. A list is read once an apply.
func TestFakeCheckOnce(t *testing.T) {
	s, fake, _, settings := fakeFixture(t)
	settings("one.lab.example", "60")
	local := filepath.Join(t.TempDir(), "local.lst")
	require.NoError(t, os.WriteFile(local, []byte("local-one.lab.example\n"), 0o644))
	withLocal := func() {
		content, err := os.ReadFile(s.options.SettingsPath)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(s.options.SettingsPath, append(content, "\tlist local_domain_lists '"+local+"'\n"...), 0o644))
	}
	withLocal()
	require.True(t, s.Boot().OK)
	require.Equal(t, 1, fake.checked)
	require.Len(t, s.listChecks, 1)

	require.True(t, s.Apply().OK)
	require.Equal(t, 1, fake.checked, "the same settings and lists: no check")
	require.Len(t, fake.built, 1)

	require.NoError(t, os.WriteFile(local, []byte("local-one.lab.example\nlocal-two.lab.example\n"), 0o644))
	require.True(t, s.Apply().OK)
	require.Equal(t, 2, fake.checked, "a local list that changed")
	require.Len(t, s.listChecks, 1, "what the check of the list before found is forgotten")
	require.True(t, s.Apply().OK)
	require.Equal(t, 2, fake.checked)

	settings("one.lab.example", "61")
	withLocal()
	require.True(t, s.Apply().OK)
	require.Equal(t, 3, fake.checked, "another configuration")
	require.Len(t, fake.built, 2)
	s.Stop()
}

// A change of one list reads only that list again for the firewall sets: a big local list
// that did not change is not read again.
func TestFakeDeriveOnce(t *testing.T) {
	s, _, system, settings := fakeFixture(t)
	settings("one.lab.example", "60")
	local := filepath.Join(t.TempDir(), "subnets.lst")
	require.NoError(t, os.WriteFile(local, []byte("11.0.0.0/24\n11.0.2.0/24\n"), 0o644))
	withLocal := func(extra string) {
		content, err := os.ReadFile(s.options.SettingsPath)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(s.options.SettingsPath, append(content, "\tlist local_subnet_lists '"+local+"'\n"+extra...), 0o644))
	}
	withLocal("")
	require.True(t, s.Boot().OK)
	content, err := os.ReadFile(local)
	require.NoError(t, err)
	sum := sha256.Sum256(content)
	var before *engine.AddressRule
	for key, found := range s.read {
		if key.sum == sum {
			require.NoError(t, found.err)
			before = &found.rules[0]
		}
	}
	require.NotNil(t, before, "the local list was read for the firewall")
	// another subnet in the user list
	settings("one.lab.example", "60")
	withLocal("\toption user_subnet_list_type 'dynamic'\n\tlist user_subnets '100.71.99.8'\n")
	require.True(t, s.Apply().OK)
	last := system.plans[len(system.plans)-1]
	require.Contains(t, last.Subnets, netip.MustParsePrefix("100.71.99.8/32"))
	require.Contains(t, last.Subnets, netip.MustParsePrefix("11.0.2.0/24"))
	found := false
	for key, read := range s.read {
		if key.sum == sum {
			found = true
			require.Same(t, before, &read.rules[0], "the local list was read again")
		}
	}
	require.True(t, found)
	s.Stop()
}
