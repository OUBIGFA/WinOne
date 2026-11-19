package main

import (
	"context"
	"encoding/json"
	"io"

	"github.com/yetone/magpie/internal/provider"
)

// The original warm-up entry points own account selection, running windows,
// concurrency and the smallest request. A page action creates no new schedule.
func readAccountWarmups(ctx context.Context, in request, output io.Writer) error {
	var results []provider.WarmAllResult
	if in.Operation == "accounts.warm-all" {
		var err error
		results, err = provider.WarmAll(ctx, in.Agent)
		if err != nil {
			return err
		}
	} else {
		if err := provider.WarmNow(ctx, in.Agent, in.User); err != nil {
			return err
		}
		results = []provider.WarmAllResult{{User: in.User}}
	}
	// A provider error may echo its token. Keep the account and failure state
	// in IPC, but do not send vendor error bodies to the native settings page.
	for i := range results {
		if results[i].Err != "" {
			results[i].Err = "Warm-up could not be completed; check the sign-in, network or official client"
		}
	}
	return json.NewEncoder(output).Encode(map[string]any{"results": results})
}
