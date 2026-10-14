//go:build windows

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkerHelpersUseTheWinOneLog(t *testing.T) {
	before := workerLogging
	t.Cleanup(func() { workerLogging = before })
	if RunWorkerHelper(nil) != -1 {
		t.Fatal("empty helper invocation handled")
	}
	dir := t.TempDir()
	module := filepath.Join(dir, "modules", "wintray")
	logger, err := newAppLogger(module)
	if err != nil {
		t.Fatal(err)
	}
	logger.Warn("worker helper diagnostic")
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "logs", "host.log"))
	if err != nil || !strings.Contains(string(data), "worker helper diagnostic") {
		t.Fatal(string(data), err)
	}
	if _, err := os.Stat(filepath.Join(module, "wintray.log")); !os.IsNotExist(err) {
		t.Fatal("independent helper log exists", err)
	}
}
