// Cindy's teammate tools served by the OneBot worker (Cindy b7e46bc,
// Apache-2.0): routines (lizi xdt-helper botRoutineTools), teammate creation
// (create_teammate + mcp-providers botProfiles), the workbench (bot_workbench
// + maker-ipc botWorkbenchTools), Todo (botTodoAccess) and the task digest
// (get_task_digest + taskDigestHost). The original access rules, digest
// assembly, route advice and payload bounds run unchanged. WinOne supplies
// conversations through the Host bridge and reads Pi transcripts directly.
// Results are the upstream tool payloads: {ok:true,...} or
// {ok:false,errorCode,data:{hint}}; routine tools keep {ok,result|message}.
import fs from 'node:fs';
import path from 'node:path';
import {randomUUID} from 'node:crypto';
import {Database} from 'bun:sqlite';
import {createBotWorkbenchAccess} from '../cindy/bots/botWorkbenchAccess.ts';
import {readBotGlance, readSchedulerBotId, readWatchedSession} from '../cindy/bots/botWorkbenchAutonomyBridge.ts';
import {boundTranscript, lastExchange} from '../cindy/bots/botWorkbenchTranscripts.ts';
import {createTaskDigestReader, sessionFactsFromRow} from '../cindy/bots/taskDigest.ts';
import {deriveFollowUpUsage, suggestFollowUpRoute} from '../cindy/autonomy/followUpRoute.ts';
import {glanceForModel} from '../cindy/helper/botWorkbenchTools.ts';
import {boundTaskDigest} from '../cindy/helper/taskDigestTool.ts';
import {preflightTodoEvents, queryTodoItems, TodoError} from '../cindy/bots/shared/teammate-todo.ts';
import {readTaskResponsibilities, responsibleSessionIds, setTaskResponsibility} from '../cindy/bots/botTaskResponsibility.ts';
import {NEEDS_OWNER_REQUEST} from '../cindy/tasks/taskActionAuthorization.ts';
import {buildHookCommand, generateHookScript, hookScriptSlug, resolveHookScriptPath} from '../cindy/scheduler-host/hook-script-generator.ts';
import {visibleRows} from './bots-service.mjs';
import {readBranch} from './history.mjs';
import {oneBotError} from './content-service.mjs';

const okPayload = data => ({ok: true, ...data});
const errorPayload = (errorCode, hint, data = {}) => ({ok: false, errorCode, data: {...data, hint}});
/** Cindy taskActionHost TEAMMATE_STANDING_GRANT_TOOLS: owner decisions, never the teammate's own initiative. */
const STANDING_GRANT_ROUTINE_TOOLS = new Set(['save', 'delete', 'runNow', 'preRunHook']);
/** Cindy readSessionTranscript reads the latest 60 rows before bounding them. */
const TRANSCRIPT_ROWS = 60;
/** Cindy readSessionLastExchange looks back 20 rows for the last ask and answer. */
const LAST_EXCHANGE_ROWS = 20;
/** CONTENT_PREVIEW_CAP of schedule_set_pre_run_hook. */
const HOOK_CONTENT_PREVIEW_CAP = 4000;

export function createOneBotTeammateTools({owner, paths, profiles, routines, call, host, startBackground, prStatuses, event}) {
  /** Cindy resolveWorkbenchCaller: only an active teammate's main chat or private topic. */
  function resolveCaller(callerSessionId) {
    const link = typeof callerSessionId === 'string' ? profiles.sessionOwnerOrNull(callerSessionId) : null;
    if (!link) return {ok: false, errorCode: 'NOT_A_BOT_SESSION', message: '当前任务不属于任何伙伴'};
    let profile;
    try { profile = profiles.get(link.botId); } catch { return {ok: false, errorCode: 'NOT_A_BOT_SESSION', message: '当前任务不属于任何伙伴'}; }
    if (profile.status !== 'active') return {ok: false, errorCode: 'BOT_SESSION_INACTIVE', message: '伙伴或它的主任务已停用'};
    if (link.role !== 'canonical' && link.role !== 'thread') return {ok: false, errorCode: 'BOT_MAIN_TASK_REQUIRED', message: '只有伙伴的主任务可以使用工作台'};
    return {ok: true, botId: link.botId};
  }

  /** Read-only rows of the task store (the coordinator owns its writer). */
  function delegationRows(sql, ...args) {
    const file = path.join(paths.root, 'tasks.db');
    paths.assertPath(file);
    if (!fs.existsSync(file)) return [];
    const db = new Database(file, {readonly: true});
    try { return db.query(sql).all(...args); } finally { db.close(); }
  }
  /** Cindy listBotDelegationChildren: this teammate's background tasks and their state. */
  function listDelegations(botId) {
    const out = new Map();
    for (const row of delegationRows('SELECT child_session_id AS childSessionId,status,last_error AS lastError FROM bot_delegations WHERE requesting_bot_id=? AND child_session_id IS NOT NULL ORDER BY created_at DESC', botId)) {
      if (out.has(row.childSessionId)) continue;
      // Same as the background task card: a timeout ends as failed + TIMEOUT prefix.
      out.set(row.childSessionId, row.status === 'failed' && /^TIMEOUT(?:_|:)/i.test(row.lastError ?? '') ? 'timed-out' : row.status);
    }
    return out;
  }
  const responsibleIds = async botId => responsibleSessionIds(listDelegations(botId).keys(), await readTaskResponsibilities(paths.root, botId));

  async function conversation(sessionId) {
    const row = await host.taskConversation(sessionId);
    return row?.found ? row : null;
  }
  /** Visible owner/assistant rows of a Pi transcript (the rows Cindy keeps in `messages`). */
  async function transcriptItems(sessionId, limit) {
    const row = await conversation(sessionId);
    if (!row) return [];
    return visibleRows(await readBranch(row)).slice(-limit).map(item => ({role: item.role, text: item.text, at: item.at}));
  }

  const workbench = () => createBotWorkbenchAccess({
    // The Host admitted this call for the turn's principal; the access rules
    // below still decide which tasks it may read or continue.
    hasActionAuthorization: () => true,
    resolveCaller: async id => resolveCaller(id),
    schedulerBotId: readSchedulerBotId,
    readWatched: readWatchedSession,
    readTarget: async sessionId => {
      const row = await conversation(sessionId);
      if (!row) return null;
      return {id: row.id, status: row.archived ? 'archived' : 'active', source: row.store === 'onebot' ? 'bot' : 'desktop', remoteHostId: null, orcaRole: null,
        botLinked: row.store === 'onebot' || profiles.sessionOwnerOrNull(row.id) !== null, delegationChild: row.oneBotTask === true};
    },
    readGlance: readBotGlance,
    readSessionTranscript: async sessionId => boundTranscript(await transcriptItems(sessionId, TRANSCRIPT_ROWS)),
    readRemoteTranscript: async () => ({ok: false, errorCode: 'REMOTE_UNAVAILABLE', message: '另一台电脑上的任务请直接打开那件任务查看'}),
    notifyChanged: botId => event({type: 'workbench-changed', botId}),
    listDelegations: async botId => listDelegations(botId),
    readResponsibleSessionIds: responsibleIds,
    saveResponsibility: (botId, sessionId, responsible) => setTaskResponsibility(paths.root, botId, sessionId, responsible, () => {
      if (owner.closed) throw oneBotError('OWNER_SCOPE_CHANGED', 'OneBot 已关闭');
    }),
    sendToSession: async ({targetSessionId, message, dispatcherSessionId}) => host.sendToTask({sessionId: targetSessionId, message, dispatcherSessionId,
      clientId: `bot-workbench:${randomUUID()}`}),
    sendToRemoteSession: async () => ({ok: false, errorCode: 'REMOTE_UNAVAILABLE', message: '这台电脑没有开启跨电脑发送'}),
    isOwnerScopeCurrent: () => !owner.closed,
  });

  /** Cindy createDesktopTaskDigestDeps on WinOne's Chat stores (local tasks only). */
  const digest = createTaskDigestReader({
    now: Date.now,
    watched: readWatchedSession,
    local: {
      readSession: async sessionId => {
        const row = await conversation(sessionId);
        return row ? sessionFactsFromRow({title: row.title, status: row.archived ? 'archived' : 'active', source: row.store === 'onebot' ? 'bot' : 'desktop',
          agentKind: 'pi', model: row.route?.model, providerId: row.route?.providerId, effort: row.route?.effort, fastMode: row.route?.fastMode}) : null;
      },
      readActivity: async sessionId => {
        const facts = await host.taskFacts(sessionId);
        const activity = facts?.activity;
        if (!activity) return null;
        const first = activity.interactions?.[0];
        const row = await conversation(sessionId);
        return {phase: activity.phase, interactionKind: first ? first.kind === 'question' ? 'ask_user_question' : 'permission' : null,
          startedAtMs: activity.startedAtMs || null, lastActivityAtMs: activity.lastActivityAtMs || null,
          // WinOne has no set_session_runtime, so there is no generation to pass back.
          runtimeGeneration: null,
          route: row?.route ? {harness: 'pi', model: row.route.model, provider_id: row.route.providerId || null, effort: row.route.effort || null, fast: row.route.fastMode === true} : null};
      },
      readLastExchange: async sessionId => lastExchange(await transcriptItems(sessionId, LAST_EXCHANGE_ROWS)),
      // Pi keeps the input queue inside the running Chat worker; reading it
      // would start that worker, so the count stays unknown.
      queuedCount: async () => null,
      prStatus: async sessionId => prStatuses(sessionId),
      suggestRoute: async ({callerSessionId, targetSessionId, current, failure}) => {
        if (!callerSessionId) return null;
        const facts = await host.taskFacts(targetSessionId, callerSessionId);
        const chain = (facts?.chain ?? []).map(route => ({harness: 'pi', model: route.model, providerId: route.providerId || null, effort: route.effort ?? '', fastMode: route.fastMode ?? false}));
        if (!chain.length) return null;
        const windowOf = route => facts.windows?.[`${route.providerId}/${route.model}`] ?? null;
        // WinOne has no runtime usage reading for another task: only routes
        // that do not shrink the context window are suggested.
        const usage = deriveFollowUpUsage({live: null, closed: null, dbContextTokens: null});
        const next = suggestFollowUpRoute({
          chain, current: {harness: 'pi', model: current.model, providerId: current.provider_id}, failure,
          visitedRoutes: [], sameHarnessOnly: false, contextTokens: usage.contextTokens,
          currentContextWindow: windowOf({providerId: current.provider_id, model: current.model}), windowOf,
        });
        return next ? {harness: 'pi', model: next.model, provider_id: next.providerId, effort: next.effort || null, fast: next.fastMode} : null;
      },
    },
  });

  /** Cindy botTodoAccess on this teammate's own Todo file. */
  async function todos(botId, operation, input, ownerAuthored) {
    try {
      if (operation === 'list') {
        const all = await call('todos.read', {botId});
        const query = input && typeof input === 'object' && !Array.isArray(input) ? input : {};
        if (query.id || query.key) {
          const items = all.items.filter(todo => query.id ? todo.id === query.id : todo.key === query.key);
          return {ok: true, items, total: items.length};
        }
        return {ok: true, ...queryTodoItems(all.items, query)};
      }
      if (operation === 'update') {
        const todo = await call('todos.patch', {botId, patch: input, ownerAuthored});
        event({type: 'workbench-changed', botId});
        return {ok: true, todo};
      }
      if (operation === 'preflight') {
        // A folder hint never puts an event out of scope; it is echoed back unchanged.
        const events = Array.isArray(input) ? input : [];
        const unscoped = events.map(item => { if (!item) return item; const {project: _project, ...rest} = item; return rest; });
        return {ok: true, events: preflightTodoEvents(await call('todos.read', {botId}), unscoped, []).map((item, index) =>
          events[index]?.project === undefined ? item : {...item, project: events[index].project})};
      }
      const result = await call('todos.ingest', {botId, input});
      event({type: 'workbench-changed', botId});
      return {ok: true, ...result};
    } catch (error) {
      return {ok: false, errorCode: error instanceof TodoError ? error.code : error?.code ?? 'INTERNAL', message: error instanceof Error ? error.message : 'INTERNAL'};
    }
  }

  /** schedule_set_pre_run_hook in a teammate: install a script (or generate one) and self-test it. */
  async function installPreRunHook(callerSessionId, botId, args) {
    const script = typeof args.script === 'string' ? args.script.trim() : '';
    const description = typeof args.description === 'string' ? args.description.trim() : '';
    if (!script && !description) return {ok: false, message: 'invalid request: script 与 description 至少提供一个'};
    const chat = await conversation(callerSessionId);
    if (!chat?.directory) return {ok: false, message: '无法确定伙伴任务的工作目录'};
    const scheduleName = typeof args.scheduleName === 'string' ? args.scheduleName : undefined;
    const fallbackDir = path.join(paths.home(botId), 'schedule-checks');
    let written;
    if (script) {
      // Cindy installHookScript script mode: the agent wrote the check; the
      // path, command and self-test are the Host's. A teammate passes no
      // schedule, so there is no earlier script to overwrite.
      const {filePath} = resolveHookScriptPath({workingDir: chat.directory, fallbackDir, slug: hookScriptSlug(scheduleName, description || undefined), reusePath: null, exists: fs.existsSync});
      fs.mkdirSync(path.dirname(filePath), {recursive: true});
      fs.writeFileSync(filePath, script.endsWith('\n') ? script : `${script}\n`, 'utf8');
      // WinOne runs checks with its bundled runtime (hasSystemNode=false).
      written = {command: buildHookCommand(filePath, undefined, 'xdt-node'), filePath, content: script};
    } else {
      written = await generateHookScript({maker: null, fallbackDir, hasSystemNode: async () => false,
        requestText: (_maker, prompt, options) => host.auxiliaryText({prompt, maxTokens: options.maxTokens, timeoutMs: options.timeoutMs})},
      {description, scheduleName, workingDir: chat.directory});
    }
    const tested = await host.testPreRunHook({sessionId: callerSessionId, command: written.command, scheduleName});
    if (tested.error && !tested.preRunHook) return {ok: false, message: tested.error};
    return {ok: true, result: {command: written.command, filePath: written.filePath, attached: false, test: tested.preRunHook ?? tested.value,
      content: written.content.length > HOOK_CONTENT_PREVIEW_CAP ? `${written.content.slice(0, HOOK_CONTENT_PREVIEW_CAP)}…(truncated)` : written.content,
      hint: '尚未挂载:在 routine_save 的 preRunHook.command 里带上返回的 command,再用 routine_list 核对。'}};
  }

  /** botRoutineTools: the native routine service, scoped to the calling teammate. */
  async function routineTool(operation, params) {
    const caller = resolveCaller(params.callerSessionId);
    if (!caller.ok) return {ok: false, message: '当前调用未绑定伙伴主任务'};
    if (STANDING_GRANT_ROUTINE_TOOLS.has(operation) && params.principal === 'autonomous') {
      return errorPayload(NEEDS_OWNER_REQUEST.errorCode, NEEDS_OWNER_REQUEST.message);
    }
    const args = params.args ?? {};
    try {
      if (operation === 'preRunHook') return await installPreRunHook(params.callerSessionId, caller.botId, args);
      if (operation === 'list') return {ok: true, result: await routines.call('list', {botId: caller.botId})};
      if (operation === 'sources') return {ok: true, result: await routines.call('sources', {})};
      if (operation === 'save') {
        const {id, ...input} = args;
        return {ok: true, result: await routines.call('save', {botId: caller.botId, input, id})};
      }
      const method = {history: 'history', delete: 'remove', runNow: 'runNow'}[operation];
      if (!method) throw oneBotError('UNKNOWN_METHOD', '不支持的例行任务操作');
      return {ok: true, result: await routines.call(method, {botId: caller.botId, id: args.id})};
    } catch (error) {
      return {ok: false, message: error instanceof Error ? error.message : 'Routine operation failed'};
    }
  }

  async function create(params) {
    const caller = profiles.sessionOwnerOrNull(params.callerSessionId);
    if (!caller || !['canonical', 'thread'].includes(caller.role)) return errorPayload('NOT_A_BOT_SESSION', '当前调用未绑定伙伴主任务');
    const args = params.args ?? {};
    const text = (value, max) => typeof value === 'string' ? value.trim().slice(0, max) : '';
    const name = text(args.name, 200);
    if (!name) return errorPayload('INVALID_ARGS', 'name is required');
    try {
      const profile = await call('profiles.create', {input: {name, description: text(args.description, 4000), identitySource: text(args.identity_source, 12000), prepareInvitation: true}});
      void startBackground().catch(() => {});
      return okPayload({action: 'created', bot: {id: profile.id, name: profile.name, description: profile.description ?? ''}});
    } catch (error) {
      return errorPayload(error?.code === 'INVALID_PARAMS' ? 'INVALID_ARGS' : 'INTERNAL', error instanceof Error ? error.message : String(error));
    }
  }

  return {
    async call(method, params) {
      const args = params.args ?? {};
      if (method.startsWith('teammate.routines.')) return routineTool(method.slice('teammate.routines.'.length), params);
      if (method === 'teammate.create') return create(params);
      if (method.startsWith('teammate.todos.')) {
        const caller = resolveCaller(params.callerSessionId);
        if (!caller.ok) return errorPayload(caller.errorCode, caller.message);
        const operation = method.slice('teammate.todos.'.length);
        const input = operation === 'update' ? args.patch : operation === 'preflight' ? args.events : args;
        const result = await todos(caller.botId, operation, input, params.ownerAuthorized === true);
        return result.ok ? okPayload(result) : errorPayload(result.errorCode, result.message);
      }
      if (method === 'teammate.digest') {
        const caller = resolveCaller(params.callerSessionId);
        if (!caller.ok) return errorPayload(caller.errorCode, caller.message);
        const result = await digest({sessionId: String(args.session_id ?? ''), callerSessionId: params.callerSessionId});
        return result.ok ? okPayload({...boundTaskDigest(result.digest)}) : errorPayload(result.errorCode, result.message);
      }
      const access = workbench();
      let result;
      try {
        switch (method) {
          case 'teammate.workbench.get':
            result = await access.get({callerSessionId: params.callerSessionId});
            return result.ok ? okPayload({workbench: glanceForModel(result.workbench)}) : errorPayload(result.errorCode, result.message);
          case 'teammate.workbench.read':
            result = await access.read({callerSessionId: params.callerSessionId, taskId: args.task_id});
            return result.ok ? okPayload({task_id: result.taskId, transcript: result.transcript}) : errorPayload(result.errorCode, result.message);
          case 'teammate.workbench.continue':
            if (typeof args.message !== 'string' || !args.message.trim()) return errorPayload('INVALID_ARGS', 'message 不能为空。');
            result = await access.continueTask({callerSessionId: params.callerSessionId, taskId: args.task_id, message: args.message});
            return result.ok ? okPayload({task_id: result.taskId, delivery: result.delivery, ...(result.queuedMessageId ? {queued_message_id: result.queuedMessageId} : {})})
              : errorPayload(result.errorCode, result.message);
          case 'teammate.workbench.responsibility':
            // Cindy autonomousActionAllowed: a teammate may release on its own;
            // taking responsibility needs the owner's request in this turn.
            if (args.responsible === true && params.principal === 'autonomous') return errorPayload(NEEDS_OWNER_REQUEST.errorCode, NEEDS_OWNER_REQUEST.message);
            result = await access.setResponsibility({callerSessionId: params.callerSessionId, taskId: args.task_id, responsible: args.responsible});
            return result.ok ? okPayload({task_id: result.taskId, responsible: result.responsible}) : errorPayload(result.errorCode, result.message);
          default:
            throw oneBotError('UNKNOWN_METHOD', '不支持的伙伴工具');
        }
      } catch (error) {
        if (error?.code === 'UNKNOWN_METHOD') throw error;
        return errorPayload('INTERNAL', error instanceof Error ? error.message : String(error));
      }
    },
    /** The owner's own workbench view of one task (Cindy readBotWorkbenchTaskForOwner). */
    readForOwner: (botId, taskId) => workbench().readForOwner({botId, taskId}),
  };
}
