package main

import (
	"errors"
	"fmt"
	"os"

	"golang.zabbix.com/plugin/opcua/plugin"
	"golang.zabbix.com/sdk/errs"
	sdkplugin "golang.zabbix.com/sdk/plugin"
	"golang.zabbix.com/sdk/plugin/flag"
)

//nolint:gochecknoglobals // required ALL_CAPS by build scripts, mirrors the example plugin's main.go.
var (
	PLUGIN_VERSION_MAJOR = 0
	PLUGIN_VERSION_MINOR = 1
	PLUGIN_VERSION_PATCH = 0
	PLUGIN_VERSION_RC    = ""
)

func main() {
	args, err := flag.HandleFlags()
	if err != nil {
		exitWithError(errs.Wrap(err, "failed to handle flags: "))
	}

	pluginInfo := &sdkplugin.Info{
		Name:         plugin.Name,
		BinName:      os.Args[0],
		MajorVersion: PLUGIN_VERSION_MAJOR,
		MinorVersion: PLUGIN_VERSION_MINOR,
		PatchVersion: PLUGIN_VERSION_PATCH,
		Alphatag:     PLUGIN_VERSION_RC,
	}

	p, err := plugin.New()
	if err != nil {
		exitWithError(errs.Wrap(err, "failed to initialize plugin: "))
	}

	err = flag.DecideActionFromFlags(args, p, pluginInfo, nil)
	if err != nil {
		if errors.Is(err, errs.ErrExitGracefully) {
			exitGracefully()
		}

		exitWithError(errs.Wrap(err, "failed to execute plugin functions: "))
	}

	err = p.Run()
	if err != nil {
		exitWithError(errs.Wrap(err, "failed to run plugin: "))
	}
}

func exitWithError(err error) {
	fmt.Fprintf(os.Stderr, "%s\n", err.Error())
	//nolint:revive,nolintlint // this is called only from main().
	os.Exit(1)
}

func exitGracefully() {
	//nolint:revive,nolintlint // this is called only from main().
	os.Exit(0)
}
