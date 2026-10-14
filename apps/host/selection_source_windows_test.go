//go:build windows

package main

import "testing"

func TestSelectionAllowsSettingsAndResultsButNotWorkers(t *testing.T) {
	for _, sample := range []struct {
		path string
		want bool
	}{
		{`C:\WinOne\WinOne.exe`, true},
		{`C:\WinOne\shared\ui\WinOne.Results.exe`, true},
		{`C:\WinOne\selection\WinOne.Selection.exe`, false},
		{`C:\Other\Editor.exe`, true},
	} {
		if got := selectionSourceAllowed(`C:\WinOne`, sample.path); got != sample.want {
			t.Fatalf("%s allowed=%v", sample.path, got)
		}
	}
}
