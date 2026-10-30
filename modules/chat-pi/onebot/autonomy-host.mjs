// WinOne host data for Cindy's teammate follow-up (bot-autonomy/hostData.ts,
// Cindy b7e46bc, Apache-2.0). Teammates live in the OneBot profile store,
// their delegated tasks in the task store and every conversation in WinOne
// Chat; the Host bridge supplies conversation facts, never transcripts.
import {oneBotOwnerScope} from './im-scope.mjs';
import {readAllowSchedulerFollow} from '../cindy/autonomy/follower.ts';
import {readTaskResponsibilities, responsibleSessionIds} from '../cindy/bots/botTaskResponsibility.ts';

/** Teammate directory: status, the `allowSchedulerFollow` capability and canonical chat. */
export async function readBotDirectory() {
  const owner = oneBotOwnerScope();
  const out = new Map();
  for (const profile of owner.profiles.list()) {
    out.set(profile.id, {
      status: profile.status,
      allowSchedulerFollow: readAllowSchedulerFollow(JSON.stringify(profile.capabilities ?? {})),
      // WinOne has no cloud companions running on another computer.
      shadowed: false,
      canonicalSessionId: profile.canonicalSessionId ?? null,
    });
  }
  return out;
}

/**
 * Cindy LocalSessionMeta for the given conversations. The Host reads WinOne
 * Chat (ordinary and OneBot stores); the lane role comes from this store's
 * durable session links. WinOne Chat has no hidden or Orca sessions.
 */
export async function readLocalSessionMetas(ids) {
  const unique = [...new Set(ids)].filter(id => typeof id === 'string' && id);
  const owner = oneBotOwnerScope();
  if (!unique.length || !owner.host) return [];
  const rows = await owner.host.sessionMetas(unique);
  return rows.map(row => {
    const lane = owner.profiles.sessionOwnerOrNull(row.id);
    return {
      id: row.id,
      title: row.title,
      workingDir: row.workingDir || null,
      status: row.archived ? 'archived' : 'active',
      hiddenAt: null,
      source: row.store === 'onebot' ? 'bot' : 'desktop',
      orcaRole: null,
      laneRole: lane?.role ?? null,
      laneBotId: lane?.botId ?? null,
      // Pi's durable session resumes an unfinished turn when it reopens;
      // WinOne records no separate "interrupted by exit" state. The turn
      // times below only serve that seed and input-error banners, which
      // WinOne Chat does not have; errors carry their activity time instead.
      interrupted: false,
      activeTurnStartedAt: null,
      lastTurnEndedAt: null,
    };
  });
}

/** See readLocalSessionMetas: WinOne keeps no interrupted-turn record to seed from. */
export async function readInterruptedSessionMetas() {
  return [];
}

/**
 * Task ownership: delegation sources (bot_delegations) and each teammate's
 * task-responsibility records. A teammate whose record is unreadable claims
 * nothing (fail closed), exactly as upstream.
 */
export async function readTaskOwnership(userDataDir) {
  const owner = oneBotOwnerScope();
  const bots = await readBotDirectory();
  const delegatedByBot = new Map();
  for (const row of await owner.delegationRows()) {
    if (!row.sessionId) continue;
    const set = delegatedByBot.get(row.botId) ?? new Set();
    set.add(row.sessionId);
    delegatedByBot.set(row.botId, set);
  }
  const responsibleByBot = new Map();
  const creatorBySession = new Map();
  const takenOver = [];
  for (const [botId, bot] of bots) {
    if (bot.status === 'archived' || bot.status === 'deleting') continue;
    const delegated = delegatedByBot.get(botId) ?? new Set();
    let ids;
    try {
      ids = responsibleSessionIds(delegated, await readTaskResponsibilities(userDataDir, botId));
    } catch {
      continue;
    }
    responsibleByBot.set(botId, new Set(ids));
    for (const id of ids) {
      if (delegated.has(id)) {
        if (!creatorBySession.has(id)) creatorBySession.set(id, botId);
      } else takenOver.push([id, botId]);
    }
  }
  for (const [id, botId] of takenOver) if (!creatorBySession.has(id)) creatorBySession.set(id, botId);
  return {bots, creatorBySession, responsibleByBot};
}

/** Other computers were removed with Cindy cloud: there is never a remote target. */
export function toRemoteTargets() {
  return [];
}
