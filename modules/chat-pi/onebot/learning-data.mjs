// Cindy SQL projections over Host-listed Pi branches. No renderer path or
// model-selected conversation can enter these reads.
import fs from 'node:fs/promises';
import {readBranch} from './history.mjs';
import {oneBotError} from './content-service.mjs';

/** Cindy botLearningReview REVIEW_ROLES: the rows a learning review reads. */
const REVIEW_ROLES = new Set(['user', 'assistant', 'tool_use', 'tool_result', 'ask_user', 'plan_review']);

const text = content => typeof content === 'string' ? content : (content ?? []).filter(part => part.type === 'text').map(part => part.text).join('\n');
/** An answered ask_user_question result as Cindy's ask_user row ({answers}). */
function askUserContent(content) {
  try {
    const answers = {};
    for (const item of JSON.parse(text(content))) if (typeof item?.question === 'string' && typeof item.answer === 'string') answers[item.question] = item.answer;
    return {answers};
  } catch { return {answers: {}}; }
}
/**
 * Cindy `messages` rows of one Pi branch, in timeline order. `rowid` is the
 * position on this branch: rewound entries are off the branch, so positions
 * of surviving rows only grow as the conversation continues.
 */
export function learningTranscript(branch, overlay = () => ({})) {
  const rows = [];
  let receipt;
  for (const entry of branch) {
    if (entry.type === 'custom' && entry.customType === 'winone.input') receipt = entry.data;
    if (entry.type !== 'message') continue;
    const message = entry.message, createdAt = Date.parse(entry.timestamp);
    if (message?.role === 'user') {
      const input = receipt; receipt = undefined;
      const clientId = input?.id ?? entry.id;
      rows.push({id: clientId, role: 'user', content: JSON.stringify({text: input?.presentation?.text ?? input?.text ?? text(message.content)}),
        createdAt, meta: {...(input?.presentation?.hidden ? {origin: {kind: 'session'}} : {}),
          ...(input?.hostDispatched ? {origin: {kind: 'scheduler'}} : {}), ...overlay(clientId)}});
    } else if (message?.role === 'assistant') {
      for (const part of Array.isArray(message.content) ? message.content : []) if (part.type === 'toolCall') {
        rows.push({id: entry.id + ':' + part.id, role: 'tool_use', content: JSON.stringify({toolName: part.name, input: part.arguments}), createdAt, meta: {}});
      }
      if (message.stopReason === 'toolUse') continue;
      rows.push({id: entry.id, role: 'assistant', content: JSON.stringify(text(message.content)), createdAt,
        meta: {turnCompleted: ['stop', 'length'].includes(message.stopReason), ...overlay(entry.id)}});
    } else if (message?.role === 'toolResult') {
      // The owner's answer to a question is a decision, reviewed like their words.
      rows.push(message.toolName === 'ask_user_question'
        ? {id: entry.id, role: 'ask_user', content: JSON.stringify(askUserContent(message.content)), createdAt, meta: {}}
        : {id: entry.id, role: 'tool_result', content: JSON.stringify(text(message.content)), createdAt, meta: {}});
    }
  }
  return rows.map((row, index) => ({...row, rowid: index + 1}));
}
// Parsed branches by file state: a learning review reads one conversation
// several times; an unchanged file is not parsed again. Overlays stay live.
const branches = new Map();
export async function branchRows(session) {
  const info = session.sessionFile ? await fs.stat(session.sessionFile).catch(() => null) : null;
  const key = info ? `${session.sessionFile}:${info.size}:${info.mtimeMs}` : null;
  const cached = key ? branches.get(session.id) : null;
  if (cached?.key === key) return cached.rows;
  const rows = learningTranscript(await readBranch(session));
  if (key) {
    branches.delete(session.id);
    branches.set(session.id, {key, rows});
    if (branches.size > 32) branches.delete(branches.keys().next().value);
  }
  return rows;
}
export async function rowsForSession(owner, link) {
  if (!owner.host) throw oneBotError('NOT_READY', '伙伴对话读取接口尚未就绪');
  const session = await owner.host.conversation(link.id);
  if (session.archived) return [];
  const meta = new Map((owner.profiles.messageMetaFor?.(link.id) ?? []).map(row => [row.clientId, row.data]));
  return (await branchRows({...session, id: link.id})).map(row => ({...row, meta: {...row.meta, ...meta.get(row.id)}}));
}
/** The active link a learning read may use (Cindy's bot_session_links/profile join). */
function learningLink(owner, sessionId, roles) {
  const link = owner.profiles.sessionOwnerOrNull(sessionId);
  if (!link || !roles.includes(link.role) || link.archivedAt != null) return null;
  try { if (owner.profiles.get(link.botId).status !== 'active') return null; } catch { return null; }
  return {...link, id: sessionId};
}
// Bind every assistant/tool row to the nearest actual user input on this branch.
// The private Host stamp distinguishes the owner from quoted/group speakers.
const reviewRow = (row,input) => ({id: row.id, role: row.role, content: row.content, meta: JSON.stringify(row.meta), order: row.rowid, inputMeta: input?JSON.stringify(input.meta):null});
export async function queryLearningRows(owner, sql, args) {
  // Cindy botLearningReview: the reply under review and its teammate.
  if (sql.includes('p.canonical_session_id AS canonicalSessionId') && sql.includes('AS inputMeta')) {
    const [sessionId, replyId] = args, link = learningLink(owner, sessionId, ['canonical', 'thread', 'channel']);
    if (!link) return [];
    const rows = await rowsForSession(owner, link), index = rows.findIndex(row => row.id === replyId && row.role === 'assistant');
    if (index < 0) return [];
    const row = rows[index], input = rows.slice(0, index).findLast(item => item.role === 'user'), bot = owner.profiles.get(link.botId);
    return [{botId: link.botId, role: link.role, canonicalSessionId: bot.canonicalSessionId ?? null, capabilities: JSON.stringify(bot.capabilities ?? {}),
      rowid: row.rowid, meta: JSON.stringify(row.meta), inputMeta: input ? JSON.stringify(input.meta) : null}];
  }
  // Cindy botLearningFeedback: receipts attach only to a main-chat reply.
  if (sql.includes('SELECT m.agent_meta AS meta FROM messages m') && sql.includes("b.role IN ('canonical', 'history')")) {
    const [sessionId, replyId] = args, link = learningLink(owner, sessionId, ['canonical', 'history']);
    if (!link) return [];
    const row = (await rowsForSession(owner, link)).find(item => item.id === replyId && item.role === 'assistant');
    return row ? [{meta: JSON.stringify(row.meta)}] : [];
  }
  const reviewQuery = ['/* prev-completed */', '/* topic-learning-progress */', '/* ledger-floor */'].find(marker => sql.includes(marker))
    ?? (sql.includes("ORDER BY CASE WHEN role='user' THEN 0 ELSE 1 END, rowid DESC") ? 'turn' : sql.includes('ORDER BY rowid ASC LIMIT ?') ? 'gap' : null);
  if (reviewQuery) {
    const link = learningLink(owner, args[0], ['canonical', 'thread', 'channel']);
    if (!link) return [];
    const rows = await rowsForSession(owner, link);
    if (reviewQuery === '/* prev-completed */') {
      const prev = rows.filter(row => row.role === 'assistant' && row.meta.turnCompleted === true && row.rowid < args[1]).at(-1);
      return prev ? [{id: prev.id, rowid: prev.rowid}] : [];
    }
    if (reviewQuery === '/* topic-learning-progress */') {
      const row = rows.filter(item => item.role === 'assistant' && item.rowid <= args[1] && item.meta.botLearningProgress && typeof item.meta.botLearningProgress === 'object').at(-1);
      return row ? [{meta: JSON.stringify(row.meta)}] : [];
    }
    if (reviewQuery === '/* ledger-floor */') {
      const row = rows.find(item => item.id === args[1]);
      return row ? [{rowid: row.rowid}] : [];
    }
    if (reviewQuery === 'turn') {
      // Cindy: user rows first, then the rest newest first, within the turn.
      const [, through, after] = args, limit = Number(/LIMIT (\d+)/.exec(sql)?.[1] ?? 80);
      const selected = rows.filter(row => REVIEW_ROLES.has(row.role) && row.rowid > after && row.rowid <= through);
      return [...selected.filter(row => row.role === 'user').reverse(), ...selected.filter(row => row.role !== 'user').reverse()].slice(0, limit).map(row=>reviewRow(row,rows.slice(0,row.rowid).findLast(input=>input.role==='user')));
    }
    const [, after, through, , limit] = args;
    return rows.filter(row => REVIEW_ROLES.has(row.role) && row.rowid > after && row.rowid <= through).slice(0, limit).map(row=>reviewRow(row,rows.slice(0,row.rowid).findLast(input=>input.role==='user')));
  }
  if (sql.includes('nightly-learning-count')) {
    const [sessionId, replyId] = args, link = owner.profiles.sessionOwnerOrNull(sessionId);
    if (!link || !['canonical', 'thread', 'channel'].includes(link.role) || link.archivedAt != null || owner.profiles.get(link.botId).status !== 'active') return [];
    const rows = await rowsForSession(owner, {...link, id: sessionId});
    const index = rows.findIndex(row => row.id === replyId && row.role === 'assistant');
    if (index < 0) return [];
    const row = rows[index], input = rows.slice(0, index).findLast(item => item.role === 'user');
    return [{botId: link.botId, role: link.role, createdAt: row.createdAt, meta: JSON.stringify(row.meta),
      inputMeta: input ? JSON.stringify(input.meta) : null, inputContent: input?.content ?? null}];
  }
  if (sql.includes('FROM bot_profiles p JOIN bot_profile_versions')) {
    let bot;
    try { bot = owner.profiles.get(args[0]); } catch (error) { if (error.code === 'NOT_FOUND') return []; throw error; }
    if (bot.status !== 'active') return [];
    const session = bot.canonicalSessionId && owner.host ? await owner.host.conversation(bot.canonicalSessionId) : null;
    return [{canonicalSessionId: bot.canonicalSessionId, capabilities: JSON.stringify(bot.capabilities),
      agentKind: 'pi', model: session?.model?.model ?? null, providerId: session?.model?.provider ?? null}];
  }
  if (sql.includes('nightly-learning-turns') || sql.includes('nightly-learning-skill-usage')) {
    const [botId, after, until] = args;
    owner.profiles.get(botId);
    const result = [];
    for (const link of owner.profiles.sessions(botId).filter(row => ['canonical', 'thread', 'channel'].includes(row.role) && row.archivedAt == null)) {
      const rows = await rowsForSession(owner, link);
      let input;
      for (const row of rows) {
        if (row.role === 'user') input = row;
        if (row.createdAt <= after || row.createdAt > until) continue;
        if (sql.includes('nightly-learning-skill-usage') && row.role === 'tool_use') result.push({content: row.content, createdAt: row.createdAt});
        if (sql.includes('nightly-learning-turns') && row.role === 'assistant') result.push({sessionId: link.id, id: row.id, content: row.content,
          meta: JSON.stringify(row.meta), createdAt: row.createdAt, role: link.role, inputId: input?.id ?? null, inputContent: input?.content ?? null, inputMeta: input ? JSON.stringify(input.meta) : null});
      }
    }
    return result.sort((a, b) => a.createdAt - b.createdAt).slice(0, sql.includes('nightly-learning-turns') ? 401 : 5000);
  }
  throw oneBotError('UNSUPPORTED_QUERY', '不支持的伙伴学习查询');
}
