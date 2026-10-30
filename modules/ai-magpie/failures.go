package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/netproxy"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type upstreamFailure struct {
	Status, RetryAfterSeconds int
	Limited                   bool
}

const failureBodyLimit = 64 << 10

// A bounded error prefix is used only for upstream's classification. Never
// write it to diagnostics, usage records, or the response sent to the Host.
func appendFailureBody(prefix, chunk []byte) []byte {
	return append(prefix, chunk[:min(len(chunk), failureBodyLimit-len(prefix))]...)
}

func (e *upstreamFailure) Error() string { return fmt.Sprintf("AI request failed (HTTP %d)", e.Status) }

func retryDelay(header string) int {
	if len(header) > 128 {
		return 0
	}
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil {
		return max(0, min(seconds, 86400))
	}
	if date, err := http.ParseTime(header); err == nil {
		return max(0, min(int(time.Until(date).Seconds())+1, 86400))
	}
	return 0
}
func failureEvent(err error) streamEvent {
	event := streamEvent{Type: "error", Error: "AI request did not complete"}
	var failure *upstreamFailure
	if errors.As(err, &failure) && failure.Status >= 400 && failure.Status <= 599 {
		event.Status = failure.Status
		event.RetryAfterSeconds = max(0, min(failure.RetryAfterSeconds, 86400))
		event.Limited = failure.Limited
	}
	return event
}

// magpie's model discovery joins endpoint errors into human-readable strings.
// Observe only response metadata through Go's transport extension point, before
// that conversion. The observer is request-scoped and never retains URLs,
// Authorization headers, bodies or prompts. This client exists only in the
// isolated Worker; its original timeout/redirect policy and transport remain.
type observationKey struct{}
type failureObservation struct {
	mu      sync.Mutex
	failure *upstreamFailure
}

func (o *failureObservation) result() *upstreamFailure {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.failure
}

type metadataTransport struct{ base http.RoundTripper }

func (t metadataTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	response, err := base.RoundTrip(request)
	if observer, ok := request.Context().Value(observationKey{}).(*failureObservation); ok && response != nil && response.StatusCode >= 400 {
		var prefix []byte
		if response.Body != nil {
			reader := bufio.NewReaderSize(response.Body, failureBodyLimit)
			// Peek retains both the bytes and any eventual read error for Fetch.
			prefix, _ = reader.Peek(failureBodyLimit)
			response.Body = struct {
				io.Reader
				io.Closer
			}{reader, response.Body}
		}
		observer.mu.Lock()
		observer.failure = &upstreamFailure{Status: response.StatusCode, RetryAfterSeconds: retryDelay(response.Header.Get("Retry-After")), Limited: gateway.WinOneLimitedResponse(response.StatusCode, prefix)}
		observer.mu.Unlock()
	}
	return response, err
}
func init() {
	// The standalone adapter bypasses Magpie's CLI startup. Install its real
	// proxy dispatcher before wrapping it for safe failure observations.
	netproxy.Install()
	client := *http.DefaultClient
	client.Transport = metadataTransport{base: client.Transport}
	http.DefaultClient = &client
}
func observeFailures(ctx context.Context) (context.Context, *failureObservation) {
	observer := &failureObservation{}
	return context.WithValue(ctx, observationKey{}, observer), observer
}
