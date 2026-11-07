//go:build windows

package mcpapp

import "testing"

func TestNativeMcpAppFramePolicySeparatesRootFromDeclaredDescendants(t *testing.T) {
	own := browserOrigin(testViewID, true) + "/resource"
	if !sameResourceDocument(testViewID, own) || !sameResourceDocument(testViewID, own+"#anchor") {
		t.Fatal("the root resource was rejected")
	}
	for _, address := range []string{browserOrigin(testViewID, false) + "/sandbox", own + "?x=1", "https://frames.example/resource", "file:///private.txt", "about:blank"} {
		if sameResourceDocument(testViewID, address) {
			t.Errorf("root escaped to %s", address)
		}
	}
	domains := []string{"https://frames.example", "https://*.frames.example", "http://local.example:*"}
	tests := map[string]bool{
		"https://frames.example/page": true, "https://frames.example:443/page": true, "https://child.frames.example/page": true,
		"https://frames.example:444/page": false, "http://frames.example/page": false, "https://notframes.example/page": false,
		"https://user:pass@frames.example/page": false, "https://frames.example.evil/page": false,
		"http://local.example:43123/page": true, "http://sub.local.example/page": false,
		"file:///private.txt": false, "javascript:alert(1)": false, "about:blank": true, "about:srcdoc": true,
	}
	for address, want := range tests {
		if got := frameURLAllowed(address, domains); got != want {
			t.Errorf("frame %s allowed=%v want=%v", address, got, want)
		}
	}
}
