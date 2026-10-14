//go:build windows

package app

import (
	"winone/internal/diagnostics"
	"winone/modules/wintray/internal/logging"
)

var workerLogging bool

// RunWorkerHelper preserves the upstream task helper protocol without starting its settings UI.
// A negative return value means this is not a helper invocation.
func RunWorkerHelper(args []string) int { workerLogging = true; return runAppTaskHelper(args) }

// Scheduled-task helpers share WinOne's destination too. Standalone upstream
// entry points keep their own logger; they are not part of the WinOne worker.
func newAppLogger(directory string) (*logging.Logger, error) {
	if !workerLogging {
		return logging.New(directory)
	}
	writer, err := diagnostics.NewModuleWriter(directory, "wintray")
	if err != nil {
		return nil, err
	}
	return logging.NewOutput(writer), nil
}
