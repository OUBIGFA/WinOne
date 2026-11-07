package main

import (
	"path/filepath"
	"testing"
)

func TestSandboxInitRequiresAnExplicitPrivateDocumentAndProfile(t *testing.T) {
	valid := initialization{URL: "http://127.0.0.1:18765/0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef/__boot__", Directory: filepath.Clean(t.TempDir()), SDK: "// reviewed host SDK"}
	if err := validateInit(valid); err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"https://example.com", "http://localhost:18765/__boot__", "http://127.0.0.1:18765/kv", valid.URL + "?redirect=other", valid.URL + "#fragment", "http://user@127.0.0.1:18765/0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef/__boot__"} {
		bad := valid
		bad.URL = address
		if err := validateInit(bad); err == nil {
			t.Errorf("untrusted URL accepted: %q", address)
		}
	}
	bad := valid
	bad.Directory = "relative"
	if validateInit(bad) == nil {
		t.Fatal("a relative profile could reach a default user directory")
	}
	bad = valid
	bad.SDK = ""
	if validateInit(bad) == nil {
		t.Fatal("a missing SDK was accepted")
	}
}
