//go:build windows

package chat

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/chat/pluginhost"
	"winone/desktopmygo/chat/pluginhost/pluginpage"
	"winone/desktopmygo/core"
	"winone/desktopmygo/host"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// Real installed SDK deposit → original gallery → native preview gate/lightbox
// → production Chat import/draft acknowledgement, in one real Windows window.
// This test never sends the draft or uses the user's clipboard/accounts/profile.
func TestRealNativePluginMediaHandover(t *testing.T) {
	if os.Getenv("WINONE_NATIVE_RICH_MATRIX") != "1" {
		t.Skip("explicit private desktop/native Host media entry")
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
	directory := filepath.Join(filepath.Dir(profile), "media-chat")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	var conversation struct{ ID string }
	call("chat.conversation.new", map[string]string{"directory": directory, "title": "Native media handover"}, &conversation)
	if conversation.ID == "" {
		t.Fatal("real media destination was not created")
	}
	var existing struct{ Attachments []model.Attachment }
	existingPath := filepath.Join(directory, "existing-owner-note.txt")
	if err := os.WriteFile(existingPath, []byte("Keep the existing attachment"), 0600); err != nil {
		t.Fatal(err)
	}
	call("chat.attachments.add", map[string]any{"id": conversation.ID, "paths": []string{existingPath}}, &existing)
	if len(existing.Attachments) != 1 {
		t.Fatal("the real owner's original attachment was not imported")
	}
	image := pngBytes(t)
	manifest := map[string]any{"schemaVersion": 3, "id": "native-ui-media", "name": "Native media handover fixture", "version": "1.0.0", "description": "Isolated media handover", "minCindyVersion": "1.2.3", "entry": "main.js", "panel": map[string]any{"title": "Media source", "position": "left", "html": "panel.html"}, "cindy": map[string]any{"media": []string{"deposit"}}}
	encoded, _ := json.Marshal(manifest)
	files := map[string][]byte{"ghost.json": encoded, "owned.png": image,
		"main.js":    []byte(`const channel=new BroadcastChannel('native-media');channel.onmessage=async event=>{if(!event.data?.requestId)return;const blob=await(await fetch('owned.png')).blob();const data=await new Promise((resolve,reject)=>{const reader=new FileReader();reader.onload=()=>resolve(reader.result.split(',')[1]);reader.onerror=reject;reader.readAsDataURL(blob)});const result=await cindy.send({type:'cindy-request',kind:'deposit_media',data});channel.postMessage({reply:event.data.requestId,result})};`),
		"panel.html": []byte(`<!doctype html><html><head></head><body><button id="deposit">Deposit fixture media</button><img id="gallery"><script src="page.js"></script></body></html>`),
		"page.js":    []byte(`const channel=new BroadcastChannel('native-media');window.nativeMediaDeposit=()=>new Promise(resolve=>{const requestId=crypto.randomUUID();const receive=event=>{if(event.data?.reply===requestId){channel.removeEventListener('message',receive);resolve(event.data.result)}};channel.addEventListener('message',receive);channel.postMessage({requestId})});document.documentElement.dataset.realMedia='ready';`)}
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for name, data := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	packagePath := filepath.Join(filepath.Dir(profile), "native-ui-media.cindy")
	if err := os.WriteFile(packagePath, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	var inspection struct{ Token string }
	call("chat.ghosts.inspect", map[string]string{"path": packagePath, "language": "en-US"}, &inspection)
	var installed struct{ PluginID string }
	call("chat.ghosts.install", map[string]any{"token": inspection.Token, "confirmed": true}, &installed)
	var session struct{ URL string }
	call("chat.ghosts.page-session", map[string]string{"pluginId": installed.PluginID, "page": "panel", "language": "en-US"}, &session)
	var window *mygo.Window
	var chat *Chat
	var source *pluginpage.Page
	var jobs atomic.Int64
	outside, forceOutside := "Native focus guard", true
	env := &core.Env{Host: client, Directory: profile, Activity: &core.Activity{}, Rail: &core.Rail{}, Refresh: func() {}, Toast: func(string) {}, Focuses: func() int { return 0 }, TakeIntents: func(string) []core.Intent { return nil }, Navigate: func(string, func()) {}, Open: func(string, core.Intent) {}, RequestChange: func(fn func()) { fn() }}
	env.Post = func(fn func()) {
		if window != nil && !window.IsDestroyed() {
			window.Update(fn)
		}
	}
	env.Spawn = func(fn func()) { jobs.Add(1); go func() { defer jobs.Add(-1); fn() }() }
	env.WebViews = func(options mygo.WebViewOptions) (*mygo.WebView, error) { return window.NewWebView(options) }
	mygo.RunOnMain(func() {
		window = mygo.NewWindow(mygo.WindowOptions{Title: "Native plugin media handover", Width: 1200, Height: 900, Hidden: true, Content: ui.View(func(c *ui.Context) {
			c.SetTheme(kit.Theme(c.Preferences(), false))
			ui.Row(c).Fill().MinWidth(0).Children(func() {
				ui.Column(c).Width(280).FillHeight().MinWidth(0).Children(func() {
					field := ui.TextInput(c.Key("native-focus-guard"), &outside).FillWidth().Label("Native focus guard")
					if forceOutside {
						field.Focus()
					}
					if source != nil {
						source.View(c)
					}
				})
				ui.Column(c).Grow(1).Basis(0).FillHeight().MinWidth(0).Children(func() {
					if chat != nil {
						chat.View(c)
					}
				})
			})
			pluginhost.For(env).View(c)
		})})
		chat = New(env)
		chat.setSidebar(false)
		chat.show(conversation.ID)
		var err error
		source, err = pluginpage.New(env, session.URL, "Original plugin media panel", installed.PluginID)
		if err != nil {
			t.Error(err)
		}
		window.Show()
		window.Focus()
	})
	if source == nil {
		t.Fatal("real plugin source is unavailable")
	}
	defer mygo.RunOnMain(func() { source.Close(); chat.Close(); pluginhost.For(env).Close(); window.Destroy() })
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
		mygo.RunOnMain(func() { t.Logf("native media owner error=%q opening=%v files=%+v", chat.err, chat.opening, chat.files) })
		t.Fatalf("%s did not settle", label)
	}
	await("real native Chat owner", func() bool { return chat.activeID == conversation.ID && !chat.opening && !chat.draftLoading })
	mygo.RunOnMain(func() { chat.updateText("Keep the owner's unsent draft", existing.Attachments, conversation.ID) })
	await("real owner draft saved", func() bool { return !chat.saving })
	deadline := time.Now().Add(15 * time.Second)
	loaded := false
	for time.Now().Before(deadline) {
		value, err := source.WebView().Page().Eval(`return document.readyState==='complete'&&document.documentElement.dataset.realMedia==='ready'`)
		if err == nil && value == true {
			loaded = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !loaded {
		t.Fatal("original plugin media DOM did not load")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	deposited, err := source.WebView().Page().EvalContext(ctx, `return await window.nativeMediaDeposit()`)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if result, ok := deposited.(map[string]any); !ok || result["ok"] != true {
		t.Fatalf("original SDK deposit failed: %v", deposited)
	}
	t.Logf("actual original SDK deposit: %v", deposited)
	value, err := source.WebView().Page().Eval(`return await (await fetch('/gallery')).json()`)
	if err != nil {
		t.Fatal(err)
	}
	gallery, ok := value.([]any)
	if !ok || len(gallery) != 0 {
		t.Fatalf("a user deposit was incorrectly published as a generated gallery work: %v", value)
	}
	// The original deposit receipt explicitly grants panel media/<hash><ext>;
	// deposits must not pollute ghost-gallery, whose lifecycle is different.
	deposit := deposited.(map[string]any)
	hash, hashOK := deposit["hash"].(string)
	ext, extOK := deposit["ext"].(string)
	if !hashOK || len(hash) != 64 || !extOK || ext != ".png" {
		t.Fatalf("actual deposit lost its scoped media identity: %v", deposit)
	}
	uri := "cindy-ghost://native-ui-media/media/" + hash + ext
	raw, _ := json.Marshal(uri)
	if _, err := source.WebView().Page().Eval(`document.getElementById('gallery').src=` + string(raw) + `;return true`); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(10 * time.Second)
	decoded := false
	for time.Now().Before(deadline) {
		value, err := source.WebView().Page().Eval(`return document.getElementById('gallery').complete&&document.getElementById('gallery').naturalWidth===2`)
		if err == nil && value == true {
			decoded = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !decoded {
		t.Fatal("actual ledger image did not decode in the source WebView")
	}
	visible := func(label string) bool {
		var tree struct{ Nodes []richNativeNode }
		if err := json.Unmarshal(richNativeCall(t, "tree", map[string]any{"window": window.ID()}), &tree); err != nil {
			t.Fatal(err)
		}
		for _, node := range tree.Nodes {
			if node.Label == label && !slices.Contains(node.States, "offscreen") {
				return true
			}
		}
		return false
	}
	// Use the real focused native field and OnWillNavigate delivery. Merely
	// owning the root foreground must not let a blurred guest ask the Gate.
	await("native field, not source guest, owns focus", func() bool { return window.IsFocused() && !source.IsFocused() && jobs.Load() == 0 })
	var navigation atomic.Uint32
	off := source.WebView().Page().OnWillNavigate(func(*mygo.NavigateEvent) { navigation.Add(1) })
	defer off()
	external := "https://untrusted.example.invalid/a%20b"
	navigate := func(address string) {
		t.Helper()
		raw, _ := json.Marshal(address)
		if _, err := source.WebView().Page().Eval(`location.href=` + string(raw) + `;return true`); err != nil {
			t.Fatal(err)
		}
	}
	type gatePrompt struct {
		ID      string
		Request struct{ Kind, Body, GhostName, CancelText string }
	}
	readPrompts := func() []gatePrompt {
		var pending struct{ Prompts []gatePrompt }
		call("chat.ghosts.pending", map[string]int{"after": 0}, &pending)
		return pending.Prompts
	}
	navigate(external)
	await("blurred guest navigation canceled", func() bool { return navigation.Load() == 1 && jobs.Load() == 0 })
	if prompts := readPrompts(); len(prompts) != 0 {
		t.Fatalf("blurred native guest asked for external permission: %+v", prompts)
	}
	mygo.RunOnMain(func() { forceOutside = false; window.Invalidate() })
	await("real source embedded focus", func() bool { window.Focus(); source.Focus(); return source.IsFocused() })
	navigate(external)
	var confirm gatePrompt
	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		for _, prompt := range readPrompts() {
			if prompt.Request.Kind == "confirm" && prompt.Request.Body == external {
				confirm = prompt
			}
		}
		if confirm.ID != "" && visible(confirm.Request.CancelText) && visible(external) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if confirm.ID == "" || confirm.Request.GhostName != "Native media handover fixture" || !visible(external) {
		t.Fatalf("actual Host external-link Gate did not reach its owner native confirmation: %+v", confirm)
	}
	externalShot, err := window.CapturePage()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("WINONE_NATIVE_UI_SHOTS"), "native-external-gate.png"), externalShot, 0600); err != nil {
		t.Fatal(err)
	}
	var confirmationTree struct{ Nodes []richNativeNode }
	_ = json.Unmarshal(richNativeCall(t, "tree", map[string]any{"window": window.ID()}), &confirmationTree)
	canceled := false
	for _, node := range confirmationTree.Nodes {
		if node.Label == confirm.Request.CancelText && slices.Contains(node.Actions, "press") && !slices.Contains(node.States, "disabled") && !slices.Contains(node.States, "offscreen") {
			richNativeCall(t, "act", map[string]any{"window": window.ID(), "id": node.ID, "action": "press"})
			canceled = true
			break
		}
	}
	if !canceled {
		t.Fatal("the actual owner native Cancel control is absent")
	}
	await("native external confirmation canceled", func() bool { return jobs.Load() == 0 })
	if prompts := readPrompts(); len(prompts) != 0 || source.WebView().Page().URL() != session.URL {
		t.Fatalf("cancel left an external request or escaped the private guest: %+v URL=%s", prompts, source.WebView().Page().URL())
	}
	t.Log("real native field denied blurred navigation; focused guest reached the original normalized external Gate; native Cancel acknowledged, no system browser open")
	await("source focus restored after external Cancel", func() bool { window.Focus(); source.Focus(); return source.IsFocused() })
	preview := strings.Replace(uri, "/media/", "/preview/", 1)
	navigate(preview)
	deadline = time.Now().Add(20 * time.Second)
	opened := false
	for time.Now().Before(deadline) {
		if visible(i18n.T("pluginViews.sendToChat")) {
			opened = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !opened {
		t.Fatal("real preview-media event did not open the production native lightbox")
	}
	var tree struct{ Nodes []richNativeNode }
	_ = json.Unmarshal(richNativeCall(t, "tree", map[string]any{"window": window.ID()}), &tree)
	button := ""
	for _, node := range tree.Nodes {
		if node.Label == i18n.T("pluginViews.sendToChat") {
			button = node.ID
			break
		}
	}
	if button == "" {
		t.Fatal("actual native media handover button is absent")
	}
	before, err := window.CapturePage()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("WINONE_NATIVE_UI_SHOTS"), "native-media-preview.png"), before, 0600); err != nil {
		t.Fatal(err)
	}
	mygo.RunOnMain(func() {
		if chat.text != "Keep the owner's unsent draft" || len(chat.files) != 1 || chat.files[0].Path != existing.Attachments[0].Path {
			t.Error("a preview or ledger lookup changed the draft before native confirmation")
		}
	})
	richNativeCall(t, "act", map[string]any{"window": window.ID(), "id": button, "action": "press"})
	await("real production Chat import acknowledgement", func() bool { return len(chat.files) == 2 && !chat.busy && !chat.saving })
	if visible(i18n.T("pluginViews.sendToChat")) {
		t.Fatal("native preview did not receive its successful import/draft acknowledgement")
	}
	mygo.RunOnMain(func() {
		if chat.text != "Keep the owner's unsent draft" {
			t.Error("media handover replaced or sent the owner's draft")
		}
	})
	var imported string
	mygo.RunOnMain(func() { imported = chat.files[1].Path })
	data, err := os.ReadFile(imported)
	if err != nil || !bytes.Equal(data, image) {
		t.Fatalf("real attachment import did not retain ledger bytes: %q %v", imported, err)
	}
	var draft struct {
		Text        string
		Attachments []struct{ Path string }
	}
	call("chat.draft.get", map[string]string{"id": conversation.ID}, &draft)
	if draft.Text != "Keep the owner's unsent draft" || len(draft.Attachments) != 2 || draft.Attachments[0].Path != existing.Attachments[0].Path || draft.Attachments[1].Path != imported {
		t.Fatalf("actual persisted draft/attachment differs: %+v", draft)
	}
	var state model.Snapshot
	call("chat.state", map[string]string{"id": conversation.ID}, &state)
	if len(state.Messages) != 0 || state.Streaming {
		t.Fatal("media handover sent or queued a message instead of merging the unsent draft")
	}
	after, err := window.CapturePage()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("WINONE_NATIVE_UI_SHOTS"), "native-media-chat-draft.png"), after, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("original SDK user deposit/ledger URI/native preview → user confirm → actual Chat attachments.add + retained text/existing attachment + persisted draft ACK; imported bytes=%d; opaque source %s; native source=%s", len(data), uri, source.ID())
}
