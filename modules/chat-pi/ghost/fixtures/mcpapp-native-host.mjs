// Explicit system-check process: production plugin Host + actual Cindy package.
// No real user profile/accounts/settings are used; the native viewer connects
// through this fixture's private control server, not a mock MCP/App runtime.
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import {once} from 'node:events';
import {randomBytes} from 'node:crypto';
import {fileURLToPath} from 'node:url';
import {createGhostHost} from '../host.mjs';
import {getMcpAppsHost} from '../../cindy/ghost/apps/desktop/src/main/mcp-apps/runtime.ts';
import {writeFixturePackage, fixtureManifest, CSV} from './mcpapp.mjs';

const repository = fileURLToPath(new URL('../../../../', import.meta.url));
const directory = path.resolve(process.argv[2] ?? '');
if (!directory.startsWith(path.join(repository, '_temp') + path.sep) || process.env.WINONE_MCP_APPS_NATIVE !== '1') throw Error('Explicit repository-private MCP App system check is required');
const executable = process.env.WINONE_GHOST_SANDBOX, sdk = process.env.WINONE_GHOST_SDK;
if (!executable || !sdk || !path.isAbsolute(executable) || !path.isAbsolute(sdk)) throw Error('Use the current built plugin sandbox and original SDK');
await fs.mkdir(directory, {recursive: true});
const scopes = new Map();
const events = [], messages = [], approvals = [], diagnostics = [];
let epoch = 0;
const snapshot = () => ({workingDir: directory, permissionMode: 'bypassPermissions', permission: 'full', readOnly: false, planModeEnabled: false,
  remoteHostId: null, archived: false, mcpAppEpoch: epoch, isCompanion: false, sessionInstanceId: 'native-fixture-instance', instanceId: 'native-fixture-instance', permissionGeneration: 1, model: 'fixture'});
const taskId = 'native-fixture-task';
const host = createGhostHost({directory, executable, sdkFile: sdk,
  mailSDKFile: fileURLToPath(new URL('../../cindy/ghost/apps/desktop/src/main/cindy-brain/runtime/mailComposeRuntime.js', import.meta.url)),
  snapshotWorker: process.env.WINONE_GHOST_SNAPSHOT, runtimeExecutable: process.execPath,
  nodeWorker: fileURLToPath(new URL('../node-worker.mjs', import.meta.url)), masterKey: randomBytes(32).toString('base64'),
  event: event => { events.push(event); if (event.type === 'diagnostic') diagnostics.push(event.message); },
  host: async (method, input) => {
    if (method === 'session-snapshot') return input.sessionId === taskId ? snapshot() : null;
    if (method === 'handoff-authorize') return {ok: input.sessionId === taskId && input.items.every(file => path.isAbsolute(file.path) && file.path.startsWith(directory + path.sep))};
    if (method === 'models') return [];
    if (method === 'ui' && input.kind === 'save') return {path: path.join(directory, 'download.csv')};
    if (method === 'task' && input.op === 'tool-scope' && input.sessionId === taskId) {
      const scope = {scopeId: randomBytes(16).toString('hex'), revision: 1, workingDir: directory, instanceId: 'native-fixture-instance', planModeEnabled: false};
      scopes.set(scope.scopeId, scope); return scope;
    }
    if (method === 'task' && input.op === 'tool-authorize') {
      const scope = scopes.get(input.scopeId), request = input.request;
      const authorized = !!scope && input.sessionId === taskId && request?.tool === 'ghost_call' && request.args?.ghost_id === 'local-mcp-app' && ['open', 'save', 'slow'].includes(request.args?.tool);
      if (authorized) approvals.push(request);
      return {authorized};
    }
    if (method === 'task' && input.op === 'tool-release') {scopes.delete(input.scopeId);return {released: true};}
    if (method === 'task' && input.op === 'mcp-message') {
      if (input.sessionId !== taskId || input.expectedEpoch !== epoch) throw Error('Stale actual App input');
      await host.call('mcp-app-message-admit', {sessionId: taskId, requestId: input.clientId});
      messages.push(input); return {accepted: true, requestId: input.clientId};
    }
    // New Host seams must be named explicitly. A typo or missing source
    // operation cannot become a fixture's broad permission success.
    throw Error('Unhandled local fixture Host method: ' + method + (input?.op ? '/' + input.op : '') + (input?.kind ? '/' + input.kind : ''));
  },
});
let overrides = {};
if (process.env.WINONE_MCP_APPS_NATIVE_FRAME_ORIGIN) {
  const origin = new URL(process.env.WINONE_MCP_APPS_NATIVE_FRAME_ORIGIN);
  if (origin.protocol !== 'http:' || origin.hostname !== 'public.frames-mcp-fixture.localhost' || origin.port || origin.username || origin.password) throw Error('Only the owned local frame fixture is allowed');
  overrides = {network: {hosts: ['public.frames-mcp-fixture.localhost']}, mcpApps: {ui: {...fixtureManifest.mcpApps.ui,
    resources: [{...fixtureManifest.mcpApps.ui.resources[0], csp: {frameDomains: [origin.origin]}}]}}};
}
const packagePath = await writeFixturePackage(directory, overrides);
const inspection = await host.call('inspect', {path: packagePath});
const installed = await host.call('install', {token: inspection.token, confirmed: true});
const csvPath = path.join(directory, 'report.csv'); await fs.writeFile(csvPath, CSV);
const modelResult = await host.call('agent-call', {ghostId: 'local-mcp-app', namespace: null, tool: 'open', args: {title: 'Native initial'}, attachments: [csvPath], session: {id: taskId, workingDir: directory, approved: false}});
if (!modelResult.ok) throw Error('Actual plugin call failed: ' + JSON.stringify(modelResult));
const [card] = await host.call('mcp-app-list', {sessionId: taskId});
if (!card) throw Error('Actual plugin did not publish an App card');
const key = randomBytes(32).toString('hex');
const server = http.createServer(async (req, res) => {
  if (req.headers.authorization !== 'Bearer ' + key) {res.writeHead(403); res.end(); return;}
  try {
    let result;
    if (req.url === '/call' && req.method === 'POST') {
      const chunks = []; for await (const chunk of req) chunks.push(chunk);
      const {method, params} = JSON.parse(Buffer.concat(chunks).toString());
      result = await host.call(method.replace(/^chat\.ghosts\./, ''), params);
    } else if (req.url === '/state') {
      result = {events, messages, approvals, diagnostics, modelResult, cards: await host.call('mcp-app-list', {sessionId: taskId}),
        downloaded: await fs.readFile(path.join(directory, 'download.csv'), 'utf8').catch(() => null),
        modelContext: (await getMcpAppsHost().modelContext(taskId)).text};
    } else if (req.url === '/clear') {epoch++; result = {};}
    else {res.writeHead(404);res.end();return;}
    res.writeHead(200, {'Content-Type': 'application/json'}); res.end(JSON.stringify(result));
  } catch (error) {res.writeHead(500, {'Content-Type': 'application/json'});res.end(JSON.stringify({error: error.message}));}
}).listen(0, '127.0.0.1');
await once(server, 'listening');
process.stdout.write(JSON.stringify({baseUrl: 'http://127.0.0.1:' + server.address().port, key, sessionId: taskId, clientId: card.clientId, pluginId: installed.pluginId}) + '\n');
process.stdin.resume();
process.stdin.once('end', async () => {
  await host.close(); server.closeIdleConnections(); await new Promise(resolve => server.close(resolve)); process.exit(0);
});
