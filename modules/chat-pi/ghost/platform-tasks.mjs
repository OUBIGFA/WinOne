// Plugins acting on Chat: new turns, errands, visible tasks, workspaces, routines
// and OneBot companions. The slots keep their tickets and limits; each operation
// the Host performs is one named request, answered from WinOne's real conversations.
import fs from 'node:fs';
import path from 'node:path';
import {handlePluginTaskRequest} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/taskSlot.ts';
import {handlePluginObserveRequest} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/pluginObserveSlot.ts';
import {handlePluginCompanionRequest, createCompanionWatchHub} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/pluginCompanionSlot.ts';
import {hasPluginCompanionsApproval, hasPluginObserveApproval, hasPluginTargetApproval} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/taskCapability.ts';
import {handleRoutineRequest} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/routineSlot.ts';
import {GhostWorkspaceSlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/workspaceSlot.ts';
import {createGhostTargets} from './platform-targets.mjs';
import {createGhostCompanions} from './platform-companions.mjs';
import {createGhostTaskService} from './platform-task-service.mjs';
import {isPathInsideDir} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/dirDeposit.ts';
import {ghostInstallApprovalToken} from '../cindy/ghost/apps/desktop/src/shared/ghost.ts';
import {isGhostPickedDir} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/pickGrantsStore.ts';
import {installedGhostStoragePart} from '../cindy/ghost/apps/desktop/src/shared/pluginIdentity.ts';
import {readGhostRecommendationEntries,replaceGhostRecommendations} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostRecommendationStore.ts';
import {buildGhostRecommendationSnapshot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostRecommendationSnapshot.ts';
import {clampPluginTaskPermissionMode} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/pluginTaskPrefsStore.ts';

const denied = () => ({ok: false, error: {code: 'PERMISSION_DENIED', message: 'PERMISSION_DENIED', retryable: false}});

export function createGhostTasks({root, find, current, isRunning, sendToGhost, manager, host, push, ui, agent, errand, cards, kv, log, preferences}) {
  let focused = null;
  const guardFor = (id, approved) => {
    const captured = find(id);
    return () => {
      if (!captured || !current(captured) || !approved(find(id))) throw Object.assign(Error('PERMISSION_DENIED'), {code: 'PERMISSION_DENIED'});
    };
  };
  // Use Cindy's actual throttled subscription hub, including its idle teardown.
  const companionHub = createCompanionWatchHub({send: sendToGhost, approved: id => hasPluginCompanionsApproval(find(id)) && isRunning(id)});
  // Migrate only our known prototype recommendation file; the original store
  // is the sole writer now and the old bytes remain available for recovery.
  const recommendationsFile = path.join(root, 'recommendations.json');
  if(fs.existsSync(recommendationsFile)){
    const saved=JSON.parse(fs.readFileSync(recommendationsFile,'utf8')),existing=new Set(readGhostRecommendationEntries().map(row=>row.id));
    for(const [id,items] of Object.entries(saved))if(!existing.has(id))replaceGhostRecommendations(id,items);
  }
  // A card click is revealed exactly once by the original Agent Slot. The
  // runner must not also reveal `new`: background requests cannot steal focus.
  agent.setRunner(request => host('task', {op: 'agent-turn', request}));
  function capturePreferences(id, shouldContinue) {
    const ghost = find(id), config = {...preferences?.read?.(id)};
    const signature = JSON.stringify(config);
    return {config, assertCurrent() {
      if (!ghost || !current(ghost) || shouldContinue && !shouldContinue()) {
        throw Object.assign(Error('插件或发起任务已变化，请重新发起任务'), {code: 'PERMISSION_DENIED'});
      }
      if (JSON.stringify({...preferences?.read?.(id)}) !== signature) throw Error('插件任务设置已变化，请重新发起任务');
    }};
  }
  errand.setRunner(async (request, hooks) => {
    const captured = capturePreferences(request.ghostId);
    // The configured directory is a user-owned Host setting. Only a directory
    // merely relayed by plugin prose needs the picker ledger; it cannot replace
    // the user's setting. The native adapter resolves against an existing
    // errand's route before validating, so a user-changed model is preserved.
    const workingDir = captured.config.workingDir ?? request.workingDir;
    if (!captured.config.workingDir && workingDir && !isGhostPickedDir(request.ghostId, workingDir)) {
      return {ok: false, errorCode: 'INVALID_REQUEST', message: 'workingDir 不在该插件的用户亲选目录台账中'};
    }
    captured.assertCurrent();
    const started = await host('task', {op: 'errand-start', request: {
      ...request, taskConfig: captured.config,
      permissionMode: clampPluginTaskPermissionMode(captured.config.permissionMode, 'plan'),
      ...(workingDir ? {workingDir} : {}),
    }});
    if (!started.ok) return started;
    hooks?.onSession?.(started.sessionId);
    hooks?.onDispatched?.(started.sessionId);
    return host('task', {op: 'errand-wait', request: {sessionId: started.sessionId, requestId: started.requestId}});
  });
  const reveal = sessionId => {
    // The Host knows whether a linked task belongs to Chat or a local companion;
    // a bare UI push would open a OneBot-owned session in the wrong page.
    void host('task', {op: 'reveal-session', taskId: sessionId}).catch(error => log.warn(error.message));
  };
  agent.setRevealSession?.(reveal);
  errand.setRevealSession?.(reveal);
  const workspace = new GhostWorkspaceSlot({
    getGhost: find, getMutationTarget: id => { const ghost = find(id); return ghost ? ghostInstallApprovalToken(ghost.approval) : null; },
    showDirectoryDialog: async ({ghostName, purpose}) => (await ui('pick-directory', {ghostName, purpose}))?.path ?? null,
    resolveCallContext: callId => cards.inFlightCallInfoOf(callId),
    getSessionDirInfo: async sessionId => { const snapshot = await host('session-snapshot', {sessionId}); return snapshot ? {workingDir: snapshot.workingDir, remoteHostId: null} : null; },
    statDir: async dirAbs => { try { return (await fs.promises.stat(dirAbs)).isDirectory() ? 'ok' : 'not-directory'; } catch (error) { return error.code === 'ENOTDIR' ? 'not-directory' : 'not-found'; } },
    isInsideWorkdir: (dirAbs, workdirAbs) => { try { return isPathInsideDir(fs.realpathSync.native(workdirAbs), fs.realpathSync.native(dirAbs)); } catch { return false; } },
    confirmDir: async ({ghostId, sessionId, dirAbs}) => {
      const decision = await ui('grant', {sessionId, ghostId, ghostName: find(ghostId)?.manifest.name ?? ghostId, lane: 'workspace', items: [{name: path.basename(dirAbs), absPath: dirAbs, size: 0, isDirectory: true}]});
      return decision?.confirmed ? {ok: true} : {ok: false, message: '用户拒绝了本次工作区会话请求,不要重试;如确有需要请先与用户沟通'};
    },
    log,
  });
  workspace.setSessionService({
    findActiveSessionByWorkdir: async dirAbs => (await host('task', {op: 'find-by-workdir', dirAbs})).sessionId ?? null,
    createDraftSession: async ({dirAbs, title, ghostId, sourceSessionId, shouldContinue}) => {
      const captured = capturePreferences(ghostId, shouldContinue);
      const route = preferences?.resolve ? await preferences.resolve(captured.config, sourceSessionId) : undefined;
      // `dirAbs` already passed the original workspace Slot's live-call or
      // native-picker grant. An unrelated saved default must not block it.
      captured.assertCurrent();
      const result = await host('task', {op: 'create-draft', dirAbs, title, ghostId, sourceSessionId,
        ...(route ? {route} : {}), permissionMode: clampPluginTaskPermissionMode(captured.config.permissionMode)});
      return result.sessionId ?? null;
    },
    focusSession: reveal,
  });
  const taskService = createGhostTaskService({root, find, current, host, ui, cards, log, preferences});
  const targets = createGhostTargets({find, current, host, taskService, log});
  const taskHandler = async (pluginId, request) => {
    try { return await taskService.handle(pluginId, request); }
    catch (error) {
      // Keep a bounded diagnostic, never the request payload or SQL parameters.
      log.warn(`Plugin task ${request.kind}: ${error.name}: ${String(error.message).split('\n')[0].slice(0, 256)}`);
      throw error;
    }
  };
  const observe = {
    projectTasks: project => host('task', {op: 'project-tasks', project}),
    openTask: async (taskId, guard) => { guard(); await host('task', {op: 'open-task', taskId}); },
  };
  const companion = createGhostCompanions({host, ui, canAnswer: id => hasPluginTargetApproval(find(id))});
  return {
    focused: () => focused,
    setFocused(value) { focused = value; },
    workspace: (id, message, isCurrent) => workspace.handleRequest(id, message, isCurrent),
    tasks: (id, message) => message.type === 'targets-request' ? targets.handle(id, message) : handlePluginTaskRequest(id, message, {getGhost: find, handler: taskHandler, isCurrent: () => !!find(id)?.enabled}),
    observe: (id, message) => hasPluginObserveApproval(find(id)) ? handlePluginObserveRequest(observe, message, guardFor(id, hasPluginObserveApproval)) : denied(),
    companions: (id, message) => hasPluginCompanionsApproval(find(id)) ? handlePluginCompanionRequest(companion, companionHub, message, {ghostId: id, assertAuthorized: guardFor(id, hasPluginCompanionsApproval)}) : denied(),
    routine: (ghost, message) => handleRoutineRequest(ghost, message, async () => ({
      registerSource: source => host('task', {op: 'routine-source', source}),
      publish: (sourceId, event) => host('task', {op: 'routine-publish', sourceId, event}),
    }), () => current(ghost)),
    recommendations: async (id,items)=>{
      const ghost=find(id);if(!ghost||!current(ghost))return denied();
      const result=replaceGhostRecommendations(id,items);
      if(result.ok)push('recommendations',{ghostId:id,items:readGhostRecommendationEntries().find(row=>row.id===id)?.items??[]});
      return result;
    },
    // Cindy's home page offers these; a plugin's runtime list replaces its manifest's.
    recommendationList: language=>{
      const snapshot=buildGhostRecommendationSnapshot('local',manager.list(),readGhostRecommendationEntries(),[]),key=language==='en-US'?'en':'zh-CN';
      return snapshot.sources.filter(row=>row.enabled).flatMap(row=>(row.items??[]).map(item=>({pluginId:row.ghostId,id:item.id,
        label:item.locales?.[key]?.label??item.locales?.en?.label??item.label,prompt:item.locales?.[key]?.prompt??item.locales?.en?.prompt??item.prompt}))).slice(0,12);
    },
    hostEvent: () => { companionHub.revalidate(); companionHub.invalidate(); return {ok: true}; },
    runtimeStateChanged() { companionHub.revalidate(); },
    preferenceDetails: id => taskService.preferenceDetails(id),
    validatePreferences: (id, value) => taskService.validatePreferences(id, value),
    writePreferences: (id, value) => taskService.writePreferences(id, value),
    database: () => taskService.database(),
    admit: params => taskService.admit(params),
    admitTarget: params => targets.admit(params),
    reviewTarget: params => targets.review(params),
    retryWriteAccess: params => taskService.retryWriteAccess(params.pluginId,params.taskId),
    teamOperation: params => taskService.teamOperation(params),
    worktreeReadiness: params => taskService.worktreeReadiness(params),
    refs: params => taskService.refs(params),
    uninstall: (id, remove) => taskService.uninstall(id, remove),
    async close() { companionHub.dispose(); await targets.close(); await taskService.close(); },
  };
}
