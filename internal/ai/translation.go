// Package ai is WinOne's shared text-AI boundary. Modules supply text, never
// credentials or provider wiring; the pinned magpie gateway adapts LLM APIs.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"
)

const googleEndpoint = "https://translate-pa.googleapis.com/v1/translateHtml"

// zencrop MachineTranslationEngine.cpp uses the public community client key.
const googleCommunityKey = "AIzaSyATBXajvzQLTDHEQbcpq0Ihe0vWDHmO520"

type Preferences struct {
	LimitRestOff    bool              `json:"-"`
	GlobalGateway   bool              `json:"-"`
	Auxiliaries     []RuntimeProvider `json:"-"`
	MagpieSettings  *MagpieSettings   `json:"-"`
	ModelMetadata   ModelMetadata     `json:"-"`
	RequestLimits   RequestLimits     `json:"-"`
	AdditionalTools bool              `json:"-"`
	Headers         map[string]string `json:"-"` // Immutable request snapshot, separate from the legacy UI projection.
	ProviderID      string            `json:"-"`
	ProviderName    string            `json:"-"` // Request snapshot only; not a second configuration field.
	ModelName       string            `json:"-"`
	ProviderIcon    string            `json:"-"`
	Revision        uint64            `json:"revision"`
	Engine          string            `json:"engine"`
	Target          string            `json:"target"`
	Protocol        string            `json:"protocol"`
	BaseURL         string            `json:"baseUrl"`
	Model           string            `json:"model"`
	Prompt          string            `json:"prompt"`
	HasKey          bool              `json:"hasKey"`
	AuthMode        string            `json:"authMode,omitempty"`
	Agent           string            `json:"agent,omitempty"`
	AgentBackend    string            `json:"agentBackend,omitempty"`
	Preset          string            `json:"preset,omitempty"`
	PresetRegion    string            `json:"presetRegion,omitempty"`
	OCREngine       string            `json:"ocrEngine,omitempty"`
}

func DefaultPreferences() Preferences {
	return Preferences{Engine: "google", Target: "auto", Protocol: "chat", OCREngine: "local"}
}

// Smart Chinese/English direction matches zencrop TranslationTypes.h.
func targetLanguage(target, text string) string {
	if target != "auto" && target != "" {
		return target
	}
	han, latin := 0, 0
	inLatin := false
	for _, r := range text {
		switch {
		case r >= 0x3040 && r <= 0x30ff || r >= 0x31f0 && r <= 0x31ff || r >= 0xac00 && r <= 0xd7af:
			return "zh-CN"
		case r >= 0x3400 && r <= 0x4dbf || r >= 0x4e00 && r <= 0x9fff || r >= 0xf900 && r <= 0xfaff:
			han++
			inLatin = false
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z':
			if !inLatin {
				latin++
			}
			inLatin = true
		default:
			inLatin = false
		}
	}
	if han > 0 && han >= latin*2 {
		return "en"
	}
	return "zh-CN"
}

func translateGoogle(ctx context.Context, client *http.Client, endpoint, text, target string) (string, error) {
	body, _ := json.Marshal([]any{[]any{[]string{html.EscapeString(text)}, "auto", target}, "wt_lib"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json+protobuf")
	req.Header.Set("X-Goog-API-Key", googleCommunityKey)
	response, err := client.Do(req)
	if err != nil {
		return "", errors.New("翻译连接失败，请检查网络后重试")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("翻译服务返回 HTTP %d，请稍后重试", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024+1))
	if err != nil || len(data) > 1024*1024 {
		return "", errors.New("翻译响应过大或读取失败")
	}
	var result []json.RawMessage
	var translated []string
	if json.Unmarshal(data, &result) != nil || len(result) == 0 || json.Unmarshal(result[0], &translated) != nil || len(translated) != 1 || strings.TrimSpace(translated[0]) == "" {
		return "", errors.New("翻译服务未返回有效结果")
	}
	return html.UnescapeString(translated[0]), nil
}

type Translator struct {
	Executable, DataDirectory, AccountBackend, UsagePath string
	RootDataDirectory                                    string
	TransportOnly                                        bool
	RuntimeSettings                                      *MagpieSettings
	// Host-owned local masking service, resolved before a request worker starts.
	// Nil in the shared router and transport-only workers prevents recursion.
	PrivacyService func(context.Context) (string, string, error)
	// Plugin-owned windows can outlive a request, but not the owning Host.
	// Nil preserves the one-shot worker policy for command-line callers/tests.
	PluginLifetime context.Context
}

func (t Translator) Translate(ctx context.Context, p Preferences, key, text string) (string, error) {
	return t.translate(ctx, p, key, text, nil)
}

// TranslateStream preserves the built-in engine's one-shot behavior while LLM
// translations publish provider deltas through the common generation boundary.
func (t Translator) TranslateStream(ctx context.Context, p Preferences, key, text string, progress func(Delta) error) (string, error) {
	return t.translate(ctx, p, key, text, progress)
}
func (t Translator) translate(ctx context.Context, p Preferences, key, text string, progress func(Delta) error) (string, error) {
	if strings.TrimSpace(text) == "" || len([]rune(text)) > 20000 {
		return "", errors.New("请选择 1–20000 字的文字")
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	target := targetLanguage(p.Target, text)
	if p.Engine == "google" {
		return translateGoogle(ctx, &http.Client{Timeout: 45 * time.Second}, googleEndpoint, text, target)
	}
	prompt := p.Prompt
	if prompt == "" {
		prompt = DefaultTranslationPrompt
	}
	prompt = strings.ReplaceAll(prompt, "{target}", target)
	return t.generate(ctx, p, key, Generation{Text: text, System: prompt}, progress)
}

// Generation is the module-facing payload. Provider and model selection come
// exclusively from the shared settings snapshot, never from a module request.
type Generation struct {
	Text     string
	System   string
	ImagePNG []byte
	Messages []Message
	// Auxiliary calls may request a smaller output budget and their own
	// bounded deadline without rewriting saved provider/model preferences.
	MaxTokens int
	Timeout   time.Duration
}

func (t Translator) Generate(ctx context.Context, p Preferences, key string, input Generation) (string, error) {
	return t.generate(ctx, p, key, input, nil)
}

// GenerateStream publishes real deltas while retaining the same validation,
// account isolation and lifetime as non-streaming callers.
func (t Translator) GenerateStream(ctx context.Context, p Preferences, key string, input Generation, progress func(Delta) error) (string, error) {
	if progress == nil {
		return "", errors.New("AI 流式接收器不可用")
	}
	return t.generate(ctx, p, key, input, progress)
}
func (t Translator) generate(ctx context.Context, p Preferences, key string, input Generation, progress func(Delta) error) (string, error) {
	if err := ValidateGeneration(input); err != nil {
		return "", err
	}
	if len(input.ImagePNG) > 0 && p.ModelMetadata.Vision != nil && !*p.ModelMetadata.Vision && (p.MagpieSettings == nil || p.MagpieSettings.Vision == "off") {
		return "", errors.New("此模型已关闭图像输入，请在 AI 服务中启用或选择其他模型")
	}
	timeout := input.Timeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	release, err := t.acquireRequest(ctx, p, key)
	if err != nil {
		return "", err
	}
	defer release()
	agent := ""
	if p.AuthMode == "account" {
		agent = p.Agent
	}
	request := map[string]any{"providerId": p.ProviderID, "agent": agent, "protocol": p.Protocol, "baseUrl": p.BaseURL, "model": p.Model, "key": key, "system": input.System, "text": input.Text, "imagePng": input.ImagePNG, "messages": input.Messages}
	request["additionalTools"] = p.AdditionalTools
	request["limitRestOff"] = p.LimitRestOff
	request["headers"] = p.Headers
	metadata := p.ModelMetadata
	if input.MaxTokens > 0 && (metadata.MaxTokens == 0 || input.MaxTokens < metadata.MaxTokens) {
		metadata.MaxTokens = input.MaxTokens
	}
	request["modelMetadata"] = metadata
	request["magpieSettings"] = p.MagpieSettings
	auxiliary, stopAuxiliary, err := t.auxiliaryBridge(ctx, p.Auxiliaries, p.MagpieSettings)
	if err != nil {
		return "", err
	}
	defer stopAuxiliary()
	request["auxiliaryProviders"] = auxiliary
	if p.Preset != "" {
		request["preset"] = p.Preset
		request["presetRegion"] = p.PresetRegion
	}
	if agent != "" {
		request["agentBackend"] = p.AgentBackend
		// The process must follow the same backend as the payload, including
		// callers which start with a plain Translator.
		t.AccountBackend = p.AgentBackend
	}
	if progress != nil {
		request["stream"] = true
		return t.invokeStream(ctx, request, progress)
	}
	output, err := t.invoke(ctx, request)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	var result struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if json.Unmarshal(output, &result) != nil {
		return "", errors.New("AI 服务未能启动或响应无效，请修复安装")
	}
	if err != nil || result.Error != "" {
		if p.AuthMode == "account" {
			return "", errors.New("智能体调用失败，请检查登录状态、模型和官方客户端")
		}
		return "", errors.New("AI 调用失败，请检查 AI 服务中的地址、模型和密钥；图片识别需要支持图像的模型")
	}
	if strings.TrimSpace(result.Text) == "" {
		return "", errors.New("AI 未返回结果")
	}
	return result.Text, nil
}
