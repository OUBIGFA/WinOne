package ai

import (
	"testing"
	"time"
)

func TestGatewayImageBudgetIncludesDrawingAndImageDownload(t *testing.T) {
	for _, settings := range []*MagpieSettings{nil, {}} {
		for _, path := range []string{"/v1/images/generations", "/v1/images/edits"} {
			if got := settings.GatewayWait(path); got != 11*time.Minute {
				t.Fatalf("%s: outer deadline %v, want 11m", path, got)
			}
		}
		if got := settings.GatewayWait("/v1/chat/completions"); got != 10*time.Minute {
			t.Fatalf("image setting changed ordinary requests: %v", got)
		}
	}
}
