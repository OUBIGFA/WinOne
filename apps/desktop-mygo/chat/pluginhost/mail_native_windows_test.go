//go:build windows

package pluginhost

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

func TestMain(m *testing.M) {
	if os.Getenv("WINONE_MAIL_NATIVE") != "1" {
		os.Exit(m.Run())
	}
	profile := os.Getenv("WINONE_MAIL_NATIVE_PROFILE")
	if !filepath.IsAbs(profile) || !strings.Contains(filepath.ToSlash(profile), "/_temp/") {
		fmt.Fprintln(os.Stderr, "Use the explicit private-desktop mail profile")
		os.Exit(1)
	}
	mygo.App.SetName("WinOne Native Mail Check")
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(profile, "native-browser"))
	mygo.App.OnWindowAllClosed(func() {})
	code := 1
	mygo.App.WhenReady(func() { go func() { code = m.Run(); mygo.App.Quit() }() })
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

type nativeMailFixture struct {
	BaseURL   string `json:"baseURL"`
	Key       string `json:"key"`
	SessionID string `json:"sessionID"`
	CallID    string `json:"callID"`
	Source    string `json:"source"`
	directory string
}
type nativeMailState struct {
	Provider struct {
		BodyHTML                 string `json:"bodyHtml"`
		Body                     string `json:"body"`
		Writes, Sends, MarkReads int
		Attachments              []mailAttachment
	}
	Calls []struct {
		Method string
		Params json.RawMessage
	}
	ExternalRequests int
}

func startNativeMailFixture(t *testing.T) nativeMailFixture {
	t.Helper()
	repository, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	directory, err := os.MkdirTemp(os.Getenv("WINONE_MAIL_NATIVE_PROFILE"), "backend-")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(filepath.Join(repository, "dist/resources/tools/bun-1.4.2/bun.exe"), filepath.Join(repository, "modules/chat-pi/ghost/fixtures/mail-native-host.mjs"), directory)
	command.Dir = repository
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	diagnostic, err := os.Create(filepath.Join(directory, "backend-stderr.log"))
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = diagnostic
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	t.Cleanup(func() {
		_ = stdin.Close()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			_ = command.Process.Kill()
			<-done
		}
		_ = diagnostic.Close()
	})
	line := make(chan []byte, 1)
	go func() { reader := bufio.NewReader(stdout); value, _ := reader.ReadBytes('\n'); line <- value }()
	var fixture nativeMailFixture
	select {
	case value := <-line:
		if json.Unmarshal(value, &fixture) != nil || fixture.CallID == "" {
			t.Fatalf("Actual mail backend did not publish a card; see %s", diagnostic.Name())
		}
	case <-time.After(40 * time.Second):
		t.Fatalf("Mail backend startup did not settle; see %s", diagnostic.Name())
	}
	fixture.directory = directory
	return fixture
}
func (f nativeMailFixture) request(route string, input any, out any) error {
	var body io.Reader
	method := "GET"
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
		method = "POST"
	}
	request, err := http.NewRequest(method, f.BaseURL+route, body)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+f.Key)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
		return fmt.Errorf("actual fixture response %d: %s", response.StatusCode, data)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(out)
}
func awaitNativeMail(t *testing.T, label string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(18 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("%s did not settle", label)
}
func mailNativeBool(page *mygo.Page, expression string) bool {
	value, err := page.Eval(expression)
	return err == nil && value == true
}
func mailNativeWindow(t *testing.T, fixture nativeMailFixture) (*Host, *mygo.Window, *bool) {
	t.Helper()
	harness := testhost.New(t, func(method string, params json.RawMessage) (any, error) {
		var result any
		err := fixture.request("/call", map[string]any{"method": method, "params": json.RawMessage(params)}, &result)
		return result, err
	})
	previous := i18n.Locale()
	i18n.SetLocale(i18n.English)
	t.Cleanup(func() { i18n.SetLocale(previous) })
	host := For(harness.Env)
	dark, enabled := false, false
	window := mygo.NewWindow(mygo.WindowOptions{Title: "WinOne Native Mail Check", Width: 960, Height: 840, Hidden: true, Content: ui.View(func(c *ui.Context) {
		c.SetTheme(kit.Theme(c.Preferences(), dark))
		if !enabled {
			return
		}
		ui.Scroll(c).Fill().Padding(10).Children(func() { host.Card(c, fixture.CallID) })
		host.View(c) // Production Root lifecycle, not a test-only MailView call.
	})})
	harness.Env.Directory = filepath.Join(fixture.directory, "native-window")
	harness.Env.Post = func(fn func()) {
		if !window.IsDestroyed() {
			window.Update(fn)
		}
	}
	harness.Env.WebViews = window.NewWebView
	t.Cleanup(func() { mygo.RunOnMain(func() { host.Close(); host.MailClose(); window.Destroy() }) })
	window.Update(func() { enabled = true })
	window.Show()
	window.Focus()
	if window.NativeHandle() == 0 {
		t.Fatal("mail UI has no real OS window")
	}
	return host, window, &dark
}
func traceNativeMail(t *testing.T, fixture nativeMailFixture, host *Host) {
	t.Helper()
	t.Cleanup(func() {
		var state nativeMailState
		if fixture.request("/state", nil, &state) == nil {
			data, _ := json.MarshalIndent(state, "", "  ")
			_ = os.WriteFile(filepath.Join(fixture.directory, "native-state.json"), data, 0600)
		}
		if t.Failed() {
			mygo.RunOnMain(func() {
				for id, item := range host.cards.byCall {
					t.Logf("card %s busy=%v dirty=%v sending=%v error=%q waiters=%d", id, item.busy, item.formDirty, item.formSending, item.err, len(item.formWaiters))
				}
			})
		}
	})
}

func nativeMailDocument(t *testing.T, host *Host, callID, location string) *mailHTMLDocument {
	t.Helper()
	var document *mailHTMLDocument
	deadline := time.Now().Add(18 * time.Second)
	for time.Now().Before(deadline) {
		ready := false
		mygo.RunOnMain(func() {
			document = host.cards.mail.documents[callID+":bodyHtml:"+location]
			ready = document != nil && !document.closed.Load() && document.ready && document.view != nil
		})
		if ready {
			return document
		}
		time.Sleep(25 * time.Millisecond)
	}
	mygo.RunOnMain(func() {
		if item := host.cards.byCall[callID]; item != nil {
			t.Logf("mail card loading=%v fields=%d mail=%v error=%q", item.loading, len(item.Fields), item.Mail != nil, item.err)
		}
		for id, doc := range host.cards.mail.documents {
			t.Logf("document %s ready=%v loading=%v error=%q view=%v", id, doc.ready, doc.loading, doc.err, doc.view != nil)
		}
	})
	if document != nil && document.view != nil {
		value, err := document.view.Page().Eval(`return {url:location.href,state:document.readyState,marker:document.querySelector('meta[name="winone-mail-document"]')?.content,helpers:typeof WinOneMail,binding:typeof __winoneMailBind,body:document.body.innerText.slice(0,160)}`)
		t.Logf("actual native document: %v %v", value, err)
	}
	t.Fatal("actual mail HTML editor did not settle")
	return nil
}

// Actual native context + locked sanitizer/serializer + Go form delivery +
// actual SDK/provider. Eval drives deterministic input/IME events, not user
// permission gestures or a claim about a physical IME device.
func TestNativeMailHTMLUsesOriginalSerializerCIDAutosaveAndHostLifecycle(t *testing.T) {
	if os.Getenv("WINONE_MAIL_NATIVE") != "1" {
		t.Skip("explicit private desktop and real WebView2")
	}
	fixture := startNativeMailFixture(t)
	host, window, dark := mailNativeWindow(t, fixture)
	traceNativeMail(t, fixture, host)
	document := nativeMailDocument(t, host, fixture.CallID, "inline")
	page := document.view.Page()
	awaitNativeMail(t, "actual CID preview image", func() bool {
		return mailNativeBool(page, `Array.from(document.querySelectorAll('img[data-cindy-mail-cid]')).some(image=>image.complete&&image.naturalWidth>0)`)
	})
	if !mailNativeBool(page, `!globalThis.mailAuthoredScriptRan&&!globalThis.mailAuthoredEventRan&&!globalThis.cindy&&document.querySelectorAll('script,[onclick]').length===0`) {
		t.Fatal("mail-authored script/event or plugin SDK reached native editor")
	}
	if mailNativeBool(page, `document.documentElement.outerHTML.includes(`+nativeMailQuote(fixture.Key)+`)`) {
		t.Fatal("native transport key entered the mail document")
	}
	var baseline nativeMailState
	if err := fixture.request("/state", nil, &baseline); err != nil {
		t.Fatal(err)
	}
	if baseline.Provider.Writes != 0 {
		t.Fatal("mounting/sanitizing a preview wrote the provider draft")
	}
	if _, err := page.Eval(`window.mailEditingNode=document.querySelector('b');document.dispatchEvent(new CompositionEvent('compositionstart',{bubbles:true}));mailEditingNode.textContent='Unfinished IME';mailEditingNode.dispatchEvent(new InputEvent('input',{bubbles:true}));return true`); err != nil {
		t.Fatal(err)
	}
	time.Sleep(250 * time.Millisecond)
	var composing nativeMailState
	if err := fixture.request("/state", nil, &composing); err != nil {
		t.Fatal(err)
	}
	if composing.Provider.Writes != baseline.Provider.Writes {
		t.Fatal("partial IME text reached autosave")
	}
	if _, err := page.Eval(`mailEditingNode.textContent='Typed body 😀';document.dispatchEvent(new CompositionEvent('compositionend',{bubbles:true}));return true`); err != nil {
		t.Fatal(err)
	}
	var saved nativeMailState
	awaitNativeMail(t, "original provider autosave", func() bool {
		return fixture.request("/state", nil, &saved) == nil && saved.Provider.Writes > baseline.Provider.Writes && strings.Contains(saved.Provider.BodyHTML, "Typed body 😀")
	})
	if saved.Provider.Sends != 0 {
		t.Fatal("editing a draft sent mail")
	}
	if !strings.Contains(saved.Provider.BodyHTML, ".signature{color:#234567}") || !strings.Contains(saved.Provider.BodyHTML, "cid:original@fixture") || strings.Contains(saved.Provider.BodyHTML, "winone-mail.localhost") || strings.Contains(saved.Provider.BodyHTML, "cindy-media:") {
		t.Fatalf("serializer lost styles/CID or saved native preview URLs: %s", saved.Provider.BodyHTML)
	}
	awaitNativeMail(t, "autosave retains the actual editing node", func() bool {
		return mailNativeBool(page, `document.querySelector('b')===window.mailEditingNode&&mailEditingNode.textContent==='Typed body 😀'`)
	})
	mygo.RunOnMain(func() { window.Update(func() { *dark = true }) })
	awaitNativeMail(t, "original preview theme paint", func() bool { return mailNativeBool(page, `document.documentElement.classList.contains('dark')`) })
	mygo.RunOnMain(func() { window.Update(func() { *dark = false }); window.SetSize(560, 840) })
	awaitNativeMail(t, "narrow native HTML has no horizontal overflow", func() bool {
		return mailNativeBool(page, `document.documentElement.scrollWidth<=document.documentElement.clientWidth+1&&document.body.scrollWidth<=document.documentElement.clientWidth+1`)
	})
	if data, err := document.view.CapturePage(); err == nil {
		if err = os.WriteFile(filepath.Join(fixture.directory, "native-mail-light-narrow.png"), data, 0600); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Fatal(err)
	}
	var afterTheme nativeMailState
	if err := fixture.request("/state", nil, &afterTheme); err != nil {
		t.Fatal(err)
	}
	if afterTheme.Provider.Writes != saved.Provider.Writes {
		t.Fatal("theme/width change saved preview paint as authored mail")
	}
	// Selection and MIME mutations use the same production card-action route.
	if err := fixture.request("/selection", map[string]any{}, nil); err != nil {
		t.Fatal(err)
	}
	mygo.RunOnMain(func() {
		item := host.cards.byCall[fixture.CallID]
		host.cardDispatch(item, "card-action", map[string]any{"action": "mail:attach:" + item.Mail.Key})
	})
	var withAttachment nativeMailState
	awaitNativeMail(t, "native attachment picker/snapshot/provider write", func() bool {
		return fixture.request("/state", nil, &withAttachment) == nil && len(withAttachment.Provider.Attachments) == 2
	})
	var added mailAttachment
	for _, file := range withAttachment.Provider.Attachments {
		if file.Key != "original" {
			added = file
		}
	}
	awaitNativeMail(t, "native card attachment broadcast", func() bool {
		ok := false
		mygo.RunOnMain(func() {
			item := host.cards.byCall[fixture.CallID]
			ok = strings.Contains(item.values["attachmentItems"], added.Key)
		})
		return ok
	})
	mygo.RunOnMain(func() { host.mailMutate(host.cards.byCall[fixture.CallID], "insert", "attachmentItems", added.Key) })
	awaitNativeMail(t, "original MIME insertion in provider HTML", func() bool {
		return fixture.request("/state", nil, &withAttachment) == nil && strings.Contains(withAttachment.Provider.BodyHTML, "image-1@cindy.local")
	})
	mygo.RunOnMain(func() { host.mailMutate(host.cards.byCall[fixture.CallID], "remove", "attachmentItems", added.Key) })
	awaitNativeMail(t, "original MIME removal in provider HTML", func() bool {
		return fixture.request("/state", nil, &withAttachment) == nil && len(withAttachment.Provider.Attachments) == 1 && !strings.Contains(withAttachment.Provider.BodyHTML, "image-1@cindy.local")
	})
	mygo.RunOnMain(func() { host.Close() })
	if !document.closed.Load() || !document.view.IsDestroyed() {
		t.Fatal("production Host.Close did not release native mail context")
	}
	mailDocuments.Lock()
	_, mounted := mailDocuments.byID[document.id]
	mailDocuments.Unlock()
	if mounted {
		t.Fatal("closed mail retained its native image capability")
	}
	t.Log("actual SDK/provider + original HTML sanitizer/serializer, CID bytes, full form autosave, IME event barrier, unchanged editing node, theme/narrow, attachment insert/remove, production Root Close")
}

func TestNativeMailReaderExplicitOpenThreadsExternalOptInLongBodyAndOpenDraft(t *testing.T) {
	if os.Getenv("WINONE_MAIL_NATIVE") != "1" {
		t.Skip("explicit private desktop and real WebView2")
	}
	fixture := startNativeMailFixture(t)
	host, window, _ := mailNativeWindow(t, fixture)
	traceNativeMail(t, fixture, host)
	_ = nativeMailDocument(t, host, fixture.CallID, "inline")
	mygo.RunOnMain(func() { host.OpenMail(fixture.SessionID, "https://mail.example.invalid/selected") })
	var reader *mailReaderView
	awaitNativeMail(t, "production Root mail reader", func() bool {
		ok := false
		mygo.RunOnMain(func() {
			reader = host.cards.mail.reader
			ok = reader != nil && !reader.loading && len(reader.rows) == 8
		})
		return ok
	})
	var state nativeMailState
	awaitNativeMail(t, "Reader body views", func() bool { return fixture.request("/state", nil, &state) == nil && state.Provider.MarkReads == 1 })
	if state.ExternalRequests != 0 {
		t.Fatal("Reader fetched external images before explicit opt-in")
	}
	var selected *mailReaderRow
	mygo.RunOnMain(func() {
		for _, row := range reader.rows {
			if row.summary.MessageID == "selected" {
				selected = row
			}
		}
	})
	if selected == nil || selected.card == nil {
		t.Fatal("selected Reader message was not presented")
	}
	document := nativeMailDocument(t, host, selected.card.CallID, "expanded")
	page := document.view.Page()
	if !mailNativeBool(page, `document.body.contentEditable==='false'&&Array.from(document.querySelectorAll('img')).filter(i=>!i.hasAttribute('data-cindy-mail-cid')).every(i=>!i.getAttribute('src'))`) {
		t.Fatal("Reader body was editable or unsolicited images were enabled")
	}
	mygo.RunOnMain(func() {
		document.allowExternal = true
		document.signature = ""
		document.update(document.source, document.images, true, document.theme)
	})
	awaitNativeMail(t, "explicit original external image opt-in", func() bool { return fixture.request("/state", nil, &state) == nil && state.ExternalRequests > 0 })
	awaitNativeMail(t, "opted-in image did not block the actual body", func() bool {
		return mailNativeBool(page, `Array.from(document.querySelectorAll('img[data-cindy-mail-external-src]')).some(i=>i.complete&&i.naturalWidth>0)`)
	})
	var earlier *mailReaderRow
	mygo.RunOnMain(func() {
		earlier = reader.rows[0]
		earlier.visible = true
		earlier.expanded = true
		host.readMailRow(reader, earlier, true)
		window.Invalidate()
	})
	awaitNativeMail(t, "explicit thread expansion markRead", func() bool { return fixture.request("/state", nil, &state) == nil && state.Provider.MarkReads == 2 })
	mygo.RunOnMain(func() { host.OpenMail(fixture.SessionID, "https://mail.example.invalid/large") })
	awaitNativeMail(t, "Reader larger than compose limits", func() bool {
		ok := false
		mygo.RunOnMain(func() {
			reader = host.cards.mail.reader
			ok = reader != nil && !reader.loading && len(reader.rows) == 1 && reader.rows[0].card != nil
		})
		return ok
	})
	large := nativeMailDocument(t, host, reader.rows[0].card.CallID, "expanded")
	awaitNativeMail(t, "whole long read-only document", func() bool {
		return mailNativeBool(large.view.Page(), `document.body.textContent.length>400000&&document.body.contentEditable==='false'`)
	})
	mygo.RunOnMain(func() { host.OpenMail(fixture.SessionID, "https://mail.example.invalid/draft") })
	var draftID string
	awaitNativeMail(t, "actual openDraft resolves original current card", func() bool {
		mygo.RunOnMain(func() { draftID = host.cards.mail.enlarged })
		return draftID != "" && draftID != fixture.CallID
	})
	_ = nativeMailDocument(t, host, draftID, "expanded")
	mygo.RunOnMain(func() {
		item := host.cards.byCall[draftID]
		if item.SessionID != fixture.SessionID || item.Mail.AccountID != "fixture-account" {
			t.Error("Reader opened another owner/task/account's composer")
		}
	})
	t.Log("real Root Reader, provider-pinned 8-message metadata, explicit markRead, no automatic external image fetch, original opt-in, full >400K read-only body, original openDraft card linkage")
}
func TestNativeMailCountdownUndoNowAndLateReceiptUseTheActualProvider(t *testing.T) {
	if os.Getenv("WINONE_MAIL_NATIVE") != "1" {
		t.Skip("explicit private desktop and real WebView2")
	}
	fixture := startNativeMailFixture(t)
	host, _, _ := mailNativeWindow(t, fixture)
	traceNativeMail(t, fixture, host)
	document := nativeMailDocument(t, host, fixture.CallID, "inline")
	action := func(name string) {
		t.Helper()
		mygo.RunOnMain(func() {
			item := host.cards.byCall[fixture.CallID]
			host.cardDispatch(item, "card-action", map[string]any{"action": "mail:" + name + ":" + item.Mail.Key})
		})
	}
	pending := func() *mailPendingSend {
		var value *mailPendingSend
		mygo.RunOnMain(func() {
			item := host.cards.byCall[fixture.CallID]
			if item.Mail.PendingSend != nil {
				copy := *item.Mail.PendingSend
				value = &copy
			}
		})
		return value
	}
	action("send")
	awaitNativeMail(t, "visible original ten-second countdown", func() bool { return pending() != nil })
	if remaining := pending().Deadline - time.Now().UnixMilli(); remaining < 8000 {
		t.Fatalf("native poll lost the visible full grace: %dms", remaining)
	}
	awaitNativeMail(t, "countdown disables actual HTML editor", func() bool { return mailNativeBool(document.view.Page(), `document.body.contentEditable==='false'`) })
	var state nativeMailState
	if err := fixture.request("/state", nil, &state); err != nil {
		t.Fatal(err)
	}
	if state.Provider.Sends != 0 {
		t.Fatal("native countdown was called sent before any provider receipt")
	}
	action("cancel")
	awaitNativeMail(t, "native Undo restores the provider draft", func() bool {
		ok := false
		mygo.RunOnMain(func() {
			item := host.cards.byCall[fixture.CallID]
			ok = item.Mail.State == "draft" && item.Mail.PendingSend == nil
		})
		return ok
	})
	if err := fixture.request("/late", map[string]any{}, nil); err != nil {
		t.Fatal(err)
	}
	action("send")
	awaitNativeMail(t, "new full grace before Send now", func() bool { return pending() != nil })
	action("now")
	awaitNativeMail(t, "actual late provider send committed", func() bool { return fixture.request("/state", nil, &state) == nil && state.Provider.Sends == 1 })
	mygo.RunOnMain(func() {
		if host.cards.byCall[fixture.CallID].Mail.State == "sent" {
			t.Error("native UI fabricated a sent result before the held provider receipt")
		}
	})
	action("send")
	if err := fixture.request("/state", nil, &state); err != nil {
		t.Fatal(err)
	}
	if state.Provider.Sends != 1 {
		t.Fatal("repeated native action replayed pending mail")
	}
	if err := fixture.request("/settle", map[string]any{}, nil); err != nil {
		t.Fatal(err)
	}
	awaitNativeMail(t, "known late receipt reaches native sent footer", func() bool {
		sent := false
		mygo.RunOnMain(func() { sent = host.cards.byCall[fixture.CallID].Mail.State == "sent" })
		return sent
	})
	action("continue")
	if err := fixture.request("/state", nil, &state); err != nil {
		t.Fatal(err)
	}
	if state.Provider.Sends != 1 {
		t.Fatal("Continue re-sent mail instead of invoking the original continuation slot")
	}
	t.Log("real native countdown/readonly/Undo/now, actual held provider receipt, duplicate-send rejection, known sent footer and Continue without re-send")
}

func nativeMailQuote(value string) string { encoded, _ := json.Marshal(value); return string(encoded) }
