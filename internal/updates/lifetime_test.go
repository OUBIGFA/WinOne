package updates

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestCheckReleasesItsIdleConnection(t *testing.T) {
	closed := make(chan struct{}, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(release)) }))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			select {
			case closed <- struct{}{}:
			default:
			}
		}
	}
	server.StartTLS()
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := server.Client().Transport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig.ServerName = target.Hostname()
	transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", target.Host)
	}
	original := http.DefaultTransport
	http.DefaultTransport = transport
	defer func() { http.DefaultTransport = original; transport.CloseIdleConnections() }()
	// Exercise the default production transport path while directing its TLS
	// socket only to this test server. No external publisher is contacted.
	result, err := (Checker{Repository: "example/WinOne", CurrentVersion: "1.1.0"}).Check(context.Background())
	if err != nil || result.State != "available" {
		t.Fatal(result, err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("completed update retained its idle connection")
	}
}
