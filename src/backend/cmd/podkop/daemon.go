//go:build linux

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/podkop-net/podkop-experimental/src/backend/api"
	"github.com/podkop-net/podkop-experimental/src/backend/clock"
	"github.com/podkop-net/podkop-experimental/src/backend/dataplane"
	"github.com/podkop-net/podkop-experimental/src/backend/firewall"
	"github.com/podkop-net/podkop-experimental/src/backend/logbuf"
	"github.com/podkop-net/podkop-experimental/src/backend/probe"
	"github.com/podkop-net/podkop-experimental/src/backend/supervisor"
	"github.com/podkop-net/podkop-experimental/src/backend/ubus"

	"github.com/spf13/cobra"
)

var commandDaemonFlagUCI string

var commandDaemon = &cobra.Command{
	Use:   "daemon",
	Short: "Run the backend: sing-box, the firewall, the routing and the DNS of the LAN, from the settings",
	Long: "Run the backend until it is told to end.\n\n" +
		"It brings the router up from the settings, with sing-box inside this process. SIGHUP\n" +
		"applies the settings again: only what differs is changed, and settings that cannot\n" +
		"be used change nothing. SIGTERM takes everything back and ends the process.\n\n" +
		"What happens is printed a line an event, \"[level] message\".",
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// one backend at a time, and a way for the release command to know this one is gone
		lock, err := takeLock(false)
		if err != nil {
			os.Stderr.WriteString("error: another backend runs: " + err.Error() + "\n")
			os.Exit(1)
		}
		defer lock.Close()
		processSettings()
		// the packet traces of checks whose process was killed before it removed them
		_ = firewall.DisarmTraces()
		ctx, cancel := context.WithCancel(context.Background())
		signals := make(chan os.Signal, 4)
		signal.Notify(signals, syscall.SIGHUP, syscall.SIGTERM, os.Interrupt)
		reload := make(chan struct{}, 1)
		go func() {
			for received := range signals {
				if received == syscall.SIGHUP {
					// applying takes a while: one more request waits, more than one are one
					select {
					case reload <- struct{}{}:
					default:
					}
					continue
				}
				cancel()
			}
		}()
		// the log of this run, for the user interface: the backend's own lines, which also
		// go to the system log, and those of sing-box
		logs := logbuf.New(256 << 10)
		// the checks with made-up names: who reaches sing-box through FakeIP, see probe
		watcher := probe.NewWatcher()
		go func() {
			err := probe.ServeDeadEnd(ctx)
			if err != nil {
				os.Stdout.WriteString("[warn] the connections of the FakeIP checks have no place to end: " + err.Error() + "\n")
			}
		}()
		say := func(level string, message string) {
			os.Stdout.WriteString("[" + level + "] " + message + "\n")
			logs.Write("backend", level, message)
		}
		setClock(ctx, say)
		backend := supervisor.New(supervisor.Options{
			SettingsPath: commandDaemonFlagUCI,
			System:       supervisor.Router{},
			Watcher:      watcher,
			StatePath:    statePath,
			ConfigDir:    configDir,
			DownloadURL:  downloadURL,
			Log:          say,
			EngineLog: func(level string, message string) {
				logs.Write("sing-box", level, message)
			},
		})
		go func() {
			err := api.Serve(ctx, api.SocketPath, backend, logs)
			if err != nil {
				os.Stdout.WriteString("[warn] the socket for commands is not there: " + err.Error() + "\n")
			}
		}()
		// the commands of the page answered here, with no program started for a call
		go ubus.Serve(ctx, ubus.Socket(), busObject(ctlEnvironment(), backend), func(message string) {
			say("warn", message)
		})
		if err := backend.Run(ctx, reload); err != nil {
			// sing-box did not close and holds its sockets: the router is given back, and
			// a new process is the only way on
			os.Stdout.WriteString("[error] the backend ends: " + err.Error() + "\n")
			os.Exit(1)
		}
	},
}

var commandRelease = &cobra.Command{
	Use:   "release",
	Short: "Give the router back what a backend that is gone left taken",
	Long: "Give the router back what a backend left taken: the DNS of the LAN, the firewall\n" +
		"table, the routing. A backend that ends as it should does this itself; this command is\n" +
		"for the one that was killed. It waits for a running backend to end first, and does\n" +
		"nothing when there is nothing to give back.",
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		lock, err := takeLock(true)
		if err != nil {
			os.Stderr.WriteString("error: the backend still runs: " + err.Error() + "\n")
			os.Exit(1)
		}
		defer lock.Close()
		supervisor.New(supervisor.Options{
			System:    supervisor.Router{},
			ConfigDir: configDir,
			Log: func(level string, message string) {
				os.Stdout.WriteString("[" + level + "] " + message + "\n")
			},
		}).Release()
	},
}

const (
	// statePath tells the guard of the package what the backend is doing.
	statePath = dataplane.RunDir + "/state"
	// configDir keeps the configuration that runs and the last one refused, for
	// show_sing_box_config.
	configDir = dataplane.RunDir
	// downloadURL is the download test of urltest groups whose section keeps the
	// default: 64 KiB through the member before a group switches to it. A server frozen
	// after ~16 KB passes the usual small test, not this one. podkop-engine set it for its
	// sing-box service; the backend writes it into the configuration itself.
	downloadURL = "https://speed.cloudflare.com/__down?bytes=65536"
	// workDir is the working directory of sing-box, where YACD is downloaded to.
	workDir = dataplane.DataDir
	// fileLimit is the number of open files the backend may have. The default of a
	// service, 4096, runs out at some 2000 proxied connections, and then DNS queries fail
	// for want of a socket.
	fileLimit = 65536
)

// processSettings sets what the process needs whatever started it: an init script
// that a package manager kept from another package would give none of it.
func processSettings() {
	var limit syscall.Rlimit
	if syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit) == nil && limit.Cur < fileLimit {
		wanted := syscall.Rlimit{Cur: fileLimit, Max: max(limit.Max, fileLimit)}
		if syscall.Setrlimit(syscall.RLIMIT_NOFILE, &wanted) != nil {
			// not allowed to raise the ceiling: as far as it goes
			wanted = syscall.Rlimit{Cur: limit.Max, Max: limit.Max}
			_ = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &wanted)
		}
	}
	if os.MkdirAll(workDir, 0o755) == nil {
		_ = os.Chdir(workDir)
	}
	// a rule set with nothing cached starts empty and is downloaded after the start: an
	// apply that adds a list does not leave the LAN without DNS while it downloads. The
	// environment tells sing-box, unless the service says otherwise.
	if _, set := os.LookupEnv(startEmptyVariable); !set {
		_ = os.Setenv(startEmptyVariable, "1")
	}
}

// startEmptyVariable is the switch of sing-box (route/rule) for rule sets that start
// empty instead of downloading during the start.
const startEmptyVariable = "SING_BOX_RULE_SET_START_EMPTY"

// clockWait is how long the start waits for the NTP servers: without the WAN, at boot,
// none answers, and the start goes on with the clock it has.
const clockWait = 3 * time.Second

// setClock sets the clock before the first start of sing-box, which downloads lists
// and speaks TLS (see clock).
func setClock(ctx context.Context, say func(level string, message string)) {
	ctx, cancel := context.WithTimeout(ctx, clockWait)
	defer cancel()
	result, err := clock.Sync(ctx, clock.Servers, clock.Threshold, nil)
	switch {
	case err != nil:
		say("info", "the clock is not checked: "+err.Error())
	case result.Set:
		way := "behind"
		offset := result.Offset
		if offset < 0 {
			way, offset = "ahead", -offset
		}
		say("warn", "the clock was "+offset.Round(time.Second).String()+" "+way+"; it is set from the NTP server "+result.Server)
	}
}

// lockPath is the file a running backend holds locked.
const lockPath = "/var/run/podkop.lock"

// lock takes the lock of the backend. With patience it waits for a backend that is
// ending, as long as the service manager gives one to end and a little more.
func takeLock(patience bool) (*os.File, error) {
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(40 * time.Second)
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return file, nil
		}
		if !patience || time.Now().After(deadline) {
			file.Close()
			return nil, err
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func init() {
	commandDaemon.Flags().StringVar(&commandDaemonFlagUCI, "uci", "/etc/config/podkop", "The settings file")
	mainCommand.AddCommand(commandDaemon, commandRelease)
}
