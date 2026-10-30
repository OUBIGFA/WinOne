// Cindy's exact connection-lane list/reset/archive service, with Pi transcripts
// replacing Cindy's messages table. A per-call Host snapshot supplies only
// registered OneBot sessions and cheap live activity; reads never start Pi.
import {
  configureBotImLaneRuntime, listBotImConnectionLanes, resetBotImConnectionLane,
  archiveBotImConnectionLane, parseLaneRouteKey,
} from '../cindy/tasks/im/botImLaneRuntime.ts';
import {readBotImConnection} from '../cindy/bots/botImConnectionStore.ts';
import {companionConnectionRouteKeyPrefix} from '../cindy/bots/imBotBindingRoute.ts';
import {oneBotOwnerScope, withIMScope} from './im-scope.mjs';
import {oneBotError} from './content-service.mjs';
import {readBranch} from './history.mjs';

function snapshot() { return oneBotOwnerScope().laneSnapshot; }
function conversation(id) {
  const row = snapshot().sessions.get(id);
  if (!row) throw oneBotError('NOT_FOUND', '渠道会话不存在');
  return row;
}
async function transcript(id) {
  const state = snapshot();
  if (!state.transcripts.has(id)) state.transcripts.set(id, readBranch(conversation(id)));
  return state.transcripts.get(id);
}
// Pi entry ids are stable across restarts and distinguish messages committed
// in the same millisecond, just as Cindy's rowid archive anchors do.
function visibleActivity(branch) {
  return branch.filter(entry => entry.type === 'message' && ['user', 'assistant'].includes(entry.message?.role));
}
function titleName(row) {
  const marker = row.title?.indexOf(' · ') ?? -1;
  return marker < 0 ? null : row.title.slice(marker + 3).trim() || null;
}
const store = {
  async listLinks(botId) {
    return oneBotOwnerScope().profiles.sessions(botId).flatMap(link => {
      const row = snapshot().sessions.get(link.id);
      return row && !row.archived && ['canonical', 'channel'].includes(link.role)
        ? [{sessionId: link.id, role: link.role, routeKey: link.routeKey}] : [];
    });
  },
  async readActivity(id) {
    const branch = await transcript(id), rows = visibleActivity(branch), latest = rows.at(-1);
    const boundary = branch.findLastIndex(entry => entry.customType === 'winone.context-reset' || entry.type === 'compaction');
    const name = titleName(conversation(id));
    return {
      lastActivityAt: latest ? Date.parse(latest.timestamp) : null,
      lastTurnFailed: !!latest && branch.indexOf(latest) > boundary && ['error', 'aborted'].includes(latest.message.stopReason),
      contextMessageCount: branch.slice(boundary + 1).filter(entry => entry.type === 'message' && entry.message?.role === 'user').length,
      origin: {chatName: name, senderName: name},
    };
  },
  async readArchiveActivity(id, archive) {
    const rows = visibleActivity(await transcript(id));
    if (typeof archive === 'number') {
      const recent = rows.filter(row => Date.parse(row.timestamp) > archive);
      return {messageId: recent.at(-1)?.id ?? null, hasNewActivity: recent.length > 0};
    }
    const anchor = archive?.messageId ? rows.findIndex(row => row.id === archive.messageId) : -1;
    const recent = rows.slice(anchor + 1);
    return {messageId: recent.at(-1)?.id ?? null, hasNewActivity: recent.length > 0};
  },
};
configureBotImLaneRuntime({
  isTurnRunning: id => ['running', 'needs-interaction'].includes(snapshot().activity[id]?.phase),
  restartSession: async id => {
    await oneBotOwnerScope().host.restartLane(id);
    snapshot().transcripts.delete(id);
  },
}, store);

async function knownGroups(record) {
  const prefix = companionConnectionRouteKeyPrefix(record.channel, record.botContextId), groups = new Map();
  for (const link of await store.listLinks(record.botId)) {
    if (link.role !== 'channel') continue;
    const lane = parseLaneRouteKey(prefix, link.routeKey);
    if (lane && lane.kind !== 'guest-dm') groups.set(lane.chatId, {chatId: lane.chatId, chatName: titleName(conversation(link.sessionId))});
  }
  return [...groups.values()];
}

export async function callOneBotIMLanes(method, params) {
  const owner = oneBotOwnerScope();
  if (!owner.host) throw oneBotError('NOT_READY', 'OneBot 渠道会话接口尚未就绪');
  if (typeof params.id !== 'string') throw oneBotError('INVALID_PARAMS', '请选择 IM 连接');
  const record = readBotImConnection(params.id);
  if (!record) throw oneBotError('NOT_FOUND', 'IM 连接不存在');
  const facts = await owner.host.laneSessions();
  return withIMScope({...owner, laneSnapshot: {sessions: new Map(facts.conversations.map(row => [row.id, row])), activity: facts.activity, transcripts: new Map()}}, async () => {
    const options = {knownGroups};
    if (method === 'list') return listBotImConnectionLanes(record.id, options);
    if (method === 'groups') return {connectionId: record.id, groups: (await knownGroups(record)).map(group => ({...group, activation: record.groupActivation?.[group.chatId] === 'always' ? 'always' : 'mention'}))};
    if (method === 'retire') {
      // Removing a connection archives only its hidden lanes. The owner's
      // main chat is shared by the teammate and must continue to exist.
      const prefix = companionConnectionRouteKeyPrefix(record.channel, record.botContextId), ids = [];
      if (record.botContextId) for (const link of await store.listLinks(record.botId)) {
        if (link.role === 'channel' && parseLaneRouteKey(prefix, link.routeKey)) {
          await owner.host.archiveLane(link.sessionId); ids.push(link.sessionId);
        }
      }
      return {archivedSessionIds: ids};
    }
    if (typeof params.sessionId !== 'string') throw oneBotError('INVALID_PARAMS', '请选择渠道会话');
    if (method === 'reset') return resetBotImConnectionLane(record.id, params.sessionId, options);
    if (method === 'archive') return archiveBotImConnectionLane(record.id, params.sessionId, options);
    throw oneBotError('UNKNOWN_METHOD', '不支持的渠道会话操作');
  });
}
