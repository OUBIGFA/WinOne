package ai

import (
	"testing"
	"time"
)

func TestGatewayImageBudgetIncludesDrawingAndImageDownload(t *testing.T) {
	for _, c := range []struct{ minutes, want int }{{0, 11}, {1, 3}, {30, 61}, {60, 121}, {90, 121}} {
		settings := &MagpieSettings{ImageTimeout: c.minutes}
		for _, path := range []string{"/v1/images/generations", "/v1/images/edits"} {
			if got := settings.GatewayWait(path); got != time.Duration(c.want)*time.Minute {
				t.Fatalf("%s %d minutes: outer deadline %v, want %dm", path, c.minutes, got, c.want)
			}
		}
		if got := settings.GatewayWait("/v1/chat/completions"); got != 10*time.Minute {
			t.Fatalf("image setting changed ordinary requests: %v", got)
		}
	}
	var unset *MagpieSettings
	if unset.GatewayWait("/v1/images/generations") != 11*time.Minute {
		t.Fatal("missing settings lost the upstream default")
	}
}
