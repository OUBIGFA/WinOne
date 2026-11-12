package aiservices

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNetworkErrorDoesNotExposeRuntimeCapability(t *testing.T) {
	_, h, p, tt := setup(t)
	p.gateway.runtimeURL = "http://127.0.0.1:9/session/private-capability/"
	var request nativeAPI
	request.request(p, "GET", "/api/caller-keys", nil, func(json.RawMessage) { t.Fatal("unreachable request succeeded") })
	settle(h, tt)
	if request.err == "" || strings.Contains(request.err, "private-capability") {
		t.Fatalf("network error exposed capability: %q", request.err)
	}
}
