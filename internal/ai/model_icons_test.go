package ai

import "testing"

func TestModelIconFollowsTheModelInsteadOfTheServingProvider(t *testing.T) {
	for _, sample := range []struct{ model, icon string }{
		{"google/gemini-2.5-pro", "gemini-color"}, {"models/Gemini-3-flash", "gemini-color"}, {"anthropic.claude-sonnet-4", "claude-color"},
		{"openai/gpt-5", "openai"}, {"o3-mini", "openai"}, {"qwen/Qwen3.5-Plus", "qwen-color"}, {"deepseek-ai/DeepSeek-V3.2", "deepseek-color"},
		{"moonshot/kimi-k2", "kimi-color"}, {"grok-4", "grok"}, {"z-ai/glm-5", "zai"}, {"glm-4.6v", "glmv-color"},
		{"meta-llama/Llama-3.3-70B", "meta-color"}, {"google/gemma-3-27b", "gemma-color"}, {"command-r-plus", "cohere-color"},
		{"hunyuan-vision", "hunyuan-color"}, {"step-3.5-flash", "stepfun-color"}, {"mistralai/codestral-latest", "mistral-color"},
		{"private-deployment-001", "model"}, {"mygeminimodel", "model"},
	} {
		if got := ModelIcon(sample.model); got != sample.icon {
			t.Errorf("%s: %s != %s", sample.model, got, sample.icon)
		}
	}
	service := Provider{AuthMode: "api-key", BaseURL: "https://openrouter.ai/api/v1"}
	if got := providerIcon(service); got != "openrouter-color" {
		t.Fatal("model icon mapping changed reply provider branding", got)
	}
}
