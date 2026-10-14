//go:build windows

package ai

import (
	"net/http"
	"testing"
	"time"
)

func TestGatewayRetryAfterPreservesVendorWait(t *testing.T) {
	now := time.Date(2026, 10, 8, 1, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		value string
		want  time.Duration
	}{{"120", 2 * time.Minute}, {now.Add(3 * time.Minute).Format(http.TimeFormat), 3 * time.Minute}, {"invalid", 0}, {"-1", 0}, {"999999", 24 * time.Hour}} {
		if got := gatewayRetryAfter(test.value, now); got != test.want {
			t.Fatalf("%s: %s != %s", test.value, got, test.want)
		}
	}
}
