import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import {once} from 'node:events';
import {fileURLToPath} from 'node:url';
import {createGhostConnections} from './platform-connections.mjs';
import {createGhostAccounts} from './platform-accounts.mjs';
import {ghostAgentTools} from './agent-tools.mjs';
import {GhostSetupChangeBus} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostSetupChangeBus.ts';
import {evaluateGhostSetupAssessment} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostSetupStatus.ts';
import {createGhostKvStore} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostKvStore.ts';
import {createOneBotPaths} from '../onebot/content-service.mjs';
import {createIMSecrets} from '../onebot/im-secrets.mjs';

const root = fileURLToPath(new URL('../../../', import.meta.url));
const waitFor = async (check, label) => {
  const until = Date.now() + 5000;
  while (Date.now() < until) { if (check()) return; await new Promise(resolve => setTimeout(resolve, 10)); }
  assert.fail(label);
};
async function fixture(t, options = {}) {
  const base = process.env.WINONE_CHAT_TEST_DIRECTORY ?? path.join(root, '_temp/plugin-task-close/resume-20261106/mcpapps/unit');
  await fs.mkdir(base, {recursive: true});
  const directory = await fs.mkdtemp(path.join(base, 'connections-'));
  let available = true, companion = !!options.companion, epoch = 0;
  const warnings = [];
  const owner = {}, ui = [], pushes = [], resumes = [], log = {info() {}, warn(message) {warnings.push(message);if (process.env.WINONE_MCP_TRACE) console.error('connection fixture warning:', message);}}, name = (id, key) => 'fixture-' + id + '-' + key;
  const ghost = {enabled: true, namespace: null, namespaceState: 'confirmed', dir: path.join(directory, 'fixture'), approval: {state: 'approved', revision: 'fixture-revision'},
    manifest: {id: 'fixture', name: 'Fixture account', kind: 'chip', tools: [{name: 'work', description: 'Business fixture', parameters: {type: 'object'}}],
      network: {hosts: ['api.example.invalid'], secrets: [{key: 'token', label: 'Private token', required: true, source: 'user', inject: {header: 'Authorization', format: 'Bearer {value}', hosts: ['api.example.invalid']}}]}}};
  const secrets = createIMSecrets(createOneBotPaths(directory), Buffer.alloc(32, 23).toString('base64'), log);
  const kv = createGhostKvStore({getRootDir: () => path.join(directory, 'kv'), log});
  const bus = new GhostSetupChangeBus();
  const find = id => id === 'fixture' ? ghost : null;
  const scope = {capture: () => owner, isCurrent: value => value === owner && available, isStable: () => available};
  const host = async (method, input, signal) => {
    if (process.env.WINONE_MCP_TRACE) console.error('connection fixture host:', method);
    if (method === 'session-snapshot') return {workingDir: directory, remoteHostId: null, isCompanion: companion, mcpAppEpoch: epoch};
    if (method === 'ui') {ui.push(input); return options.answer ? options.answer(input, signal) : {action: 'submit', actionId: input.request.steps.find(step => step.action.kind === 'inline_form').action.id, value: 'native-fixture-secret'};}
    if (method === 'connection-resume') {
      resumes.push(input);
      const result = options.resume ? await options.resume(input) : {ok: true};
      return {...result, ok: result.ok === true && input.expectedEpoch === epoch};
    }
    throw Error('unexpected fixture Host request: ' + method);
  };
  const accounts = createGhostAccounts({root: directory, secrets, secretName: name, find, host, ui: (kind, input) => host('ui', {kind, ...input}), kv, changeBus: bus, log, ownerScope: scope});
  const assessment = id => evaluateGhostSetupAssessment(ghost.manifest, accounts.probes(id, ghost.manifest), {revision: bus.currentRevision(id), strict: true});
  const visibility = {listGhosts: () => [ghost], findInstalledInstance: find, isAvailableForActiveSession: () => available, isDisabledForWorkdir: () => false};
  const create = () => createGhostConnections({root: directory, find, current: () => available && ghost.enabled, visibility, assessment, changeBus: bus, accounts,
    secrets, secretName: name, ownerScope: scope, host, push: (channel, payload) => pushes.push({channel, payload}), log});
  const connections = create();
  t.after(async () => {await connections.close(); accounts.close(); secrets.close();});
  return {directory, ghost, connections, create, ui, pushes, resumes, warnings, accounts, secrets, name, assessment,
    session: {id: 'task', workingDir: directory}, clear() {epoch++;}, disable() {ghost.enabled = false; connections.stop('fixture');}, available(value) {available = value;}};
}

async function modelBridge(t, f) {
  const requests = [];
  const server = http.createServer(async (req, res) => {
    const chunks = []; for await (const chunk of req) chunks.push(chunk);
    const {method, params} = JSON.parse(Buffer.concat(chunks).toString()); requests.push({method, params});
    let result;
    try {
      if (method === 'agent-roster') result = [{id: 'fixture', name: 'Fixture'}];
      else if (method === 'agent-connect-account') result = await f.connections.connect({...params, session: f.session});
      else if (method === 'agent-cancel') {f.connections.cancel(params.cancelKey); result = {ok: true};}
      else throw Error('connection must not dispatch business tools: ' + method);
      res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify(result));
    } catch (error) {
      if (process.env.WINONE_MCP_TRACE) console.error('connection fixture failed:', error.stack);
      res.writeHead(500, {'Content-Type': 'application/json'}); res.end(JSON.stringify({message: 'Fixture Host operation failed'}));
    }
  }).listen(0, '127.0.0.1');
  await once(server, 'listening'); t.after(() => server.close());
  const tools = await ghostAgentTools({ghostBridge: {baseUrl: 'http://127.0.0.1:' + server.address().port, key: 'fixture-key-only'}, model: {}}, async () => {throw Error('a Host native credential card is its own consent');});
  return {tool: tools.tools.find(tool => tool.name === 'connect_account'), requests};
}

test('connect_account uses the original MCP tool and real Host private executor/vault, never model credentials or a business call', async t => {
  const f = await fixture(t), bridge = await modelBridge(t, f);
  assert.ok(bridge.tool);
  assert.deepEqual(Object.keys(bridge.tool.parameters.properties).sort(), ['id', 'kind', 'namespace', 'reauthorize']);
  const result = await bridge.tool.execute('connection', {kind: 'plugin', id: 'fixture'});
  assert.equal(f.secrets.read(f.name('fixture', 'token')), 'native-fixture-secret');
  assert.equal(f.assessment('fixture').state, 'ready');
  assert.match(result.content[0].text, /ready/);
  assert.equal(f.ui.length, 1);
  assert.equal(f.ui[0].kind, 'plugin-setup');
  assert.ok(!JSON.stringify([result, bridge.requests, f.ui, f.pushes]).includes('native-fixture-secret'));
  assert.equal(bridge.requests.filter(request => request.method === 'agent-connect-account').length, 1);
  const again = await f.connections.connect({kind: 'plugin', id: 'fixture', session: f.session});
  assert.equal(again.status, 'ready'); assert.equal(f.ui.length, 1, 'saved setup does not open a duplicate card');
});

test('connect_account explicit token reconfiguration opens the native card and cancellation retains the previous credential', async t => {
  let cancel = false;
  const f = await fixture(t, {answer: input => cancel ? {} : {action: 'submit', actionId: input.request.steps[0].action.id, value: 'reconfigured-native-fixture'}});
  f.secrets.write(f.name('fixture', 'token'), 'previous-native-fixture');
  const changed = await f.connections.connect({kind: 'plugin', id: 'fixture', session: f.session, reauthorize: true});
  assert.equal(changed.status, 'ready'); assert.equal(f.ui.length, 1);
  assert.equal(f.secrets.read(f.name('fixture', 'token')), 'reconfigured-native-fixture');
  cancel = true;
  const cancelled = await f.connections.connect({kind: 'plugin', id: 'fixture', session: f.session, reauthorize: true});
  assert.equal(cancelled.errorCode, 'SETUP_CANCELLED');
  assert.equal(f.secrets.read(f.name('fixture', 'token')), 'reconfigured-native-fixture');
});

test('connect_account a revoked or aborted native card cannot commit a late credential', async t => {
  let release;
  const deferred = new Promise(resolve => {release = resolve;});
  const f = await fixture(t, {answer: () => deferred});
  const waiting = f.connections.connect({kind: 'plugin', id: 'fixture', session: f.session});
  await waitFor(() => f.ui.length === 1, 'native connection card did not open');
  f.disable();
  release({action: 'submit', actionId: f.ui[0].request.steps[0].action.id, value: 'late-native-fixture'});
  assert.equal((await waiting).errorCode, 'SETUP_CANCELLED');
  assert.equal(f.secrets.read(f.name('fixture', 'token')), null);
  assert.deepEqual(f.connections.bridge.pendingSnapshots(), []);
});

test('connect_account teammate uses the original durable pending card, idempotent continuation and re-opened credential action', async t => {
  let release;
  const deferred = new Promise(resolve => {release = resolve;});
  const f = await fixture(t, {companion: true, answer: () => deferred});
  const pending = await f.connections.connect({kind: 'plugin', id: 'fixture', session: f.session});
  assert.equal(pending.errorCode, 'SETUP_REQUIRED'); assert.ok(pending.requestId);
  const same = await f.connections.connect({kind: 'plugin', id: 'fixture', session: f.session});
  assert.equal(same.requestId, pending.requestId);
  await waitFor(() => f.ui.length === 1, 'teammate native credential card did not open');
  const cards = await f.connections.pending('task');
  assert.equal(cards.length, 1); assert.equal(cards[0].snapshot.requestId, pending.requestId);
  release({action: 'submit', actionId: f.ui[0].request.steps[0].action.id, value: 'teammate-native-fixture'});
  await waitFor(() => f.resumes.length === 1, 'authorization did not use the original continuation');
  await waitFor(() => f.pushes.some(push => push.channel === 'plugin-authorization' && push.payload.card.snapshot.terminal), 'durable card was not retired');
  assert.equal(f.resumes[0].clientId, 'bot-authorization-resume:' + pending.requestId);
  assert.equal(f.resumes[0].targetSessionId, 'task');
  assert.equal(f.resumes[0].expectedEpoch, 0);
  assert.equal(f.secrets.read(f.name('fixture', 'token')), 'teammate-native-fixture');
  assert.ok(!JSON.stringify(f.pushes).includes('teammate-native-fixture'));
});

test('connect_account retained teammate card reopens the same real native action after a Host adapter restart', async t => {
  const replies = [];
  const f = await fixture(t, {companion: true, answer: () => new Promise(resolve => replies.push(resolve))});
  const first = await f.connections.connect({kind: 'plugin', id: 'fixture', session: f.session});
  await waitFor(() => replies.length === 1, 'first native card was not presented');
  await f.connections.close();
  const reopened = f.create(); t.after(() => reopened.close());
  const second = await reopened.connect({kind: 'plugin', id: 'fixture', session: f.session});
  assert.equal(second.requestId, first.requestId, 'restoration must reuse the durable original card');
  await waitFor(() => replies.length === 2, 'restored native card was not presented');
  replies[1]({action: 'submit', actionId: f.ui[1].request.steps[0].action.id, value: 'restored-native-fixture'});
  await waitFor(() => f.resumes.length === 1, 'restored card did not dispatch original continuation');
  assert.equal(f.resumes[0].clientId, 'bot-authorization-resume:' + first.requestId);
  assert.equal(f.resumes[0].expectedEpoch, 0);
  assert.equal(f.secrets.read(f.name('fixture', 'token')), 'restored-native-fixture');
});

test('connect_account native continuation carries the original epoch and a late Host acceptance cannot revive a cleared card', async t => {
  let release, submit;
  const accepted = new Promise(resolve => {release = resolve;});
  const f = await fixture(t, {companion: true, answer: input => new Promise(resolve => {submit = () => resolve({action: 'submit', actionId: input.request.steps[0].action.id, value: 'late-epoch-native-fixture'});}), resume: () => accepted});
  const pending = await f.connections.connect({kind: 'plugin', id: 'fixture', session: f.session});
  await waitFor(() => submit !== undefined, 'native pending action did not open');
  submit();
  await waitFor(() => f.resumes.length === 1, 'real continuation request did not reach Host');
  assert.equal(f.resumes[0].clientId, 'bot-authorization-resume:' + pending.requestId);
  assert.equal(f.resumes[0].expectedEpoch, 0);
  f.clear(); release({ok: true});
  await waitFor(() => f.warnings.length > 0, 'late native receipt was not settled as a failure');
  assert.deepEqual(await f.connections.pending('task'), []);
  await new Promise(resolve => setImmediate(resolve));
  assert.ok(!f.pushes.some(push => push.channel === 'plugin-authorization' && push.payload.card.snapshot.terminal), 'cleared presentation must not be written as a new terminal card');
  assert.equal((await f.connections.action({op: 'open', requestId: pending.requestId})).requested, false);
});

test('connect_account missing supported action reports documented login instead of inventing a connected account', async t => {
  const f = await fixture(t);
  f.ghost.manifest.network.secrets = [];
  const result = await f.connections.connect({kind: 'plugin', id: 'fixture', session: f.session});
  assert.equal(result.ok, false); assert.equal(result.errorCode, 'PLUGIN_CONNECTION_METHOD_REQUIRED');
  assert.equal(f.ui.length, 0); assert.equal(f.resumes.length, 0);
});
