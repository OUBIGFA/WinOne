//go:build windows

package main

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestNativeTerminalInputPreservesByteBoundaries(t *testing.T) {
	for _, input := range [][]byte{{0xe4, 0xb8}, {0xad, 0, 0x1b, '[', 'A'}, {0xff, 0xfe, 1, 2}} {
		encoded := base64.StdEncoding.EncodeToString(input)
		decoded, err := terminalInputData("", &encoded)
		if err != nil || !bytes.Equal(decoded, input) {
			t.Fatalf("VT bytes were changed: %v %x", err, decoded)
		}
	}
	text := "中文\x1b[A"
	decoded, err := terminalInputData(text, nil)
	if err != nil || string(decoded) != text {
		t.Fatal("legacy terminal input changed")
	}
	invalid := "!not-base64"
	if _, err := terminalInputData("", &invalid); err == nil {
		t.Fatal("invalid binary input accepted")
	}
	encoded := "YQ=="
	if _, err := terminalInputData("other", &encoded); err == nil {
		t.Fatal("ambiguous input accepted")
	}
}
