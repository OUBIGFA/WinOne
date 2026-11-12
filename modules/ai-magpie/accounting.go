package main

import (
	"errors"
	"github.com/yetone/magpie/internal/budget"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/gwbudget"
	"github.com/yetone/magpie/internal/usage"
	"os"
	"path/filepath"
)

// Key limits remain part of the local gateway. Use the upstream core limiter
// directly, without registering a plugin manager or its background services.
func serveGatewayLimits() func() { return gwbudget.Limit(budget.Limiter{}) }

// The private ledger is required by named-key budgets and provider spending
// caps. It exposes no independent usage service, exports or prompt archives.
func configureAccounting(in request) error {
	usage.WinOneSkipAppend = gateway.WinOneTransportOnly
	path := os.Getenv("WINONE_AI_USAGE_PATH")
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) {
		return errors.New("invalid accounting path")
	}
	usage.WinOnePath = path
	usage.WinOneRecord = func(r usage.Record) usage.Record {
		if r.Error != "" {
			r.Error = "Request failed"
		}
		if in.Operation == "gateway.serve" {
			return r
		}
		r.Agent = "winone"
		if in.ProviderID != "" {
			r.Provider = in.ProviderID
		}
		if in.Agent != "" {
			r.ProviderAccount = in.ProviderID
		}
		return r
	}
	return nil
}
