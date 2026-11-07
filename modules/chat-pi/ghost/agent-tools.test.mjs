import test from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import {once} from 'node:events';
import {ghostAgentTools} from './agent-tools.mjs';

async function bridge(t, handler) {
  const requests = [];
  const server = http.createServer(async (req, res) => {
    const chunks = [];
    for await (const chunk of req) chunks.push(chunk);
    const body = JSON.parse(Buffer.concat(chunks).toString());
    requests.push(body);
    res.setHeader('Content-Type', 'application/json');
    res.end(JSON.stringify(await handler(body.method, body.params)));
  }).listen(0, '127.0.0.1');
  await once(server, 'listening');
  t.after(() => server.close());
  return {requests, config: {ghostBridge: {baseUrl: 'http://127.0.0.1:' + server.address().port, key: 'k'.repeat(16)}, model: {}}};
}
const roster = [{id: 'fixture', name: 'Fixture', recall: 'Use for fixtures'}];
const text = result => result.content.map(part => part.text).join('');

test('no installed plugin adds no tools and no prompt', async t => {
  const {config} = await bridge(t, async () => []);
  assert.deepEqual(await ghostAgentTools(config, async () => {}), {tools: [], prompt: ''});
});

test('a broken plugin host leaves the task without plugin tools rather than failing it', async () => {
  const result = await ghostAgentTools({ghostBridge: {baseUrl: 'http://127.0.0.1:1', key: 'k'.repeat(16)}, model: {}}, async () => {});
  assert.deepEqual(result, {tools: [], prompt: ''});
});

test('the upstream switchboard is exposed without market or publishing tools', async t => {
  const {config} = await bridge(t, async () => roster);
  const {tools, prompt} = await ghostAgentTools(config, async () => {});
  assert.deepEqual(tools.map(tool => tool.name).sort(), ['connect_account', 'ghost_call', 'ghost_forge_guide', 'ghost_forge_install', 'ghost_forge_pack', 'ghost_forge_scaffold', 'ghost_info', 'ghost_list', 'ghost_manual']);
  assert.match(prompt, /fixture/);
});

test('a call waits for permission, and a denial never reaches the plugin host', async t => {
  const {config, requests} = await bridge(t, async method => method === 'agent-roster' ? roster : {ok: true, result: {said: 'hi'}});
  const seen = [];
  let allow = false;
  const {tools} = await ghostAgentTools(config, async request => { seen.push(request); if (!allow) throw Error('denied by user'); });
  const call = tools.find(tool => tool.name === 'ghost_call');
  await assert.rejects(call.execute('t1', {ghost_id: 'fixture', tool: 'echo', args: {text: 'x'}}), /denied/);
  assert.ok(!requests.some(request => request.method === 'agent-call'));
  allow = true;
  const result = await call.execute('t2', {ghost_id: 'fixture', tool: 'echo', args: {text: 'x'}});
  assert.match(text(result), /said/);
  const sent = requests.find(request => request.method === 'agent-call');
  assert.equal(sent.params.approved, true);
  assert.equal(seen[1].tool, 'ghost_call');
});

test('reading a list or a manual asks no permission', async t => {
  const {config} = await bridge(t, async method => method === 'agent-roster' ? roster : []);
  const {tools} = await ghostAgentTools(config, async () => { throw Error('should not ask'); });
  const list = tools.find(tool => tool.name === 'ghost_list');
  assert.ok(text(await list.execute('t1', {})));
});

test('a failed plugin call is reported as a tool failure', async t => {
  const {config} = await bridge(t, async method => method === 'agent-roster' ? roster : {ok: false, errorCode: 'TOOL_NOT_FOUND', message: 'no such tool'});
  const {tools} = await ghostAgentTools(config, async () => {});
  await assert.rejects(tools.find(tool => tool.name === 'ghost_call').execute('t', {ghost_id: 'fixture', tool: 'absent', args: {}}), /no such tool|TOOL_NOT_FOUND/);
});
