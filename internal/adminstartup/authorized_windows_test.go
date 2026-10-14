//go:build windows

package adminstartup

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/logintask"
	"winone/internal/settings"
	"winone/internal/transport"
)

func TestAuthorizedStartupUsesOnlyFixedProductCommands(t *testing.T) {
	for _, test := range []struct {
		mode logintask.LaunchMode
		exe  string
		args []string
	}{
		{logintask.InteractiveLaunch, `C:\WinOne\WinOne.exe`, []string{"--open-settings"}},
		{logintask.LoginLaunch, `C:\WinOne\WinOne.Host.exe`, []string{"--autorun"}},
	} {
		exe, args, err := authorizedStartCommand(`C:\WinOne\WinOne.exe`, test.mode)
		if err != nil || exe != test.exe || !reflect.DeepEqual(args, test.args) {
			t.Fatal(exe, args, err)
		}
	}
	if _, _, err := authorizedStartCommand(`C:\WinOne\WinOne.exe`, "interactive --other"); err == nil {
		t.Fatal("untrusted mode accepted")
	}
}

func TestAdministratorStartupChecksThePipeServerTokenNotItsClaim(t *testing.T) {
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-admin-token-probe-%d-%d`, os.Getpid(), time.Now().UnixNano())
	listener, err := transport.Listen(pipe)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, err := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := host.New(store)
	actual := windows.GetCurrentProcessToken().IsElevated()
	service.SetAdministrator(!actual)
	go transport.Serve(ctx, listener, service)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	state, err := readAdministratorStartupState(ctx, pipe, executable)
	if err != nil || !state.host || state.administrator != actual {
		t.Fatal(state, err)
	}
	if _, err = readAdministratorStartupState(ctx, pipe, filepath.Join(t.TempDir(), "other.exe")); err == nil {
		t.Fatal("unexpected server executable accepted")
	}
}

func TestAuthorizedStartHelperRejectsOrdinaryTokenBeforeLaunching(t *testing.T) {
	if windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("requires ordinary token")
	}
	handled, err := RunAuthorizedStartHelper([]string{"--authorized-start", "interactive"}, `C:\WinOne\WinOne.exe`)
	if !handled || err == nil {
		t.Fatal("ordinary helper started a privileged session")
	}
}
func TestAuthorizedStartupWaitsForOrdinaryHostAndWindow(t *testing.T) {
	probes := 0
	err := waitAuthorizedStartup(context.Background(), func(context.Context) (administratorStartupState, error) {
		probes++
		switch probes {
		case 1:
			return administratorStartupState{host: true, window: true}, nil
		case 2:
			return administratorStartupState{window: true}, nil
		default:
			return administratorStartupState{}, nil
		}
	}, time.Millisecond)
	if err != nil || probes != 3 {
		t.Fatal(probes, err)
	}
}
func TestAuthorizedStartupCanReuseAnElevatedHost(t *testing.T) {
	err := waitAuthorizedStartup(context.Background(), func(context.Context) (administratorStartupState, error) {
		return administratorStartupState{host: true, administrator: true, window: true}, nil
	}, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
}
func TestAuthorizedStartupDoesNotTreatProbeErrorsAsShutdown(t *testing.T) {
	denied := errors.New("access denied")
	if err := waitAuthorizedStartup(context.Background(), func(context.Context) (administratorStartupState, error) { return administratorStartupState{}, denied }, time.Millisecond); !errors.Is(err, denied) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := waitAuthorizedStartup(ctx, func(context.Context) (administratorStartupState, error) {
		return administratorStartupState{host: true}, nil
	}, time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
