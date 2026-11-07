//go:build windows

package chat

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/plugins/automation"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/chat/pluginhost"
	"winone/desktopmygo/core"
	"winone/desktopmygo/host"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/internal/ai"
)

func TestMain(m *testing.M) {
	if os.Getenv("WINONE_NATIVE_RICH_MATRIX") != "1" {
		os.Exit(m.Run())
	}
	profile := os.Getenv("WINONE_NATIVE_UI_REAL_PROFILE")
	if !filepath.IsAbs(profile) || !strings.Contains(filepath.ToSlash(profile), "/_temp/") {
		fmt.Fprintln(os.Stderr, "native rich matrix needs its explicit repository-private profile")
		os.Exit(1)
	}
	mygo.App.SetName("WinOne Native Rich Production Matrix")
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(profile, "native-rich-browser"))
	mygo.App.OnWindowAllClosed(func() {})
	code := 1
	mygo.App.WhenReady(func() { go func() { code = m.Run(); mygo.App.Quit() }() })
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

type richNativeNode struct {
	ID, Parent, Role, Label, Value string
	States, Actions                []string
	Bounds                         struct{ X, Y, Width, Height float64 }
}

// The automation plugin reads the real native accessibility tree and delivers
// the platform's control actions. It is linked only into this test executable,
// never production. It is not used to mint plugin permission/user gestures.
func richNativeCall(t *testing.T, method string, params any) json.RawMessage {
	t.Helper()
	connection, err := net.DialTimeout("unix", os.Getenv("MYGO_AUTOMATION"), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(connection).Encode(map[string]any{"id": 1, "method": method, "params": params}); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(connection).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var reply struct {
		Result json.RawMessage
		Error  *struct{ Code, Message string }
	}
	if err := json.Unmarshal(line, &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Error != nil {
		t.Fatalf("native %s: %s: %s", method, reply.Error.Code, reply.Error.Message)
	}
	return reply.Result
}

func richNativeFiles(t *testing.T, directory string) {
	t.Helper()
	bitmap := image.NewRGBA(image.Rect(0, 0, 160, 96))
	for y := range 96 {
		for x := range 160 {
			bitmap.SetRGBA(x, y, color.RGBA{R: uint8(30 + x), G: uint8(60 + y), B: 100, A: 255})
		}
	}
	var picture bytes.Buffer
	if err := png.Encode(&picture, bitmap); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "native-rich.png"), picture.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	video, err := os.ReadFile(os.Getenv("WINONE_NATIVE_UI_VIDEO"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "native-rich.mp4"), video, 0600); err != nil {
		t.Fatal(err)
	}
	// A silent, real WAV fixture. The matrix never presses playback and does
	// not use a physical microphone/speaker as component evidence.
	pcm := make([]byte, 16000*2)
	wave := make([]byte, 44+len(pcm))
	copy(wave, "RIFF")
	binary.LittleEndian.PutUint32(wave[4:], uint32(len(wave)-8))
	copy(wave[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wave[16:], 16)
	binary.LittleEndian.PutUint16(wave[20:], 1)
	binary.LittleEndian.PutUint16(wave[22:], 1)
	binary.LittleEndian.PutUint32(wave[24:], 16000)
	binary.LittleEndian.PutUint32(wave[28:], 32000)
	binary.LittleEndian.PutUint16(wave[32:], 2)
	binary.LittleEndian.PutUint16(wave[34:], 16)
	copy(wave[36:], "data")
	binary.LittleEndian.PutUint32(wave[40:], uint32(len(pcm)))
	copy(wave[44:], pcm)
	if err := os.WriteFile(filepath.Join(directory, "native-rich.wav"), wave, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRealNativeRichProductionMatrix(t *testing.T) {
	if os.Getenv("WINONE_NATIVE_RICH_MATRIX") != "1" {
		t.Skip("explicit private desktop/real Host/native rich matrix entry")
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
	configuration.Providers = []ai.Provider{{ID: "native-rich", Name: "Native rich fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: "http://127.0.0.1:1", Models: []string{"model"}}}
	configuration.Tasks = map[string]ai.Selection{"chat": {Provider: "native-rich", Model: "model"}, "default": {Provider: "native-rich", Model: "model"}}
	call("capture.ai.configuration.save", map[string]any{"configuration": configuration, "keys": map[string]string{"native-rich": "isolated-no-model-call"}}, nil)
	directory := filepath.Join(filepath.Dir(profile), "rich-workspace")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	richNativeFiles(t, directory)
	var conversation struct{ ID string }
	call("chat.conversation.new", map[string]string{"directory": directory, "title": "Native rich production matrix"}, &conversation)
	if conversation.ID == "" {
		t.Fatal("the real Host did not create the owned fixture conversation")
	}
	var window *mygo.Window
	var page *Chat
	dark := false
	env := &core.Env{Host: client, Directory: profile, Activity: &core.Activity{}, Rail: &core.Rail{}, Refresh: func() {}, Toast: func(string) {}, Focuses: func() int { return 0 }, TakeIntents: func(string) []core.Intent { return nil }, Navigate: func(string, func()) {}, Open: func(string, core.Intent) {}, RequestChange: func(fn func()) { fn() }}
	env.Post = func(fn func()) {
		if window != nil && !window.IsDestroyed() {
			window.Update(fn)
		}
	}
	env.WebViews = func(options mygo.WebViewOptions) (*mygo.WebView, error) { return window.NewWebView(options) }
	mygo.RunOnMain(func() {
		window = mygo.NewWindow(mygo.WindowOptions{Title: "Native rich production matrix", Width: 1100, Height: 900, Hidden: true, Content: ui.View(func(c *ui.Context) {
			c.SetTheme(kit.Theme(c.Preferences(), dark))
			if page != nil {
				page.View(c)
			}
		})})
		page = New(env)
		page.setSidebar(false)
		page.show(conversation.ID)
		window.Show()
		window.Focus()
	})
	defer mygo.RunOnMain(func() { page.Close(); pluginhost.For(env).Close(); window.Destroy() })
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
			t.Logf("native page error=%q active=%q opening=%v model=%v", page.err, page.activeID, page.opening, page.ai != nil)
		})
		t.Fatalf("%s did not settle", label)
	}
	await("real native owner", func() bool { return page.activeID == conversation.ID && !page.opening && page.ai != nil })
	mygo.RunOnMain(func() {
		// Model payloads are the fixture in this rendering matrix. The Host
		// lease/catalog/preview authority stay real, but a background empty
		// session poll must not replace a chosen fixture during its screenshot.
		if page.session != nil {
			page.session.disposed = true
			if page.session.cancel != nil {
				page.session.cancel()
			}
		}
	})
	nodes := func() []richNativeNode {
		t.Helper()
		var tree struct{ Nodes []richNativeNode }
		if err := json.Unmarshal(richNativeCall(t, "tree", map[string]any{"window": window.ID()}), &tree); err != nil {
			t.Fatal(err)
		}
		return tree.Nodes
	}
	find := func(label, action string) *richNativeNode {
		for _, node := range nodes() {
			if node.Label == label && !slices.Contains(node.States, "offscreen") && !slices.Contains(node.States, "disabled") && (action == "" || slices.Contains(node.Actions, action)) {
				copy := node
				return &copy
			}
		}
		return nil
	}
	act := func(label, action, text string) {
		t.Helper()
		node := find(label, action)
		if node == nil {
			t.Fatalf("real native %s control %q is missing", action, label)
		}
		richNativeCall(t, "act", map[string]any{"window": window.ID(), "id": node.ID, "action": action, "text": text})
	}
	shots := os.Getenv("WINONE_NATIVE_UI_SHOTS")
	if err := os.MkdirAll(shots, 0700); err != nil {
		t.Fatal(err)
	}
	defer i18n.SetLocale(i18n.Chinese)
	completed := 0
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		for _, theme := range []bool{false, true} {
			for _, width := range []int{1100, 560} {
				mygo.RunOnMain(func() { i18n.SetLocale(locale); dark = theme; window.SetContentSize(width, 900); window.Invalidate() })
				for _, sample := range richProductionCases(locale, directory) {
					name := fmt.Sprintf("%s-%t-%d-%s", locale, theme, width, sample.kind)
					messageID := "native-matrix-" + name
					mygo.RunOnMain(func() {
						page.view.Messages = []model.Message{{ID: messageID, Role: "assistant", Content: model.TextContent(sample.source)}}
						page.view.Running = false
						page.ui.followLatest()
						window.Invalidate()
					})
					// Native accessibility/viewport conditions, never a fixed render
					// sleep. Rich source parse, images and diagrams are real components.
					var operation struct{ Props struct{ Text string } }
					payload := strings.TrimSuffix(strings.TrimPrefix(sample.source, "```cindy-ui\n"), "\n```")
					if err := json.Unmarshal([]byte(payload), &operation); err != nil {
						t.Fatal(err)
					}
					deadline := time.Now().Add(15 * time.Second)
					ready := false
					for time.Now().Before(deadline) {
						tree := nodes()
						ready = true
						for _, label := range sample.labels {
							found := false
							for _, node := range tree {
								found = found || strings.Contains(node.Label, label)
							}
							ready = ready && found
						}
						if sample.kind == "image" {
							decoded := false
							for _, node := range tree {
								decoded = decoded || node.Role == "image" && node.Label == sample.labels[0] && node.Bounds.Width >= 159 && node.Bounds.Height >= 95
							}
							ready = ready && decoded
						}
						if sample.kind == "math" || sample.kind == "diagram" {
							drawn := false
							for _, node := range tree {
								drawn = drawn || node.Label == operation.Props.Text && node.Role != "text" && node.Bounds.Width > 0 && node.Bounds.Height > 0
							}
							ready = ready && drawn
						}
						if ready {
							break
						}
						time.Sleep(20 * time.Millisecond)
					}
					if !ready {
						t.Fatalf("%s did not render all actual native content: %+v", name, nodes())
					}
					label := "Full capability " + sample.kind
					if locale == i18n.Chinese {
						label = "完整能力 " + sample.kind
					}
					value := func(bind string, want any) bool {
						for key, values := range page.richMemory.Values {
							if strings.Contains(key, ":message:"+messageID+":") && values[bind] == want {
								return true
							}
						}
						return false
					}
					switch sample.kind {
					case "input":
						act(label, "setValue", "Grace")
						await("native rich input", func() bool { return value("entry", "Grace") })
					case "check":
						act(label, "press", "")
						await("native rich checkbox", func() bool { return value("approved", true) })
					case "range":
						act(label, "increment", "")
						await("native rich slider", func() bool { return value("amount", float64(4)) })
					case "button":
						act(label, "press", "")
						wanted := "Updated by the native button"
						if locale == i18n.Chinese {
							wanted = "由原生按钮更新"
						}
						await("native rich set action", func() bool { return value("entry", wanted) })
					case "form":
						act(label, "press", "")
						await("native rich form", func() bool { return value("approved", true) })
					case "details":
						act(label, "press", "")
						await("native details expansion", func() bool { return value("sample", true) })
					case "tabs":
						second := "Second tab"
						if locale == i18n.Chinese {
							second = "第二标签"
						}
						act(second, "press", "")
						await("native rich tab selection", func() bool { return value("sample", "second") })
					}
					image, err := window.CapturePage()
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(shots, name+".png"), image, 0600); err != nil {
						t.Fatal(err)
					}
					tree := nodes()
					for _, node := range tree {
						// Vertical offscreen content remains scrollable. No logical
						// native table/control may extend beyond the horizontal window.
						if node.Bounds.Width > 0 && (node.Bounds.X < -1 || node.Bounds.X+node.Bounds.Width > float64(width)+1) {
							t.Fatalf("%s native %s %q escapes the horizontal viewport: %+v", name, node.Role, node.Label, node.Bounds)
						}
					}
					serialized, _ := json.MarshalIndent(tree, "", "  ")
					if err := os.WriteFile(filepath.Join(shots, name+".json"), serialized, 0600); err != nil {
						t.Fatal(err)
					}
					completed++
					t.Logf("native production %s: %d accessible nodes, real frame %d bytes", name, len(tree), len(image))
				}
			}
		}
	}
	if completed != 27*8 {
		t.Fatalf("native capability matrix lost cases: %d", completed)
	}
	t.Log("26 rich kinds plus the line-chart variant × Chinese/English × light/dark × wide/narrow, using real production controls and Host-issued image bytes")
}
