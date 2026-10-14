//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"winone/internal/ai"
	"winone/internal/chat"
)

func TestTitleRetriesOnlyAnUnansweredTransientProviderFailure(t *testing.T) {
	for _, sample := range []struct {
		name, partial string
		status, want  int
	}{
		{"unavailable", "", 503, 2}, {"rate-limit", "", 429, 2}, {"bad-request", "", 400, 1}, {"authentication", "", 401, 1}, {"partial-result", "部分标题", 503, 1},
	} {
		t.Run(sample.name, func(t *testing.T) {
			calls := 0
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, err := generateTitleText(ctx, func(context.Context) (string, error) {
				calls++
				return sample.partial, &ai.AttemptFailure{Status: sample.status}
			})
			if calls != sample.want || err == nil {
				t.Fatalf("calls=%d error=%v", calls, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	_, err := generateTitleText(ctx, func(context.Context) (string, error) { calls++; cancel(); return "", &ai.AttemptFailure{Status: 503} })
	if calls != 1 || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled retry: calls=%d error=%v", calls, err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	calls = 0
	_, err = generateTitleText(ctx, func(context.Context) (string, error) {
		calls++
		return "", &ai.AttemptFailure{Status: 429, RetryAfter: time.Minute}
	})
	if calls != 1 || err == nil {
		t.Fatalf("Retry-After exceeded title budget: calls=%d error=%v", calls, err)
	}
}

func TestTitleGenerationUsesSavedTitleModelThroughRealMagpie(t *testing.T) {
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("explicit isolated AI bundle")
	}
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string `json:"model"`
			Stream   bool   `json:"stream"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "title-model" || r.Header.Get("Authorization") != "Bearer title-fixture" {
			t.Error("saved title model or credential did not reach provider")
		}
		var prompt string
		for _, message := range body.Messages {
			if message.Role == "user" {
				prompt = message.Content
			}
		}
		auto := strings.Contains(prompt, "<user_message>\n调整链接浏览窗口和鼠标手势\n</user_message>")
		regenerate := strings.Contains(prompt, "<recent_conversation>\nUser: 修复已保存的链接访问\nAssistant: 已通过验证\nUser: 继续\n</recent_conversation>")
		if (!auto && !regenerate) || !strings.Contains(prompt, "Output only the title") || strings.Contains(prompt, "客户端传入的过期文本") {
			t.Error("title model received the task as a conversation turn instead of Cindy's one-shot prompt")
		}
		if calls.Add(1) == 1 {
			http.Error(w, "fixture temporary failure", http.StatusServiceUnavailable)
			return
		}
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"链接设置与鼠标手势\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		} else {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"链接设置与鼠标手势"},"finish_reason":"stop"}]}`)
		}
	}))
	defer upstream.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	root := t.TempDir()
	service := newAIService(ctx, root, filepath.Join(root, "WinOne.Host.exe"))
	service.store, _ = ai.Open(service.storePath)
	config := service.store.Configuration()
	config.Providers = []ai.Provider{{ID: "title-provider", Name: "Title fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: upstream.URL, Models: []string{"dialogue-model", "title-model"}}}
	key := "title-fixture"
	if _, err := service.store.SaveConfiguration(config, map[string]*string{"title-provider": &key}); err != nil {
		t.Fatal(err)
	}
	service.translator = ai.Translator{Executable: executable, DataDirectory: filepath.Join(root, "runtime")}
	store, err := chat.Open(filepath.Join(root, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	prefs := store.Preferences()
	prefs.TitleModel = ai.Selection{Provider: "title-provider", Model: "title-model"}
	if _, err = store.SavePreferences(prefs); err != nil {
		t.Fatal(err)
	}
	conversation, err := store.NewConversation("", root, "Chat", ai.Selection{Provider: "title-provider", Model: "dialogue-model"})
	if err != nil {
		t.Fatal(err)
	}
	control := &chatControl{ctx: ctx, store: store, ai: service, language: func() string { return "zh-CN" }}
	generated, err := control.generateTitle(ctx, conversation.ID, "调整链接浏览窗口和鼠标手势")
	if err != nil || generated.Title != "链接设置与鼠标手势" {
		t.Fatal(generated.Title, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("title transient failure was not retried once: %d", calls.Load())
	}
	if _, err = store.RenameConversation(conversation.ID, "我的标题"); err != nil {
		t.Fatal(err)
	}
	manual, err := control.generateTitle(ctx, conversation.ID, "another request")
	if err != nil || manual.Title != "我的标题" {
		t.Fatal(manual.Title, err)
	}
	// Regeneration obtains its material from the saved Pi branch and uses the
	// same standalone provider call, never a continuation of the user's task.
	file := filepath.Join(conversation.SessionDirectory, "title.jsonl")
	data := `{"type":"message","id":"first","parentId":null,"message":{"role":"user","content":"修复已保存的链接访问"}}
{"type":"message","id":"reply","parentId":"first","message":{"role":"assistant","content":[{"type":"text","text":"已通过验证"}]}}
{"type":"message","id":"last","parentId":"reply","message":{"role":"user","content":"继续"}}
`
	if err = os.WriteFile(file, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = store.BindSession(conversation.ID, file); err != nil {
		t.Fatal(err)
	}
	regenerated, err := control.generateTitle(ctx, conversation.ID, "客户端传入的过期文本", true)
	if err != nil || regenerated.Title != "链接设置与鼠标手势" || !regenerated.TitleManual {
		t.Fatal(regenerated, err)
	}
}

func TestProjectPermissionThroughHostUpdatesEveryConversation(t *testing.T) {
	root := t.TempDir()
	store, err := chat.Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.CreateProject(root, "Project")
	if err != nil {
		t.Fatal(err)
	}
	first, _ := store.NewConversation(project.ID, "", "First", ai.Selection{})
	second, _ := store.NewConversation(project.ID, "", "Second", ai.Selection{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	control := &chatControl{ctx: ctx, store: store, engine: chat.NewService(ctx, store, chat.ServiceOptions{})}
	catalog, err := control.configureProjectPermission(ctx, project.ID, "auto")
	if err != nil {
		t.Fatal(err)
	}
	for _, conversation := range catalog.Conversations {
		if (conversation.ID == first.ID || conversation.ID == second.ID) && conversation.Permission != "auto" {
			t.Fatal(conversation)
		}
	}
}
