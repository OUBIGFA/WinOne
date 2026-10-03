//go:build windows

// Package adminstartup routes an explicitly remembered preference through the
// already-authorized task. Startup never asks for new UAC consent by itself.
package adminstartup

import (
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"time"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/logintask"
	"winone/internal/residency"
	"winone/internal/transport"
)

type Launcher interface {
	Configured() (bool, error)
	Run(logintask.LaunchMode) (string, error)
}

func StartAuthorized(mode logintask.LaunchMode, launcher Launcher) error {
	ready, err := launcher.Configured()
	if err != nil {
		return err
	}
	if !ready {
		return errors.New("管理员授权需要重新确认")
	}
	_, err = launcher.Run(mode)
	return err
}
func Try(ctx context.Context, directory, pipe, executable string, mode logintask.LaunchMode, standard bool, draft ...string) (bool, error) {
	if !strings.EqualFold(filepath.Clean(directory), filepath.Join(os.Getenv("LOCALAPPDATA"), "WinOne")) {
		return false, nil
	}
	expectedPipe, err := transport.PipeName()
	if err != nil {
		return false, err
	}
	if pipe != expectedPipe {
		return false, nil
	}
	elevated := windows.GetCurrentProcessToken().IsElevated()
	prefer := false
	if !standard && !elevated {
		preferences, err := configuration.ReadCommittedSettings(directory)
		if err != nil {
			return false, err
		}
		prefer = preferences.Startup != nil && preferences.Startup.PreferAdministrator
	}
	waitContext, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	waited := false
	rechecked := false
	for {
		response, err := transport.CallWithTimeout(waitContext, pipe, host.Request{Version: 1, ID: "administrator-startup-check", Method: "snapshot"}, 250*time.Millisecond)
		if err != nil {
			if waited && waitContext.Err() != nil {
				return true, waitContext.Err()
			}
			starting, checkErr := residency.AdministratorStarting(pipe)
			if checkErr != nil {
				return false, checkErr
			}
			if starting && !elevated {
				waited = true
				select {
				case <-waitContext.Done():
					return true, waitContext.Err()
				case <-time.After(100 * time.Millisecond):
				}
				continue
			}
			if waited && !rechecked {
				rechecked = true
				continue
			}
			if !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
				return false, err
			}
			if standard || elevated || !prefer {
				return false, nil
			}
			launcher, err := logintask.NewAdministratorLauncher(executable)
			if err != nil {
				return false, err
			}
			if err = StartAuthorized(mode, launcher); err != nil {
				return false, err
			}
			return true, nil
		}
		if response.Error != nil {
			return false, errors.New(response.Error.Message)
		}
		var running host.Snapshot
		if err = json.Unmarshal(response.Result, &running); err != nil {
			return false, err
		}
		if running.RestartPending {
			waited = true
			// A duplicate launch must not create a window while the old session exits.
			select {
			case <-waitContext.Done():
				return true, waitContext.Err()
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}
		if standard || elevated || !running.Administrator {
			return false, nil
		}
		if mode == logintask.LoginLaunch {
			return true, nil
		}
		draftID := ""
		if len(draft) > 0 {
			draftID = draft[0]
		}
		params, _ := json.Marshal(map[string]string{"draftId": draftID})
		opened, err := transport.CallWithTimeout(waitContext, pipe, host.Request{Version: 1, ID: "administrator-open", Method: "application.open", Params: params}, time.Second)
		if err != nil {
			return false, err
		}
		if opened.Error != nil {
			if opened.Error.Code == "restart_busy" {
				continue
			}
			return false, errors.New(opened.Error.Message)
		}
		var accepted struct {
			Accepted bool `json:"accepted"`
		}
		if err = json.Unmarshal(opened.Result, &accepted); err != nil {
			return false, err
		}
		if !accepted.Accepted {
			return false, errors.New("管理员窗口未能打开")
		}
		return true, nil
	}
}
