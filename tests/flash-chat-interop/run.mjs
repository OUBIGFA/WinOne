// Interoperability test of WinOne's native flash chat (Go,
// apps/desktop-mygo/flashchat) with the React/Chitchatter flash chat page
// (apps/desktop/frontend, built). A headless Microsoft Edge runs the built
// interconnect-chat.html inside a harness that answers its Host calls the
// way the React window's frame did; a Go peer (flashchat/internal/
// interoppeer) runs the same protocol natively; a local WebTorrent tracker
// from the Go peer replaces the public ones. Both directions are tested:
// the page creates a room the Go peer joins, then the Go peer creates one
// the page joins.
//
// Usage (from the repository root):
//   node tests/flash-chat-interop/run.mjs [--edge <msedge.exe>] [--dist <frontend dist>] [--peer <interoppeer.exe>] [--out <dir>]
// Environment: WINONE_PLAYWRIGHT_MODULE (default dist/resources/tools/
// playwright-1.58.2/node_modules/playwright), WINONE_GO (go.exe, used to
// build the peer when --peer is not given).
import {createRequire} from "node:module";
import {spawn, spawnSync} from "node:child_process";
import {createServer} from "node:http";
import {mkdirSync, readFileSync, existsSync, writeFileSync} from "node:fs";
import {resolve, dirname, extname, join} from "node:path";
import {fileURLToPath} from "node:url";
import {createInterface} from "node:readline";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const arg = (name, fallback) => {
  const i = process.argv.indexOf(name);
  return i > 0 ? process.argv[i + 1] : fallback;
};
const out = resolve(arg("--out", join(root, "_temp/flash-chat-interop")));
mkdirSync(out, {recursive: true});
const dist = resolve(arg("--dist", join(root, "apps/desktop/frontend/dist")));
const edge = arg("--edge", "C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe");
if (!existsSync(join(dist, "interconnect-chat.html"))) throw Error(`Build the React frontend first: ${dist}/interconnect-chat.html is missing`);
const require = createRequire(join(root, "package.json"));
const {chromium} = require(process.env.WINONE_PLAYWRIGHT_MODULE || join(root, "dist/resources/tools/playwright-1.58.2/node_modules/playwright"));

// ---------------------------------------------------------------- Go peer
let peerExe = arg("--peer");
if (!peerExe) {
  peerExe = join(out, "interoppeer.exe");
  const go = process.env.WINONE_GO || join(root, "dist/resources/tools/go-1.27.3/bin/go.exe");
  const env = {...process.env, GOROOT: dirname(dirname(go)), GOTOOLCHAIN: "local", CGO_ENABLED: "0", GOCACHE: process.env.GOCACHE || join(root, "dist/resources/cache/go")};
  const built = spawnSync(go, ["build", "-o", peerExe, "./flashchat/internal/interoppeer"], {cwd: join(root, "apps/desktop-mygo"), env, stdio: "inherit"});
  if (built.status !== 0) throw Error("cannot build the Go peer");
}
const peer = spawn(peerExe, [], {stdio: ["pipe", "pipe", "pipe"]});
let peerLog = "";
peer.stderr.on("data", d => { peerLog += d; });
const replies = [];
let waiting = null;
createInterface({input: peer.stdout}).on("line", line => {
  const value = JSON.parse(line);
  if (waiting) { const w = waiting; waiting = null; w(value); } else replies.push(value);
});
async function go(command) {
  peer.stdin.write(JSON.stringify(command) + "\n");
  const value = replies.length ? replies.shift() : await new Promise(r => { waiting = r; });
  if (value.error) throw Error(`Go peer ${command.cmd}: ${value.error}`);
  return value;
}

// ----------------------------------------------------------- page harness
// The harness plays the React window's PeerChatFrame: it hosts the page
// in an iframe of its own origin and answers interconnect-native calls.
const harness = `<!doctype html><meta charset="utf-8"><style>html,body{margin:0;height:100%}iframe{border:0;width:100%;height:100%}</style>
<iframe id="chat" src="/interconnect-chat.html"></iframe><script>
window.calls = [];
const shares = {}; let n = 0;
const frame = document.getElementById("chat");
frame.addEventListener("load", () => frame.contentWindow.postMessage({type:"interconnect-theme", language:"zh-CN", theme:"light"}, location.origin));
addEventListener("message", event => {
  if (event.source !== frame.contentWindow) return;
  const data = event.data; if (data?.type !== "interconnect-native") return;
  calls.push({method: data.method, params: data.params});
  let result;
  switch (data.method) {
    case "preferences.get": result = {nickname: "Edge"}; break;
    case "preferences.save": result = {nickname: data.params.nickname}; break;
    case "files": result = [{path: "C:\\\\interop\\\\report.pdf", name: "report.pdf", size: 3, type: "application/octet-stream"}]; break;
    case "share": { const id = "share-" + (++n); shares[id] = {id, spec: {mode: "share_server"}, state: {status: "running", ready: true, relayReady: true, code: "tc-edge-" + n}}; result = {id}; break; }
    case "connect": { const id = "connect-" + (++n); shares[id] = {id, spec: {mode: "share_client"}, state: {status: "running", ready: true, address: location.host + "/received.html"}}; result = {id}; break; }
    case "list": result = Object.values(shares); break;
    case "stop": delete shares[data.params]; result = null; break;
    default: frame.contentWindow.postMessage({type: "interconnect-result", id: data.id, error: "unsupported"}, location.origin); return;
  }
  frame.contentWindow.postMessage({type: "interconnect-result", id: data.id, result}, location.origin);
});
</script>`;
const types = {".html": "text/html; charset=utf-8", ".js": "text/javascript", ".css": "text/css", ".woff2": "font/woff2", ".svg": "image/svg+xml", ".png": "image/png", ".json": "application/json", ".mjs": "text/javascript", ".wasm": "application/wasm"};
const server = createServer((req, res) => {
  const path = decodeURIComponent(new URL(req.url, "http://x").pathname);
  if (path === "/harness.html") { res.writeHead(200, {"content-type": types[".html"]}); res.end(harness); return; }
  if (path === "/favicon.ico") { res.writeHead(204); res.end(); return; }
  if (path === "/received.html") { res.writeHead(200, {"content-type": types[".html"]}); res.end("<!doctype html><title>received</title><p>share page</p>"); return; }
  const file = resolve(dist, "." + path);
  if (!file.startsWith(dist) || !existsSync(file)) { res.writeHead(404); res.end(); return; }
  res.writeHead(200, {"content-type": types[extname(file)] || "application/octet-stream"});
  res.end(readFileSync(file));
});
await new Promise(r => server.listen(0, "127.0.0.1", r));
const origin = `http://127.0.0.1:${server.address().port}`;

// --------------------------------------------------------------- checks
const results = [];
async function check(name, fn) {
  const started = Date.now();
  try { await fn(); results.push({name, ok: true, ms: Date.now() - started}); console.log("PASS", name); }
  catch (error) { results.push({name, ok: false, error: String(error)}); console.log("FAIL", name, String(error)); throw error; }
}
async function until(label, fn, ms = 30000) {
  const deadline = Date.now() + ms; let last;
  for (;;) {
    try { const v = await fn(); if (v) return v; } catch (e) { last = e; }
    if (Date.now() > deadline) throw Error(`timed out: ${label}${last ? " (" + last + ")" : ""}`);
    await new Promise(r => setTimeout(r, 150));
  }
}
const state = () => go({cmd: "state"});

const browser = await chromium.launch({executablePath: edge, headless: true,
  // Real host candidates instead of mDNS names, so the native peer can
  // reach the page on this computer without multicast DNS.
  args: ["--disable-features=WebRtcHideLocalIpsWithMdns"]});
let exitCode = 0;
try {
  const tracker = (await go({cmd: "tracker"})).url;
  const rtc = '{"iceServers":[]}';
  const page = await browser.newPage({viewport: {width: 1100, height: 760}});
  page.on("console", m => { if (m.type() === "error") console.log("[page]", m.text()); });
  await page.goto(origin + "/harness.html");
  const chat = page.frameLocator("#chat");
  const text = () => page.frames().find(f => f.url().endsWith("/interconnect-chat.html")).evaluate(() => document.body.innerText);

  async function networkSettings() {
    await chat.getByRole("button", {name: "网络设置", exact: true}).click();
    await chat.getByLabel("信令服务（每行一个 WebSocket 地址）").fill(tracker);
    await chat.getByLabel("WebRTC 配置（JSON，可设置 STUN / TURN）").fill(rtc);
    await chat.getByRole("button", {name: "关闭", exact: true}).first().click();
  }

  // ---- Direction 1: the page creates, the native peer joins.
  let room, password, edgePeer;
  await check("page creates a room on the local tracker", async () => {
    await until("nickname loaded", async () => (await chat.getByLabel("个人昵称").inputValue()) === "Edge");
    await networkSettings();
    await chat.getByRole("button", {name: "创建房间", exact: true}).click();
    await chat.getByRole("button", {name: "房间信息", exact: true}).click();
    room = await chat.locator("#chat-active-room").inputValue();
    password = await chat.locator("#chat-active-password").inputValue();
    await page.keyboard.press("Escape");
    if (!room || !password) throw Error("no room details");
  });
  await check("page message sent before the native peer joins", async () => {
    await chat.getByLabel("输入消息，支持 Markdown 和代码块").fill("history from edge");
    await chat.getByLabel("输入消息，支持 Markdown 和代码块").press("Enter");
    await until("own message shown", async () => (await text()).includes("history from edge"));
    // Over one 16 KiB frame: the history then travels as a chunked transfer.
    await chat.getByLabel("输入消息，支持 Markdown 和代码块").fill("big-edge " + "长".repeat(9000));
    await chat.getByLabel("输入消息，支持 Markdown 和代码块").press("Enter");
    await until("big message shown", async () => (await text()).includes("big-edge"));
  });
  await check("native peer joins the page's room and verifies it", async () => {
    await go({cmd: "join", roomId: room, password, nickname: "MyGo", trackers: [tracker], rtc});
    const s = await until("native peer sees Edge", async () => { const s = await state(); return s.peers?.length === 1 && s.peers[0].name === "Edge" ? s : null; });
    if (!s.peers[0].verified) throw Error("Edge not verified: " + JSON.stringify(s.peers));
    edgePeer = s.peers[0].peerId;
  });
  await check("history reaches the native peer on join, chunked over 16 KiB", async () => {
    await until("history", async () => (await state()).group.some(m => m.text === "history from edge" && m.received));
    await until("big history", async () => (await state()).group.some(m => m.text === "big-edge " + "长".repeat(9000)));
  });
  await check("page lists the native peer, verified", async () => {
    await until("member shown", async () => { const t = await text(); return t.includes("MyGo (") && t.includes("身份已校验"); });
  });
  await check("group message native → page", async () => {
    await go({cmd: "send", text: "hello from **mygo**"});
    await until("page shows it", async () => (await text()).includes("hello from mygo"));
  });
  await check("group message page → native", async () => {
    await chat.getByLabel("输入消息，支持 Markdown 和代码块").fill("hello from edge");
    await chat.getByRole("button", {name: "发送消息", exact: true}).click();
    await until("native has it", async () => (await state()).group.some(m => m.text === "hello from edge"));
  });
  await check("typing page → native", async () => {
    await chat.getByLabel("输入消息，支持 Markdown 和代码块").pressSequentially("typ");
    await until("native sees typing", async () => (await state()).peers[0].typingGroup, 5000);
    await chat.getByLabel("输入消息，支持 Markdown 和代码块").fill("");
  });
  await check("typing native → page", async () => {
    await go({cmd: "typing"});
    await until("page shows typing", async () => (await text()).includes("正在输入"), 5000);
  });
  await check("private message native → page", async () => {
    await go({cmd: "dm", peer: edgePeer, text: "dm from mygo"});
    await chat.getByRole("button", {name: /^私聊 · MyGo/}).click();
    await until("page shows the private message", async () => (await text()).includes("dm from mygo"));
  });
  await check("private message page → native", async () => {
    await chat.getByLabel("输入消息，支持 Markdown 和代码块").fill("dm from edge");
    await chat.getByLabel("输入消息，支持 Markdown 和代码块").press("Enter");
    await until("native has the private message", async () => (await state()).dms[edgePeer]?.some(m => m.text === "dm from edge"));
    const s = await state();
    if (s.group.some(m => m.text.startsWith("dm from"))) throw Error("private message reached the group");
    await chat.getByRole("button", {name: "全部成员", exact: true}).click();
    if ((await text()).includes("dm from edge")) throw Error("page shows the private message in the group");
  });
  await check("file offer native → page, and the page receives through its Host", async () => {
    const {code} = await go({cmd: "offer", files: [{name: "notes.txt", type: "text/plain", size: 5, path: "C:\\interop\\notes.txt"}]});
    await until("page shows the offer", async () => (await text()).includes("浏览并接收"));
    await chat.getByRole("button", {name: "浏览并接收", exact: true}).first().click();
    await until("page asked its Host to connect", async () => (await page.evaluate(() => calls)).some(c => c.method === "connect" && c.params === code));
    // The page opens the local share address its Host returned.
    await until("share page shown", async () => (await chat.locator('iframe[title="浏览并接收"]').getAttribute("src"))?.endsWith("/received.html"));
    await chat.locator("[data-slot=dialog-close]").click();
  });
  await check("file offer page → native", async () => {
    await chat.getByRole("button", {name: "分享文件到房间", exact: true}).click();
    await until("native sees the offer", async () => (await state()).offers[edgePeer]?.magnetURI?.startsWith("tc-edge-"));
  });
  await check("page stops its offer, native sees it withdrawn", async () => {
    await chat.getByRole("button", {name: "停止分享文件", exact: true}).click();
    await until("withdrawn", async () => !(await state()).offers[edgePeer]);
  });
  await check("native nickname change reaches the page", async () => {
    await go({cmd: "nickname", nickname: "MyGo2"});
    await until("renamed", async () => (await text()).includes("MyGo2 ("));
  });
  await check("page nickname change reaches the native peer", async () => {
    await chat.getByRole("button", {name: "房间信息", exact: true}).click();
    const nick = chat.getByRole("dialog").getByLabel("个人昵称");
    await nick.fill("Edge2");
    await nick.blur();
    await page.keyboard.press("Escape");
    await until("renamed", async () => (await state()).peers[0]?.name === "Edge2");
  });
  await check("native peer leaves, page sees it go", async () => {
    await go({cmd: "leave"});
    await until("page waits again", async () => (await text()).includes("等待其他成员加入"), 20000);
  });
  await page.close();

  // ---- Direction 2: the native peer creates, the page joins.
  const page2 = await browser.newPage({viewport: {width: 1100, height: 760}});
  await page2.goto(origin + "/harness.html");
  const chat2 = page2.frameLocator("#chat");
  const text2 = () => page2.frames().find(f => f.url().endsWith("/interconnect-chat.html")).evaluate(() => document.body.innerText);
  let created;
  await check("native peer creates a room; page joins with its id and password", async () => {
    created = await go({cmd: "join", nickname: "MyGo", trackers: [tracker], rtc});
    await go({cmd: "send", text: "history from mygo"});
    await go({cmd: "send", text: "big-mygo " + "史".repeat(9000)});
    await until("nickname loaded", async () => (await chat2.getByLabel("个人昵称").inputValue()) === "Edge");
    await chat2.getByRole("button", {name: "网络设置", exact: true}).click();
    await chat2.getByLabel("信令服务（每行一个 WebSocket 地址）").fill(tracker);
    await chat2.getByLabel("WebRTC 配置（JSON，可设置 STUN / TURN）").fill(rtc);
    await chat2.getByRole("button", {name: "关闭", exact: true}).first().click();
    await chat2.getByRole("button", {name: "加入房间", exact: true}).click();
    await chat2.getByLabel("房间标识").fill(created.roomId);
    await chat2.getByLabel("房间密码").fill(created.password);
    await chat2.getByRole("button", {name: "加入房间", exact: true}).click();
    await until("page meets the native peer", async () => { const s = await state(); return s.peers?.length === 1 && s.peers[0].verified; });
  });
  await check("history reaches the page on join, chunked over 16 KiB", async () => {
    await until("page shows history", async () => (await text2()).includes("history from mygo"));
    await until("page shows the big message", async () => (await text2()).includes("big-mygo " + "史".repeat(9000)));
  });
  await check("messages both ways in the native peer's room", async () => {
    await go({cmd: "send", text: "second from mygo"});
    await until("page", async () => (await text2()).includes("second from mygo"));
    await chat2.getByLabel("输入消息，支持 Markdown 和代码块").fill("second from edge");
    await chat2.getByLabel("输入消息，支持 Markdown 和代码块").press("Enter");
    await until("native", async () => (await state()).group.some(m => m.text === "second from edge"));
  });
  await check("page leaves, native peer sees it go", async () => {
    await chat2.getByRole("button", {name: "离开房间", exact: true}).first().click();
    await until("gone", async () => (await state()).peers == null || (await state()).peers.length === 0, 20000);
    const errors = (await state()).errors;
    if (errors?.length) throw Error("native errors: " + errors.join("; "));
  });
} catch (error) {
  exitCode = 1;
  console.error(error);
  console.error(peerLog.slice(-4000));
} finally {
  await go({cmd: "quit"}).catch(() => {});
  peer.kill();
  await browser.close();
  server.close();
  const passed = results.filter(r => r.ok).length;
  writeFileSync(join(out, "results.json"), JSON.stringify({edge, dist, at: new Date().toISOString(), results}, null, 1));
  console.log(`${passed}/${results.length} checks passed`);
  process.exit(exitCode || (passed === results.length ? 0 : 1));
}
