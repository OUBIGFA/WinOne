package ai

import "testing"

func TestGatewayDoesNotExposeRemovedAIServices(t *testing.T) {
	for _, path := range []string{"/mcp/magpie/tools", "/mcp/magpie/image", "/mcp/magpie/vision", "/mcp/magpie/web-search", "/v1/magpie/usage", "/v1/magpie/trace", "/v1/magpie/route", "/v1/magpie/browser", "/v1/magpie/route-events", "/v1/magpie/concurrency"} {
		if IsGatewayServicePath(path) {
			t.Errorf("removed AI service is still public: %s", path)
		}
	}
	for _, path := range []string{"/v1/models", "/v1/chat/completions", "/v1/responses", "/v1/messages", "/api/tags", "/api/chat"} {
		if !IsGatewayServicePath(path) {
			t.Errorf("retained model protocol disappeared: %s", path)
		}
	}
}
