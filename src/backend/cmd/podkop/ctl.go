//go:build linux

package main

import (
	"os"

	"github.com/podkop-net/podkop-experimental/src/backend"
	"github.com/podkop-net/podkop-experimental/src/backend/api"
	"github.com/podkop-net/podkop-experimental/src/backend/ctl"
	"github.com/podkop-net/podkop-experimental/src/backend/diag"
	C "github.com/sagernet/sing-box/constant"

	"github.com/spf13/cobra"
)

// The commands of the user interface, which the shell backend had as the commands of
// /usr/bin/podkop: "podkop get_status" is "podkop ctl get_status" (see main).
var commandCtl = &cobra.Command{
	Use:                "ctl <command> [arguments]",
	Short:              "The commands of the user interface: checks, status, the log, the Clash API",
	DisableFlagParsing: true,
	Run: func(cmd *cobra.Command, args []string) {
		os.Exit(ctl.Run(ctlEnvironment(), args, os.Stdout, os.Stderr))
	},
}

// ctlEnvironment is where the commands find the router and the backend: the same for the
// program and for the commands the running backend answers on ubus (ctl.BusObject).
func ctlEnvironment() ctl.Environment {
	return ctl.Environment{
		Version:        podkop.Version,
		SingBoxVersion: C.Version,
		Engine:         podkop.Engine(C.Version),
		Features:       ctl.PageFeatures,
		Socket:         api.SocketPath,
		Paths:          diag.RouterPaths,
		ConfigDir:      configDir,
		AutostartPath:  "/etc/rc.d/S99podkop",
		ServicePath:    "/etc/init.d/podkop",
	}
}

func init() {
	mainCommand.AddCommand(commandCtl)
}
