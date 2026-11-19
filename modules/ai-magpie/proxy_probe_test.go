package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"os"
	"strings"
	"testing"
	"time"
)

type proxyProbeMemoryTransport func(*http.Request) (*http.Response, error)

func (f proxyProbeMemoryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type proxyProbeMemoryConn struct{ net.Conn }

func (proxyProbeMemoryConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 7890}
}

// The trace HTTP exchange stays in memory. The original connection trace and
// proxy redaction still run, with no listener, DNS lookup or vendor credential.
func TestProxyProbeUsesOriginalConnectionTraceAndRedactsLogin(t *testing.T) {
	old := proxyTestClient
	t.Cleanup(func() { proxyTestClient = old })
	for _, refused := range []bool{false, true} {
		proxyTestClient = &http.Client{Transport: proxyProbeMemoryTransport(func(r *http.Request) (*http.Response, error) {
			if trace := httptrace.ContextClientTrace(r.Context()); trace != nil {
				if trace.GotConn != nil {
					trace.GotConn(httptrace.GotConnInfo{Conn: proxyProbeMemoryConn{}})
				}
				if trace.WroteRequest != nil {
					trace.WroteRequest(httptrace.WroteRequestInfo{})
				}
				// Give the original round-trip clock measurable input; a fully
				// instantaneous memory response legitimately reports zero ms.
				time.Sleep(5 * time.Millisecond)
				if trace.GotFirstResponseByte != nil {
					trace.GotFirstResponseByte()
				}
			}
			if refused {
				return nil, fmt.Errorf("http://user:fixture-password@127.0.0.1:7890 HTTP 407")
			}
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("ip=203.0.113.9\nloc=US\n")), Request: r}, nil
		})}
		out, err := os.CreateTemp(t.TempDir(), "proxy-*.json")
		if err != nil {
			t.Fatal(err)
		}
		prior := os.Stdout
		os.Stdout = out
		err = accountOperation(t.Context(), request{Operation: "proxies.test", ProviderPreferences: &ProviderPreferences{Proxy: "http://user:fixture-password@127.0.0.1:7890"}}, strings.NewReader(""))
		os.Stdout = prior
		if err != nil {
			out.Close()
			t.Fatalf("proxy test did not reach the original probe: %v", err)
		}
		if _, err = out.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(out)
		out.Close()
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			OK    bool   `json:"ok"`
			IP    string `json:"ip"`
			Loc   string `json:"loc"`
			MS    int64  `json:"ms"`
			Error string `json:"error"`
			Why   string `json:"why"`
		}
		if json.Unmarshal(raw, &result) != nil {
			t.Fatalf("invalid proxy result: %s", raw)
		}
		if strings.Contains(string(raw), "fixture-password") {
			t.Fatal("proxy login leaked into the probe result")
		}
		if refused {
			if result.OK || result.Error == "" || result.Why == "" {
				t.Fatalf("proxy rejection was not retained: %s", raw)
			}
		} else if !result.OK || result.IP != "203.0.113.9" || result.Loc != "US" || result.MS < 1 {
			t.Fatalf("original trace result was lost: %s", raw)
		}
	}
}
