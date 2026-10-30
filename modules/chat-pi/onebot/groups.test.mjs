// Cindy's local group chats on the OneBot worker: the original group service
// with WinOne's lanes, native turns, 分工 work folders and attachments.
import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {execFileSync} from 'node:child_process';
import {createOneBotRuntime} from './runtime.mjs';

const base = process.env.WINONE_ONEBOT_TEST_OUTPUT;
if (!base || !path.isAbsolute(base)) throw Error('Run through build/test-onebot.mjs with an isolated output');
const PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64');
let nextSession = 0;
const sessionId = () => (`GROUPLANE${String(++nextSession).padStart(4, '0')}`).padEnd(26, 'A').replace(/[^A-Z2-7]/g, 'A');

async function fixture({gitProject = false} = {}) {
  const directory = await fs.mkdtemp(path.join(base, 'groups-'));
  // The isolated output sits in a repository checkout; an unreadable gitfile
  // ends git discovery here, as a WinOne data folder outside any repository.
  await fs.writeFile(path.join(directory, '.git'), 'gitdir: missing\n');
  const turns = [], terminals = new Map(), lanes = new Map(), closed = [], recycled = [], events = [], aux = [];
  let planReply = {needsPlan: false}, runtime;
  const tasks = {
    dispatch: async input => { turns.push(input); await input.onAccepted?.(); return {ok: true, targetSessionId: input.targetSessionId, wakeKind: 'created', execution: {instanceId: 'native', generation: turns.length}}; },
    watchTurn: (sessionId, clientId, execution, onTerminal) => terminals.set(clientId, onTerminal),
    abortGroupLane: async () => {},
    hasPendingInteraction: () => false,
    readSession: async id => ({id, status: 'active'}),
  };
  const host = {
    groupLane: async input => {
      const id = input.sessionId ?? sessionId();
      await runtime.call('sessions.bind', {botId: input.botId, sessionId: id, routeKey: input.routeKey, role: 'group'});
      lanes.set(id, input);
      return {ok: true, sessionId: id};
    },
    closeGroupLanes: async ids => { closed.push(...ids); return {closed: ids.length}; },
    worktreeState: async () => ({gitInstalled: gitProject, isGitRepo: gitProject}),
    groupWorktree: async () => ({ok: false, message: 'not a repository'}),
    recycleGroupFolder: async directory => { recycled.push(directory); await fs.rm(directory, {recursive: true, force: true}); },
    groupProjectDir: async dir => ({ok: true, dir}),
    auxiliaryText: async request => { aux.push(request); return {ok: true, text: JSON.stringify(planReply)}; },
    conversation: async id => ({id, model: {provider: 'fixture', model: 'model'}}),
  };
  runtime = createOneBotRuntime({directory, host, tasks, event: value => events.push(value)});
  for (const [id, name] of [['alpha', 'Alpha'], ['beta', 'Beta'], ['gamma', 'Gamma']]) await runtime.call('profiles.create', {input: {id, name}});
  return {directory, runtime, turns, terminals, lanes, closed, recycled, events, aux, setPlan: value => { planReply = value; }};
}
async function eventually(check, message) {
  for (let n = 0; n < 250; n++) {
    const value = await check();
    if (value) return value;
    await new Promise(resolve => setTimeout(resolve, 20));
  }
  throw Error(message);
}
const botOf = turn => turn.clientId.split(':').at(-1);

test('group files stream into the group store and refuse unrelated local paths', async () => {
  const f = await fixture();
  try {
    const {groupId} = await f.runtime.call('groups.create', {input: {name: 'Uploads', botIds: ['alpha', 'beta']}});
    const bytes = Buffer.from('Native group attachment');
    const upload = await f.runtime.call('groups.files.begin', {groupId, name: 'notes.txt', size: bytes.length});
    await assert.rejects(f.runtime.call('groups.files.chunk', {token: upload.token, offset: 3, base64: bytes.toString('base64')}), /顺序/);
    await f.runtime.call('groups.files.chunk', {token: upload.token, offset: 0, base64: bytes.toString('base64')});
    const file = await f.runtime.call('groups.files.finish', {token: upload.token});
    assert.equal(file.size, bytes.length);
    assert.ok(file.path.startsWith(path.join(f.directory, 'bot-groups', groupId)));
    const read = await f.runtime.call('groups.files.read', {path: file.path, offset: 0});
    assert.equal(Buffer.from(read.base64, 'base64').toString(), bytes.toString());
    const secret = path.join(f.directory, 'private.txt');
    await fs.writeFile(secret, 'must not be returned');
    await assert.rejects(f.runtime.call('groups.files.read', {path: secret}), /群聊/);
    await assert.rejects(f.runtime.call('groups.files.begin', {groupId: '../outside', name: 'notes.txt', size: 1}), /群聊/);
    const picture = await f.runtime.call('groups.files.begin', {groupId, name: 'picture.png', size: PNG.length});
    await f.runtime.call('groups.files.chunk', {token: picture.token, offset: 0, base64: PNG.toString('base64')});
    const image = await f.runtime.call('groups.files.finish', {token: picture.token, image: true});
    assert.match(image.url, /^cindy-media:\/\/blobs\//);
    const resolvedImage=await f.runtime.call('groups.files.describe', {path: image.url});
    assert.equal(resolvedImage.kind, 'image');
    assert.deepEqual(Buffer.from((await f.runtime.call('groups.files.read',{path:resolvedImage.path})).base64,'base64'),PNG);
  } finally { await f.runtime.close(); }
});

test('a git project plan uses one registered named worktree and keeps the source checkout untouched', async () => {
  const f = await fixture({gitProject: true});
  const repo = path.join(f.directory, 'project');
  await fs.mkdir(repo);
  const git = (...args) => execFileSync('git', args, {cwd: repo, encoding: 'utf8', windowsHide: true});
  git('init', '-b', 'main');
  git('config', 'core.autocrlf', 'false');
  await fs.writeFile(path.join(repo, 'report.md'), 'committed original\n');
  git('add', 'report.md');
  git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', 'commit', '-m', 'fixture');
  await fs.writeFile(path.join(repo, 'report.md'), 'unsaved source edit\n');
  try {
    const {groupId} = await f.runtime.call('groups.create', {input: {name: 'Project', botIds: ['alpha', 'beta']}});
    await f.runtime.call('groups.update', {input: {groupId, projectDir: repo}});
    f.setPlan({needsPlan: true, goal: 'Write and review the report', steps: [{botId: 'alpha', task: 'Write'}, {botId: 'beta', task: 'Review'}]});
    await f.runtime.call('groups.send', {input: {groupId, clientId: 'git-plan', text: 'Write and review the report'}});
    const plan = await eventually(async () => (await f.runtime.call('groups.get', {groupId})).group.plans.find(item => item.status === 'proposed'), 'no plan');
    const start = await f.runtime.call('groups.plan.start', {input: {groupId, planId: plan.id}});
    assert.equal(start.ok, true, JSON.stringify(start));
    const first = await eventually(() => f.turns[0], 'no first step');
    const lane = f.lanes.get(first.targetSessionId);
    assert.equal(path.dirname(lane.workingDir), path.join(repo, '.cindy-worktrees'));
    assert.match(lane.sessionId, /^[A-Z2-7]{26}$/);
    const branch = execFileSync('git', ['branch', '--show-current'], {cwd: lane.workingDir, encoding: 'utf8', windowsHide: true}).trim();
    assert.match(branch, /^cindy\//);
    assert.equal(await fs.readFile(path.join(lane.workingDir, 'report.md'), 'utf8'), 'committed original\n');
    await fs.writeFile(path.join(lane.workingDir, 'report.md'), 'shared draft\n');
    await f.terminals.get(first.clientId)({status: 'completed', resultText: 'Draft ready'});
    await eventually(async () => (await f.runtime.call('groups.get', {groupId})).group.plans.find(item => item.status === 'waiting'), 'no handoff');
    await f.runtime.call('groups.plan.continue', {input: {groupId, planId: plan.id}});
    const second = await eventually(() => f.turns[1], 'no second step');
    assert.equal(f.lanes.get(second.targetSessionId).workingDir, lane.workingDir);
    assert.equal(await fs.readFile(path.join(repo, 'report.md'), 'utf8'), 'unsaved source edit\n');
    assert.equal(git('branch', '--show-current').trim(), 'main');
    assert.equal((git('worktree', 'list', '--porcelain').match(/^worktree /gm) ?? []).length, 2);
    await f.terminals.get(second.clientId)({status: 'completed', resultText: 'Reviewed'});
  } finally { await f.runtime.close(); }
});

test('a group round asks each member in its own lane and keeps the shared timeline', async () => {
  const f = await fixture();
  try {
    const created = await f.runtime.call('groups.create', {input: {name: 'Team', botIds: ['alpha', 'beta']}});
    assert.equal(created.ok, true, JSON.stringify(created));
    const groupId = created.groupId;
    assert.deepEqual((await f.runtime.call('groups.list')).groups.map(group => [group.name, group.members.map(member => member.botId)]), [['Team', ['alpha', 'beta']]]);
    // An unaddressed message: the organizer's automatic 分工 check says no, so everyone answers.
    assert.equal((await f.runtime.call('groups.send', {input: {groupId, clientId: 'owner-1', text: 'Hello team'}})).ok, true);
    await eventually(() => f.turns.length === 2, 'both members were not asked');
    assert.equal(f.aux.length, 1, 'the organizer decides whether to split the request');
    assert.deepEqual(f.turns.map(botOf).sort(), ['alpha', 'beta']);
    for (const turn of f.turns) {
      assert.match(turn.message, /Hello team/);
      assert.ok(turn.persistedContent.startsWith('[UI_ACTION_TRIGGER]'), 'a lane turn is a hidden Host instruction');
      assert.equal(f.lanes.get(turn.targetSessionId).routeKey, `group:${groupId}`);
    }
    await f.terminals.get(f.turns.find(turn => botOf(turn) === 'alpha').clientId)({status: 'completed', resultText: 'Alpha says hi'});
    await f.terminals.get(f.turns.find(turn => botOf(turn) === 'beta').clientId)({status: 'completed', resultText: 'NO_REPLY'});
    // The next circle lets members answer each other, one at a time; both stay
    // silent and the round ends.
    for (const index of [2, 3]) {
      const turn = await eventually(() => f.turns[index], 'the second circle did not continue');
      if (botOf(turn) === 'beta') assert.match(turn.message, /Alpha says hi/);
      await f.terminals.get(turn.clientId)({status: 'completed', resultText: 'NO_REPLY'});
    }
    const group = await eventually(async () => {
      const detail = (await f.runtime.call('groups.get', {groupId})).group;
      return detail.round.status === 'idle' && detail.messages.some(message => message.kind === 'round-end') ? detail : null;
    }, 'the round did not end');
    assert.deepEqual(group.messages.map(message => [message.kind, message.authorKind, message.content]),
      [['message', 'user', 'Hello team'], ['message', 'bot', 'Alpha says hi'], ['round-end', 'system', '']]);
    assert.equal(group.round.canContinue, true);
    assert.ok(f.events.some(event => event.type === 'group-changed' && event.payload?.groupId === groupId));
    // The same message is not posted twice.
    assert.equal((await f.runtime.call('groups.send', {input: {groupId, clientId: 'owner-1', text: 'Hello team'}})).messageId, group.messages[0].id);
  } finally { await f.runtime.close(); }
});

test('members leaving or a deleted group archive and close their lanes', async () => {
  const f = await fixture();
  try {
    const {groupId} = await f.runtime.call('groups.create', {input: {name: 'Pair', botIds: ['alpha', 'beta']}});
    await f.runtime.call('groups.update', {input: {groupId, replyMode: 'mentioned'}});
    await f.runtime.call('groups.send', {input: {groupId, clientId: 'mention-1', text: '@Beta please check', mentions: {all: false, botIds: ['beta']}}});
    await eventually(() => f.turns.length === 1, 'the mentioned member was not asked');
    assert.equal(botOf(f.turns[0]), 'beta');
    await f.terminals.get(f.turns[0].clientId)({status: 'completed', resultText: 'Checked'});
    const betaLane = f.turns[0].targetSessionId;
    await eventually(async () => (await f.runtime.call('groups.get', {groupId})).group.round.status === 'idle', 'the round did not settle');
    assert.equal((await f.runtime.call('groups.setMembers', {input: {groupId, botIds: ['alpha', 'gamma']}})).ok, true);
    assert.deepEqual(f.closed, [betaLane]);
    const link = (await f.runtime.call('profiles.get', {botId: 'beta'})).sessions.find(session => session.id === betaLane);
    assert.equal(link.role, 'group');
    assert.ok(link.archivedAt > 0);
    assert.equal(link.routeKey, `archived:${betaLane}`);
    // A group needs at least two members.
    assert.equal((await f.runtime.call('groups.setMembers', {input: {groupId, botIds: ['alpha']}})).errorCode, 'MEMBER_LIMIT');
    assert.equal((await f.runtime.call('groups.delete', {groupId})).ok, true);
    assert.deepEqual((await f.runtime.call('groups.list')).groups, []);
    assert.deepEqual(f.recycled, [], 'a group that never ran 分工 has no folder to recycle');
  } finally { await f.runtime.close(); }
});

test('a 分工 plan runs each step in the group folder and hands its files on', async () => {
  const f = await fixture();
  try {
    const {groupId} = await f.runtime.call('groups.create', {input: {name: 'Builders', botIds: ['alpha', 'beta']}});
    f.setPlan({needsPlan: true, steps: [{botId: 'alpha', task: 'Draft the report'}, {botId: 'beta', task: 'Review the draft'}]});
    await f.runtime.call('groups.send', {input: {groupId, clientId: 'plan-1', text: 'Write and review the weekly report', division: true}});
    const proposed = await eventually(async () => (await f.runtime.call('groups.get', {groupId})).group.plans.find(plan => plan.status === 'proposed'), 'no plan was proposed');
    assert.deepEqual(proposed.steps.map(step => [step.botName, step.task]), [['Alpha', 'Draft the report'], ['Beta', 'Review the draft']]);
    assert.equal((await f.runtime.call('groups.plan.start', {input: {groupId, planId: proposed.id}})).ok, true);
    const first = await eventually(() => f.turns[0], 'the first step did not start');
    const lane = f.lanes.get(first.targetSessionId);
    const folder = path.join(f.directory, 'bot-groups', groupId, 'files');
    assert.equal(lane.routeKey, `group:${groupId}:plan:${proposed.id}`);
    assert.equal(lane.workingDir, folder);
    await fs.writeFile(path.join(folder, 'report.md'), '# Weekly report\n');
    await f.terminals.get(first.clientId)({status: 'completed', resultText: 'Draft is in report.md'});
    const waiting = await eventually(async () => (await f.runtime.call('groups.get', {groupId})).group.plans.find(plan => plan.id === proposed.id && plan.status === 'waiting'), 'the first step did not finish');
    assert.equal(waiting.steps[0].status, 'done');
    const handoff = (await f.runtime.call('groups.get', {groupId})).group.messages.find(message => message.planId === proposed.id && message.kind === 'message');
    assert.deepEqual(handoff.files, ['report.md']);
    assert.equal((await f.runtime.call('groups.plan.continue', {input: {groupId, planId: proposed.id}})).ok, true);
    const second = await eventually(() => f.turns[1], 'the second step did not start');
    assert.match(second.message, /report\.md/, 'the next step reads the earlier hand-off');
    await f.terminals.get(second.clientId)({status: 'completed', resultText: 'Reviewed'});
    const done = await eventually(async () => (await f.runtime.call('groups.get', {groupId})).group.plans.find(plan => plan.id === proposed.id && plan.status === 'done'), 'the plan did not finish');
    assert.equal(done.workDir, folder);
    assert.equal((await f.runtime.call('groups.delete', {groupId})).ok, true);
    assert.deepEqual(f.recycled, [path.join(f.directory, 'bot-groups', groupId)]);
  } finally { await f.runtime.close(); }
});

test('attachments go with the turn: managed images and local files', async () => {
  const f = await fixture();
  try {
    const {groupId} = await f.runtime.call('groups.create', {input: {name: 'Files', botIds: ['alpha', 'beta']}});
    await f.runtime.call('groups.update', {input: {groupId, replyMode: 'mentioned'}});
    const image = await f.runtime.call('groups.attachments.image', {base64: PNG.toString('base64')});
    assert.match(image.url, /^cindy-media:\/\/blobs\/[0-9a-f]{64}\.png$/);
    const staged = await f.runtime.call('groups.attachments.stage', {groupId, name: 'notes.txt', base64: Buffer.from('notes').toString('base64')});
    assert.ok(staged.path.startsWith(path.join(f.directory, 'bot-groups', groupId, 'attachments')));
    await assert.rejects(f.runtime.call('groups.attachments.image', {base64: Buffer.from('not an image').toString('base64')}), /PNG/);
    const sent = await f.runtime.call('groups.send', {input: {groupId, clientId: 'files-1', text: '@Alpha see these', mentions: {all: false, botIds: ['alpha']},
      attachments: [{id: 'img', name: 'chart.png', path: '', url: image.url, category: 'image', mimeType: 'image/png'},
        {id: 'txt', name: 'notes.txt', path: staged.path, category: 'text', mimeType: 'text/plain'}]}});
    assert.equal(sent.ok, true, JSON.stringify(sent));
    const turn = await eventually(() => f.turns[0], 'the member was not asked');
    assert.deepEqual(turn.attachments, [{name: 'chart.png', url: image.url}, {name: 'notes.txt', path: staged.path}]);
    const message = (await f.runtime.call('groups.get', {groupId})).group.messages[0];
    assert.deepEqual(message.attachments.map(item => [item.name, item.category, item.url, item.path]),
      [['chart.png', 'image', image.url, null], ['notes.txt', 'text', null, staged.path]]);
    // A path that is not a file on this computer is refused before anything is posted.
    const refused = await f.runtime.call('groups.send', {input: {groupId, clientId: 'files-2', text: 'missing', attachments: [{name: 'gone.txt', path: path.join(f.directory, 'gone.txt'), category: 'text', mimeType: 'text/plain'}]}});
    assert.equal(refused.errorCode, 'INVALID_PARAMS');
  } finally { await f.runtime.close(); }
});

test('a group lane keeps memory but has no Skill access without a live group grant', async () => {
  const f = await fixture();
  try {
    const {groupId} = await f.runtime.call('groups.create', {input: {name: 'Tools', botIds: ['alpha', 'beta']}});
    await f.runtime.call('groups.update', {input: {groupId, replyMode: 'mentioned'}});
    await f.runtime.call('groups.send', {input: {groupId, clientId: 'tools-1', text: '@Alpha hi', mentions: {all: false, botIds: ['alpha']}}});
    const lane = (await eventually(() => f.turns[0], 'the member was not asked')).targetSessionId;
    await assert.rejects(f.runtime.call('session.tool', {sessionId: lane, method: 'skills.query', args: {}, ownerAuthorized: true}), error => error.code === 'GROUP_AUTHORIZATION_REQUIRED');
    const written = await f.runtime.call('session.tool', {sessionId: lane, method: 'memory.write', ownerAuthorized: true,
      args: {input: {type: 'project', name: 'team-goal', title: 'Team goal', description: 'Shared goal', body: 'Ship the weekly report.'}}});
    assert.equal(written.filename, 'project_team-goal.md');
    const context = await f.runtime.call('runtime.context', {sessionId: lane});
    assert.equal(context.role, 'group');
  } finally { await f.runtime.close(); }
});
