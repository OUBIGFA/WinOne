package ai

import (
	"net/url"
	"strings"
)

// Service branding follows the actual endpoint or signed-in account, never
// the model family: an OpenRouter GPT request is still served by OpenRouter.
func providerIcon(p Provider) string {
	if p.AuthMode == "account" {
		return AccountIcon(p.Agent)
	}
	// Preset artwork comes from the bundled catalog. The renderer accepts
	// local asset names only; custom endpoints continue to derive their brand.
	if p.Preset != "" && providerIDPattern.MatchString(p.Icon) {
		return p.Icon
	}
	return endpointIcon(p.BaseURL)
}

// AccountIcon is shared by saved services, sign-in choices and account rows.
func AccountIcon(agent string) string {
	switch agent {
	case "codex", "chatgpt-api":
		return "openai"
	case "claude":
		return "claude-color"
	case "gemini":
		return "gemini-color"
	case "copilot":
		return "githubcopilot"
	case "kimi":
		return "kimi-color"
	case "qwen":
		return "qwen-color"
	case "grok":
		return "xai"
	case "antigravity":
		return "antigravity-color"
	case "cursor":
		return "cursor"
	case "devin":
		return "devin-color"
	case "kiro":
		return "kiro-color"
	case "zed":
		return "zed"
	case "factory":
		return "factory"
	case "workbuddy", "workbuddy-ai":
		return "workbuddy-color"
	case "commandcode-plan":
		return "commandcode"
	case "qoder", "qoder-cn":
		return "qoder-color"
	case "mimo-app":
		return "mimocode"
	case "zcode":
		return "zcode"
	case "cline":
		return "cline"
	case "opencode-zen-free":
		return "opencode"
	case "minimax-code", "minimax-code-global":
		return "minimax-color"
	case "trae-cn", "trae-global":
		return "trae"
	}
	return "service"
}
func endpointIcon(baseURL string) string {
	endpoint, err := url.Parse(baseURL)
	if err != nil {
		return "service"
	}
	hostname := strings.ToLower(endpoint.Hostname())
	for _, brand := range []struct{ domain, icon string }{
		{"openai.com", "openai"}, {"anthropic.com", "anthropic"}, {"deepseek.com", "deepseek-color"},
		{"generativelanguage.googleapis.com", "gemini-color"}, {"openrouter.ai", "openrouter-color"}, {"x.ai", "xai"},
		{"moonshot.cn", "kimi-color"}, {"moonshot.ai", "kimi-color"}, {"bigmodel.cn", "zhipu-color"},
		{"dashscope.aliyuncs.com", "qwen-color"}, {"dashscope-intl.aliyuncs.com", "qwen-color"}, {"mistral.ai", "mistral-color"},
		{"minimax.io", "minimax-color"}, {"minimaxi.com", "minimax-color"}, {"tencentcloudapi.com", "tencentcloud-color"},
		{"openai.azure.com", "azure-color"}, {"volces.com", "doubao-color"}, {"siliconflow.cn", "siliconcloud-color"}, {"siliconflow.com", "siliconcloud-color"},
	} {
		if hostname == brand.domain || strings.HasSuffix(hostname, "."+brand.domain) {
			return brand.icon
		}
	}
	return "service"
}
