//go:build windows

// Read-only readiness probe for resource measurements; never starts a module.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"
	"winone/internal/catalog"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/transport"
)

func main() {
	pipe := flag.String("pipe", "", "isolated Host pipe")
	expected := flag.Bool("expect-enabled", true, "expected state of all bundled modules")
	flag.Parse()
	if err := probe(*pipe, *expected); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type healthSnapshot struct {
	modules.ControlSnapshot
	ConfigurationError string `json:"configurationError"`
}

func healthy(snapshot healthSnapshot, expected bool) (bool, error) {
	if snapshot.ConfigurationError != "" {
		return false, errors.New(snapshot.ConfigurationError)
	}
	wanted := map[string]bool{}
	for _, manifest := range catalog.Builtins() {
		wanted[manifest.ID] = true
	}
	if len(snapshot.Modules) != len(wanted) {
		return false, errors.New("bundled module inventory is incomplete")
	}
	ready := true
	for _, module := range snapshot.Modules {
		id := module.Manifest.ID
		if !wanted[id] {
			return false, errors.New("unexpected or duplicate benchmark module")
		}
		delete(wanted, id)
		if module.Enabled != expected || module.PendingStop {
			return false, errors.New("benchmark module state differs from the requested scenario")
		}
		if !expected {
			if module.Runtime.State != "disabled" {
				return false, fmt.Errorf("%s is not fully disabled", id)
			}
			continue
		}
		switch module.Runtime.State {
		case "ready", "available":
		case "starting", "stopping":
			ready = false
		default:
			return false, fmt.Errorf("%s: %s", id, module.Runtime.Detail)
		}
	}
	return ready, nil
}

func probe(pipe string, expected bool) error {
	if pipe == "" {
		return errors.New("pipe is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	for {
		response, err := transport.CallWithTimeout(ctx, pipe, host.Request{Version: 1, ID: "resource-health", Method: "modules.snapshot", Params: json.RawMessage(`null`)}, 3*time.Second)
		if err != nil {
			return err
		}
		if response.Error != nil {
			return errors.New(response.Error.Message)
		}
		var snapshot healthSnapshot
		if err = json.Unmarshal(response.Result, &snapshot); err != nil {
			return err
		}
		ready, err := healthy(snapshot, expected)
		if err != nil {
			return err
		}
		if ready {
			return json.NewEncoder(os.Stdout).Encode(snapshot)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
