//go:build windows

package onboarding

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestSendHintHasOneWinnerAcrossConcurrentClaimsAndReopens(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "onboarding")
	if err := PrepareDirectory(directory); err != nil {
		t.Fatal(err)
	}
	var winners atomic.Int32
	var group sync.WaitGroup
	for range 16 {
		group.Add(1)
		go func() {
			defer group.Done()
			show, err := ClaimSendHint(directory)
			if err != nil {
				t.Error(err)
			}
			if show {
				winners.Add(1)
			}
		}()
	}
	group.Wait()
	if winners.Load() != 1 {
		t.Fatalf("%d windows received first-use hint", winners.Load())
	}
	if show, err := ClaimSendHint(directory); err != nil || show {
		t.Fatalf("reopened Host showed hint again: %v %v", show, err)
	}
	raw, err := os.ReadFile(filepath.Join(directory, ResultSendHintMarker))
	if err != nil || string(raw) != "1\n" {
		t.Fatalf("marker was not durable: %q %v", raw, err)
	}
}

func TestSendHintFailureDoesNotClaimOtherLocations(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "missing")
	if show, err := ClaimSendHint(directory); err == nil || show {
		t.Fatal("missing machine directory was silently accepted")
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("claim unexpectedly initialized its own directory")
	}
}
