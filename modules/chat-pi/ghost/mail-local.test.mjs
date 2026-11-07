import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import JSZip from 'jszip';
import {createGhostHost} from './host.mjs';
import {localMailManifest, localMailMain} from './fixtures/mail-local.mjs';

const root = fileURLToPath(new URL('../../../', import.meta.url));
const system = {skip: !process.env.WINONE_GHOST_SANDBOX};
async function fixture(t) {
  const base = process.env.WINONE_CHAT_TEST_DIRECTORY ?? path.join(root, '_temp/plugin-task-close/resume-20261106/mcpapps/mail');
  await fs.mkdir(base, {recursive: true}); const directory = await fs.mkdtemp(path.join(base, 'mail-'));
  const events = [];
  const host = createGhostHost({directory, executable: process.env.WINONE_GHOST_SANDBOX, sdkFile: process.env.WINONE_GHOST_SDK,
    mailSDKFile: fileURLToPath(new URL('../cindy/ghost/apps/desktop/src/main/cindy-brain/runtime/mailComposeRuntime.js', import.meta.url)), snapshotWorker: process.env.WINONE_GHOST_SNAPSHOT,
    runtimeExecutable: process.execPath, nodeWorker: fileURLToPath(new URL('./node-worker.mjs', import.meta.url)), masterKey: Buffer.alloc(32, 31).toString('base64'),
    event: event => events.push(event), host: async (method, args) => {
      if (method === 'session-snapshot') return {workingDir: directory, remoteHostId: null, permissionMode: 'bypassPermissions', mcpAppEpoch: 0};
      if (method === 'library-read-root') return 'not-granted';
      throw Error('Unhandled local mail fixture seam: ' + method);
    }});
  t.after(() => host.close());
  const zip = new JSZip(); zip.file('ghost.json', JSON.stringify(localMailManifest)); zip.file('main.js', localMailMain);
  const file = path.join(directory, 'mail.cindy'); await fs.writeFile(file, await zip.generateAsync({type: 'nodebuffer'}));
  const inspected = await host.call('inspect', {path: file});const installed = await host.call('install', {token: inspected.token, confirmed: true});
  const session = {id: 'mail-task', workingDir: directory, approved: false};
  const call = async (tool, args = {}) => {
    const result = await host.call('agent-call', {ghostId: localMailManifest.id, namespace: null, tool, args, session, language: 'en'});
    assert.equal(result.ok, true, JSON.stringify(result));return result;
  };
  const prime = await call('prime');
  let callId = prime.result.xdt_card_id;
  assert.ok(callId, 'only the actual Host mail publication can mint a native card id');
  const latestCard = async () => {
    let card;
    for (let n = 0; n < 8; n++) {card = await host.call('card-get', {callId});if (!card.mail.supersededBy) return card;callId = card.mail.supersededBy;}
    throw Error('Invalid superseded mail card chain');
  };
  return {host, call, get callId() {return callId;}, installed, events,
    card: latestCard,
    action: async (name, values) => {const card = await latestCard();const result = await host.call('card-action', {callId: card.callId, action: 'mail:' + name + ':' + card.mail.key, ...(values ? {formValues: values} : {}), ...(name === 'change' ? {change: true} : {})});assert.equal(result.ok, true, JSON.stringify(result));return result;}};
}
async function until(read, predicate, label, maximum = 5000) {
  const deadline = Date.now() + maximum;let value;
  while (Date.now() < deadline) {value = await read();if (predicate(value)) return value;await new Promise(resolve => setTimeout(resolve, 25));}
  assert.fail(label + ': ' + JSON.stringify(value));
}
const fields = card => Object.fromEntries(card.mail.fields.map(field => [field.name, field.value]));

test('Local mail actual SDK presents Host card, gives a full 10-second undo/send grace and never double-sends', system, async t => {
  const f = await fixture(t);let card = await f.card();assert.equal(card.mail.state, 'draft');
  await f.action('send', fields(card));
  card = await until(f.card, card => !!card.mail.pendingSend, 'real send countdown was not accepted');
  assert.equal((await f.call('stats')).result.sends, 0);
  await f.action('cancel');
  await until(f.card, card => !card.mail.pendingSend && card.mail.state === 'draft', 'undo did not retain the draft');
  assert.equal((await f.call('stats')).result.sends, 0);
  card = await f.card();await f.action('send', fields(card));
  card = await until(f.card, card => !!card.mail.pendingSend, 'new full countdown did not start');
  const deadline = card.mail.pendingSend.deadline;
  assert.ok(deadline - Date.now() > 8000, 'the actual Host must expose a full grace period');
  await until(f.card, card => card.mail.state === 'sent', 'known provider receipt did not become sent', 13000);
  const sent = await f.call('stats');
  assert.equal(sent.result.sends, 1);assert.ok(sent.result.history.find(entry => entry.send).at >= deadline);
  const repeated = await f.call('send');assert.equal(repeated.result.data.alreadySent, true);
  assert.equal((await f.call('stats')).result.sends, 1);
});

test('Local mail actual SDK merges independent provider/editor edits and preserves conflicting edits without sending', system, async t => {
  const f = await fixture(t);const card = await f.card();const values = {...fields(card), body: 'one two LOCAL'};
  await f.action('change', values);await f.call('mutate', {body: 'REMOTE one two'});
  await until(f.card, card => card.mail.state === 'draft' && fields(card).body === 'REMOTE one two LOCAL', 'original three-way merge did not publish', 7000);
  const merged = await f.call('stats');
  assert.equal(merged.result.body, 'REMOTE one two LOCAL');assert.equal(merged.result.sends, 0);
  const active = await until(f.card, card => card.mail.state === 'draft' && fields(card).body === 'REMOTE one two LOCAL', 'Host editor did not finish its merged publication');
  await f.action('change', {...fields(active), body: 'LOCAL replacement'});await f.call('mutate', {body: 'REMOTE replacement'});
  const conflict = await until(f.card, card => card.mail.state === 'error', 'overlap conflict was not retained', 7000);
  assert.equal(fields(conflict).body, 'LOCAL replacement');assert.equal((await f.call('stats')).result.sends, 0);
  const read = await f.call('get');assert.ok(read.result.data.conflicts.includes('body'), JSON.stringify(read));assert.equal(read.result.data.cardEdits.body, 'LOCAL replacement');
  const blocked = await f.call('send');assert.equal(blocked.result.execution, 'not_executed');
});

test('Local mail sandbox restart restores the existing draft but cannot resume an unattended countdown', system, async t => {
  const f = await fixture(t);const card = await f.card();await f.action('send', fields(card));
  await until(f.card, card => !!card.mail.pendingSend, 'countdown not active before restart');
  await f.host.call('enabled', {pluginId: f.installed.pluginId, enabled: false});await f.host.call('enabled', {pluginId: f.installed.pluginId, enabled: true});
  const read = await f.call('get');assert.equal(read.result.data.readBack.draftId, 'draft-local');
  assert.equal((await f.call('stats')).result.sends, 0);
  // The source deliberately retains the persisted countdown presentation.
  // A real native Undo after restart restores a draft, never a hidden timer.
  await f.action('cancel');
  const recovered = await until(f.card, card => card.mail.state === 'draft' && !card.mail.pendingSend, 'native recovery Undo did not reset the old countdown');
  await f.action('send', fields(recovered));
  const shown = await until(f.card, card => !!card.mail.pendingSend, 'restored send did not require a new countdown');
  assert.ok(shown.mail.pendingSend.deadline - Date.now() > 8000);
  await f.action('now');await until(f.card, card => card.mail.state === 'sent', 'send-now did not confirm current restored draft');
  assert.equal((await f.call('stats')).result.sends, 1);
});

test('Local mail original reader marks only explicit opens and resolves the real current Host draft card', system, async t => {
  const f = await fixture(t);const url = 'https://mail.example.invalid/draft-local';
  assert.equal((await f.host.call('mail-preview', {sessionId: 'mail-task', url, read: false})).supported, true);
  const background = await f.host.call('mail-preview', {sessionId: 'mail-task', url, read: true});assert.equal(background.message.bodyText, 'one two');
  assert.equal((await f.call('stats')).result.markReads, 0);
  const opened = await f.host.call('mail-preview', {sessionId: 'mail-task', url, read: true, opened: true});
  assert.equal(opened.message.accountId, 'local-account');assert.ok(opened.draftCallId);assert.equal((await f.call('stats')).result.markReads, 1);
});
