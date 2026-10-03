// Package updates checks published versions without downloading or executing code.
package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"winone/internal/configjson"
	"winone/internal/distribution"
)

const maxResponseBytes = 1 << 20

var ownerPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)
var repoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)

// Repository is an application-owned owner/name setting, never a URL from an
// imported profile. Transport permits isolated tests without relaxing URL rules.
type Checker struct {
	Repository     string
	CurrentVersion string
	Transport      http.RoundTripper
}
type Result struct {
	State          string     `json:"state"`
	CurrentVersion string     `json:"currentVersion"`
	LatestVersion  string     `json:"latestVersion,omitempty"`
	ReleaseURL     string     `json:"releaseUrl,omitempty"`
	PublishedAt    *time.Time `json:"publishedAt,omitempty"`
}

// CheckError exposes a stable UI code while retaining cancellation/network causes.
type CheckError struct {
	Code  string
	cause error
}

func (e *CheckError) Error() string { return "update check: " + e.Code }
func (e *CheckError) Unwrap() error { return e.cause }
func failed(code string, cause error) (Result, error) {
	return Result{}, &CheckError{Code: code, cause: cause}
}

func requestFailure(err error) (Result, error) {
	if errors.Is(err, context.Canceled) {
		return failed("cancelled", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return failed("timeout", err)
	}
	return failed("network", err)
}

func (c Checker) Check(ctx context.Context) (Result, error) {
	if _, err := distribution.CompareVersions(c.CurrentVersion, c.CurrentVersion); err != nil {
		return failed("invalid_version", err)
	}
	result := Result{State: "not_configured", CurrentVersion: c.CurrentVersion}
	if c.Repository == "" {
		return result, nil
	}
	parts := strings.Split(c.Repository, "/")
	if len(parts) != 2 || !ownerPattern.MatchString(parts[0]) || !repoPattern.MatchString(parts[1]) || parts[1] == "." || parts[1] == ".." {
		return failed("invalid_source", nil)
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+c.Repository+"/releases/latest", nil)
	if err != nil {
		return failed("invalid_source", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "WinOne/"+c.CurrentVersion)
	client := http.Client{Transport: c.Transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return requestFailure(err)
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return failed("not_found", nil)
	case http.StatusTooManyRequests:
		return failed("rate_limited", nil)
	case http.StatusForbidden:
		if response.Header.Get("X-RateLimit-Remaining") == "0" || response.Header.Get("Retry-After") != "" {
			return failed("rate_limited", nil)
		}
		return failed("unavailable", nil)
	default:
		return failed("unavailable", fmt.Errorf("HTTP %d", response.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return requestFailure(err)
	}
	if len(body) > maxResponseBytes {
		return failed("invalid_response", nil)
	}
	// GitHub may add response fields; allow additions, but never ambiguous JSON.
	if err = configjson.ValidateObject(body); err != nil {
		return failed("invalid_response", err)
	}
	var release struct {
		Tag         string     `json:"tag_name"`
		Draft       *bool      `json:"draft"`
		Prerelease  *bool      `json:"prerelease"`
		PublishedAt *time.Time `json:"published_at"`
	}
	if err = json.Unmarshal(body, &release); err != nil {
		return failed("invalid_response", err)
	}
	if release.Draft == nil || release.Prerelease == nil || *release.Draft || *release.Prerelease || release.PublishedAt == nil || release.PublishedAt.IsZero() {
		return failed("invalid_response", nil)
	}
	version := strings.TrimPrefix(release.Tag, "v")
	order, err := distribution.CompareVersions(c.CurrentVersion, version)
	if err != nil || strings.Contains(version, "-") {
		return failed("invalid_response", err)
	}
	result.State = "up_to_date"
	if order < 0 {
		result.State = "available"
	}
	result.LatestVersion = version
	result.PublishedAt = release.PublishedAt
	// Ignore html_url from the response: only open this configured publisher's tag.
	result.ReleaseURL = "https://github.com/" + c.Repository + "/releases/tag/" + url.PathEscape(release.Tag)
	return result, nil
}
