// WinOne composition of Cindy's teammate follow-up (bot-autonomy/service.ts,
// Cindy b7e46bc, Apache-2.0). The original runtime, task watch, events and
// follow-up rules run unchanged; this module supplies WinOne's facts:
// conversation activity comes from the Host's Chat services (pushed on change,
// re-read while a watched task is still open), metadata from the Host bridge,
// ownership from the OneBot stores, and reminders go to the Windows tray.
import {BotAutonomyRuntime, SWEEP_INTERVAL_MS} from '../cindy/autonomy/runtime.ts';
import {TaskWatch, errorCode} from '../cindy/autonomy/taskWatch.ts';
import {configureBotAutonomyHost, getActiveBotAutonomy, scopeIsCurrent, setActiveBotAutonomy} from '../cindy/autonomy/hub.ts';
import {setBotAutonomyBotsChangedListener} from '../cindy/autonomy/botsChangedSignal.ts';
import {readBotAutonomySettingsOverride} from '../cindy/bots/bot-autonomy-settings-store.ts';
import {registerBotAutonomyReader} from '../cindy/bots/botWorkbenchAutonomyBridge.ts';
import {readLocalSessionMetas, readTaskOwnership} from './autonomy-host.mjs';
import {activeOwnerScopeKey, ownerScopedUserDataPath, withIMScope} from './im-scope.mjs';
import {createLogger} from './logger.ts';

const log = createLogger('bot-autonomy');
/** Teammate profile / lifecycle broadcasts often arrive in bursts (upstream value). */
const BOTS_CHANGED_DEBOUNCE_MS = 300;

/** WinOne approval request → Cindy pending interaction (kind, id, permission name or question). */
export function pendingInteractions(row) {
  return (row?.interactions ?? []).flatMap(request => {
    if (!request || typeof request !== 'object' || typeof request.id !== 'string') return [];
    return request.kind === 'question'
      ? [{kind: 'ask_user_question', requestId: request.id, questions: (Array.isArray(request.questions) ? request.questions : [])
        .map(question => ({question: typeof question?.question === 'string' ? question.question : ''}))}]
      : [{kind: 'permission', requestId: request.id, toolName: typeof request.tool === 'string' ? request.tool : ''}];
  });
}

/**
 * WinOne Chat activity → Cindy island snapshot. Idle, stopped (interrupted)
 * and paused conversations leave the projection, like a dismissed island
 * entry; a completed turn stays until the next one, as the island card does.
 */
export function islandSnapshot(row) {
  const phase = ['running', 'needs-interaction', 'completed', 'error'].includes(row?.phase) ? row.phase : null;
  if (!phase) return null;
  const first = pendingInteractions(row)[0];
  return {sessionId: row.sessionId, phase, currentTurnActive: phase === 'running' || phase === 'needs-interaction',
    startedAtMs: row.startedAtMs || null, lastActivityAtMs: row.lastActivityAtMs || null, currentActionSummary: null,
    ...(phase === 'needs-interaction' ? {interactionKind: first?.kind ?? 'ask_user_question'} : {}),
    attention: phase !== 'running', workflow: null, turnGeneration: Number.isSafeInteger(row.turnGeneration) ? row.turnGeneration : null,
    gracefulStopState: 'none', source: 'live'};
}

export function createOneBotAutonomy({owner, host, readSessionExecution, isCompletionHandledByTeammate, ensureOwnBuiltins}) {
  const rows = new Map();
  // The runtime this owner started. The hub holds one per process; a second
  // owner (tests reopen a data directory) replaces it, as upstream startRuntime does.
  let current = null;
  let refreshTimer = null, refreshing = null, botsChangedTimer = null;
  configureBotAutonomyHost({
    readSessionExecution: id => readSessionExecution?.(id) ?? null,
    isCompletionHandledByTeammate: (sessionId, botId, execution) => isCompletionHandledByTeammate(sessionId, botId, execution),
    getPendingInteractions: id => pendingInteractions(rows.get(id)),
  });
  setBotAutonomyBotsChangedListener(() => {
    if (botsChangedTimer) return;
    botsChangedTimer = setTimeout(() => withIMScope(owner, () => {
      botsChangedTimer = null;
      const active = getActiveBotAutonomy();
      if (!active || !scopeIsCurrent(active.scope)) return;
      active.runtime.refreshFollowers().catch(error => log.warn('autonomy follower refresh failed', {error: errorCode(error)}));
    }), BOTS_CHANGED_DEBOUNCE_MS);
    botsChangedTimer.unref?.();
  });

  function apply(next, complete) {
    const watch = getActiveBotAutonomy()?.runtime.watch;
    const seen = new Set();
    for (const row of next) {
      if (typeof row?.sessionId !== 'string' || !row.sessionId) continue;
      seen.add(row.sessionId);
      const previous = rows.get(row.sessionId);
      rows.set(row.sessionId, row);
      if (watch && JSON.stringify(previous) !== JSON.stringify(row)) watch.applyActivity(row.sessionId, islandSnapshot(row));
    }
    // A full read also reports conversations that left the projection.
    if (complete) for (const id of [...rows.keys()]) {
      if (seen.has(id)) continue;
      rows.delete(id);
      watch?.applyActivity(id, null);
    }
  }
  /**
   * One shared Host read; the watch then recomputes only changed rows. A
   * failed read is logged and never stops the routine engine that started us.
   */
  function refresh() {
    if (!host) return Promise.resolve();
    refreshing ??= Promise.resolve().then(() => host.activitySnapshots()).then(next => withIMScope(owner, () => apply(next, true)))
      .catch(error => log.warn('autonomy activity read failed', {error: errorCode(error)}))
      .finally(() => { refreshing = null; });
    return refreshing;
  }
  // Island activity is pushed per event upstream. WinOne pushes phase changes
  // and re-reads timing only while the watch still needs a sweep, so a long
  // healthy reply is not judged stuck and an idle owner adds no polling.
  function syncRefreshTimer() {
    const watch = getActiveBotAutonomy()?.runtime.watch;
    const needed = !!watch && watch.needsSweep();
    if (!needed && refreshTimer) { clearInterval(refreshTimer); refreshTimer = null; }
    else if (needed && !refreshTimer) {
      refreshTimer = setInterval(() => withIMScope(owner, () => void refresh().then(syncRefreshTimer)), SWEEP_INTERVAL_MS);
      refreshTimer.unref?.();
    }
  }

  function stop() {
    if (refreshTimer) clearInterval(refreshTimer);
    if (botsChangedTimer) clearTimeout(botsChangedTimer);
    refreshTimer = botsChangedTimer = null;
    if (!current) return;
    current.stop();
    if (getActiveBotAutonomy()?.runtime === current) {
      setActiveBotAutonomy(null);
      registerBotAutonomyReader(null);
    }
    current = null;
  }

  /** Mirrors upstream startRuntime for the routine engine of this owner scope. */
  function start(engine) {
    const existing = getActiveBotAutonomy();
    const scope = activeOwnerScopeKey();
    if (existing?.runtime === current && existing?.engine === engine && existing.scope === scope) return;
    stop();
    existing?.runtime.stop();
    const userDataDir = ownerScopedUserDataPath();
    const assertScope = () => { if (!scopeIsCurrent(scope)) throw Object.assign(new Error('owner changed'), {code: 'OWNER_CHANGED'}); };
    const runtime = new BotAutonomyRuntime({
      engine,
      ensureOwnBuiltins: async botId => { await ensureOwnBuiltins?.(botId); },
      now: Date.now,
      log,
      isCurrent: () => scopeIsCurrent(scope),
      createWatch: onChange => new TaskWatch({
        now: Date.now,
        log,
        onChange: () => { onChange(); syncRefreshTimer(); },
        getActivitySnapshots: () => [...rows.values()].map(islandSnapshot).filter(Boolean),
        readSessionExecution: id => readSessionExecution?.(id) ?? null,
        isToolCallInFlight: id => rows.get(id)?.toolInFlight === true,
        getPendingInteractions: id => pendingInteractions(rows.get(id)),
        readLocalSessions: async ids => { assertScope(); const result = await readLocalSessionMetas(ids); assertScope(); return result; },
        readInterruptedSessions: async () => [],
        readOwnership: async () => { assertScope(); const result = await readTaskOwnership(userDataDir); assertScope(); return result; },
        readSchedulerOverride: () => { assertScope(); return readBotAutonomySettingsOverride(); },
        listRemoteDevices: async () => null,
        invokeRemote: async () => { throw Object.assign(new Error('Other computers are not part of WinOne'), {code: 'DEVICE_LINK_NOT_CONNECTED'}); },
      }),
      subscribeActivity: watch => {
        for (const row of rows.values()) watch.applyActivity(row.sessionId, islandSnapshot(row));
        return () => {};
      },
      remind: row => {
        // Same reminder as Cindy's desktop notification, through WinOne's tray.
        void host?.remind({sessionId: row.sessionId, title: row.title, detail: row.pendingQuestion ?? ''})
          .catch(error => log.warn('autonomy reminder failed', {error: errorCode(error)}));
      },
      push: payload => {
        if (!scopeIsCurrent(scope)) return;
        owner.event({type: 'autonomy-changed', payload});
      },
    });
    setActiveBotAutonomy({scope, engine, runtime});
    current = runtime;
    // Cindy service.ts: the workbench and task digest read the watch only here.
    // Before ownership is read, a glance is "not ready", never "no tasks".
    const live = () => getActiveBotAutonomy()?.runtime === runtime && scopeIsCurrent(scope);
    registerBotAutonomyReader({
      glance: botId => (live() && runtime.watch.isReady() ? runtime.glance(botId) : null),
      schedulerBotId: () => (live() ? runtime.watch.getSchedulerBotId() : null),
      watched: sessionId => (live() ? runtime.watch.get(sessionId) ?? null : null),
    });
    // The first watch read needs the current activity of every conversation.
    void refresh().then(() => current === runtime ? withIMScope(owner, () => runtime.start()) : undefined)
      .then(syncRefreshTimer)
      .catch(error => log.warn('autonomy start failed', {error: errorCode(error)}));
    log.info('autonomy started');
  }

  return {
    start,
    stop,
    /** Host push: changed conversation activity (never a full list). */
    activity(params) {
      if (!Array.isArray(params?.snapshots)) throw Object.assign(new Error('Invalid activity'), {code: 'INVALID_PARAMS'});
      apply(params.snapshots, false);
      syncRefreshTimer();
      return {applied: params.snapshots.length};
    },
    ensureBot(botId) {
      void getActiveBotAutonomy()?.runtime.ensureBot(botId);
    },
  };
}
