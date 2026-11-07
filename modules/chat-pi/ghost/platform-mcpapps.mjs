// Reuse Cindy's complete local MCP Apps backend and official AppBridge. Native
// WebView2 is only the content/transport host; it neither interprets tool calls
// nor grants capabilities, and the original plugin business call is not replayed.
import fs from 'node:fs';
import fsp from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import {Readable} from 'node:stream';
import {once} from 'node:events';
import {createHash, randomBytes, randomUUID, timingSafeEqual} from 'node:crypto';
import {McpAppsHost} from '../cindy/ghost/apps/desktop/src/main/mcp-apps/host.ts';
import {setMcpAppsHost, readAppServer} from '../cindy/ghost/apps/desktop/src/main/mcp-apps/runtime.ts';
import {publishPluginAppResult, pluginModelResult} from '../cindy/ghost/apps/desktop/src/main/mcp-apps/pluginCards.ts';
import {refreshPluginApps, setPluginPageToolRunner} from '../cindy/ghost/apps/desktop/src/main/mcp-integrations/plugin-app-registry.ts';
import {createPluginFileAccess} from '../cindy/ghost/apps/desktop/src/main/mcp-apps/pluginFileAccess.ts';
import {handleMcpAppOperation, handleMcpAppFrameOperation} from '../cindy/ghost/apps/desktop/src/main/mcp-apps/operations.ts';
import {mcpAppDirectSandboxDocument} from '../cindy/ghost/apps/desktop/src/main/mcp-apps/sandboxBridge.ts';
import {saveAppDownload} from '../cindy/ghost/apps/desktop/src/main/mcp-apps/downloadFile.ts';
import {MCP_APPS_MAX_BYTES} from '../cindy/ghost/apps/desktop/src/main/mcp-apps/protocol.ts';
import {parseMcpAppRecord, parseMcpAppHistory, HOST_APP_SDK_PATH} from '../cindy/ghost/packages/maker-shared/src/mcpApps.ts';
import {withHostAppSdk} from '../cindy/ghost/packages/maker-shared/src/mcpAppSdk.ts';
import {McpAppHeightGuard} from '../cindy/ghost/packages/maker-shared/src/mcpAppController.ts';
import {ghostFileMime} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/runtime/ghostFiles.ts';
import {configureNativeMcpAppDocuments} from './seams/mcp-app-documents.ts';

const digest = value => createHash('sha256').update(value).digest('hex');
const VIEW_ID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const CALLER = /^native:[0-9]{1,10}:[A-Za-z0-9_-]{8,128}$/;
const unavailable = () => Error('MCP_APP_VIEW_UNAVAILABLE');

/** Windows custom schemes have real, separate *.localhost origins in MyGo. */
export function nativeMcpAppOrigins(viewId) {
  if (!VIEW_ID.test(viewId)) throw unavailable();
  return {proxy: `http://${viewId}.cindy-mcp-app.localhost`, resource: `http://${viewId}-view.cindy-mcp-app.localhost`};
}
export function nativeMcpAppFileUrls(value, viewId) {
  const {resource} = nativeMcpAppOrigins(viewId);
  const visit = input => {
    if (typeof input === 'string') {
      const prefix = `cindy-mcp-app://${viewId}-view/`;
      return input.startsWith(prefix) ? resource + input.slice(prefix.length - 1) : input;
    }
    if (Array.isArray(input)) return input.map(visit);
    if (!input || typeof input !== 'object') return input;
    return Object.fromEntries(Object.entries(input).map(([key, item]) => [key, visit(item)]));
  };
  return visit(value);
}

// The native wrapper observes only the original trusted proxy's self-posted UI
// state. App RPC still travels through the upstream direct-sandbox document.
const NATIVE_STATE_OBSERVER = `<script>window.addEventListener('message',event=>{
 if(event.source!==window||event.origin!==location.origin)return;
 const message=event.data;
 if(!message||message.jsonrpc!=='2.0'||!['ui/notifications/initialized','ui/notifications/size-changed','ui/notifications/host-context-changed','ui/notifications/sandbox-failed','ui/notifications/sandbox-escape'].includes(message.method))return;
 void fetch('/state',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(message)}).catch(()=>{});
});
void fetch('/environment',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({
 timeZone:Intl.DateTimeFormat().resolvedOptions().timeZone,
 deviceCapabilities:{touch:navigator.maxTouchPoints>0,hover:matchMedia('(hover: hover)').matches}
})}).catch(()=>{});
</script>`;

export function nativeMcpAppResource(html, viewId) {
  const {resource} = nativeMcpAppOrigins(viewId);
  const prepared = withHostAppSdk(html, resource + HOST_APP_SDK_PATH);
  // WebView2's frame Cancel can replace the document with chrome-error://.
  // Cancel ordinary in-document navigation before that native boundary, so
  // authors cannot accidentally lose their form. This is UX only: genuine COM
  // frame/origin checks remain authoritative and revoke a bypassing document.
  const guard = `(()=>{const expected=${JSON.stringify(resource)};
 if(window.navigation)window.navigation.addEventListener('navigate',event=>{
  let target;try{target=new URL(event.destination.url);}catch{event.preventDefault();return;}
  if(target.origin!==expected||target.pathname!=='/resource'||target.search)event.preventDefault();
 },{capture:true});})();`;
  return prepared.replace('<script data-cindy-runtime>', '<script data-cindy-runtime>' + guard);
}

export function createGhostMcpApps({root, manager, find, current, ownerScope, host, push, log,
  runPageTool, authorizeTool, sendMessage, resolveLibraryPath, handoff, revisionKey}) {
  let closed = false;
  const scope = ownerScope.capture(), owner = {};
  const active = () => !closed && ownerScope.isCurrent(scope) && ownerScope.isStable();
  const documents = new Map(), writes = new Map(), nativeOwners = new Map(), retiredCallers = new Map();
  const cardsRoot = path.join(root, 'mcp-app-cards');
  let server, address, starting;
  if (!revisionKey) {
    const keyFile = path.join(root, 'mcp-app-revision.key');
    fs.mkdirSync(root, {recursive: true, mode: 0o700});
    try { fs.writeFileSync(keyFile, randomBytes(32).toString('hex'), {flag: 'wx', mode: 0o600}); }
    catch (error) { if (error.code !== 'EEXIST') throw error; }
    const key = fs.readFileSync(keyFile, 'utf8');
    if (!/^[a-f0-9]{64}$/.test(key)) throw Error('MCP_APP_REVISION_KEY_UNAVAILABLE');
    revisionKey = () => key;
  }
  const sessionEpoch = snapshot => snapshot.mcpAppEpoch ?? snapshot.clearedAt ?? null;
  const cardPath = (sessionId, clientId) => path.join(cardsRoot, digest(sessionId), digest(clientId) + '.json');
  async function sessionSnapshot(sessionId) {
    if (!active()) throw unavailable();
    const snapshot = await host('session-snapshot', {sessionId});
    if (!active() || !snapshot || snapshot.remoteHostId != null || snapshot.archived === true)
      throw Error('MCP_APP_SESSION_UNAVAILABLE');
    return snapshot;
  }
  const readCard = async (sessionId, clientId) => {
    const snapshot = await sessionSnapshot(sessionId);
    const envelope = JSON.parse(await fsp.readFile(cardPath(sessionId, clientId), 'utf8'));
    const record = parseMcpAppRecord(envelope);
    if (!record || envelope.sessionId !== sessionId || envelope.clientId !== clientId ||
        envelope.epoch !== sessionEpoch(snapshot)) throw Error('MCP_APP_CARD_UNAVAILABLE');
    return record;
  };
  const writeCard = (sessionId, clientId, record) => {
    const target = cardPath(sessionId, clientId);
    const previous = writes.get(target) ?? Promise.resolve();
    const work = previous.catch(() => {}).then(async () => {
      const snapshot = await sessionSnapshot(sessionId);
      const temporary = target + '.' + randomUUID() + '.tmp';
      await fsp.mkdir(path.dirname(target), {recursive: true, mode: 0o700});
      try {
        await fsp.writeFile(temporary, JSON.stringify({sessionId, clientId, epoch: sessionEpoch(snapshot), mcpApp: record}), {flag: 'wx', mode: 0o600});
        const latest = await sessionSnapshot(sessionId);
        if (sessionEpoch(latest) !== sessionEpoch(snapshot)) throw Error('MCP_APP_CARD_UNAVAILABLE');
        await fsp.rename(temporary, target);
      } finally { await fsp.rm(temporary, {force: true}); }
      // Only public history/title and the card identity reach the native UI.
      const summary = parseMcpAppHistory({mcpApp: record});
      push('mcp-app', {sessionId, clientId, ...summary});
    });
    writes.set(target, work);
    void work.finally(() => { if (writes.get(target) === work) writes.delete(target); }).catch(() => {});
    return work;
  };
  const pluginFiles = createPluginFileAccess({
    libraryPath: (id, relative) => resolveLibraryPath ? resolveLibraryPath(id, relative) : Promise.resolve(null),
    libraryMime: ghostFileMime,
    openHandoff: (id, ref) => handoff?.store.openForPage(id, ref) ?? Promise.resolve(null),
  });
  const backend = new McpAppsHost({
    owner: () => active() ? owner : undefined,
    config: readAppServer, revisionKey, readCard, writeCard, pluginFiles,
    authorizeTool: async (...input) => {
      if (!authorizeTool) throw Error('MCP_APP_APPROVAL_UNAVAILABLE');
      await authorizeTool(...input);
    },
    sendMessage: async (...input) => {
      if (!sendMessage) throw Error('MCP_APP_MESSAGE_UNAVAILABLE');
      await sendMessage(...input);
    },
    downloadFile: input => saveAppDownload({...input, showSaveDialog: async name => {
      const answer = await host('ui', {kind: 'save', defaultPath: name});
      return answer?.path ?? null;
    }}),
    disposeDocument: id => documents.delete(id),
  });
  setMcpAppsHost(backend);
  setPluginPageToolRunner(async (grant, args, signal) => {
    if (!runPageTool) throw Error('MCP_APP_HOST_NOT_READY');
    await grant.assertCurrent();
    const result = await runPageTool(grant, args, signal);
    await grant.assertCurrent();
    return result;
  });
  const clearDocuments = configureNativeMcpAppDocuments(() => documents.clear());
  function refresh() {
    refreshPluginApps(manager.list(), {find, evidence: id => manager.approvedInstallEvidence(id), available: active});
  }
  refresh();

  async function body(req) {
    const chunks = []; let bytes = 0;
    for await (const chunk of req) {
      bytes += chunk.length;
      if (bytes > MCP_APPS_MAX_BYTES) throw Error('MCP_APP_MESSAGE_TOO_LARGE');
      chunks.push(chunk);
    }
    return JSON.parse(Buffer.concat(chunks).toString('utf8'));
  }
  async function startServer() {
    if (address) return address;
    starting ??= (async () => {
      server = http.createServer(async (req, res) => {
        const parts = req.url?.split('/');
        const viewId = parts?.[1], document = documents.get(viewId);
        const expected = document ? Buffer.from('Bearer ' + document.key) : Buffer.alloc(0);
        const supplied = Buffer.from(req.headers.authorization ?? '');
        if (!document || !active() || supplied.length !== expected.length || !timingSafeEqual(supplied, expected)) {
          res.writeHead(403); res.end(); return;
        }
        const lifetime = new AbortController();
        res.on('close', () => lifetime.abort());
        try {
          if (parts.length === 3 && ['sandbox', 'resource', 'sdk'].includes(parts[2]) && req.method === 'GET') {
            await backend.assertViewAuthorized(document.caller, document.sessionId, viewId);
            const {proxy, resource} = nativeMcpAppOrigins(viewId), source = document.document;
            const sandbox = parts[2] === 'sandbox', sdk = parts[2] === 'sdk';
            if (sdk && !source.sdk) { res.writeHead(404); res.end(); return; }
            const text = sdk ? source.sdk.source : sandbox
              ? mcpAppDirectSandboxDocument(resource + '/resource', proxy + '/bridge')
                .replace('if(stopped||event.source!==frame.contentWindow)return;', `if(stopped||event.source!==frame.contentWindow||event.origin!==${JSON.stringify(resource)})return;`)
                .replace('</body>', NATIVE_STATE_OBSERVER + '</body>')
              : nativeMcpAppResource(source.html, viewId);
            res.writeHead(200, {'Content-Type': sdk ? 'text/javascript; charset=utf-8' : 'text/html; charset=utf-8',
              'Cache-Control': 'no-store', 'X-Content-Type-Options': 'nosniff',
              ...(!sdk ? {'Content-Security-Policy': sandbox
                ? `default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; frame-src ${resource}; connect-src 'self'; object-src 'none'; base-uri 'none'; form-action 'none'`
                : source.csp, 'Permissions-Policy': 'camera=(), microphone=(), geolocation=(), clipboard-write=()'} : {})});
            res.end(text);
          } else if (parts.length === 3 && parts[2] === 'environment' && req.method === 'POST') {
            await backend.assertViewAuthorized(document.caller, document.sessionId, viewId);
            const environment = await body(req);
            await handleMcpAppOperation(backend, document.caller, document.sessionId, {op: 'context', viewId,
              context: {timeZone: environment.timeZone, deviceCapabilities: environment.deviceCapabilities}});
            res.writeHead(200, {'Content-Type': 'application/json', 'Cache-Control': 'no-store'}); res.end('{}');
          } else if (parts.length === 3 && parts[2] === 'state' && req.method === 'POST') {
            await backend.assertViewAuthorized(document.caller, document.sessionId, viewId);
            const message = await body(req);
            if (!message || message.jsonrpc !== '2.0') throw unavailable();
            const state = document.state;
            switch (message.method) {
              case 'ui/notifications/initialized': state.initialized = true; break;
              case 'ui/notifications/size-changed': {
                const height = message.params?.height;
                if (typeof height !== 'number' || !Number.isFinite(height)) throw unavailable();
                state.height = document.heightGuard.accept(height);
                break;
              }
              case 'ui/notifications/host-context-changed': {
                const mode = message.params?.displayMode;
                if (mode === 'inline' || mode === 'fullscreen') state.displayMode = mode;
                break;
              }
              case 'ui/notifications/sandbox-escape': backend.displayMode(document.caller, document.sessionId, viewId, 'inline'); state.displayMode = 'inline'; break;
              case 'ui/notifications/sandbox-failed': state.failed = true; break;
              default: throw unavailable();
            }
            res.writeHead(200, {'Content-Type': 'application/json', 'Cache-Control': 'no-store'});
            res.end(JSON.stringify(state));
          } else if (parts.length === 3 && parts[2] === 'bridge' && req.method === 'POST') {
            const result = await handleMcpAppFrameOperation(backend, document.caller, document.sessionId, viewId,
              await body(req), url => host('open-external', {url}));
            if (documents.get(viewId) !== document || !active()) throw unavailable();
            res.writeHead(200, {'Content-Type': 'application/json', 'Cache-Control': 'no-store'});
            res.end(JSON.stringify(nativeMcpAppFileUrls(result, viewId)));
          } else if (parts.length === 4 && parts[2] === 'files' && VIEW_ID.test(parts[3]) && ['GET', 'HEAD'].includes(req.method)) {
            // No base64/JSON copy: original View-local grants and Range stream
            // straight to the native protocol response, with cancellation.
            await backend.assertViewAuthorized(document.caller, document.sessionId, viewId);
            const response = await backend.readPluginFile(viewId, parts[3], new Request('https://cindy.invalid/',
              {method: req.method, headers: req.headers.range ? {Range: req.headers.range} : {}, signal: lifetime.signal}));
            res.writeHead(response.status, Object.fromEntries(response.headers));
            if (response.body) Readable.fromWeb(response.body).on('error', () => res.destroy()).pipe(res);
            else res.end();
          } else { res.writeHead(404); res.end(); }
        } catch (error) {
          if (!res.headersSent) { res.writeHead(400, {'Content-Type': 'application/json', 'Cache-Control': 'no-store'}); res.end(JSON.stringify({error: /^MCP_APP_/.test(error.message) ? error.message : 'MCP_APP_OPERATION_FAILED'})); }
          else res.destroy();
        }
      });
      server.listen(0, '127.0.0.1');
      await once(server, 'listening');
      if (!active()) { server.close(); throw unavailable(); }
      address = `http://127.0.0.1:${server.address().port}`;
      return address;
    })();
    return starting;
  }
  const control = async ({caller, ownerPid, sessionId, input}) => {
    if (!CALLER.test(caller) || !Number.isSafeInteger(ownerPid) || ownerPid < 1 || ownerPid > 0xffffffff || !caller.startsWith(`native:${ownerPid}:`)) throw unavailable();
    const checkCaller = () => {
      if ((retiredCallers.get(caller) ?? 0) > Date.now()) throw unavailable();
      if (retiredCallers.has(caller)) retiredCallers.delete(caller);
    };
    checkCaller();
    if (input?.op === 'open') {
      let callers = nativeOwners.get(ownerPid);
      if (!callers) nativeOwners.set(ownerPid, callers = new Set());
      callers.add(caller);
    }
    await sessionSnapshot(sessionId);
    checkCaller();
    refresh();
    let supplied;
    if (input?.op === 'context' && documents.get(input.viewId)?.caller === caller) {
      const width = input.context?.containerDimensions?.width;
      if (typeof width === 'number' && Number.isFinite(width)) documents.get(input.viewId).heightGuard.resize(width);
    }
    const result = await handleMcpAppOperation(backend, caller, sessionId, input, {supply(document) {
      supplied = document;
      const heightGuard = new McpAppHeightGuard();
      heightGuard.resize(input.context?.containerDimensions?.width);
      documents.set(document.viewId, {caller, sessionId, key: randomBytes(32).toString('hex'), document, heightGuard,
        state: {height: 320, displayMode: 'inline', initialized: false, failed: false}});
    }});
    if (!supplied) {
      if (input.op === 'teardown') nativeOwners.get(ownerPid)?.delete(caller);
      return result;
    }
    const document = documents.get(supplied.viewId);
    try {
      const baseUrl = await startServer();
      await backend.assertViewAuthorized(caller, sessionId, supplied.viewId);
      // Large HTML/SDK/file bytes travel over the private native HTTP channel,
      // not the worker's bounded JSON-lines output or a model-visible result.
      return {...result, policy: supplied.policy, bridge: {baseUrl: baseUrl + '/' + supplied.viewId, key: document.key}};
    } catch (error) {
      await backend.teardown(caller, sessionId, supplied.viewId, true).catch(() => {});
      throw error;
    }
  };
  return {
    backend, refresh, native: control, modelResult: pluginModelResult,
    async publish(ghost, sessionId, tool, args, result, hashes = [], files = []) {
      refresh();
      // The original helper is a no-op for tools without a UI resource. Its
      // writeCard callback performs the session check only when it creates a
      // real App card; ordinary plugin calls need no new snapshot/availability.
      await publishPluginAppResult(ghost, sessionId, tool, args, result, hashes, files);
    },
    async list({sessionId}) {
      await sessionSnapshot(sessionId);
      refresh();
      const names = await fsp.readdir(path.join(cardsRoot, digest(sessionId))).catch(error => { if (error.code === 'ENOENT') return []; throw error; });
      const cards = [];
      for (const name of names) {
        if (!/^[a-f0-9]{64}\.json$/.test(name)) continue;
        const envelope = JSON.parse(await fsp.readFile(path.join(cardsRoot, digest(sessionId), name), 'utf8'));
        const record = await readCard(sessionId, envelope.clientId).catch(() => null);
        if (!record) continue;
        const config = readAppServer(record.serverId);
        if (!config || backend.serverRevision(config) !== record.serverRevision) continue;
        const summary = parseMcpAppHistory({mcpApp: record});
        cards.push({clientId: envelope.clientId, ...summary, pluginId: config.plugin?.instance, trust: config.plugin?.trust});
      }
      return cards;
    },
    modelContext: sessionId => backend.modelContext(sessionId),
    async releaseCaller(caller) { await backend.releaseCaller(caller); },
    async releaseOwner(ownerPid) {
      if (!Number.isSafeInteger(ownerPid) || ownerPid < 1 || ownerPid > 0xffffffff) throw unavailable();
      const callers = nativeOwners.get(ownerPid) ?? new Set();
      nativeOwners.delete(ownerPid);
      for (const [retired, until] of retiredCallers) if (until <= Date.now()) retiredCallers.delete(retired);
      for (const caller of callers) retiredCallers.set(caller, Date.now() + 180_000);
      await Promise.all([...callers].map(caller => backend.releaseCaller(caller)));
    },
    async stop(id) { await backend.revokePluginConnections(id); },
    async close() {
      if (closed) return;
      closed = true;
      clearDocuments();
      await backend.reset();
      documents.clear(); nativeOwners.clear(); retiredCallers.clear();
      await Promise.allSettled([...writes.values()]);
      if (server) { server.closeIdleConnections(); await new Promise(resolve => server.close(resolve)); }
    },
  };
}
