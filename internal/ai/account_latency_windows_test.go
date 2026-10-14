//go:build windows

package ai

import (
	"context"
	"os"
	"testing"
	"time"
)

// Opt-in timings exercise the packaged worker in an empty, isolated profile.
// They never authenticate or borrow credentials from an installed client.
func TestAccountReadLatency(t *testing.T) {
	exe := os.Getenv("WINONE_AI_TEST_EXE")
	if exe == "" {
		t.Skip("set WINONE_AI_TEST_EXE")
	}
	runtime := Translator{Executable: exe, DataDirectory: t.TempDir()}
	for _, operation := range []struct{ method, agent, backend string }{
		{"accounts.catalog", "", ""},
		{"accounts.list", "codex", "builtin"},
		{"accounts.list", "cline", "plugin"},
	} {
		t.Run(operation.method+"/"+operation.agent, func(t *testing.T) {
			scoped := runtime.ForProvider("latency-" + operation.agent)
			scoped.AccountBackend = operation.backend
			for attempt := 0; attempt < 2; attempt++ {
				start := time.Now()
				output, err := scoped.Accounts(context.Background(), operation.method, operation.agent)
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("read %d: %s (%d bytes)", attempt+1, time.Since(start), len(output))
			}
		})
	}
}
