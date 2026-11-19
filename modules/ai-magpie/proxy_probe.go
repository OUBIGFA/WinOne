package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/yetone/magpie/internal/netproxy"
)

// The proxy test reads the current form without keys or a save. Every check,
// trace, latency and failure explanation comes from the pinned upstream.
func readProxyProbe(ctx context.Context, in request, output io.Writer) error {
	choice := ""
	if in.ProviderPreferences != nil {
		if err := in.ProviderPreferences.validate(); err != nil {
			return err
		}
		choice = in.ProviderPreferences.Proxy
	}
	if choice == "" {
		probe, err := http.NewRequestWithContext(ctx, http.MethodGet, proxyTraceURL, nil)
		if err != nil {
			return err
		}
		p, err := netproxy.Func(probe)
		if err != nil {
			return err
		}
		if p != nil {
			choice = p.String()
		}
	}
	var result proxyTestJSON
	if choice == "" || choice == "direct" {
		leg := traceLeg(netproxy.With(ctx, "direct"), http.DefaultClient)
		result = proxyTestJSON{OK: leg.Error == "", Ms: leg.Ms, IP: leg.IP, Loc: leg.Loc, Error: leg.Error, At: time.Now().Unix()}
	} else {
		result = testProxy(ctx, choice)
	}
	return json.NewEncoder(output).Encode(result)
}
