//go:build windows

package flashchat

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
)

type nativeClient struct {
	p       *completePage
	w       *mygo.Window
	aside   bool
	created int
}

func nativeAwait(t *testing.T, label string, check func() bool) {
	t.Helper()
	budget := 45 * time.Second
	// Public trackers can schedule their next announcement after 120 s.
	// The local tracker is immediate; don't impose its deadline on the network.
	if os.Getenv("WINONE_FLASHCHAT_PUBLIC_RELAY") == "1" {
		budget = 150 * time.Second
	}
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		var done bool
		mygo.RunOnMain(func() { done = check() })
		if done {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out: " + label)
}
func newNativeClient(t *testing.T, name string, relay bool) *nativeClient {
	t.Helper()
	n := &nativeClient{}
	env := &core.Env{Directory: filepath.Join(os.Getenv("WINONE_FLASHCHAT_NATIVE_PROFILE"), name)}
	n.p = newCompletePage(env)
	mygo.RunOnMain(func() {
		n.w = mygo.NewWindow(mygo.WindowOptions{Title: name, Width: 1000, Height: 800, Hidden: true, Content: ui.View(func(c *ui.Context) {
			if n.aside {
				ui.Text(c, "Other page")
			} else {
				n.p.View(c)
			}
		})})
		env.Post = func(fn func()) {
			mygo.RunOnMain(func() {
				if !n.w.IsDestroyed() {
					n.w.Update(fn)
				}
			})
		}
		env.WebViews = func(options mygo.WebViewOptions) (*mygo.WebView, error) {
			n.created++
			policy := ""
			if relay {
				policy = ",iceTransportPolicy:'relay'"
			}
			options.Page.PreloadScript += `;window.__nativePCs=[];const OriginalPC=window.RTCPeerConnection;window.RTCPeerConnection=class extends OriginalPC{constructor(config,...rest){super({...config` + policy + `},...rest);window.__nativePCs.push(this)}};`
			options.Page.PreloadScript += `window.__nativeSockets=[];const OriginalSocket=window.WebSocket;window.WebSocket=class extends OriginalSocket{constructor(...a){super(...a);window.__nativeSockets.push(this)}};`
			options.Page.PreloadScript += `window.__nativeErrors=[];window.addEventListener('error',e=>window.__nativeErrors.push(e.message));window.addEventListener('unhandledrejection',e=>window.__nativeErrors.push(String(e.reason)));`
			options.Page.PreloadScript += `window.__nativeMedia=[];const createElement=document.createElement.bind(document);document.createElement=(tag,...args)=>{const e=createElement(tag,...args);if(tag==='audio'||tag==='video')window.__nativeMedia.push(e);return e};`
			return n.w.NewWebView(options)
		}
		n.w.Show()
	})
	t.Cleanup(func() {
		if t.Failed() && n.p.engine.Load() != nil {
			mygo.RunOnMain(func() {
				t.Log(name, "state", n.p.snapshot.RoomID, n.p.snapshot.Peers, n.p.snapshot.Alert, n.p.actionError)
				t.Log(name, "previews", n.p.snapshot.Previews, "requested", n.p.previewRequested, "errors", n.p.previewErrors, "frame count", len(n.p.frames), "selected", n.p.selected, "lists", n.p.lists)
			})
			frame, _ := n.w.CapturePage()
			_ = os.WriteFile(filepath.Join(os.Getenv("WINONE_FLASHCHAT_NATIVE_PROFILE"), name+"-failure.png"), frame, 0600)
			status, err := mygo.EvalAs[any](n.p.engine.Load(), `window.__nativePCs.filter(p=>p.connectionState==='connected').map(p=>({state:p.connectionState,ice:p.iceConnectionState,signaling:p.signalingState}))`)
			t.Log(name, "connections", status, err)
			sockets, _ := mygo.EvalAs[any](n.p.engine.Load(), `window.__nativeSockets.map(s=>({url:s.url,state:s.readyState}))`)
			t.Log(name, "sockets", sockets, "trackers", n.p.snapshot.Settings.Trackers)
			startup, _ := mygo.EvalAs[any](n.p.engine.Load(), `({errors:window.__nativeErrors,body:document.body.innerText,ready:document.readyState,bridge:!!window.__flashNativeInvoke})`)
			t.Log(name, "startup", startup)
			media, _ := mygo.EvalAs[any](n.p.engine.Load(), `({visibility:document.visibilityState,media:window.__nativeMedia.map(e=>({tag:e.tagName,ready:e.readyState,network:e.networkState,paused:e.paused,duration:String(e.duration),error:e.error?.message,src:e.src.slice(0,30),connected:e.isConnected}))})`)
			t.Log(name, "media diagnostics", media)
		}
		mygo.RunOnMain(func() { n.p.Close(); n.w.Destroy() })
	})
	nativeAwait(t, name+" ready", func() bool { return n.p.snapshot.Ready && !n.p.busy && n.p.formRoom != "" })
	return n
}
func nativeInvoke(t *testing.T, n *nativeClient, scope, action string, args map[string]any) json.RawMessage {
	t.Helper()
	type result struct {
		value json.RawMessage
		err   error
	}
	completed := make(chan result, 1)
	go func() { value, err := n.p.invoke(scope, action, args); completed <- result{value, err} }()
	select {
	case outcome := <-completed:
		if outcome.err != nil {
			t.Fatalf("%s/%s: %v", scope, action, outcome.err)
		}
		return outcome.value
	case <-time.After(40 * time.Second):
		t.Fatalf("native command timed out: %s/%s", scope, action)
		return nil
	}
}
func nativeHas(p *completePage, scope, text string) bool {
	for _, m := range p.snapshot.Rooms[scope].Messages {
		if m.Text == text {
			return true
		}
	}
	return false
}
func nativePeerID(p *completePage, name string) string {
	for _, peer := range p.snapshot.Peers {
		if peer.Name == name || strings.HasPrefix(peer.Name, name+" (") {
			return peer.PeerID
		}
	}
	return ""
}

// Native controls and encrypted communication use the production command
// boundary. Separate profiles exercise identity, histories and peer isolation.
func TestNativeFlashChatFlows(t *testing.T) {
	if os.Getenv("WINONE_FLASHCHAT_NATIVE_WINDOW") != "1" {
		t.Skip("explicit private desktop required")
	}
	relay := os.Getenv("WINONE_FLASHCHAT_PUBLIC_RELAY") == "1"
	trackerURL := ""
	if !relay {
		root := os.Getenv("WINONE_TEST_REPOSITORY")
		if root == "" {
			t.Fatal("missing test repository")
		}
		tracker := exec.Command("node", filepath.Join(root, "tests/flashchat-complete/native-tracker.cjs"))
		stdout, err := tracker.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err = tracker.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = tracker.Process.Kill(); _ = tracker.Wait() })
		scanner := bufio.NewScanner(stdout)
		if !scanner.Scan() {
			t.Fatal("local tracker did not start")
		}
		trackerURL = scanner.Text()
	}
	a, b := newNativeClient(t, "Native A", relay), newNativeClient(t, "Native B", relay)
	room := "native-flow-" + randomToken()[:16]
	configure := func(n *nativeClient, name string) {
		prefs := map[string]any{"customUsername": name, "playSoundOnNewMessage": false}
		if trackerURL != "" {
			prefs["isEnhancedConnectivityEnabled"] = false
			prefs["trackerUrls"] = []string{trackerURL}
			prefs["rtcConfig"] = map[string]any{"iceServers": []any{}}
		}
		nativeInvoke(t, n, "shell", "settings", map[string]any{"value": prefs})
	}
	join := func(n *nativeClient, name, room, password string) {
		nativeInvoke(t, n, "shell", "join", map[string]any{"room": room, "nickname": name, "private": password != "", "password": password})
		nativeAwait(t, "entered room", func() bool { return n.p.snapshot.RoomID != "" && n.p.snapshot.Rooms != nil })
	}
	configure(a, "Native A")
	configure(b, "Native B")
	join(a, "Native A", room, "")
	join(b, "Native B", room, "")
	nativeAwait(t, "pair verified", func() bool {
		return len(a.p.snapshot.Peers) == 1 && len(b.p.snapshot.Peers) == 1 && a.p.snapshot.Peers[0].Verified == 2 && b.p.snapshot.Peers[0].Verified == 2
	})
	nativeInvoke(t, a, "room:", "send", map[string]any{"text": "Group message from A"})
	nativeAwait(t, "group received", func() bool { return nativeHas(b.p, "", "Group message from A") })
	nativeInvoke(t, b, "room:", "send", map[string]any{"text": "Group reply from B"})
	nativeAwait(t, "group reply", func() bool { return nativeHas(a.p, "", "Group reply from B") })
	t.Log("bidirectional group messages and verified identities")
	// Remote native presentation must receive actual media, not merely show
	// the sender's local capture. Devices are synthetic on the private desktop.
	for _, kind := range []string{"camera", "microphone", "screen"} {
		nativeInvoke(t, a, "media", kind, map[string]any{"enabled": true})
	}
	nativeAwait(t, "remote camera and screen frames", func() bool {
		camera, screen := false, false
		for _, stream := range b.p.snapshot.Media.Streams {
			if stream.Peer != "" && b.p.frames[stream.ID] != nil {
				camera = camera || stream.Kind == "camera"
				screen = screen || stream.Kind == "screen"
			}
		}
		return camera && screen
	})
	nativeAwait(t, "remote microphone channel", func() bool {
		for _, peer := range b.p.snapshot.Peers {
			if peer.AudioChannels["microphone"] == "PLAYING" {
				return true
			}
		}
		return false
	})
	for _, kind := range []string{"screen", "camera", "microphone"} {
		nativeInvoke(t, a, "media", kind, map[string]any{"enabled": false})
	}
	nativeAwait(t, "remote media stopped", func() bool { return len(b.p.snapshot.Media.Streams) == 0 })
	t.Log("remote native camera, screen frames and microphone channel; stop released remote media")
	c := newNativeClient(t, "Native C", relay)
	configure(c, "Native C")
	join(c, "Native C", room, "")
	nativeAwait(t, "public history", func() bool {
		return nativeHas(c.p, "", "Group message from A") && nativeHas(c.p, "", "Group reply from B") && len(a.p.snapshot.Peers) == 2
	})
	var aAtB, bAtA string
	nativeAwait(t, "direct listeners", func() bool {
		aAtB = nativePeerID(b.p, "Native A")
		bAtA = nativePeerID(a.p, "Native B")
		_, ok := b.p.snapshot.Rooms[aAtB]
		return aAtB != "" && bAtA != "" && ok
	})
	nativeInvoke(t, a, "room:"+bAtA, "send", map[string]any{"text": "Private before opening"})
	nativeAwait(t, "first direct message", func() bool { return nativeHas(b.p, aAtB, "Private before opening") })
	mygo.RunOnMain(func() {
		for _, r := range c.p.snapshot.Rooms {
			for _, m := range r.Messages {
				if m.Text == "Private before opening" {
					t.Error("private message leaked to C")
				}
			}
		}
	})
	nativeInvoke(t, b, "shell", "select", map[string]any{"peer": aAtB})
	nativeInvoke(t, b, "shell", "select", map[string]any{"peer": ""})
	nativeInvoke(t, b, "shell", "select", map[string]any{"peer": aAtB})
	nativeAwait(t, "direct history retained", func() bool { return nativeHas(b.p, aAtB, "Private before opening") })
	t.Log("late public history; direct first delivery, isolation and reopen")
	// A real native share grants only the chosen file. The receiver's normal
	// WebView2 download writes into this test's isolated downloads directory.
	profile := os.Getenv("WINONE_FLASHCHAT_NATIVE_PROFILE")
	file := filepath.Join(profile, "native-payload.txt")
	payload := []byte(strings.Repeat("native encrypted file\n", 200))
	if err := os.WriteFile(file, payload, 0600); err != nil {
		t.Fatal(err)
	}
	mygo.RunOnMain(func() { a.p.shareNative([]string{file}) })
	var magnet string
	nativeAwait(t, "file offer", func() bool {
		for _, p := range b.p.snapshot.Peers {
			if strings.HasPrefix(p.Name, "Native A") {
				magnet = p.OfferedFileID
			}
		}
		return magnet != "" && !a.p.busy
	})
	downloads := make(chan *mygo.Download, 2)
	off := b.p.engine.Load().OnDownloadDone(func(d *mygo.Download) { downloads <- d })
	defer off()
	mygo.RunOnMain(func() {
		b.p.command("room:", "download", map[string]any{"magnet": magnet}, nil)
		b.p.drafts[""] = "Message while downloading"
		b.p.sendNative()
	})
	select {
	case d := <-downloads:
		if d.Err != nil {
			t.Fatal(d.Err)
		}
		data, err := os.ReadFile(d.Path)
		if err != nil || !bytes.Equal(data, payload) {
			t.Fatal("download content differs", err)
		}
		if !strings.HasPrefix(filepath.Clean(d.Path), filepath.Join(profile, "downloads")+string(os.PathSeparator)) {
			t.Fatal("download escaped isolated directory")
		}
	case <-time.After(40 * time.Second):
		t.Fatal("encrypted download timed out")
	}
	nativeAwait(t, "composer usable during download", func() bool { return nativeHas(a.p, "", "Message while downloading") && !b.p.busy && !b.p.downloading })
	nativeInvoke(t, a, "room:", "rescind", nil)
	nativeAwait(t, "offer withdrawn", func() bool {
		for _, p := range b.p.snapshot.Peers {
			if strings.HasPrefix(p.Name, "Native A") {
				return p.OfferedFileID == ""
			}
		}
		return false
	})
	t.Log("native file grant, encrypted download byte match and revoke")
	// Inline preview is decoded by the hidden engine and rendered as MyGo pixels.
	picture := image.NewRGBA(image.Rect(0, 0, 80, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 80; x++ {
			picture.Set(x, y, color.RGBA{20, 120, 200, 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(profile, "native-picture.png")
	if err := os.WriteFile(imagePath, encoded.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	mygo.RunOnMain(func() { a.p.shareNative([]string{imagePath}) })
	nativeAwait(t, "inline media message", func() bool {
		for _, m := range b.p.snapshot.Rooms[""].Messages {
			if m.Magnet != "" {
				return true
			}
		}
		return false
	})
	nativeAwait(t, "image decoded into native view", func() bool {
		for _, items := range b.p.snapshot.Previews {
			for _, item := range items {
				if item.Kind == "image" && b.p.frames[item.ID] != nil {
					return true
				}
			}
		}
		return false
	})
	t.Log("encrypted inline image rendered by native image component")
	// A generated silent WAV exercises decoding, native playback state and
	// seek without using the user's files, microphone or audible test content.
	var wav bytes.Buffer
	wav.WriteString("RIFF")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(36+8000*2*4))
	wav.WriteString("WAVEfmt ")
	for _, value := range []any{uint32(16), uint16(1), uint16(1), uint32(8000), uint32(16000), uint16(2), uint16(16)} {
		_ = binary.Write(&wav, binary.LittleEndian, value)
	}
	wav.WriteString("data")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(8000*2*4))
	wav.Write(make([]byte, 8000*2*4))
	wavPath := filepath.Join(profile, "native-silent.wav")
	if err := os.WriteFile(wavPath, wav.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	mygo.RunOnMain(func() { a.p.shareNative([]string{wavPath}) })
	var audioID string
	nativeAwait(t, "native audio control", func() bool {
		for _, items := range b.p.snapshot.Previews {
			for _, item := range items {
				if item.Kind == "audio" {
					audioID = item.ID
					return true
				}
			}
		}
		return false
	})
	nativeInvoke(t, b, "room:", "playback", map[string]any{"id": audioID, "enabled": true})
	nativeAwait(t, "native audio playing", func() bool {
		for _, items := range b.p.snapshot.Previews {
			for _, item := range items {
				if item.ID == audioID {
					return item.Playing && item.Current > 0 && item.Duration >= 4
				}
			}
		}
		return false
	})
	nativeInvoke(t, b, "room:", "playback", map[string]any{"id": audioID, "enabled": false, "position": 2})
	nativeAwait(t, "native audio paused and seeked", func() bool {
		for _, items := range b.p.snapshot.Previews {
			for _, item := range items {
				if item.ID == audioID {
					return !item.Playing && item.Current >= 1.9 && item.Current < 2.2
				}
			}
		}
		return false
	})
	t.Log("native audio duration, play, pause and seek")
	// The runner supplies a synthetic camera. Record a short real WebM in the
	// hidden engine, then send it through the same encrypted file path.
	video64, err := mygo.EvalAs[string](a.p.engine.Load(), `(async()=>{const stream=await navigator.mediaDevices.getUserMedia({video:true});try{const recorder=new MediaRecorder(stream,{mimeType:'video/webm'});const chunks=[];recorder.ondataavailable=e=>chunks.push(e.data);const stopped=new Promise((resolve,reject)=>{recorder.onstop=resolve;recorder.onerror=reject});recorder.start();await new Promise(r=>setTimeout(r,1200));recorder.stop();await stopped;return await new Promise((resolve,reject)=>{const reader=new FileReader();reader.onload=()=>resolve(String(reader.result).split(',')[1]);reader.onerror=reject;reader.readAsDataURL(new Blob(chunks,{type:'video/webm'}))})}finally{stream.getTracks().forEach(t=>t.stop())}})()`)
	if err != nil {
		t.Fatal(err)
	}
	videoBytes, err := base64.StdEncoding.DecodeString(video64)
	if err != nil || len(videoBytes) == 0 {
		t.Fatal("empty video fixture", err)
	}
	videoPath := filepath.Join(profile, "native-video.webm")
	if err := os.WriteFile(videoPath, videoBytes, 0600); err != nil {
		t.Fatal(err)
	}
	mygo.RunOnMain(func() { a.p.shareNative([]string{videoPath}) })
	var videoID string
	nativeAwait(t, "native video ready", func() bool {
		for _, items := range b.p.snapshot.Previews {
			for _, item := range items {
				if item.Kind == "video" {
					videoID = item.ID
					return true
				}
			}
		}
		return false
	})
	nativeInvoke(t, b, "room:", "playback", map[string]any{"id": videoID, "enabled": true})
	nativeAwait(t, "native video frame", func() bool { return b.p.frames[videoID] != nil })
	nativeInvoke(t, b, "room:", "playback", map[string]any{"id": videoID, "enabled": false})
	t.Log("encrypted video file played as native frames")
	if relay {
		for _, n := range []*nativeClient{a, b} {
			ok, err := mygo.EvalAs[bool](n.p.engine.Load(), `(async()=>{for(const pc of window.__nativePCs){if(pc.connectionState!=='connected')continue;const stats=await pc.getStats();for(const item of stats.values())if(item.type==='transport'&&item.selectedCandidatePairId){const pair=stats.get(item.selectedCandidatePairId);if(stats.get(pair.localCandidateId)?.candidateType==='relay'&&pair.bytesSent>0&&pair.bytesReceived>0)return true}}return false})()`)
			if err != nil || !ok {
				t.Fatal("relay path not confirmed", err)
			}
		}
		t.Log("public relay selected and bidirectional byte counters positive")
	}
	// Room switching must not retain public histories inside a new private room.
	for _, n := range []*nativeClient{a, b, c} {
		nativeInvoke(t, n, "shell", "leave", nil)
		nativeAwait(t, "left", func() bool { return n.p.snapshot.RoomID == "" })
	}
	join(a, "Native A", room+"-private", "correct")
	var invite string
	mygo.RunOnMain(func() { invite = a.p.invitation() })
	join(b, "Native B", invite, "")
	join(c, "Native C", room+"-private", "different")
	nativeAwait(t, "private pair", func() bool { return len(a.p.snapshot.Peers) == 1 && len(b.p.snapshot.Peers) == 1 })
	var privateTarget, privateSender string
	mygo.RunOnMain(func() { privateTarget = a.p.snapshot.Peers[0].PeerID; privateSender = b.p.snapshot.Peers[0].PeerID })
	nativeInvoke(t, a, "room:"+privateTarget, "send", map[string]any{"text": "Direct inside private room"})
	nativeAwait(t, "private room direct message", func() bool { return nativeHas(b.p, privateSender, "Direct inside private room") })
	nativeInvoke(t, a, "room:", "send", map[string]any{"text": "Private room message"})
	nativeAwait(t, "private room delivered", func() bool { return nativeHas(b.p, "", "Private room message") })
	mygo.RunOnMain(func() {
		if nativeHas(c.p, "", "Private room message") || len(c.p.snapshot.Peers) != 0 {
			t.Error("wrong password joined private room")
		}
	})
	nativeInvoke(t, c, "shell", "leave", nil)
	nativeAwait(t, "C left", func() bool { return c.p.snapshot.RoomID == "" })
	join(c, "Native C", room+"-private", "correct")
	nativeAwait(t, "late private peer", func() bool { return len(c.p.snapshot.Peers) == 2 })
	nativeInvoke(t, b, "room:", "send", map[string]any{"text": "After private join"})
	nativeAwait(t, "new private delivery", func() bool { return nativeHas(c.p, "", "After private join") })
	mygo.RunOnMain(func() {
		if nativeHas(c.p, "", "Private room message") {
			t.Error("private history was resent")
		}
	})
	t.Log("private password isolation and no history replay")
	// Export/import calls are the same commands used by the native file dialog.
	exported := nativeInvoke(t, c, "shell", "export", nil)
	var value any
	if err := json.Unmarshal(exported, &value); err != nil {
		t.Fatal(err)
	}
	nativeInvoke(t, c, "shell", "settings", map[string]any{"value": map[string]any{"customUsername": "Temporary"}})
	nativeInvoke(t, c, "shell", "import", map[string]any{"value": value})
	restored := nativeInvoke(t, c, "shell", "export", nil)
	var expected, actual any
	_ = json.Unmarshal(exported, &expected)
	_ = json.Unmarshal(restored, &actual)
	expectedJSON, _ := json.Marshal(expected)
	actualJSON, _ := json.Marshal(actual)
	if !bytes.Equal(expectedJSON, actualJSON) {
		t.Fatal("profile round trip changed fields")
	}
	t.Log("profile keys and preferences round trip")
	mygo.RunOnMain(func() { a.p.drafts[""] = "Unsent native draft"; a.p.SetAside(); a.aside = true; a.w.Invalidate() })
	nativeAwait(t, "hidden presentation", func() bool { return strings.Contains(a.p.presentation, `"visible":false`) })
	nativeInvoke(t, b, "room:", "send", map[string]any{"text": "While page is hidden"})
	nativeAwait(t, "hidden receiver", func() bool { return nativeHas(a.p, "", "While page is hidden") })
	mygo.RunOnMain(func() { a.aside = false; a.w.Invalidate() })
	nativeAwait(t, "restored", func() bool { return strings.Contains(a.p.presentation, `"visible":true`) })
	mygo.RunOnMain(func() {
		if a.created != 1 || a.p.drafts[""] != "Unsent native draft" {
			t.Error("page switch lost engine or draft")
		}
	})
	origin := a.p.app.origin
	view := a.p.view
	mygo.RunOnMain(func() { a.p.Close() })
	if !view.IsDestroyed() {
		t.Fatal("engine retained after close")
	}
	client := http.Client{Timeout: time.Second}
	if response, err := client.Get(origin); err == nil {
		response.Body.Close()
		t.Fatal("local bridge remains after close")
	}
	t.Log(fmt.Sprintf("same engine and draft after switch; server and engine released on close (%t relay)", relay))
}

// This opt-in probe includes the complete WebView2 process family. It records
// observations without inventing a threshold for a synthetic-device scenario.
func TestNativeFlashChatResources(t *testing.T) {
	if os.Getenv("WINONE_FLASHCHAT_NATIVE_WINDOW") != "1" {
		t.Skip("explicit private desktop required")
	}
	n := newNativeClient(t, "Resource check", false)
	measure := func(scenario string) {
		t.Helper()
		output := filepath.Join(os.Getenv("WINONE_FLASHCHAT_NATIVE_PROFILE"), "resources-"+scenario+".json")
		command := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(os.Getenv("WINONE_TEST_REPOSITORY"), "build", "measure-resources.ps1"), "-RootProcessIds", fmt.Sprint(os.Getpid()), "-OutputPath", output, "-Scenario", "native-flashchat-"+scenario, "-DurationSeconds", "10")
		if data, err := command.CombinedOutput(); err != nil {
			t.Fatalf("resource measurement: %s %v", data, err)
		}
	}
	measure("lobby")
	nativeInvoke(t, n, "shell", "join", map[string]any{"room": "native-resource-" + randomToken()[:16]})
	nativeAwait(t, "resource room", func() bool { return n.p.snapshot.RoomID != "" })
	nativeInvoke(t, n, "media", "camera", map[string]any{"enabled": true})
	nativeAwait(t, "resource camera", func() bool { return n.p.frames["self-camera"] != nil })
	measure("camera")
}
