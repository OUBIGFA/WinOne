// WinOne owns preferences and lifetime; magpie owns LLM protocol adaptation.
// This entry is compiled inside the pinned upstream module by build-ai.ps1.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/yetone/magpie/internal/appdir"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
	"github.com/yetone/magpie/internal/stats"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"
)

type request struct {
	BalanceKeys        []provider.KeyAccount    `json:"balanceKeys,omitempty"` // Private saved-key snapshot; never a UI response.
	MagpieSettings     json.RawMessage          `json:"magpieSettings,omitempty"`
	AuxiliaryProviders []auxiliaryProvider      `json:"auxiliaryProviders,omitempty"`
	TestModels         []string                 `json:"testModels,omitempty"`
	TestClient         string                   `json:"testClient,omitempty"`
	ModelPreferences   map[string]modelMetadata `json:"modelPreferences,omitempty"`
	ModelMetadata      modelMetadata            `json:"modelMetadata,omitempty"`
	Icon               *iconRequest             `json:"icon,omitempty"`
	AdditionalTools bool                       `json:"additionalTools,omitempty"`
	Headers            map[string]string        `json:"headers,omitempty"`
	ProviderID         string                   `json:"providerId,omitempty"`
	Usage              *usageQuery              `json:"usage,omitempty"`
	UsageSource        string                   `json:"usageSource,omitempty"`
	GatewayPath        string                   `json:"gatewayPath,omitempty"`
	GatewayBody        json.RawMessage          `json:"gatewayBody,omitempty"`
	GatewayHeaders     http.Header              `json:"gatewayHeaders,omitempty"`
	Stream             bool                     `json:"stream,omitempty"`
	Operation          string                   `json:"operation,omitempty"`
	Agent              string                   `json:"agent,omitempty"`
	AgentBackend       string                   `json:"agentBackend,omitempty"`
	Preset             string                   `json:"preset,omitempty"`
	PresetRegion       string                   `json:"presetRegion,omitempty"`
	Method             *int                     `json:"method,omitempty"`
	Protocol           string                   `json:"protocol"`
	BaseURL            string                   `json:"baseUrl"`
	Key                string                   `json:"key"`
	Model              string                   `json:"model"`
	System             string                   `json:"system"`
	Text               string                   `json:"text"`
	ImagePNG           []byte                   `json:"imagePng,omitempty"`
	Messages           []conversationMessage    `json:"messages,omitempty"`
}

type conversationMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

const maximumRequest = 48 << 20 // One bounded PNG plus its JSON encoding, sent once.

func run(ctx context.Context, input io.Reader) (string, error) {
	return runOutput(ctx, input, nil)
}
func runOutput(ctx context.Context, input io.Reader, emit func(streamEvent) error) (string, error) {
	stopPlugins := serveBuiltinPlugins()
	defer stopPlugins()
	var in request
	decoder := json.NewDecoder(io.LimitReader(input, maximumRequest))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		return "", errors.New("invalid AI request")
	}
	if err := applyRuntimeSettings(in); err != nil {
		return "", err
	}
	if err := configureUsage(in); err != nil {
		return "", err
	}
	if len(in.ImagePNG) > 0 {
		configuration, err := png.DecodeConfig(bytes.NewReader(in.ImagePNG))
		if err != nil || len(in.ImagePNG) > 32<<20 || configuration.Width <= 0 || configuration.Height <= 0 || int64(configuration.Width)*int64(configuration.Height) > 80_000_000 {
			return "", errors.New("invalid or oversized AI image")
		}
	}
	if strings.TrimSpace(in.Model) == "" {
		return "", errors.New("model is required")
	}
	if len(in.Messages) == 0 {
		if strings.TrimSpace(in.Text) == "" || len(in.Text) > 100000 {
			return "", errors.New("text is required or too long")
		}
	} else {
		if in.Text != "" || len(in.ImagePNG) > 0 || len(in.Messages) > 64 || in.Messages[len(in.Messages)-1].Role != "user" {
			return "", errors.New("invalid conversation")
		}
		total := len(in.System)
		for _, message := range in.Messages {
			if (message.Role != "user" && message.Role != "assistant") || strings.TrimSpace(message.Content) == "" {
				return "", errors.New("invalid conversation")
			}
			total += len(message.Content)
		}
		if total > 200000 {
			return "", errors.New("conversation too long")
		}
	}
	model := "winone/" + in.Model
	if in.Agent != "" {
		account, err := findAccount(ctx, in.Agent)
		if err != nil {
			return "", err
		}
		model = account.ID + "/" + in.Model
		provider.WinOneProviders = []provider.Provider{*account}
	} else {
		p, err := configuredProvider(in)
		if err != nil {
			return "", err
		}
		// The embedded provider hook excludes account discovery and other agents'
		// configuration. Secrets live only in this one-shot process's memory.
		provider.WinOneProviders = []provider.Provider{p}
	}
	var content any = in.Text
	if err := installModelPreferences(strings.TrimSuffix(model, "/"+in.Model), in.Model, in.ModelMetadata); err != nil {
		return "", err
	}
	if err := installAuxiliaryProviders(in); err != nil {
		return "", err
	}
	if err := configureUsage(in); err != nil {
		return "", err
	}
	if len(in.ImagePNG) > 0 {
		content = []any{map[string]any{"type": "text", "text": in.Text}, map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(in.ImagePNG)}}}
	}
	messages := []map[string]any{{"role": "system", "content": in.System}}
	if len(in.Messages) == 0 {
		messages = append(messages, map[string]any{"role": "user", "content": content})
	} else {
		for _, message := range in.Messages {
			messages = append(messages, map[string]any{"role": message.Role, "content": message.Content})
		}
	}
	requestBody := map[string]any{"model": model, "stream": emit != nil, "tools": []any{}, "tool_choice": "none", "messages": messages}
	applyModelMetadata(requestBody, "/v1/chat/completions", in.ModelMetadata)
	payload, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/v1/chat/completions", bytes.NewReader(payload)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+gateway.Token)
	if emit != nil {
		streamContext, cancel := context.WithCancel(ctx)
		defer cancel()
		response := newStreamResponse(emit)
		response.cancel = cancel
		gateway.New().Handler().ServeHTTP(response, req.WithContext(streamContext))
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", response.complete()
	}
	response := httptest.NewRecorder()
	gateway.New().Handler().ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		// Vendor bodies can repeat prompts or API keys. Only expose the status.
		return "", &upstreamFailure{Status: response.Code, RetryAfterSeconds: retryDelay(response.Header().Get("Retry-After"))}
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Finish string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &out); err != nil || len(out.Choices) != 1 {
		return "", errors.New("invalid AI response")
	}
	if out.Choices[0].Finish != "stop" && out.Choices[0].Finish != "end_turn" {
		return "", errors.New("AI response did not finish")
	}
	if strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", errors.New("AI returned empty text")
	}
	return out.Choices[0].Message.Content, nil
}

func main() {
	// The embedding host supplies an isolated XDG profile. Magpie's automatic
	// portable detection must not redirect it into a neighbouring data folder.
	appdir.UseExecutable("")
	if provider.TookOpenedURL(os.Args[1:]) {
		return
	}
	reader := bufio.NewReader(io.LimitReader(os.Stdin, maximumRequest))
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var in request
	if err := decoder.Decode(&in); err != nil {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": "invalid AI request"})
		os.Exit(1)
	}
	if in.Operation == "routing.serve" || in.Operation == "privacy.serve" {
		if err := serveRouting(context.Background(), in, reader, os.Stdout); err != nil {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": err.Error()})
			os.Exit(1)
		}
		return
	}
	if in.Operation == "usage.read" {
		if err := applyRuntimeSettings(in); err != nil {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": err.Error()})
			os.Exit(1)
		}
		if err := readUsage(in, os.Stdout); err != nil {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": err.Error()})
			os.Exit(1)
		}
		return
	}
	if err := applyRuntimeSettings(in); err != nil {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": err.Error()})
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), workerRequestWait(in))
	defer cancel()
	// Only an explicit opt-in reaches upstream statistics. Send after serving
	// the request; the daily stamp and NoUsageStats are Magpie's own behavior.
	if !settings.Load().NoStats {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = stats.Send(ctx, "winone-3964ea6", "embedded", time.Now())
		}()
	}
	if in.Operation == "magpie.catalog" || in.Operation == "models.test" || in.Operation == "magpie.maintain" {
		var err error
		switch in.Operation {
		case "magpie.catalog":
			err = magpieCatalog(ctx, in, os.Stdout)
		case "models.test":
			err = testModels(ctx, in, os.Stdout)
		case "magpie.maintain":
			err = maintainMagpie(ctx, in, os.Stdout)
		}
		if err != nil {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": err.Error()})
			os.Exit(1)
		}
		return
	}
	if strings.HasPrefix(in.Operation, "icons.") {
		icon, err := iconOperation(ctx, in)
		if err != nil {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": err.Error()})
			os.Exit(1)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"icon": icon})
		return
	}
	if in.Operation == "gateway" || in.Operation == "magpie.gateway" {
		if err := serveGateway(ctx, in, os.Stdout); err != nil {
			_ = json.NewEncoder(os.Stdout).Encode(gatewayEvent{Type: "error"})
			os.Exit(1)
		}
		return
	}
	if in.Operation == "models.fetch" {
		models, err := fetchModels(ctx, in)
		if err != nil {
			_ = json.NewEncoder(os.Stdout).Encode(failureEvent(err))
			os.Exit(1)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"models": models})
		return
	}
	if in.Operation != "" && in.Operation != "generate" {
		if err := accountOperation(ctx, in, io.MultiReader(decoder.Buffered(), reader)); err != nil {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": err.Error()})
			os.Exit(1)
		}
		return
	}
	encoded, _ := json.Marshal(in)
	if in.Stream {
		encoder := json.NewEncoder(os.Stdout)
		_, err := runOutput(ctx, bytes.NewReader(encoded), func(event streamEvent) error { return encoder.Encode(event) })
		if err != nil {
			_ = encoder.Encode(failureEvent(err))
			os.Exit(1)
		}
		return
	}
	text, err := run(ctx, bytes.NewReader(encoded))
	if err != nil {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": err.Error()})
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"text": text})
}
