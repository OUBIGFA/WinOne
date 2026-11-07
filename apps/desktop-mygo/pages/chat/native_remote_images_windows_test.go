//go:build windows

package chat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/chat/pluginhost"
	"winone/desktopmygo/core"
	"winone/desktopmygo/host"
	"winone/desktopmygo/kit"
	"winone/internal/ai"
)

// Explicit native window + real authenticated Host. Only the authored answer
// and local HTTP PNG are fixtures. The production loader must use image.preview,
// the original bounded remote import and a Host-issued immutable file preview;
// neither the native view nor the fixture calls an unrestricted external fetch.
func TestRealNativeRemoteMarkdownAndRichImages(t *testing.T) {
	if os.Getenv("WINONE_NATIVE_RICH_MATRIX") != "1" {
		t.Skip("explicit private desktop/native Host image entry")
	}
	client, err := host.NewClient(os.Getenv("WINONE_NATIVE_UI_REAL_PIPE"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Cancel()
	profile := os.Getenv("WINONE_NATIVE_UI_REAL_PROFILE")
	call := func(method string, params, out any) {
		t.Helper()
		if err := client.CallInto(method, params, out); err != nil {
			t.Fatalf("%s: %v", method, err)
		}
	}
	var configuration ai.Configuration
	call("capture.ai.configuration.get", nil, &configuration)
	configuration.Providers = []ai.Provider{{ID: "native-image", Name: "Isolated native image fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: "http://127.0.0.1:1", Models: []string{"model"}}}
	configuration.Tasks = map[string]ai.Selection{"chat": {Provider: "native-image", Model: "model"}, "default": {Provider: "native-image", Model: "model"}}
	call("capture.ai.configuration.save", map[string]any{"configuration": configuration, "keys": map[string]string{"native-image": "fixture-no-model-call"}}, nil)
	work := filepath.Join(filepath.Dir(profile), "remote-image-workspace")
	if err := os.MkdirAll(work, 0700); err != nil {
		t.Fatal(err)
	}
	var conversation struct{ ID string }
	call("chat.conversation.new", map[string]string{"directory": work, "title": "Native remote images"}, &conversation)
	var bot struct{ ID, Name string }
	call("chat.onebot.profiles.create", map[string]any{"input": map[string]string{"name": "Native image owner", "description": "Isolated image fixture", "locale": "en-US"}}, &bot)
	var botConversation struct{ ID string }
	call("chat.onebot.session.open", map[string]string{"botId": bot.ID}, &botConversation)
	if conversation.ID == "" || bot.ID == "" || botConversation.ID == "" {
		t.Fatal("actual Chat/OneBot image owners were not created")
	}
	ordinary, large := markdownImagePNG(t, 0), markdownImagePNG(t, 65<<20)
	var requests atomic.Uint32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/picture.png", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		if r.URL.Path == "/large.png" {
			_, _ = w.Write(large)
		} else {
			_, _ = w.Write(ordinary)
		}
	}))
	defer server.Close()
	var window *mygo.Window
	var page *Chat
	env := &core.Env{Host: client, Directory: profile, Activity: &core.Activity{}, Rail: &core.Rail{}, Refresh: func() {}, Toast: func(string) {}, Focuses: func() int { return 0 }, TakeIntents: func(string) []core.Intent { return nil }, Navigate: func(string, func()) {}, Open: func(string, core.Intent) {}, RequestChange: func(fn func()) { fn() }}
	env.Post = func(fn func()) {
		if window != nil && !window.IsDestroyed() {
			window.Update(fn)
		}
	}
	env.WebViews = func(options mygo.WebViewOptions) (*mygo.WebView, error) { return window.NewWebView(options) }
	mygo.RunOnMain(func() {
		window = mygo.NewWindow(mygo.WindowOptions{Title: "Native HTTP Markdown and rich pictures", Width: 560, Height: 900, Hidden: true, Content: ui.View(func(c *ui.Context) {
			c.SetTheme(kit.Theme(c.Preferences(), false))
			if page != nil {
				page.View(c)
			}
		})})
		window.SetContentSize(560, 900)
		window.Show()
	})
	defer mygo.RunOnMain(func() {
		if page != nil {
			page.Close()
		}
		pluginhost.For(env).Close()
		window.Destroy()
	})
	await := func(label string, check func() bool) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			ok := false
			mygo.RunOnMain(func() { ok = check() })
			if ok {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		mygo.RunOnMain(func() {
			t.Logf("actual native image owner=%s error=%q opening=%v", page.activeID, page.err, page.opening)
		})
		t.Fatalf("%s did not settle", label)
	}
	shots := os.Getenv("WINONE_NATIVE_UI_SHOTS")
	if err := os.MkdirAll(shots, 0700); err != nil {
		t.Fatal(err)
	}
	completed := 0
	for _, owner := range []struct{ name, id, prefix string }{{"chat", conversation.ID, "chat."}, {"onebot", botConversation.ID, "chat.onebot.chat."}} {
		mygo.RunOnMain(func() {
			if page != nil {
				page.Close()
			}
			if owner.name == "onebot" {
				page = NewOneBotConversation(env, owner.id, BotBinding{ID: bot.ID, Name: bot.Name})
			} else {
				page = New(env)
				page.setSidebar(false)
				page.show(owner.id)
			}
			window.Invalidate()
		})
		await(owner.name+" actual native image owner", func() bool { return page.activeID == owner.id && !page.opening && !page.draftLoading })
		mygo.RunOnMain(func() {
			// The rendered answer is the fixture. Keep real owner/lease/draft
			// authority, but stop an empty session poll from replacing its text.
			if page.session != nil {
				page.session.disposed = true
				if page.session.cancel != nil {
					page.session.cancel()
				}
			}
			page.updateText("Keep this unsent image-owner draft", nil, owner.id)
		})
		await("owner draft persisted", func() bool { return !page.saving })
		cases := []struct {
			name, address string
			rich          bool
		}{{"markdown", server.URL + "/redirect", false}, {"rich", server.URL + "/picture.png", true}}
		if owner.name == "chat" {
			cases = append(cases, struct {
				name, address string
				rich          bool
			}{"large-markdown", server.URL + "/large.png", false})
		}
		for _, sample := range cases {
			label := "Native remote " + owner.name + " " + sample.name
			answer := "![" + label + "](" + sample.address + ")"
			if sample.rich {
				op, _ := json.Marshal(map[string]any{"version": 1, "seq": 1, "op": "create", "id": "remote-picture", "kind": "image", "props": map[string]any{"src": sample.address, "alt": label, "caption": label}})
				answer = "```cindy-ui\n" + string(op) + "\n```"
			}
			mygo.RunOnMain(func() {
				page.view.Messages = []model.Message{{ID: owner.name + "-" + sample.name, Role: "assistant", Content: model.TextContent(answer)}}
				page.ui.followLatest()
				window.Invalidate()
			})
			var tree struct{ Nodes []richNativeNode }
			deadline := time.Now().Add(20 * time.Second)
			decoded := false
			for time.Now().Before(deadline) {
				if err := json.Unmarshal(richNativeCall(t, "tree", map[string]any{"window": window.ID()}), &tree); err != nil {
					t.Fatal(err)
				}
				for _, node := range tree.Nodes {
					decoded = decoded || node.Role == "image" && node.Label == label && !slices.Contains(node.States, "offscreen") && node.Bounds.Width >= 159 && node.Bounds.Height >= 95
				}
				if decoded {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if !decoded {
				t.Fatalf("%s did not decode the actual HTTP PNG in the native production view: %+v", label, tree.Nodes)
			}
			for _, node := range tree.Nodes {
				if node.Bounds.Width > 0 && (node.Bounds.X < -1 || node.Bounds.X+node.Bounds.Width > 561) {
					t.Fatalf("%s native content escaped the narrow horizontal viewport: %+v", label, node)
				}
			}
			frame, err := window.CapturePage()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(shots, owner.name+"-"+sample.name+".png"), frame, 0600); err != nil {
				t.Fatal(err)
			}
			nodes, _ := json.MarshalIndent(tree, "", "  ")
			if err := os.WriteFile(filepath.Join(shots, owner.name+"-"+sample.name+".json"), nodes, 0600); err != nil {
				t.Fatal(err)
			}
			completed++
			t.Logf("%s: actual bitmap 160x96 in 560 native window, frame=%d bytes", label, len(frame))
		}
		var draft struct {
			Text        string
			Attachments []model.Attachment
		}
		call(owner.prefix+"draft.get", map[string]string{"id": owner.id}, &draft)
		if draft.Text != "Keep this unsent image-owner draft" || len(draft.Attachments) != 0 {
			t.Fatalf("%s image display changed the actual owner's unsent draft: %+v", owner.name, draft)
		}
		var saved model.Snapshot
		call(owner.prefix+"state", map[string]string{"id": owner.id}, &saved)
		if len(saved.Messages) != 0 || saved.Streaming {
			t.Fatal("image display sent or queued a prompt")
		}
	}
	if completed != 5 || requests.Load() < 5 {
		t.Fatalf("missing actual remote image cases: %d renders, %d HTTP requests", completed, requests.Load())
	}
	t.Log(fmt.Sprintf("real Host HTTP redirect/import/immutable preview/native render: Chat and OneBot Markdown/rich; valid large PNG %d bytes >64 MiB <100 MiB; all 5 actual images and unchanged persisted drafts", len(large)))
}
