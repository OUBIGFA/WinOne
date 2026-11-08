package ai

import "strings"

// Only AI protocols and their diagnostics are public. Upstream application
// control (self-update, restart, profiles and desktop settings) stays private.
func IsGatewayServicePath(path string) bool {
	if path == "/me" {
		return true
	}
	for _, p := range []string{"/v1", "/v1/", "/v1/codex/models", "/v1/alpha/search", "/v1/magpie/statusline", "/v1/magpie/route-events", "/v1/magpie/route-events/snapshot", "/v1/magpie/mcp", "/v1/magpie/me", "/v1/magpie/claude-runs", "/v1/magpie/quotas/cards", "/v1/magpie/quotas/refresh", "/v1/magpie/quotas/codex-reset", "/v1/magpie/prove", "/v1/magpie/browser", "/v1/magpie/browser/extension"} {
		if path == p {
			return true
		}
	}
	for _, p := range []string{"/v1/models", "/v1/chat/completions", "/v1/responses", "/v1/messages", "/v1/messages/count_tokens", "/v1/systemone", "/v1/decisions", "/v1/embeddings", "/v1/rerank", "/api/version", "/api/tags", "/api/show", "/api/chat", "/api/generate", "/api/hello", "/muse-code/models", "/v1/magpie/quotas", "/v1/magpie/quotas/history", "/v1/magpie/usage", "/v1/magpie/usage/requests", "/v1/magpie/route", "/v1/magpie/trace", "/v1/magpie/concurrency", "/v1/magpie/limit", "/v1/magpie/keys"} {
		if path == p {
			return true
		}
	}
	for _, prefix := range []string{"/v1/models/", "/v1/images/", "/v1/audio/", "/v1/videos/", "/v1beta/models/", "/v1/magpie/keys/", "/v1/magpie/native/", "/mcp/", "/backend-api/codex/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return path == "/v1/videos" || path == "/v1beta/models"
}

// Match the upstream's short OpenAI-compatible paths before choosing the
// master/named-key path, so the credential kind cannot change API support.
func gatewayCanonicalPath(path string) string {
	for _, route := range []string{"/models", "/chat/completions", "/responses", "/messages", "/messages/count_tokens", "/embeddings", "/rerank", "/decisions", "/audio/transcriptions", "/audio/translations", "/audio/speech", "/videos"} {
		if path == route || (route == "/videos" || route == "/models") && strings.HasPrefix(path, route+"/") {
			return "/v1" + path
		}
	}
	return path
}
