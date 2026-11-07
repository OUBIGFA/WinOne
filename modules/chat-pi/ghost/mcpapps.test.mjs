import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {App} from '@modelcontextprotocol/ext-apps';
import {ListToolsResultSchema} from '@modelcontextprotocol/sdk/types.js';
import {GhostManager} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/GhostManager.ts';
import {GhostHandoffStore} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/handoffStore.ts';
import {findInstalledGhostByInstanceId, installedGhostStoragePart} from '../cindy/ghost/apps/desktop/src/shared/pluginIdentity.ts';
import {ghostInstallApprovalToken} from '../cindy/ghost/apps/desktop/src/shared/ghost.ts';
import {setGhostDataRoot} from './seams/app-session.ts';
import {createGhostMcpApps} from './platform-mcpapps.mjs';
import {APP_URI, PRIVATE_UI, CSV, writeFixturePackage} from './fixtures/mcpapp.mjs';

const root = fileURLToPath(new URL('../../../', import.meta.url));
const waitFor = async (test, message) => {
  const until = Date.now() + 5000;
  while (Date.now() < until) { if (test()) return; await new Promise(resolve => setTimeout(resolve, 10)); }
  assert.fail(message);
};
async function fixture(t, custom = {}) {
  const base = process.env.WINONE_CHAT_TEST_DIRECTORY ?? path.join(root, '_temp/plugin-task-close/resume-20261106/mcpapps/unit');
  await fs.mkdir(base, {recursive: true});
  const directory = await fs.mkdtemp(path.join(base, 'mcpapp-'));
  setGhostDataRoot(directory);
  let active = true, epoch = 0;
  const owner = {}, pushed = [], calls = [], approvals = [], messages = [];
  const manager = new GhostManager({getRootDir: () => path.join(directory, 'plugins'), getStateDir: () => path.join(directory, 'install-state'), getOwnerContextKey: () => 'local', log: {info() {}, warn() {}}});
  const file = await writeFixturePackage(directory);
  const inspection = await manager.inspect(file);
  assert.ok(!inspection.rejection, inspection.rejection?.reason);
  const installed = await manager.install(file, {namespace: null, expectedPackageSha256: inspection.packageSha256, uiCapabilityApproved: true});
  assert.ok(!installed.rejection, installed.rejection?.reason);
  const ghost = installed.ghost, id = installedGhostStoragePart(ghost);
  const find = instance => findInstalledGhostByInstanceId(manager.list(), instance);
  const store = new GhostHandoffStore({root: () => path.join(directory, 'handoff')});
  const [handoff] = await store.import(id, [{name: 'report.csv', bytes: Buffer.from(CSV)}], () => {});
  const libraryFile = path.join(directory, 'library.txt'); await fs.writeFile(libraryFile, 'Library fixture bytes');
  const output = path.join(directory, 'download.csv');
  const platform = createGhostMcpApps({root: directory, manager, find,
    current: value => active && find(id)?.enabled && ghostInstallApprovalToken(find(id).approval) === ghostInstallApprovalToken(value.approval),
    ownerScope: {capture: () => owner, isCurrent: value => value === owner && active, isStable: () => active},
    host: async (method, input) => method === 'session-snapshot' ? input.sessionId === 'missing-session' ? null : {workingDir: directory, remoteHostId: null, mcpAppEpoch: epoch} : method === 'ui' ? {path: output} : null,
    push: (channel, value) => pushed.push({channel, value}), log: {warn() {}},
    handoff: {store}, resolveLibraryPath: async (instance, relative) => instance === id && relative === 'library.txt' ? libraryFile : null,
    authorizeTool: async (...args) => { approvals.push(args); if (custom.authorizeTool) await custom.authorizeTool(...args); },
    sendMessage: async (session, record, content, guard) => { await guard(); messages.push({session, record, content}); },
    runPageTool: async (grant, args, signal) => {
      calls.push({grant, args});
      await grant.assertCurrent();
      if (custom.runPageTool) return custom.runPageTool(grant, args, signal);
      return {content: [{type: 'text', text: 'Saved actual App call'}], structuredContent: {value: args.value}, _meta: {private: PRIVATE_UI}};
    },
  });
  t.after(() => platform.close());
  const result = {content: [{type: 'text', text: 'Initial result'}, {type: 'resource_link', uri: 'library:library.txt', name: 'library.txt'}], structuredContent: {title: 'Initial'}, _meta: {private: PRIVATE_UI}};
  await platform.publish(ghost, 'task', 'open', {title: 'Initial'}, result, [], [handoff]);
  const [card] = await platform.list({sessionId: 'task'});
  assert.ok(card);
  async function native(op) { return platform.native({caller: 'native:4242:unitfixture', ownerPid: 4242, sessionId: 'task', input: op}); }
  return {directory, platform, native, card, result, ghost, id, handoff, store, output, pushed, calls, approvals, messages,
    clear() { epoch++; }, revoke() { active = false; }};
}

// Independent official App SDK -> real private HTTP transport -> the reused
// production operations/AppBridge/InMemory MCP backend. No mock Protocol/SDK.
class NativeTransport {
  onmessage; onerror; onclose;
  constructor(native, session) { this.native = native; this.session = session; this.active = true; this.after = 0; }
  async request(input) {
    const response = await fetch(this.session.bridge.baseUrl + '/bridge', {method: 'POST', headers: {Authorization: 'Bearer ' + this.session.bridge.key, 'Content-Type': 'application/json'}, body: JSON.stringify(input)});
    if (!response.ok) throw Error(await response.text());
    return response.json();
  }
  async start() {
    void (async () => {
      while (this.active) {
        try {
          const events = await this.request({op: 'poll', after: this.after});
          for (const event of events) { this.after = event.seq; if (event.message) this.onmessage?.(event.message); }
        } catch (error) { if (this.active) { this.onerror?.(error); await this.close(); } return; }
      }
    })();
  }
  async send(message) { await this.request({op: 'message', message}); }
  async close() { this.active = false; this.onclose?.(); }
}
async function connect(t, f) {
  const session = await f.native({op: 'open', openId: 'open-unit', clientId: f.card.clientId, context: {platform: 'desktop', theme: 'light', locale: 'zh-CN'}});
  assert.equal(session.transport, 'sandbox');
  assert.equal(session.html, undefined, 'private HTML must not enter the bounded worker/UI result');
  const transport = new NativeTransport(f.native, session);
  const app = new App({name: 'Independent official App', version: '1'}, {availableDisplayModes: ['inline', 'fullscreen']}, {autoResize: false});
  const input = [], results = [], contexts = [], order = [], teardown = [];
  app.ontoolinput = value => {input.push(value); order.push('input');};
  app.ontoolresult = value => {results.push(value); order.push('result');};
  app.onhostcontextchanged = value => contexts.push(value);
  app.onteardown = async () => {teardown.push(true); return {};};
  await app.connect(transport);
  await waitFor(() => results.length > 0, 'official App did not receive original tool input/result');
  t.after(async () => {
    await f.native({op: 'teardown', viewId: session.viewId}).catch(() => {});
    await app.close();
  });
  return {app, transport, session, input, results, contexts, order, teardown};
}

test('MCP Apps native backend uses official negotiation, app-only tools and resources; UI _meta never enters model/public list', async t => {
  const f = await fixture(t), view = await connect(t, f);
  assert.deepEqual(view.order.slice(0, 2), ['input', 'result']);
  assert.equal(view.results[0]._meta.private, PRIVATE_UI);
  assert.equal(view.input[0].arguments.title, 'Initial');
  assert.ok(!JSON.stringify(f.platform.modelResult(f.result)).includes(PRIVATE_UI));
  assert.ok(!JSON.stringify(await f.platform.list({sessionId: 'task'})).includes(PRIVATE_UI));
  const listed = await view.app.request({method: 'tools/list'}, ListToolsResultSchema);
  assert.deepEqual(listed.tools.map(tool => tool.name).sort(), ['open', 'save', 'slow']);
  await assert.rejects(view.app.callServerTool({name: 'model_only', arguments: {}}), /NOT_VISIBLE/);
  const result = await view.app.callServerTool({name: 'save', arguments: {value: 'Actual App'}});
  assert.equal(result.structuredContent.value, 'Actual App');
  assert.equal(f.calls.length, 1);
  assert.equal(f.calls[0].grant.sessionId, 'task');
  assert.equal(f.calls[0].grant.clientId, f.card.clientId);
  assert.equal(f.approvals.length, 1);
  const resource = await view.app.readServerResource({uri: APP_URI});
  assert.equal(resource.contents[0].mimeType, 'text/html;profile=mcp-app');
  assert.ok((await view.app.listServerResources()).resources.some(resource => resource.uri === APP_URI));
  assert.equal((await f.platform.list({sessionId: 'task'})).length, 1, 'page calls must not create another App card');
  await assert.rejects(f.native({op: 'message', viewId: view.session.viewId, message: {jsonrpc: '2.0', id: 2, method: 'ping'}}), /FRAME_REQUIRED/);
});

test('MCP Apps original progress/cancellation, context snapshot consumption, display mode and teardown survive real HTTP transport', async t => {
  let cancelled = false;
  const f = await fixture(t, {runPageTool: async (grant, _args, signal) => {
    grant.onProgress?.({progress: 1, total: 2, message: 'real source progress'});
    return new Promise((resolve, reject) => signal.addEventListener('abort', () => {cancelled = true; reject(Error('cancelled at plugin runner'));}, {once: true}));
  }});
  const view = await connect(t, f);
  const cancel = new AbortController(), progress = [];
  const waiting = view.app.callServerTool({name: 'slow', arguments: {}}, {signal: cancel.signal, onprogress: value => {progress.push(value); cancel.abort();}});
  await assert.rejects(waiting, /abort|cancel/i);
  await waitFor(() => cancelled, 'cancellation did not reach the original plugin runner');
  assert.equal(progress[0].message, 'real source progress');
  await view.app.updateModelContext({content: [{type: 'text', text: 'First context'}], structuredContent: {selection: 1}});
  const first = await f.platform.modelContext('task'); assert.match(first.text, /First context/);
  await view.app.updateModelContext({content: [{type: 'text', text: 'Next context'}]});
  await first.assertCurrent(); first.consume();
  assert.match((await f.platform.modelContext('task')).text, /Next context/, 'acceptance must not consume a newer context update');
  await view.app.sendMessage({role: 'user', content: [{type: 'text', text: 'Continue from App'}]});
  assert.equal(f.messages.length, 1);
  assert.equal((await view.app.requestDisplayMode({mode: 'fullscreen'})).mode, 'fullscreen');
  await f.native({op: 'context', viewId: view.session.viewId, context: {theme: 'dark', locale: 'en-US'}});
  await waitFor(() => view.contexts.some(context => context.theme === 'dark' && context.locale === 'en-US'), 'Host context was not delivered');
  await f.native({op: 'teardown', viewId: view.session.viewId});
  assert.equal(view.teardown.length, 1);
  await view.app.close();
  const reopened = await f.native({op: 'open', openId: 'open-again', clientId: f.card.clientId, context: {platform: 'desktop'}});
  assert.notEqual(reopened.viewId, view.session.viewId);
  assert.equal(f.calls.length, 1, 'reopen must not replay the original business call');
  await f.native({op: 'teardown', viewId: reopened.viewId, alreadyNotified: true});
});

test('MCP Apps native file grants support real handoff/Library Range and official downloads, and close/expiry removes URLs', async t => {
  const f = await fixture(t), view = await connect(t, f);
  const links = view.app.getHostContext().toolInfo.tool._meta['cindy/attachments'];
  const link = links.find(value => value.uri === f.handoff.ref);
  assert.ok(link._meta['cindy/file'].url);
  const token = new URL(link._meta['cindy/file'].url).pathname.split('/').pop();
  const read = await fetch(view.session.bridge.baseUrl + '/files/' + token, {headers: {Authorization: 'Bearer ' + view.session.bridge.key, Range: 'bytes=0-3'}});
  assert.equal(read.status, 206); assert.equal(await read.text(), CSV.slice(0, 4));
  const foreign = await fetch(view.session.bridge.baseUrl + '/files/' + token);
  assert.equal(foreign.status, 403);
  const downloaded = await view.app.downloadFile({contents: [{type: 'resource', resource: {uri: 'report.csv', text: CSV, mimeType: 'text/csv'}}]});
  assert.ok(!downloaded.isError); assert.equal(await fs.readFile(f.output, 'utf8'), CSV);
  await f.store.delete(f.id, f.handoff.ref);
  await f.native({op: 'teardown', viewId: view.session.viewId}); await view.app.close();
  const second = await connect(t, f);
  const unavailable = second.app.getHostContext().toolInfo.tool._meta['cindy/attachments'].find(value => value.uri === f.handoff.ref);
  assert.equal(unavailable._meta?.['cindy/file'], undefined, 'expired handoff stays metadata-only, without breaking the page');
  await f.native({op: 'teardown', viewId: second.session.viewId}); await second.app.close();
  assert.equal((await fetch(view.session.bridge.baseUrl + '/files/' + token, {headers: {Authorization: 'Bearer ' + view.session.bridge.key}})).status, 403);
});

test('MCP Apps history/owner revocation rejects old Views and late authorization without executing or replaying a tool', async t => {
  let release;
  const approved = new Promise(resolve => {release = resolve;});
  const f = await fixture(t, {authorizeTool: () => approved});
  const view = await connect(t, f);
  const pending = view.app.callServerTool({name: 'save', arguments: {value: 'late'}});
  await waitFor(() => f.approvals.length === 1, 'approval was not requested');
  f.clear(); release();
  await assert.rejects(pending, /UNAVAILABLE|abort|closed/i);
  assert.equal(f.calls.length, 0);
  assert.deepEqual(await f.platform.list({sessionId: 'task'}), []);
  f.revoke();
  await assert.rejects(f.native({op: 'open', clientId: f.card.clientId, context: {}}), /UNAVAILABLE/);
});
