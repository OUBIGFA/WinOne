//go:build windows

package flashchat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/web"
)

// Live opt-in with the user's second network. The other participant opens a
// compatible web page; only synthetic test messages are exchanged.
func TestLiveNativeFlashChat(t *testing.T) {
	room := os.Getenv("WINONE_FLASHCHAT_LIVE_ROOM")
	if os.Getenv("WINONE_FLASHCHAT_NATIVE_WINDOW") != "1" || room == "" {
		t.Skip("explicit cross-network session")
	}
	profile := os.Getenv("WINONE_FLASHCHAT_NATIVE_PROFILE")
	env := &core.Env{Directory: profile}
	p := newCompletePage(env)
	var window *mygo.Window
	mygo.RunOnMain(func() {
		i18n.SetLocale(i18n.Chinese)
		window = mygo.NewWindow(mygo.WindowOptions{Title: "WinOne cross-network verification", Width: 1000, Height: 800, Hidden: true, Content: ui.View(func(c *ui.Context) { p.View(c) })})
		env.Post = func(fn func()) {
			mygo.RunOnMain(func() {
				if !window.IsDestroyed() {
					window.Update(fn)
				}
			})
		}
		env.WebViews = func(options mygo.WebViewOptions) (*mygo.WebView, error) {
			options.URL += "public/" + room
			options.Page.PreloadScript += `;window.__winoneLegacyNickname='WinOne 验收';window.__testPCs=[];window.__testSockets=[];window.__testICEErrors=[];window.RTCPeerConnection=class extends RTCPeerConnection{constructor(...args){super(...args);window.__testPCs.push(this);this.addEventListener('icecandidateerror',e=>window.__testICEErrors.push({url:e.url,code:e.errorCode,text:e.errorText}))}};window.WebSocket=class extends WebSocket{constructor(...args){super(...args);window.__testSockets.push(this)}};`
			// Record signaling stages without SDP, IP addresses or message content.
			options.Page.PreloadScript += `window.__testSignals=[];const record=(socket,direction,raw)=>{try{const m=JSON.parse(raw);window.__testSignals.push({at:Date.now(),server:socket.url,direction,hash:m.info_hash,peer:m.peer_id,offers:m.offers?.length,offer:!!m.offer,answer:!!m.answer,complete:m.complete,incomplete:m.incomplete});window.__testSignals=window.__testSignals.slice(-80)}catch{}};const PreviousSocket=window.WebSocket;window.WebSocket=class extends PreviousSocket{constructor(...args){super(...args);this.addEventListener('message',e=>record(this,'in',e.data))}send(data){record(this,'out',data);return super.send(data)}};`
			options.Page.PreloadScript += `window.__testTrace=[];const trace=data=>{window.__testTrace.push({at:Date.now(),...data});window.__testTrace=window.__testTrace.slice(-150)};const PreviousPC=window.RTCPeerConnection;window.RTCPeerConnection=class extends PreviousPC{constructor(...args){super(...args);this.traceId=window.__testPCs.length;for(const event of ['connectionstatechange','iceconnectionstatechange','signalingstatechange'])this.addEventListener(event,()=>trace({pc:this.traceId,event,state:this.connectionState,ice:this.iceConnectionState,signaling:this.signalingState}));this.addEventListener('datachannel',e=>{trace({pc:this.traceId,event:'datachannel',state:e.channel.readyState});e.channel.addEventListener('open',()=>trace({pc:this.traceId,event:'channel-open'}))})}async setRemoteDescription(description){trace({pc:this.traceId,event:'remote-description',type:description.type,candidates:[...description.sdp.matchAll(/ typ (\w+)/g)].map(m=>m[1])});try{return await super.setRemoteDescription(description)}catch(e){trace({pc:this.traceId,event:'remote-error',name:e.name});throw e}}};window.addEventListener('unhandledrejection',e=>trace({event:'unhandledrejection',name:e.reason?.name}));`
			return window.NewWebView(options)
		}
		window.Show()
	})
	defer mygo.RunOnMain(func() { p.Close(); window.Destroy() })
	send := func(text string) bool {
		encoded, _ := json.Marshal(text)
		result, err := mygo.EvalAs[bool](p.view.Page(), `(()=>{const e=document.querySelector('textarea');if(!e)return false;Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(e,`+string(encoded)+`);e.dispatchEvent(new Event('input',{bubbles:true}));return true})()`)
		if err != nil || !result {
			return false
		}
		time.Sleep(200 * time.Millisecond)
		_, err = p.view.Page().Eval(`document.querySelector('button[aria-label="发送"]').click()`)
		return err == nil
	}
	sent := false
	deadline := time.Now().Add(30 * time.Minute)
	for time.Now().Before(deadline) {
		var view *mygo.WebView
		mygo.RunOnMain(func() { view = p.view })
		if view != nil {
			body, err := mygo.EvalAs[string](view.Page(), `document.body.innerText`)
			if err == nil {
				os.WriteFile(filepath.Join(profile, "live-status.txt"), []byte(body), 0600)
				if signals, err := mygo.EvalAs[any](view.Page(), `window.__testSignals`); err == nil {
					data, _ := json.MarshalIndent(signals, "", "  ")
					os.WriteFile(filepath.Join(profile, "live-signals.json"), data, 0600)
				}
				if trace, err := mygo.EvalAs[any](view.Page(), `window.__testTrace`); err == nil {
					data, _ := json.MarshalIndent(trace, "", "  ")
					os.WriteFile(filepath.Join(profile, "live-trace.json"), data, 0600)
				}
				// Keep diagnostics even when no peer can connect. Do not record IPs,
				// room secrets, or SDP; candidate classes and errors locate the stage.
				status, statusErr := mygo.EvalAs[any](view.Page(), `(async()=>({sockets:window.__testSockets.map(s=>({url:s.url,state:s.readyState})),errors:window.__testICEErrors,connections:await Promise.all(window.__testPCs.map(async p=>({state:p.connectionState,ice:p.iceConnectionState,signaling:p.signalingState,candidates:[...(await p.getStats()).values()].filter(s=>s.type==='local-candidate'||s.type==='remote-candidate').map(s=>({side:s.type,kind:s.candidateType,protocol:s.protocol}))})))}))()`)
				if statusErr == nil {
					data, _ := json.MarshalIndent(status, "", "  ")
					os.WriteFile(filepath.Join(profile, "live-network.json"), data, 0600)
				}
				if !sent {
					sent = send("WinOne 跨网验证：电脑已进入房间。请发送：手机已连接")
				}
				if strings.Contains(body, "手机已连接") && strings.Contains(body, "WinOne 跨网验证：电脑已进入房间。请发送：手机已连接") {
					// Exclude our own instructions; the received message is a separate line.
					// Accept the natural wording used in the real phone check as well
					// as the suggested phrase, while excluding our instruction text.
					if strings.Contains("\n"+body+"\n", "\n手机已连接\n") || strings.Contains("\n"+body+"\n", "\n手机已经连接\n") {
						send("电脑已收到手机消息，跨网双向通信正常。")
						stats, err := mygo.EvalAs[any](view.Page(), `(async()=>{const pairs=[];for(const pc of window.__testPCs){if(pc.connectionState!=='connected')continue;const stats=await pc.getStats();for(const s of stats.values())if(s.type==='candidate-pair'&&s.state==='succeeded'&&s.nominated){const local=stats.get(s.localCandidateId),remote=stats.get(s.remoteCandidateId);pairs.push({local:local?.candidateType,remote:remote?.candidateType,protocol:local?.protocol,bytesSent:s.bytesSent,bytesReceived:s.bytesReceived})}}return pairs})()`)
						if err != nil {
							t.Fatal(err)
						}
						data, _ := json.MarshalIndent(stats, "", "  ")
						os.WriteFile(filepath.Join(profile, "live-connection.json"), data, 0600)
						frame, _ := view.CapturePage()
						os.WriteFile(filepath.Join(profile, "live-connected.png"), frame, 0600)
						t.Log("native WebView2 received the mobile participant's message")
						time.Sleep(10 * time.Second)
						return
					}
				}
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatal("no cross-network reply within thirty minutes; see live-status.txt and live-network.json")
}

// Explicit opt-in: ordinary tests never open a desktop or use the real profile.
func TestMain(m *testing.M) {
	if os.Getenv("WINONE_FLASHCHAT_NATIVE_WINDOW") != "1" {
		os.Exit(m.Run())
	}
	profile := os.Getenv("WINONE_FLASHCHAT_NATIVE_PROFILE")
	if !filepath.IsAbs(profile) || !strings.Contains(filepath.ToSlash(profile), "/_temp/") {
		panic("private repository profile required")
	}
	// MyGo's Windows backend uses its own LocalAppData WebView2 directory;
	// PathUserData alone does not isolate the browser process. Give each run
	// an explicit folder so simultaneous private desktops cannot share it.
	if err := os.Setenv("WEBVIEW2_USER_DATA_FOLDER", filepath.Join(profile, "webview")); err != nil {
		panic(err)
	}
	mygo.App.SetName("WinOne Flash Chat Checks")
	if err := web.Register(); err != nil {
		panic(err)
	}
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(profile, "webview"))
	mygo.App.SetPath(mygo.PathDownloads, filepath.Join(profile, "downloads"))
	mygo.App.OnWindowAllClosed(func() {})
	code := 1
	mygo.App.WhenReady(func() { go func() { code = m.Run(); mygo.App.Quit() }() })
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

func TestRealNativeFlashChat(t *testing.T) {
	if os.Getenv("WINONE_FLASHCHAT_NATIVE_WINDOW") != "1" {
		t.Skip("explicit private desktop")
	}
	profile := os.Getenv("WINONE_FLASHCHAT_NATIVE_PROFILE")
	env := &core.Env{Directory: profile}
	p := newCompletePage(env)
	var window *mygo.Window
	dark, aside := false, false
	created := 0
	mygo.RunOnMain(func() {
		i18n.SetLocale(i18n.Chinese)
		window = mygo.NewWindow(mygo.WindowOptions{Title: "WinOne Flash Chat verification", Width: 1000, Height: 800, Hidden: true, Content: ui.View(func(c *ui.Context) {
			c.SetTheme(kit.Theme(c.Preferences(), dark))
			if !aside {
				p.View(c)
			} else {
				ui.Text(c, "Other page")
			}
		})})
		env.Post = func(fn func()) {
			mygo.RunOnMain(func() {
				if !window.IsDestroyed() {
					window.Update(fn)
				}
			})
		}
		env.Navigate = func(string, func()) {}
		env.WebViews = func(options mygo.WebViewOptions) (*mygo.WebView, error) { created++; return window.NewWebView(options) }
		window.Show()
	})
	defer mygo.RunOnMain(func() { p.Close(); window.Destroy() })
	deadline := time.Now().Add(40 * time.Second)
	var view *mygo.WebView
	for time.Now().Before(deadline) {
		mygo.RunOnMain(func() { view = p.view })
		if view != nil {
			ready, err := mygo.EvalAs[bool](view.Page(), `!!document.querySelector('.Home')`)
			if err == nil && ready {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if view == nil {
		t.Fatal("web view was not created")
	}
	eval := func(code string) string {
		t.Helper()
		value, err := mygo.EvalAs[string](view.Page(), code)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	if text := eval(`document.body.innerText`); !strings.Contains(text, "加入公开房间") {
		t.Fatalf("local page did not load: %s", text)
	}
	if secure := eval(`String(isSecureContext && !!navigator.mediaDevices?.getUserMedia && !!navigator.mediaDevices?.getDisplayMedia && !!navigator.serviceWorker)`); secure != "true" {
		t.Fatal("required browser capabilities unavailable", secure)
	}
	// The runner supplies synthetic devices in this private desktop. Exercise
	// WebView2's capture and permission path, not only API feature detection.
	if got := eval(`(async()=>{window.__capture=await navigator.mediaDevices.getUserMedia({audio:true,video:true});return window.__capture.getTracks().map(t=>t.kind).sort().join(',')})()`); got != "audio,video" {
		t.Fatal("native media capture failed", got)
	}
	if got := eval(`window.__capture.getTracks().forEach(t=>t.stop());return String(window.__capture.getTracks().every(t=>t.readyState==='ended'))`); got != "true" {
		t.Fatal("native media capture did not stop")
	}
	if got := eval(`(async()=>{const capture=await navigator.mediaDevices.getDisplayMedia({video:true});const kind=capture.getVideoTracks()[0]?.kind;capture.getTracks().forEach(t=>t.stop());return kind+':'+capture.getTracks().every(t=>t.readyState==='ended')})()`); got != "video:true" {
		t.Fatal("native screen capture failed", got)
	}
	eval(`sessionStorage.setItem('winone-lifetime','kept'); 'ok'`)
	mygo.RunOnMain(func() { p.SetAside(); aside = true; window.Invalidate() })
	mygo.RunOnMain(func() {
		aside = false
		dark = true
		i18n.SetLocale(i18n.English)
		window.SetContentSize(560, 800)
		window.Invalidate()
	})
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if eval(`document.documentElement.lang + ':' + document.documentElement.style.colorScheme`) == "en-US:dark" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if eval(`sessionStorage.getItem('winone-lifetime')`) != "kept" {
		t.Fatal("switching page lost the web session")
	}
	if got := eval(`document.documentElement.lang + ':' + document.documentElement.style.colorScheme`); got != "en-US:dark" {
		t.Fatal("host appearance was not applied", got)
	}
	if eval(`String(document.documentElement.scrollWidth <= innerWidth)`) != "true" {
		t.Fatal("horizontal overflow")
	}
	if created != 1 {
		t.Fatalf("page switch created %d web views", created)
	}
	// Wait for finite MUI transitions before recording the final theme.
	// The capability checks above deliberately do not wait for animation timers.
	eval(`Promise.all(document.getAnimations().filter(a=>a.effect?.getTiming().iterations!==Infinity).map(a=>a.finished.catch(()=>{}))).then(()=> 'ok')`)
	screenshot, err := view.CapturePage()
	if err != nil {
		t.Fatal(err)
	}
	if len(screenshot) < 10000 {
		t.Fatal("empty web view screenshot")
	}
	if err := os.WriteFile(filepath.Join(profile, "native-flashchat.png"), screenshot, 0600); err != nil {
		t.Fatal(err)
	}
	// Exercise WebView2's real download event and the locally bundled
	// StreamSaver service worker without writing into the user's Downloads.
	downloads := make(chan *mygo.Download, 2)
	view.Page().OnDownloadDone(func(download *mygo.Download) { downloads <- download })
	eval(`(()=>{const frame=document.createElement('iframe');frame.hidden=true;frame.src='/streamsaver/mitm.html';frame.onload=()=>{const channel=new MessageChannel();const stream=new ReadableStream({start(c){c.enqueue(new TextEncoder().encode('WinOne native stream download'));c.close()}});channel.port1.onmessage=e=>{if(e.data.download){const link=document.createElement('iframe');link.hidden=true;link.src=e.data.download;document.body.appendChild(link)}};channel.port1.postMessage({readableStream:stream},[stream]);frame.contentWindow.postMessage({transferringReadable:true,pathname:'verification/native.txt',headers:{'Content-Disposition':"attachment; filename*=UTF-8''native.txt"}},location.origin,[channel.port2])};document.body.appendChild(frame);return 'started'})()`)
	select {
	case download := <-downloads:
		if download.Err != nil {
			t.Fatal(download.Err)
		}
		data, err := os.ReadFile(download.Path)
		if err != nil || string(data) != "WinOne native stream download" {
			t.Fatal("native download did not match", err)
		}
		if !strings.HasPrefix(filepath.Clean(download.Path), filepath.Join(profile, "downloads")+string(os.PathSeparator)) {
			t.Fatal("download escaped test profile")
		}
	case <-time.After(25 * time.Second):
		t.Fatal("native service-worker download did not complete")
	}
	origin := p.app.origin
	mygo.RunOnMain(func() { p.Close() })
	if !view.IsDestroyed() {
		t.Fatal("web view was not released")
	}
	client := http.Client{Timeout: time.Second}
	if response, err := client.Get(origin); err == nil {
		response.Body.Close()
		t.Fatal("local server remains after close")
	}
}

// Two native containers use distinct local profiles and exchange real messages
// through the public relay. This opt-in check never touches user installations.
func TestNativePeersThroughRelay(t *testing.T) {
	if os.Getenv("WINONE_FLASHCHAT_PUBLIC_RELAY") != "1" {
		t.Skip("explicit public relay verification")
	}
	profile := os.Getenv("WINONE_FLASHCHAT_NATIVE_PROFILE")
	room := fmt.Sprintf("winone-native-pair-%d", time.Now().UnixNano())
	var pages []*completePage
	var windows []*mygo.Window
	for _, name := range []string{"Desktop A", "Desktop B"} {
		env := &core.Env{Directory: filepath.Join(profile, name)}
		p := newCompletePage(env)
		mygo.RunOnMain(func() {
			i18n.SetLocale(i18n.English)
			window := mygo.NewWindow(mygo.WindowOptions{Title: name, Width: 1000, Height: 800, Hidden: true, Content: ui.View(func(c *ui.Context) { p.View(c) })})
			env.Post = func(fn func()) {
				mygo.RunOnMain(func() {
					if !window.IsDestroyed() {
						window.Update(fn)
					}
				})
			}
			env.WebViews = func(options mygo.WebViewOptions) (*mygo.WebView, error) {
				options.URL += "public/" + room
				nickname, _ := json.Marshal(name)
				options.Page.PreloadScript += `;window.__winoneLegacyNickname=` + string(nickname) + `;window.__pairPCs=[];const OriginalPC=window.RTCPeerConnection;window.RTCPeerConnection=class extends OriginalPC{constructor(config,...rest){super({...config,iceTransportPolicy:'relay'},...rest);window.__pairPCs.push(this)}};`
				return window.NewWebView(options)
			}
			pages = append(pages, p)
			windows = append(windows, window)
			window.Show()
		})
	}
	defer mygo.RunOnMain(func() {
		for i, p := range pages {
			p.Close()
			windows[i].Destroy()
		}
	})
	await := func(label string, condition func() bool) {
		t.Helper()
		deadline := time.Now().Add(40 * time.Second)
		for time.Now().Before(deadline) {
			if condition() {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal(label)
	}
	for _, p := range pages {
		await("native room did not load", func() bool {
			var view *mygo.WebView
			mygo.RunOnMain(func() { view = p.view })
			if view == nil {
				return false
			}
			ready, _ := mygo.EvalAs[bool](view.Page(), `!!document.querySelector('textarea')`)
			return ready
		})
	}
	send := func(p *completePage, message string) {
		encoded, _ := json.Marshal(message)
		if _, err := p.view.Page().Eval(`(()=>{const input=document.querySelector('textarea');Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(input,` + string(encoded) + `);input.dispatchEvent(new Event('input',{bubbles:true}))})()`); err != nil {
			t.Fatal(err)
		}
		await("send action did not become available", func() bool {
			ready, _ := mygo.EvalAs[bool](p.view.Page(), `!document.querySelector('button[aria-label="Send"]').disabled`)
			return ready
		})
		if _, err := p.view.Page().Eval(`document.querySelector('button[aria-label="Send"]').click()`); err != nil {
			t.Fatal(err)
		}
	}
	for i, message := range []string{"Desktop A sends through relay", "Desktop B replies through relay"} {
		send(pages[i], message)
		await("the other native window did not receive the message", func() bool {
			body, _ := mygo.EvalAs[string](pages[1-i].view.Page(), `document.body.innerText`)
			return strings.Contains(body, message)
		})
	}
	var paths []any
	for _, p := range pages {
		result, err := mygo.EvalAs[any](p.view.Page(), `(async()=>{const result=[];for(const pc of window.__pairPCs){if(pc.connectionState!=='connected')continue;const stats=await pc.getStats();for(const item of stats.values())if(item.type==='transport'&&item.selectedCandidatePairId){const pair=stats.get(item.selectedCandidatePairId);result.push({local:stats.get(pair.localCandidateId)?.candidateType,remote:stats.get(pair.remoteCandidateId)?.candidateType,sent:pair.bytesSent,received:pair.bytesReceived})}}return result})()`)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, result)
		valid, _ := mygo.EvalAs[bool](p.view.Page(), `(async()=>{for(const pc of window.__pairPCs){if(pc.connectionState!=='connected')continue;const stats=await pc.getStats();for(const item of stats.values())if(item.type==='transport'&&item.selectedCandidatePairId){const pair=stats.get(item.selectedCandidatePairId);if(stats.get(pair.localCandidateId)?.candidateType==='relay'&&pair.bytesSent>0&&pair.bytesReceived>0)return true}}return false})()`)
		if !valid {
			t.Fatal("native messages did not use the required relay")
		}
	}
	data, _ := json.MarshalIndent(paths, "", "  ")
	if err := os.WriteFile(filepath.Join(profile, "native-pair-paths.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
