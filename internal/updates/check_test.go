package updates

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

type failedReader struct{ err error }

func (r failedReader) Read([]byte) (int, error) { return 0, r.err }

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const release = `{"tag_name":"v1.2.0","draft":false,"prerelease":false,"published_at":"2026-10-01T00:00:00Z","body":"Release notes","html_url":"https://untrusted.invalid/"}`

func checker(status int, body string) Checker {
	return Checker{Repository: "example/WinOne", CurrentVersion: "1.1.0", Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
}

func TestManualCheckUsesConfiguredRepositoryAndVersionOrder(t *testing.T) {
	for _, current := range []string{"1.1.0", "1.2.0", "1.3.0", "1.2.0-alpha.1"} {
		t.Run(current, func(t *testing.T) {
			c := checker(200, release)
			c.CurrentVersion = current
			c.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://api.github.com/repos/example/WinOne/releases/latest" || r.Header.Get("Authorization") != "" || r.Method != "GET" {
					t.Fatal("unexpected update request", r.URL)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(release)), Header: make(http.Header), Request: r}, nil
			})
			result, err := c.Check(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			expected := "up_to_date"
			if current == "1.1.0" || current == "1.2.0-alpha.1" {
				expected = "available"
			}
			if result.State != expected || result.LatestVersion != "1.2.0" || result.ReleaseURL != "https://github.com/example/WinOne/releases/tag/v1.2.0" {
				t.Fatal(result)
			}
		})
	}
}
func TestUnconfiguredAndInvalidSourcesNeverRequestNetwork(t *testing.T) {
	for _, repository := range []string{"", "https://github.com/example/WinOne", "../WinOne", "example/../WinOne", "example/repo?token=secret"} {
		c := checker(200, release)
		c.Repository = repository
		c.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected network request"); return nil, nil })
		result, err := c.Check(context.Background())
		if repository == "" {
			if err != nil || result.State != "not_configured" {
				t.Fatal(result, err)
			}
		} else if err == nil {
			t.Fatal("invalid repository accepted")
		}
	}
}
func TestFailuresNeverReportCurrentVersionIsLatest(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body, code string
	}{
		{"no release", 404, "", "not_found"}, {"limited", 429, "", "rate_limited"}, {"forbidden", 403, "", "unavailable"}, {"server", 500, "", "unavailable"},
		{"empty", 200, "{}", "invalid_response"}, {"draft", 200, strings.Replace(release, `"draft":false`, `"draft":true`, 1), "invalid_response"},
		{"prerelease", 200, strings.Replace(release, `"prerelease":false`, `"prerelease":true`, 1), "invalid_response"},
		{"invalid version", 200, strings.Replace(release, "v1.2.0", "next", 1), "invalid_response"},
		{"duplicate", 200, strings.Replace(release, `"draft":false`, `"draft":false,"draft":true`, 1), "invalid_response"},
		{"trailing", 200, release + "{}", "invalid_response"}, {"oversized", 200, strings.Repeat("x", (1<<20)+1), "invalid_response"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			result, err := checker(item.status, item.body).Check(context.Background())
			var failure *CheckError
			if !errors.As(err, &failure) || failure.Code != item.code || result.State == "up_to_date" {
				t.Fatal(result, err)
			}
		})
	}
}
func TestCheckCancellationAndRedirects(t *testing.T) {
	c := checker(200, release)
	c.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Check(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	calls := 0
	c.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://elsewhere.invalid/"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})
	if _, err = c.Check(context.Background()); err == nil || calls != 1 {
		t.Fatal("redirect followed or accepted", calls, err)
	}
}

func TestRateLimitAndBodyDeadlineRemainExplicitFailures(t *testing.T) {
	c := checker(403, "")
	c.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Header: http.Header{"X-Ratelimit-Remaining": []string{"0"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})
	_, err := c.Check(context.Background())
	var failure *CheckError
	if !errors.As(err, &failure) || failure.Code != "rate_limited" {
		t.Fatal(err)
	}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		c.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(failedReader{cause}), Request: r}, nil
		})
		_, err = c.Check(context.Background())
		if !errors.Is(err, cause) || !errors.As(err, &failure) || failure.Code == "network" {
			t.Fatal(err)
		}
	}
}
