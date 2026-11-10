//go:build windows

// selection-driver exercises the production Host-side reader/clipboard path
// from the isolated native fixture. It does not install hooks or use a profile.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
	"winone/internal/selection"
)

func main() {
	if len(os.Args) != 2 {
		os.Exit(2)
	}
	var input struct {
		Window        uintptr `json:"window"`
		PID           uint32  `json:"pid"`
		Point, Origin []int32
	}
	if json.NewDecoder(os.Stdin).Decode(&input) != nil {
		os.Exit(2)
	}
	event := selection.Event{Window: input.Window, ProcessID: input.PID, Keyboard: len(input.Point) != 2, Kind: "selection"}
	if len(input.Point) == 2 {
		copy(event.Point[:], input.Point)
	}
	if len(input.Origin) == 2 {
		copy(event.Origin[:], input.Origin)
		event.OriginValid = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := selection.ReadSelection(ctx, os.Args[1], event)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	json.NewEncoder(os.Stdout).Encode(result)
}
