package ai

import "strings"

// Protocol traffic and key policies are the public gateway contract. Removed
// AI applications (routing, usage, search, masking and tool MCPs) have no route.
func IsGatewayServicePath(path string) bool {
	for _, route := range []string{"/me", "/v1", "/v1/", "/v1/models", "/v1/codex/models", "/v1/chat/completions", "/v1/responses", "/v1/messages", "/v1/messages/count_tokens", "/v1/systemone", "/v1/decisions", "/v1/embeddings", "/v1/rerank", "/api/version", "/api/tags", "/api/show", "/api/chat", "/api/generate", "/api/hello", "/muse-code/models", "/v1/magpie/me", "/v1/magpie/keys", "/v1/magpie/limit", "/v1/magpie/quotas", "/v1/videos", "/v1beta/models"} {
		if path == route {
			return true
		}
	}
	for _, prefix := range []string{"/v1/models/", "/v1/images/", "/v1/audio/", "/v1/videos/", "/v1beta/models/", "/v1/magpie/keys/", "/v1/magpie/native/", "/backend-api/codex/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// Canonicalize before both master-key and named-key dispatch.
func gatewayCanonicalPath(path string) string {
	for _, route := range []string{"/models", "/chat/completions", "/responses", "/messages", "/messages/count_tokens", "/embeddings", "/rerank", "/decisions", "/audio/transcriptions", "/audio/translations", "/audio/speech", "/videos"} {
		if path == route || (route == "/videos" || route == "/models") && strings.HasPrefix(path, route+"/") {
			return "/v1" + path
		}
	}
	return path
}
