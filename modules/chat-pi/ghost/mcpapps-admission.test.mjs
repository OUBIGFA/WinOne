import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import JSZip from 'jszip';
import {createGhostHost} from './host.mjs';

const root = fileURLToPath(new URL('../../../', import.meta.url));
const system = {skip: !process.env.WINONE_GHOST_SANDBOX};
async function fixture(t) {
  const base = process.env.WINONE_CHAT_TEST_DIRECTORY ?? path.join(root, '_temp/plugin-task-close/resume-20261106/mcpapps/admission');
  await fs.mkdir(base, {recursive: true});
  const directory = await fs.mkdtemp(path.join(base, 'sandbox-'));
  const events = [];
  const host = createGhostHost({directory, executable: process.env.WINONE_GHOST_SANDBOX, sdkFile: process.env.WINONE_GHOST_SDK,
    event: event => events.push(event),
    mailSDKFile: fileURLToPath(new URL('../cindy/ghost/apps/desktop/src/main/cindy-brain/runtime/mailComposeRuntime.js', import.meta.url)),
    snapshotWorker: process.env.WINONE_GHOST_SNAPSHOT, runtimeExecutable: process.execPath,
    nodeWorker: fileURLToPath(new URL('./node-worker.mjs', import.meta.url)), masterKey: Buffer.alloc(32, 29).toString('base64'),
    host: async (method, input) => {
      if (method === 'session-snapshot') return {workingDir: directory, remoteHostId: null, permissionMode: 'bypassPermissions', permissionGeneration: 'fixture-current', mcpAppEpoch: 0};
      if (method === 'handoff-authorize') return {ok: input.sessionId === 'admission-task' && input.items.every(file => file.path.startsWith(directory + path.sep))};
      throw Error('Unexpected admission fixture Host seam: ' + method);
    }});
  t.after(async () => {if (process.env.WINONE_MCP_DIAGNOSTIC) console.error('Actual sandbox admission events:', JSON.stringify(events));await host.close();});
  const zip = new JSZip();
  const tools = ['open', 'fail'].map(name => ({name, description: name, parameters: {type: 'object', properties: {cindy_handoff_files: {type: 'array'}}}}));
  tools.push({name: 'count', description: 'Read actual business execution count', parameters: {type: 'object', properties: {}}});
  zip.file('ghost.json', JSON.stringify({schemaVersion: 3, minCindyVersion: '0.1.122-beta', id: 'admission', name: 'Admission fixture', version: '1.0.0', description: 'Actual sandbox branch fixture', entry: 'main.js', kind: 'chip', tools,
    mcpApps: {ui: {resources: [{uri: 'ui://admission', file: 'app.html'}], tools: {open: {resourceUri: 'ui://admission'}}}}}));
  zip.file('main.js', `let executions=0;cindy.onHostMessage(async message=>{if(message.type!=='tool-call')return;if(message.tool==='count')return cindy.send({type:'tool-result',callId:message.callId,ok:true,result:{executions}});executions++;if(message.tool==='fail')return cindy.send({type:'tool-result',callId:message.callId,ok:false,errorCode:'FIXTURE_FAILURE',message:'Actual business failure'});return cindy.send({type:'tool-result',callId:message.callId,ok:true,result:{executions}});});`);
  zip.file('app.html', '<!doctype html><p>Standard App resource</p>');
  const file = path.join(directory, 'admission.cindy'); await fs.writeFile(file, await zip.generateAsync({type: 'nodebuffer'}));
  const inspected = await host.call('inspect', {path: file});
  const installed = await host.call('install', {token: inspected.token, confirmed: true});
  const csv = path.join(directory, 'report.csv'); await fs.writeFile(csv, 'a,b\n1,2\n');
  const png = path.join(directory, 'image.png'); await fs.writeFile(png, Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/l1sAAAAASUVORK5CYII=', 'base64'));
  const session = {id: 'admission-task', workingDir: directory, approved: false};
  return {host, directory, csv, png, installed,
    call: async (tool, args = {}) => {const value = await host.call('agent-call', {ghostId: 'admission', namespace: null, tool, args: {}, session, ...args});if (tool === 'count') assert.equal(value.ok, true, 'Actual count query failed: ' + JSON.stringify(value));return value;},
    copies: () => host.call('handoff-action', {pluginId: installed.pluginId, action: 'list'})};
}

test('MCP sandbox invalid dir rejects before handoff commit and does not execute plugin business', system, async t => {
  const f = await fixture(t);
  const result = await f.call('open', {attachments: [f.csv], dir: path.join(f.directory, 'missing-directory')});
  assert.equal(result.ok, false); assert.equal(result.errorCode, 'DIR_INVALID');
  assert.deepEqual(await f.copies(), []);
  assert.equal((await f.call('count')).result.executions, 0);
});

test('MCP sandbox invalid save directory rejects before handoff commit and does not hold copies', system, async t => {
  const f = await fixture(t);
  const result = await f.call('open', {attachments: [f.csv], saveDir: f.csv});
  assert.equal(result.ok, false); assert.equal(result.errorCode, 'DIR_INVALID');
  assert.deepEqual(await f.copies(), []);
  assert.equal((await f.call('count')).result.executions, 0);
});

test('MCP sandbox actual failed business result discards committed handoff before scope release', system, async t => {
  const f = await fixture(t);
  const result = await f.call('fail', {attachments: [f.csv]});
  assert.equal(result.ok, false); assert.equal(result.errorCode, 'FIXTURE_FAILURE');
  assert.deepEqual(await f.copies(), []);
  assert.equal((await f.call('count')).result.executions, 1);
  const removed = await f.host.call('remove', {pluginId: f.installed.pluginId, confirmed: true});
  assert.equal(removed.ok, true, 'ordinary uninstall must not be blocked by a leaked handoff hold');
});

test('MCP sandbox mixed grant_only grants actual media/files but executes no business or App publisher', system, async t => {
  const f = await fixture(t);
  const granted = await f.call('open', {attachments: [f.csv, f.png], grantOnly: true});
  assert.equal(granted.ok, true); assert.equal(granted.result.granted_count, 2);
  assert.equal(granted.result.attachments.length, 1); assert.equal(granted.result.cindy_handoff_files.length, 1);
  assert.equal((await f.copies()).length, 1);
  assert.equal((await f.call('count')).result.executions, 0);
  assert.deepEqual(await f.host.call('mcp-app-list', {sessionId: 'admission-task'}), []);
});
