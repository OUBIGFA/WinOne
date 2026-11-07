// Platform seam for Cindy GhostRuntime. A plugin gets an independent, hidden
// MyGo/WebView2 logic process; its browser has no Node or WinOne Go bindings.
import fs from 'node:fs';
import path from 'node:path';
import http from 'node:http';
import {randomBytes} from 'node:crypto';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import pageShim from './page-shim.js' with {type: 'text'};
import {installedGhostStoragePart} from '../cindy/ghost/apps/desktop/src/shared/pluginIdentity.ts';
import {resolveGhostFilePath, ghostBootHtml, ghostFileMime, GHOST_MAIL_SDK_PATH} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/runtime/ghostFiles.ts';
import {resolveGhostContentPathSync} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostContentTree.ts';

const MAX_LINE = 1 << 20;
// Match the native helper's existing bounded reply budget. In particular the
// source SDK's 50 MiB deposit (base64 + envelope) must not crash at 1 MiB.
const MAX_PLUGIN_REQUEST = 128 << 20;
// The same variables Cindy gives its plugin pages, following the window's scheme.
const pageTheme = ':root{color-scheme:light dark;--panel-bg:#fcfcfc;--surface:#f7f7f7;--text-primary:#1a1a1a;--text-secondary:#666;--border-default:#e4e4e4;background:var(--panel-bg);color:var(--text-primary)}@media (prefers-color-scheme:dark){:root{--panel-bg:#191919;--surface:#222;--text-primary:#eee;--text-secondary:#aaa;--border-default:#333}}';

// The SDK and mail helper are immutable bundle assets, not plugin-controlled
// paths. The controller verifies the current install before every request.
export function createSandboxAdapter({directory, executable, sdkFile, mailSDKFile, receive, endpoint, current, changed, log}) {
  const instances = new Map();
  function create(ghost) {
    const id = installedGhostStoragePart(ghost);
    const token = randomBytes(32).toString('hex');
    const prefix = '/' + token;
    let child, server, mailSDK, closed = false, gone, loadedResolve, loadedReject;
    let inputTail = Promise.resolve(), buffer = Buffer.alloc(0);
    const loaded = new Promise((resolve, reject) => { loadedResolve = resolve; loadedReject = reject; });
    // load can fail before GhostRuntime starts awaiting it.
    void loaded.catch(() => {});
    const send = message => {
      if (closed || !child || !child.stdin.writable) throw Error('Plugin sandbox is not running');
      const line = JSON.stringify(message) + '\n';
      if (Buffer.byteLength(line) > (message.type === 'reply' ? 128 << 20 : MAX_LINE)) throw Error('Plugin sandbox message is too large');
      inputTail = inputTail.then(async () => { if (!child.stdin.write(line)) await once(child.stdin, 'drain'); });
      void inputTail.catch(error => unexpected(error.message));
    };
    function unexpected(reason) {
      if (closed) return;
      loadedReject(Error('Plugin sandbox failed'));
      destroy();
      gone?.({reason});
    }
    function destroy() {
      if (closed) return;
      closed = true;
      if (instances.get(id) === handle) instances.delete(id);
      child?.stdin.end();
      const process = child;
      if (process && process.exitCode === null) {
        const timer = setTimeout(() => { if (process.exitCode === null) process.kill(); }, 2000);
        timer.unref?.(); process.once('exit', () => clearTimeout(timer));
      }
      for (const stream of streams) stream.end();
      streams.clear();
      server?.close();
      loadedReject(Error('Plugin sandbox was closed'));
      changed?.(id);
    }
    async function serve(req, res) {
      // The random path is the same isolation boundary as Cindy's per-plugin
      // scheme/partition. No CORS response or directory path is exposed.
      if (closed || !current(ghost)) { res.writeHead(410); res.end(); return; }
      const address = new URL(req.url, 'http://127.0.0.1');
      const taskQuery = address.pathname === prefix + '/__page_task__' && [...address.searchParams.keys()].length === 1 && /^[A-Za-z0-9-]{1,64}$/.test(address.searchParams.get('session') ?? '');
      if (!address.pathname.startsWith(prefix + '/') || address.search && !taskQuery) { res.writeHead(404); res.end(); return; }
      const localPath = address.pathname.slice(prefix.length);
      res.setHeader('Cache-Control', 'no-store');
      res.setHeader('X-Content-Type-Options', 'nosniff');
      res.setHeader('Content-Security-Policy', "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' https: data:; connect-src 'self'; frame-src 'none'; form-action 'none'; base-uri 'none'");
      if (localPath === '/__pipe') { await invoke(req, res); return; }
      if (localPath === '/__bc/stream' || localPath === '/__bc/post') { res.removeHeader('Content-Security-Policy'); await channel(req, res, localPath); return; }
      if (await endpoint?.(ghost, localPath, req, res)) return;
      if (req.method !== 'GET') { res.writeHead(405); res.end(); return; }
      if (localPath === '/__boot__') {
        const html = ghostBootHtml(token + '/' + ghost.manifest.entry).replace(`src="${GHOST_MAIL_SDK_PATH}"`, `src="${prefix}${GHOST_MAIL_SDK_PATH}"`);
        res.setHeader('Content-Type', 'text/html; charset=utf-8'); res.end(html); return;
      }
      const pageMatch = /^\/__page_(settings|panel|main|task)__$/.exec(localPath);
      if (pageMatch && pageFiles[pageMatch[1]]) { res.setHeader('Content-Type', 'text/html; charset=utf-8'); res.setHeader('Content-Security-Policy', "default-src 'none'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' https: data: blob:; media-src 'self' blob:; font-src 'self' data:; connect-src 'self'; frame-src 'none'; form-action 'none'; base-uri 'self'"); res.end(pageHtml(pageMatch[1])); return; }
      if (localPath === '/__page_shim__') { res.setHeader('Content-Type', 'text/javascript; charset=utf-8'); res.end(pageShim); return; }
      if (localPath === GHOST_MAIL_SDK_PATH) { res.setHeader('Content-Type', 'text/javascript; charset=utf-8'); res.end(mailSDK); return; }
      const requested = resolveGhostFilePath(ghost.dir, localPath);
      if (!requested) { res.writeHead(404); res.end(); return; }
      const relative = path.relative(ghost.dir, requested).split(path.sep).join('/');
      // Use the original no-follow resolver, not a lexical-only containment test.
      const file = resolveGhostContentPathSync(ghost.dir, relative, {expect: 'file', label: 'plugin asset'});
      res.setHeader('Content-Type', ghostFileMime(file));
      const stream = fs.createReadStream(file);
      stream.on('error', () => res.destroy()); stream.pipe(res);
    }
    async function invoke(req, res) {
      res.setHeader('Content-Type', 'application/json; charset=utf-8');
      if (req.method !== 'POST') { res.writeHead(405); res.end(JSON.stringify({error: 'POST required'})); return; }
      const chunks = []; let bytes = 0;
      for await (const chunk of req) {
        bytes += chunk.length;
        if (bytes > MAX_PLUGIN_REQUEST) { res.writeHead(413); res.end(JSON.stringify({error: 'Plugin request is too large'})); return; }
        chunks.push(chunk);
      }
      let request;
      try { request = JSON.parse(Buffer.concat(chunks).toString('utf8')); } catch { res.writeHead(400); res.end(JSON.stringify({error: 'Invalid plugin request'})); return; }
      if (!request || typeof request.id !== 'string' || request.id.length > 128 || !['ghost-pipe:ping', 'ghost-pipe:send'].includes(request.channel)) {
        res.writeHead(400); res.end(JSON.stringify({error: 'Invalid plugin channel'})); return;
      }
      try {
        // Same receiver as native IPC: identity and the install revision are
        // supplied by this server, never by the page's request body.
        const value = await receive(ghost, request);
        res.end(JSON.stringify({value}));
      } catch (error) {
        log?.warn('plugin request failed: ' + String(error?.message ?? error).split('\n')[0].slice(0, 256));
        res.writeHead(400); res.end(JSON.stringify({error: 'Plugin request failed'}));
      }
    }
    // A plugin's settings page opens in a web view of the window. The shim gives
    // its same-origin /kv, /secrets, /oauth... calls the private prefix.
    const pageFiles = {settings: ghost.manifest.settingsHtml, panel: ghost.manifest.panel?.html, main: ghost.manifest.mainView?.html, task: ghost.manifest.taskView?.html};
    function pageHtml(kind) {
      const file = resolveGhostContentPathSync(ghost.dir, pageFiles[kind], {expect: 'file', label: 'plugin page'});
      const html = fs.readFileSync(file, 'utf8');
      const pageDir = path.posix.dirname(pageFiles[kind]);
      const shim = '<base href="' + prefix + '/' + (pageDir === '.' ? '' : pageDir + '/') + '"><style>' + pageTheme + '</style><script src="' + prefix + '/__page_shim__"></script>';
      // Keep the original doctype first: putting shim nodes ahead of it
      // silently switched no-head pages to quirks mode and pinned their body
      // to the initial viewport instead of the author's real content height.
      if(/<head\b[^>]*>/i.test(html))return html.replace(/<head\b[^>]*>/i,match=>match+shim);
      if(/<html\b[^>]*>/i.test(html))return html.replace(/<html\b[^>]*>/i,match=>match+'<head>'+shim+'</head>');
      const doctype=/^(\s*<!doctype\b[^>]*>)/i;
      return doctype.test(html)?html.replace(doctype,match=>match+'<head>'+shim+'</head>'):'<head>'+shim+'</head>'+html;
    }
    // BroadcastChannel across processes: a post reaches every other open stream.
    const streams = new Set();
    async function channel(req, res, route) {
      if (route === '/__bc/stream') {
        res.writeHead(200, {'Content-Type': 'text/event-stream', 'Cache-Control': 'no-store'});
        res.write(':\n\n');
        streams.add(res);
        req.on('close', () => streams.delete(res));
        return;
      }
      const chunks = []; let size = 0;
      for await (const chunk of req) { size += chunk.length; if (size > MAX_PLUGIN_REQUEST) { res.writeHead(413); res.end(); return; } chunks.push(chunk); }
      const text = Buffer.concat(chunks).toString('utf8');
      JSON.parse(text);
      for (const stream of streams) stream.write('data: ' + text.replace(/\n/g, ' ') + '\n\n');
      res.writeHead(204); res.end();
    }
    const handle = {
      pageUrl(kind, sessionId) { if (!pageFiles[kind]) throw Error('This plugin has no such page'); const query = kind === 'task' ? '?session=' + encodeURIComponent(sessionId) : ''; return 'http://127.0.0.1:' + server.address().port + prefix + '/__page_' + kind + '__' + query; },
      async load() {
        if (closed) throw Error('Plugin sandbox was closed');
        const sdk = fs.readFileSync(sdkFile, 'utf8');
        mailSDK = fs.readFileSync(mailSDKFile, 'utf8');
        server = http.createServer((req, res) => { void serve(req, res).catch(() => { if (!res.headersSent) res.writeHead(404); res.end(); }); });
        server.listen(0, '127.0.0.1'); await once(server, 'listening');
        if (closed) throw Error('Plugin sandbox was closed');
        const profile = path.join(directory, id, token);
        fs.mkdirSync(profile, {recursive: true, mode: 0o700});
        const url = `http://127.0.0.1:${server.address().port}${prefix}/__boot__`;
        // Same-origin /kv and /secrets calls keep their author-facing paths;
        // the random scope is supplied by this trusted preload, not the guest.
        // Both pages and logic use one tested relay, including sender exclusion.
        const rpc = `http://127.0.0.1:${server.address().port}${prefix}/__pipe`;
        const routing = 'Object.defineProperty(globalThis,"__winonePluginRpc",{value:' + JSON.stringify(rpc) + '});\nglobalThis.__winonePluginScope=' + JSON.stringify(prefix) + ';\n' + pageShim;
        child = spawn(executable, ['--stdio'], {stdio: ['pipe', 'pipe', 'pipe'], windowsHide: true});
        child.on('error', error => unexpected(error.message));
        child.on('exit', code => { if (!closed) unexpected('exit ' + code); });
        child.stderr.on('data', () => {}); // Never forward guest-controlled text to logs.
        child.stdin.on('error', error => unexpected(error.message));
        child.stdout.on('data', chunk => {
          buffer = Buffer.concat([buffer, chunk]);
          for (;;) {
            const newline = buffer.indexOf(10); if (newline < 0) break;
            const line = buffer.subarray(0, newline); buffer = buffer.subarray(newline + 1);
            if (line.length > MAX_LINE) { unexpected('oversized message'); return; }
            let message; try { message = JSON.parse(line); } catch { unexpected('invalid message'); return; }
            if (message.type === 'loaded') loadedResolve();
            else if (message.type === 'failed') unexpected('document failed');
            else if (message.type === 'request') {
              const request = message.request;
              if (!request || typeof request.id !== 'string' || request.id.length > 128 || !['ghost-pipe:ping', 'ghost-pipe:send'].includes(request.channel)) { unexpected('invalid guest request'); return; }
              void Promise.resolve().then(() => receive(ghost, request)).then(value => {
                if (!closed) send({type: 'reply', payload: {id: request.id, value}});
              }, error => { log?.warn('plugin request failed: ' + String(error?.message ?? error)); if (!closed) send({type: 'reply', payload: {id: request.id, error: 'Plugin request failed'}}); });
            } else { unexpected('unknown message'); return; }
          }
          if (buffer.length > MAX_LINE) unexpected('oversized message');
        });
        instances.set(id, handle);
        send({url, directory: profile, sdk: routing + '\n' + sdk});
        await loaded;
      },
      destroy,
      crashForTest() { child?.kill(); },
      onGone(callback) { gone = callback; },
      send(payload) { send({type: 'deliver', payload}); },
    };
    return handle;
  }
  return {
    create,
    send(id, payload) { const instance = instances.get(id); if (!instance) throw Error('Plugin sandbox is unavailable'); instance.send(payload); },
    pageUrl(ghost, kind, sessionId) { const instance = instances.get(installedGhostStoragePart(ghost)); if (!instance) throw Error('Plugin sandbox is unavailable'); return instance.pageUrl(kind, sessionId); },
    close() { for (const handle of instances.values()) handle.destroy(); },
  };
}
