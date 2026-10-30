// Cindy's private-chat data rules on real Pi transcripts: visible rows,
// previews, topic stats, reply-quote notes and managed avatar images.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {SessionManager} from '@earendil-works/pi-coding-agent';
import {createOneBotRuntime} from './runtime.mjs';
import {chatPreview, messageRow, transcriptStats, visibleRows} from './bots-service.mjs';
import {readBranch} from './history.mjs';

const base = process.env.WINONE_ONEBOT_TEST_OUTPUT;
if (!base || !path.isAbsolute(base)) throw Error('Run through build/test-onebot.mjs with an isolated output');
const meta = {api: 'openai-completions', provider: 'fixture', model: 'fixture',
  usage: {input: 1, output: 1, cacheRead: 0, cacheWrite: 0, totalTokens: 2, cost: {input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0}}};

/** A companion chat: a hidden welcome instruction, its opening, an owner message with files, narration and a reply. */
async function transcript(directory, id) {
  const sessionDirectory = path.join(directory, 'chat', id);
  await fs.mkdir(sessionDirectory, {recursive: true});
  const manager = SessionManager.create(directory, sessionDirectory);
  manager.appendCustomEntry('winone.input', {id: 'welcome', text: 'Greet the owner', presentation: {hidden: true, toolsDisabled: true}});
  manager.appendMessage({role: 'user', content: 'Greet the owner', timestamp: 1});
  const opening = manager.appendMessage({role: 'assistant', content: [{type: 'text', text: 'Hello, I am Reader.'}], ...meta, stopReason: 'stop', timestamp: 2});
  // The model text carries a host note; the owner wrote only the presentation text.
  manager.appendCustomEntry('winone.input', {id: 'owner-1', text: '[引用说明] note\n\nSummarise the report',
    presentation: {text: 'Summarise the report'}, attachments: [{name: 'chart.png', path: 'C:/f/chart.png', mimeType: 'image/png'}, {name: 'data.csv', path: 'C:/f/data.csv', mimeType: 'text/csv'}]});
  manager.appendMessage({role: 'user', content: '[引用说明] note\n\nSummarise the report', timestamp: 3});
  manager.appendMessage({role: 'assistant', content: [{type: 'text', text: 'Reading it now.'}, {type: 'toolCall', id: 'read-1', name: 'read', arguments: {path: 'report.md'}}], ...meta, stopReason: 'toolUse', timestamp: 4});
  manager.appendMessage({role: 'toolResult', toolCallId: 'read-1', toolName: 'read', content: [{type: 'text', text: 'private tool output'}], isError: false, timestamp: 5});
  const reply = manager.appendMessage({role: 'assistant', content: [{type: 'text', text: 'The report shows growth.'}], ...meta, stopReason: 'stop', timestamp: 6});
  return {session: {id, directory, sessionDirectory, sessionFile: manager.getSessionFile()}, opening, reply};
}

test('a companion chat shows owner inputs and replies only, keyed as the page keys them', async () => {
  const directory = await fs.mkdtemp(path.join(base, 'bots-rows-'));
  const {session, opening, reply} = await transcript(directory, 'A'.repeat(26));
  const branch = await readBranch(session);
  assert.deepEqual(visibleRows(branch).map(({clientId, role, text}) => ({clientId, role, text})), [
    {clientId: opening, role: 'assistant', text: 'Hello, I am Reader.'},
    {clientId: 'owner-1', role: 'user', text: 'Summarise the report'},
    {clientId: reply, role: 'assistant', text: 'The report shows growth.'},
  ]);
  // Cindy's `messages` envelope for the reply-quote note: the owner's own words and attachment names.
  assert.deepEqual(messageRow(branch, 'owner-1'), {role: 'user', rewindAt: null,
    content: JSON.stringify({text: 'Summarise the report', images: [{originalName: 'chart.png'}], files: [{name: 'data.csv'}]})});
  assert.deepEqual(messageRow(branch, reply), {role: 'assistant', rewindAt: null, content: JSON.stringify('The report shows growth.')});
  assert.equal(messageRow(branch, 'missing'), null);
  assert.equal(messageRow(branch, 'welcome'), null, 'hidden host input is not a quoteable user message');
  const preview = await chatPreview(session, 0);
  assert.deepEqual({text: preview.text, role: preview.role, unread: preview.unread}, {text: 'The report shows growth.', role: 'assistant', unread: 2});
  assert.equal((await chatPreview(session, null)).unread, 0);
  const stats = await transcriptStats(session, 'owner-1');
  assert.equal(stats.replyCount, 3);
  assert.equal(stats.hasClient, true);
  assert.equal((await transcriptStats(session, 'welcome')).hasClient, false);
});

test('a reply-quote note rereads the quoted original from the same chat', async () => {
  const directory = await fs.mkdtemp(path.join(base, 'bots-quote-'));
  const id = 'B'.repeat(26), {session, reply} = await transcript(directory, id);
  const reads = [];
  const runtime = createOneBotRuntime({directory, tasks: {}, host: {
    messageRow: async (sessionId, clientId) => {
      reads.push([sessionId, clientId]);
      const row = messageRow(await readBranch(session), clientId);
      return row ? [row] : [];
    },
  }});
  try {
    await runtime.call('profiles.create', {input: {id: 'reader', name: 'Reader'}});
    await runtime.call('sessions.bind', {botId: 'reader', sessionId: id});
    // The Host's current send boundary prepares quote notes and teammate
    // references together; the retired replyQuote.note route is not exposed.
    const prepare = (quote, sessionId = id) => runtime.call('tasks.input.prepare', {sessionId, text: 'And the costs?', quote});
    const assistant = await prepare({clientId: reply, role: 'assistant', excerpt: '  The   report  '});
    // The renderer's excerpt is only a display snapshot; it is normalised and never trusted for the note.
    assert.deepEqual(assistant.quote, {clientId: reply, role: 'assistant', excerpt: 'The report'});
    assert.match(assistant.modelText, /^\[引用说明\]/);
    assert.match(assistant.modelText, /你自己之前回复/);
    assert.match(assistant.modelText, /<<<\nThe report shows growth\.\n>>>\n\nAnd the costs\?$/);
    const owner = await prepare({clientId: 'owner-1', role: 'user', excerpt: 'anything'});
    assert.match(owner.modelText, /主人自己/);
    assert.match(owner.modelText, /Summarise the report\n\(附件:chart\.png、data\.csv\)/);
    const gone = await prepare({clientId: 'rewound', role: 'user', excerpt: 'old'});
    assert.match(gone.modelText, /不可用/);
    assert.deepEqual(reads, [[id, reply], [id, 'owner-1'], [id, 'rewound']]);
    await assert.rejects(prepare({clientId: reply, role: 'system', excerpt: ''}), /引用的消息无效/);
    await assert.rejects(prepare({clientId: reply, role: 'assistant', excerpt: ''}, 'C'.repeat(26)));
    assert.equal(reads.length, 3, 'invalid or foreign-session quotes never reach transcript storage');
  } finally { await runtime.close(); }
});

test('portrait bytes become a managed image before the teammate row refers to them', async () => {
  const directory = await fs.mkdtemp(path.join(base, 'bots-avatar-'));
  const runtime = createOneBotRuntime({directory});
  try {
    const png = Buffer.concat([Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]), Buffer.alloc(16, 7)]);
    const hash = createHash('sha256').update(png).digest('hex');
    const profile = await runtime.call('profiles.create', {input: {id: 'portrait', name: 'Portrait', avatarImageBase64: png.toString('base64')}});
    assert.equal(profile.avatar, `cindy-media://blobs/${hash}.png`);
    assert.deepEqual(await fs.readFile(path.join(directory, 'media', 'blobs', `${hash}.png`)), png);
    // Non-image bytes never become an address.
    await assert.rejects(runtime.call('profiles.create', {input: {id: 'text', name: 'Text', avatarImageBase64: Buffer.from('not an image!').toString('base64')}}), /PNG、JPEG 或 WebP|头像图片格式无效/);
    await assert.rejects(runtime.call('profiles.get', {botId: 'text'}), /不存在/);
  } finally { await runtime.close(); }
});

test('replacing the canonical chat keeps the previous one as history under compare-and-set', async () => {
  const directory = await fs.mkdtemp(path.join(base, 'bots-canonical-'));
  const runtime = createOneBotRuntime({directory});
  try {
    const profile = await runtime.call('profiles.create', {input: {id: 'keeper', name: 'Keeper'}});
    const first = 'D'.repeat(26), second = 'E'.repeat(26);
    await runtime.call('sessions.bind', {botId: 'keeper', sessionId: first});
    const current = await runtime.call('profiles.get', {botId: 'keeper'});
    await assert.rejects(runtime.call('sessions.replaceCanonical', {botId: 'keeper', sessionId: second, expectedCanonicalSessionId: null, expectedProfileVersion: current.currentVersion}), /主任务已变化/);
    await assert.rejects(runtime.call('sessions.replaceCanonical', {botId: 'keeper', sessionId: second, expectedCanonicalSessionId: first, expectedProfileVersion: profile.currentVersion - 1}), /配置已更新/);
    const result = await runtime.call('sessions.replaceCanonical', {botId: 'keeper', sessionId: second, expectedCanonicalSessionId: first, expectedProfileVersion: current.currentVersion});
    assert.deepEqual(result, {created: true, canonicalSessionId: second, archivedCanonicalSessionId: first});
    const sessions = (await runtime.call('profiles.get', {botId: 'keeper'})).sessions;
    assert.deepEqual(sessions.map(({id, role, routeKey}) => ({id, role, routeKey})).sort((a, b) => a.id.localeCompare(b.id)), [
      {id: first, role: 'history', routeKey: `history:${first}`},
      {id: second, role: 'canonical', routeKey: 'canonical'},
    ]);
  } finally { await runtime.close(); }
});
