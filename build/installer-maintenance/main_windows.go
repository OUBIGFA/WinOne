//go:build windows

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"winone/internal/installguard"
	"winone/internal/transport"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Unlike a renamed Host, this executable cannot fall through to a tray, login
// task, account discovery or RPC service when maintenance arguments are absent.
// Require an explicit profile before acquiring any lease or touching its files.
func parseArguments(args []string) (installguard.Request, error) {
	var request installguard.Request
	var owner uint64
	flags := flag.NewFlagSet("WinOne.Maintenance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&request.Installation, "guard-installation", "", "installation guarded for the installer lifetime")
	flags.StringVar(&request.Profile, "data-dir", "", "explicit product configuration directory")
	flags.StringVar(&request.Pipe, "pipe", "", "Host pipe reservation")
	flags.Uint64Var(&owner, "installer-pid", 0, "installer process owning the maintenance session")
	flags.StringVar(&request.ReadyEvent, "installer-ready", "", "installer-created readiness event")
	flags.StringVar(&request.ReleaseEvent, "installer-release", "", "installer-created completion event")
	flags.BoolVar(&request.PrepareUninstall, "prepare-uninstall", false, "restore owned resources before removal")
	if err := flags.Parse(args); err != nil {
		return request, err
	}
	if flags.NArg() != 0 || !filepath.IsAbs(request.Installation) || !filepath.IsAbs(request.Profile) || owner == 0 || owner > 0xffffffff || request.ReadyEvent == "" {
		return request, errors.New("安装维护参数无效：必须指定安装目录、配置目录、安装器进程和就绪事件")
	}
	request.OwnerPID = uint32(owner)
	return request, nil
}

func run(args []string) error {
	request, err := parseArguments(args)
	if err != nil {
		return err
	}
	if request.Pipe == "" {
		request.Pipe, err = transport.PipeName()
		if err != nil {
			return err
		}
	}
	// Hold returns only after every deferred lease/pipe close has run. This
	// non-UI helper exits normally; it never needs Host's UI-DLL detach workaround.
	return installguard.Hold(request)
}
