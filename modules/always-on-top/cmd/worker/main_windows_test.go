//go:build windows

package main

import (
	"bytes"
	"context"
	"flag"
	"os"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWorkerExitChild(t *testing.T) {
	if os.Getenv("WINONE_AOT_EXIT_CHILD") != "1" {
		return
	}
	if _, err := windows.LoadLibrary(os.Getenv("WINONE_EXIT_FIXTURE_DLL")); err != nil {
		t.Fatal(err)
	}
	// Exercise the real entry point with invalid arguments: all worker exits,
	// including startup failure, must bypass foreign DLL detach callbacks.
	flag.CommandLine = flag.NewFlagSet("worker", flag.ContinueOnError)
	os.Args = os.Args[:1]
	main()
}

func TestWorkerExitSkipsForeignDetach(t *testing.T) {
	if os.Getenv("WINONE_EXIT_FIXTURE_DLL") == "" {
		t.Skip("use build/test-process-lifetime.ps1 for the native detach fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorkerExitChild$")
	child.Env = append(os.Environ(), "WINONE_AOT_EXIT_CHILD=1")
	output, err := child.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 2 {
		t.Fatalf("worker argument error lost its exit code: %v; %s", err, output)
	}
	if bytes.Contains(output, []byte("native-detach")) {
		t.Fatal("worker entered foreign DLL detach after its cleanup")
	}
}
