// The local Cindy plugin platform: the vendored slots hold every rule (who may ask
// for what, limits, confirmation, cancellation); this file only joins them to
// WinOne's Host, the way Cindy's cindy-brain/index.ts joins them to Electron.
// Anything Cindy keeps in its cloud, on other devices or in mobile pages stays out.
import fs from 'node:fs';
import path from 'node:path';
import {randomUUID} from 'node:crypto';
import {AsyncLocalStorage} from 'node:async_hooks';
import {GhostPipeDispatcher, toolNotFoundMessage} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/pipeDispatcher.ts';
import {GhostCardService, withCardToken} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/cardService.ts';
import {sanitizeGhostCardHtml} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/cardSanitizer.ts';
import {GhostCardActionDispatcher} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/cardActionDispatch.ts';
import {classifyGhostVisibility, classifyInstalledGhostVisibility} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostVisibility.ts';
import {readInstalledGhostManual} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostManual.ts';
import {evaluateGhostSetupAssessment} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostSetupStatus.ts';
import {GhostSetupChangeBus} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostSetupChangeBus.ts';
import {GhostFsSlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/fsSlot.ts';
import {GhostNotifySlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/notifySlot.ts';
import {GhostBadgeSlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/badgeSlot.ts';
import {GhostConfirmSlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/confirmSlot.ts';
import {GhostPickSlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/pickSlot.ts';
import {GhostPreviewSlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/previewSlot.ts';
import {GhostTaskViewSlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/taskViewSlot.ts';
import {GhostScheduleSlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/scheduleSlot.ts';
import {GhostAgentSlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/agentSlot.ts';
import {GhostErrandSlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/errandSlot.ts';
import {getDirDepositVault, getSaveDepositVault, isPathInsideDir} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/dirDeposit.ts';
import * as blobStore from '../cindy/ghost/apps/desktop/src/main/cindy-media/blobStore.ts';
import {sniffMediaMime} from '../cindy/ghost/apps/desktop/src/main/cindy-media/sniffMediaMime.ts';
import {deriveGhostSessionContext, ghostInstallApprovalToken, GHOST_PIPE_CALL_MAX_TOTAL_MS} from '../cindy/ghost/apps/desktop/src/shared/ghost.ts';
import {installedGhostStoragePart, installedGhostLogicalIdentity, deliveryNamespaceFields, pluginStoragePart} from '../cindy/ghost/apps/desktop/src/shared/pluginIdentity.ts';
import {expandGhostCommand} from '../cindy/ghost/packages/maker-shared/src/ghostCommand.ts';
import {pluginToolVisible} from '../cindy/ghost/packages/plugin-protocol/src/index.ts';
import {setGhostDataRoot} from './seams/app-session.ts';
import {setGhostLanguage} from './seams/i18n.ts';
import {configureGhostElectron} from './seams/electron.ts';
import {createGhostEvents} from './platform-events.mjs';
import {nativeCard,cardAttribute} from './platform-cards.mjs';
import {ghostCardLinkOpensDirectly} from '../cindy/ghost/packages/maker-shared/src/ghostCardForm.ts';
import {ghostCardRootCallId,ghostCardDeclaresModelSelect} from '../cindy/ghost/apps/desktop/src/shared/ghost.ts';
import {createGhostPreferences} from './platform-preferences.mjs';
import {createGhostMediaStore} from './platform-media.mjs';
import {configureGhostDatabase} from './seams/local-db.ts';
import {setGhostContextReader,createGhostPageContext} from './context.mjs';
import {isGhostDisabledForWorkdir,listDisabledGhostIdsForWorkdir,setGhostDisabledForWorkdir} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostWorkdirPrefs.ts';
import {recordGhostPickedDir} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/pickGrantsStore.ts';
import {drainGhostCallMedia} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostMediaLedger.ts';
import {createGhostLibrary} from './platform-library.mjs';
import {createGhostNode} from './platform-node.mjs';
import {createGhostAccounts} from './platform-accounts.mjs';
import {createGhostModels} from './platform-models.mjs';
import {createGhostTasks} from './platform-tasks.mjs';
import {createGhostForge} from './platform-forge.mjs';
import {createGhostBrowserExtension} from './platform-browser-extension.mjs';
import {createGhostMail} from './platform-mail.mjs';
import {nativeMailUi} from './platform-mail-ui.mjs';
import {prepareMailAttachmentSelection} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/mailAttachmentUpload.ts';
import {createGhostHandoff} from './platform-handoff.mjs';
import {createGhostMcpApps} from './platform-mcpapps.mjs';
import {createGhostConnections} from './platform-connections.mjs';
import {createNativeAppMessageSender} from './platform-app-messages.mjs';
import {pluginAppServerId} from '../cindy/ghost/apps/desktop/src/main/mcp-integrations/plugin-app-registry.ts';
import {pluginAppResult} from '../cindy/ghost/apps/desktop/src/main/mcp-apps/pluginCards.ts';
import {PluginTaskApprovalGate} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/taskCapability.ts';
import {pluginHasToolUi} from '../cindy/ghost/packages/plugin-protocol/src/index.ts';
import {ghostPermissionItems} from '../cindy/ghost/apps/desktop/src/shared/ghost.ts';
import {pluginUiPromptDeclined,declinePluginUiPrompt} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/pluginUiPreferences.ts';
import {createGhostPanelMedia} from './platform-panel-media.mjs';
import {loadGhostUnread,readGhostUnread,markGhostUnread,clearGhostUnread} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostUnreadStore.ts';
import {configureWorktreeRecoverySecrets} from '../onebot/worktree-host.mjs';

const failure = (code, message) => Object.assign(Error(message), {code});
const IMAGE_MEDIA = /\.(png|jpe?g|gif|webp|mp4|webm|mov|mp3|wav|m4a|ogg|glb)$/i;

export function createGhostPlatform({root, manager, runtime, adapter, gateway, network, kv, secrets, secretName, find, current, host, event, log, runtimeExecutable, nodeWorker, browserExtensionOptions, ownerScope}) {
  setGhostDataRoot(root);
  const clearWorktreeSecrets=configureWorktreeRecoverySecrets(root,secrets);
  configureGhostElectron({runtime: runtimeExecutable, nodeWorker, openExternal: url => host('open-external', {url})});
  const preferences = createGhostPreferences({root, find, host});
  const push = (channel, payload) => event({type: 'push', channel, payload});
  const lifecycle = createGhostEvents({root, gateway, push, log});
  const ui = (kind, payload) => host('ui', {kind, ...payload});
  const showSave = options => ui('save', options);
  const findEnabled = id => { const ghost = find(id); return ghost?.enabled ? ghost : null; };
  // Lazy callbacks share the owner ledger once platform initialization ends.
  const mediaStore = createGhostMediaStore({database: () => tasks.database(), find, current, callInfo: callId => cards.inFlightCallInfoOf(callId)});
  const grantMedia = (id, hash) => mediaStore.grant(id, hash);
  const sendToGhost = (id, payload) => { try { adapter.send(id, payload); return true; } catch { return false; } };
  const visibility = {
    listGhosts: () => manager.list(), findInstalledInstance: id => manager.findInstalledInstance(id) ?? find(id),
    isAvailableForActiveSession: () => true, isDisabledForWorkdir: isGhostDisabledForWorkdir,
  };

  const dispatcher = new GhostPipeDispatcher({getGhost: (id, namespace) => find(id) ?? manager.list().find(ghost => ghost.manifest.id === id && (namespace === undefined || ghost.namespace === namespace)) ?? null,
    runtimeStateOf: id => runtime.stateOf(id), spawn: async ghost => {
      const result=await runtime.spawn(ghost);
      if(!result.ok)return {ok:false,reason:result.reason};
      // spawn may accept an already-starting instance; only SDK readiness
      // permits the dispatcher to send its first business call.
      return await runtime.waitUntilRunning(installedGhostStoragePart(ghost))?{ok:true}:{ok:false,reason:'入口加载失败'};
    },
    sendToGhost, ownerScope, log});

  const persistCard=async card=>{const file=path.join(root,'cards',card.callId.replace(/[^\w-]/g,'_')+'.json');await fs.promises.mkdir(path.dirname(file),{recursive:true,mode:0o700});await fs.promises.writeFile(file,JSON.stringify(card));return true;};
  const listCards=async sessionId=>{
    let names;try{names=await fs.promises.readdir(path.join(root,'cards'));}catch(error){if(error.code==='ENOENT')return [];throw error;}
    const cards=await Promise.all(names.filter(name=>name.endsWith('.json')).map(async name=>JSON.parse(await fs.promises.readFile(path.join(root,'cards',name),'utf8'))));
    return cards.filter(card=>card.sessionId===sessionId).sort((a,b)=>b.updatedAt-a.updatedAt);
  };
  const cards = new GhostCardService({
    hasCardSlot: id => { const ghost = find(id); return !!ghost?.enabled && ghost.manifest.card !== undefined; },
    sanitize: sanitizeGhostCardHtml,
    persist:persistCard,
    broadcast: payload => push('card', nativeCard(payload)), log,
  });
  const persistedCard = callId => { try { return JSON.parse(fs.readFileSync(path.join(root, 'cards', callId.replace(/[^\w-]/g, '_') + '.json'), 'utf8')); } catch { return null; } };

  const changeBus = new GhostSetupChangeBus();
  const accounts = createGhostAccounts({root, secrets, secretName, find, host, ui, kv, changeBus, log, ownerScope});
  const fsScope=new AsyncLocalStorage(),pageCalls=new Map();
  const fileSlot = new GhostFsSlot({getGhost: find, dataRootDir: () => path.join(root, 'ghost-fs'),
    callInfo: callId => cards.callInfoOf(callId), inFlightCallInfo: callId => cards.inFlightCallInfoOf(callId),
    getSessionSnapshot: async(sessionId,sessionInstanceId)=>{
      const captured=fsScope.getStore(),ghost=captured?find(captured.id):null;
      const authorized=()=>{
        const info=captured?cards.inFlightCallInfoOf(captured.callId):null;
        return !!ghost&&current(ghost)&&info?.ghostId===captured.id&&info.sessionId===sessionId&&info.sessionInstanceId===sessionInstanceId;
      };
      if(!authorized())return null;
      const grant=pageCalls.get(captured.callId);
      if(grant)await grant.assertCurrent();
      const value=await host('session-snapshot',{sessionId,sessionInstanceId,live:true});
      if(!value||!authorized())return null;
      if(grant)await grant.assertCurrent();
      return {...value,permissionMode:value.permissionMode==='bypassPermissions'?'auto':value.permissionMode,isCurrent:authorized,
        // Full access still confirms writes to WinOne control files, exactly
        // like native write/edit. The Pi reviewer returns allow immediately for
        // its other Full-access writes; it does not fabricate model review.
        reviewAction:action=>host('task',{op:'file-review',sessionId,instanceId:value.instanceId,requestId:value.requestId,execution:value.execution,action})};
    },
    requestWriteConfirm: async (sessionId, payload) => ui('grant', {sessionId, ...payload}),
    writeSaveDeposit: (id, token, name, bytes) => getSaveDepositVault().write(id, token, name, bytes), log});
  const library = createGhostLibrary({root, find, current, sendToGhost, showSave, host, log, mediaStore, stopNode: id => node.stop(id)});
  const models = createGhostModels({find, host, log, dispatcher, mediaStore, preferences, sendToGhost});
  const node = createGhostNode({find, dispatcher, secrets, secretName, accounts, network, sendToGhost, ownerScope, log, ui, changeBus});
  const notify = new GhostNotifySlot({getGhost: find, broadcast: payload => push('notify', payload), log});
  const badge = new GhostBadgeSlot({getGhost: find,
    mark:(id,summary,at)=>{
      try{const result=markGhostUnread(id,summary,at);for(const evicted of result.evicted)push('badge',{ghostId:evicted,unread:false});return true;}
      catch(error){log.warn('Plugin unread status was not persisted: '+error.code);return false;}
    },
    clear:id=>clearGhostUnread(id)!==null,broadcast:payload=>push('badge',payload),log});
  const confirm = new GhostConfirmSlot({getGhost: find, showConfirm: async params => (await ui('confirm', {ghostId: params.ghostId, ghostName: params.ghostName, body: params.body, confirmText: params.confirmText, cancelText: params.cancelText, danger: params.danger}))?.confirmed === true, log});
  const pick = new GhostPickSlot({
    getPendingCallSignal: (id, callId) => dispatcher.getPendingCallSignal(id, callId), holdCall: (id, callId, budget) => dispatcher.holdCall(id, callId, budget), releaseCall: (id, callId) => dispatcher.releaseCall(id, callId),
    getGhost: find, getMutationTarget: id => { const ghost = find(id); return ghost ? ghostInstallApprovalToken(ghost.approval) : null; },
    showDirectoryDialog: async ({ghostName, purpose}) => { const picked = await ui('pick-directory', {ghostName, purpose}); return picked?.path ? fs.promises.realpath(picked.path) : null; },
    showFileDialog: async ({ghostName, purpose, multiple, accept}) => { const picked = await ui('pick-files', {ghostName, purpose, multiple, accept}); return picked?.paths?.length ? Promise.all(picked.paths.map(file => fs.promises.realpath(file))) : null; },
    importFiles: library.importPicked(grantMedia),
    depositDir: (id, dirAbs) => getDirDepositVault().deposit({ghostId: id, dirAbs, workdirAbs: null, userGranted: true}),
    recordPickedDir: recordGhostPickedDir, log});
  const preview = new GhostPreviewSlot({getGhost: find, focusedSessionId: () => tasks.focused(), broadcast: payload => { push('preview', payload); return true; }, log});
  const taskView = new GhostTaskViewSlot({getGhost: find, focusedSessionId: () => tasks.focused(), consumeUserGesture: id => errand.consumeUserGesture(id), broadcast: payload => { push('task-view', payload); return true; }, log});
  const schedule = new GhostScheduleSlot({getGhost: find, sendToWindow: payload => { push('schedule-draft', payload); return true; }, log});
  const agent = new GhostAgentSlot({getGhost: find, log});
  const errand = new GhostErrandSlot({resolveSourceSessionId: (id, callId) => cards.callInfoOf(callId)?.sessionId ?? null, getGhost: find, holdPipeCall: (id, callId, budget) => dispatcher.holdCall(id, callId, budget), releasePipeCall: (id, callId) => dispatcher.releaseCall(id, callId), consumeUserActionToken: (token, id) => agent.consumeUserActionToken(token, id), log});
  const tasks = createGhostTasks({root, find, current, isRunning: id => runtime.stateOf(id) === 'running', sendToGhost, manager, host, push, ui, agent, errand, cards, kv, log, preferences});
  const pageContext=createGhostPageContext({find,current,host,ui,log,noteUserGesture:id=>errand.noteUserGesture(id),previewNavigation:params=>panelMedia.navigation(params)});
  const panelMedia=createGhostPanelMedia({manager,find,current,adapter,runtime,database:()=>tasks.database(),pageBinding:id=>pageContext.binding(id),push});
  taskView.setRevealSession(sessionId => { void host('task', {op: 'reveal-session', taskId: sessionId}).catch(error => log.warn(error.message)); });
  const clearDatabase = configureGhostDatabase(() => tasks.database());
  setGhostContextReader(async sessionId => (await host('session-snapshot', {sessionId}))?.model);
  const browserExtension=createGhostBrowserExtension({root,manager,find,current,ownerScope,sendToGhost,runtime,ui,log,nativeOptions:browserExtensionOptions?.nativeOptions??{},createBridge:browserExtensionOptions?.createBridge});
  const mail=createGhostMail({manager,find,current,ownerScope,host,cards,dispatcher,read:persistedCard,list:listCards,persist:persistCard,push:(channel,card)=>push(channel,nativeCard(card)),assessment});
  const cardActions = new GhostCardActionDispatcher({
    resolveLiveInfo: callId => cards.callInfoOf(callId),
    resolvePersistedCard: async callId => { const card = persistedCard(callId); return card ? {ghostId: card.ghostId, sessionId: card.sessionId ?? null, html: card.html} : null; },
    reopenForAction: (callId, info) => cards.reopenForAction(callId, info), getGhost: find,
    isRunning: id => runtime.stateOf(id) === 'running',
    wake: async ghost => { const result=await runtime.spawn(ghost);if(!result.ok||!await runtime.waitUntilRunning(installedGhostStoragePart(ghost)))throw Error(result.reason??'入口加载失败'); },
    sendToGhost: (id, payload) => { adapter.send(id, payload); },
    isModelRouteAvailable: async route => route.agentKind === 'pi' && (await host('models', {})).some(model => model.providerId === route.providerId && model.id === route.model),
    issueUserActionToken: (id, sessionId) => agent.issueUserActionToken(id, sessionId),
    pickMailAttachment:async(inline,sessionId)=>{
      const choice=await ui('pick-files',{purpose:inline?'选择邮件内嵌图片':'选择邮件附件',multiple:false,accept:inline?['.png','.jpg','.jpeg','.gif','.webp']:[]});
      if(!choice?.paths?.[0])return null;
      return prepareMailAttachmentSelection(choice.paths[0],sessionId,inline,async(id,bytes)=>{
        const directory=path.join(root,'mail-staging');await fs.promises.mkdir(directory,{recursive:true,mode:0o700});
        const file=path.join(directory,randomUUID());await fs.promises.writeFile(file,bytes,{flag:'wx',mode:0o600});
        try{return (await host('mail-attachment-snapshot',{sessionId:id,path:file})).path;}finally{await fs.promises.rm(file,{force:true});}
      });
    },
    onActivityStart: () => {}, now: () => Date.now(), ownerScope, log});
  const connections=createGhostConnections({root,find,current,visibility,assessment,changeBus,accounts,secrets,secretName,ownerScope,host,push,log});
  const handoff=createGhostHandoff({root,find,current,ownerScope,host});
  const appMessages=createNativeAppMessageSender({host});
  const pageAuthorizations=new Map(),uiApproval=new PluginTaskApprovalGate();
  const pageKey=(sessionId,clientId,tool,args)=>JSON.stringify([sessionId,clientId,tool,args]);
  const mcpApps=createGhostMcpApps({root,manager,find,current,ownerScope,host,push,log,handoff,resolveLibraryPath:library.resolvePanelFilePath,
    authorizeTool:async(sessionId,tool,args,signal,source)=>{
      const ghost=manager.list().find(row=>pluginAppServerId(row)===source?.serverId);
      if(!ghost||!current(ghost))throw failure('MCP_APP_SERVER_CHANGED','插件已不可用');
      const scope=await host('task',{op:'tool-scope',sessionId},signal);
      try{
        const response=await host('task',{op:'tool-authorize',sessionId,scopeId:scope.scopeId,request:{tool:'ghost_call',args:{ghost_id:ghost.manifest.id,namespace:ghost.namespace??null,tool:tool.name,args}}},signal);
        if(response?.authorized!==true)throw failure('PERMISSION_DENIED','操作未获允许');
        const key=pageKey(sessionId,source.clientId,tool.name,args),rows=pageAuthorizations.get(key)??[];
        scope.instance=installedGhostStoragePart(ghost);scope.approval=ghostInstallApprovalToken(ghost.approval);
        scope.cancel=()=>{const values=pageAuthorizations.get(key);const index=values?.indexOf(scope)??-1;if(index>=0)values.splice(index,1);if(values&&!values.length)pageAuthorizations.delete(key);void host('task',{op:'tool-release',scopeId:scope.scopeId}).catch(error=>log.warn(error.message));};
        signal.addEventListener('abort',scope.cancel,{once:true});scope.signal=signal;rows.push(scope);pageAuthorizations.set(key,rows);
        if(signal.aborted)scope.cancel();
      }catch(error){await host('task',{op:'tool-release',scopeId:scope.scopeId});throw error;}
    },
    runPageTool:async(grant,args,signal)=>{
      const key=pageKey(grant.sessionId,grant.clientId,grant.tool,args),rows=pageAuthorizations.get(key),scope=rows?.shift();
      if(rows&&!rows.length)pageAuthorizations.delete(key);
      if(!scope)throw failure('MCP_APP_CALL_NOT_AUTHORIZED','操作没有当前批准');
      try{
        await grant.assertCurrent();
        const ghost=find(grant.instance);if(!ghost||!current(ghost)||scope.instance!==grant.instance||scope.approval!==ghostInstallApprovalToken(ghost.approval))throw failure('PLUGIN_CHANGED','插件已变化');
        const snapshot=await host('session-snapshot',{sessionId:grant.sessionId});
        if(snapshot.permissionGeneration!==scope.revision)throw failure('STALE_REVISION','任务已变化');
        const result=await callTool({ghostId:ghost.manifest.id,namespace:ghost.namespace,tool:grant.tool,args,
          session:{id:grant.sessionId,workingDir:scope.workingDir,instanceId:scope.instanceId,readOnly:scope.planModeEnabled,approved:true}},signal,grant);
        if(!result.ok)return {isError:true,content:[{type:'text',text:result.message??result.errorCode}]};
        return pluginAppResult(result.result);
      }finally{scope.signal.removeEventListener('abort',scope.cancel);await host('task',{op:'tool-release',scopeId:scope.scopeId});}
    },sendMessage:appMessages.send});
  const forge = createGhostForge({root, manager, host, log});

  async function approveToolUi(target,explicit=false){
    if(!pluginHasToolUi(target.manifest)||target.uiCapabilityApproved===true)return true;
    const id=installedGhostStoragePart(target),revision=target.approval.revision,version=target.manifest.version;
    if(!explicit&&pluginUiPromptDeclined(id,version))return false;
    return uiApproval.request(id,ghostInstallApprovalToken(target.approval),explicit,async()=>{
      const response=await ui('mcp-ui-approval',{ghostName:target.manifest.name,version,permissions:ghostPermissionItems(target.manifest).filter(item=>item.key==='ui:pages')});
      if(response?.confirmed!==true){declinePluginUiPrompt(id,version);return false;}
      return manager.approveUiCapability(id,revision,()=>current(target));
    });
  }
  function assessment(id) {
    const ghost = find(id);
    if (!ghost) throw failure('NOT_FOUND', '插件不存在');
    return evaluateGhostSetupAssessment(ghost.manifest, accounts.probes(id, ghost.manifest), {revision: changeBus.currentRevision(id), strict: true, additionalGroups: []});
  }
  function info(ghost) {
    let setup;
    try { setup = assessment(installedGhostStoragePart(ghost)); } catch (error) { log.warn('plugin setup assessment omitted: ' + error.message); }
    return {id: ghost.manifest.id, ...(ghost.namespaceState === 'pending' ? {} : deliveryNamespaceFields(ghost)), name: ghost.manifest.name,
      ...(ghost.manifest.command ? {command: ghost.manifest.command} : {}), ...(ghost.manifest.recall ? {recall: ghost.manifest.recall} : {}),
      ...(ghost.manifest.manual ? {manual: ghost.manifest.manual.items.map(({name, description}) => ({name, description}))} : {}),
      ...(setup ? {setup} : {}),
      tools: (ghost.manifest.tools ?? []).filter(tool => pluginToolVisible(ghost.manifest, tool.name, 'model')).map(tool => ({name: tool.name, description: tool.description, ...(tool.parameters ? {parameters: tool.parameters} : {})}))};
  }
  const agentVisible = ghost => ghost.enabled && ghost.manifest.kind === 'chip' && ((ghost.manifest.tools ?? []).some(tool => pluginToolVisible(ghost.manifest, tool.name, 'model')) || !!ghost.manifest.manual?.items?.length);

  async function grantAttachments(id, urls, workdir, approved, sessionId) {
    const hashes = [];
    for (const url of urls) {
      if (/^(cindy-media|xdt-image|xdt-video):/.test(url)) {
        const parsed = blobStore.parseBlobUrl(url);
        if (!parsed) throw failure('ATTACHMENT_INVALID', '附件地址无效');
        if (!await mediaStore.canHandOff(id, parsed.hash, sessionId)) throw failure('ATTACHMENT_INVALID', '该媒体没有当前会话的授权记录，请通过文件选择器交接');
        await mediaStore.grant(id, parsed.hash, sessionId); hashes.push(parsed.hash); continue;
      }
      if (!path.isAbsolute(url) && !workdir) throw failure('ATTACHMENT_INVALID', '附件路径需要是绝对路径');
      const file = await fs.promises.realpath(path.isAbsolute(url) ? url : path.join(workdir, url)).catch(() => null);
      if (!file) throw failure('ATTACHMENT_INVALID', '附件不存在：' + path.basename(url));
      if (!approved && !(workdir && isPathInsideDir(await fs.promises.realpath(workdir), file))) throw failure('ATTACHMENT_INVALID', '工作目录外的文件需要用户批准');
      const handle = await fs.promises.open(file, 'r');
      let mimeType;
      try { const head = Buffer.alloc(4096); const {bytesRead} = await handle.read(head, 0, 4096, 0); mimeType = sniffMediaMime(head.subarray(0, bytesRead)); } finally { await handle.close(); }
      if (!mimeType || !blobStore.supportedMime(mimeType)) throw failure('ATTACHMENT_INVALID', '只有图片、音视频可以作为附件；其它文件请用 dir');
      const written = await mediaStore.handoff(id, {filePath: file, mimeType}, sessionId);
      hashes.push(written.hash);
    }
    return hashes;
  }

  // Cindy's ghost_call host path. The Host states who is asking; a permission
  // answer already covered the visible arguments, as a batch confirmation does.
  async function callTool({session, ghostId, namespace, tool, args = {}, attachments, dir, saveDir, agentToolUseId, grantOnly}, signal, pageGrant) {
    const workdir = session?.workingDir ?? null, sessionId = session?.id ?? null, approved = session?.approved === true;
    const classified = classifyGhostVisibility(ghostId, workdir, visibility, namespace);
    if (!classified.ok) { log.warn('Plugin tool discovery rejected: '+String(ghostId)+' ('+classified.errorCode+')'); return classified; }
    const target = classified.ghost, id = installedGhostStoragePart(target);
    const audience=pageGrant?'app':'model';
    const visibleTool = ghost => (ghost.manifest.tools ?? []).some(candidate => candidate.name === tool && pluginToolVisible(ghost.manifest, candidate.name, audience));
    if (!grantOnly && !visibleTool(target)) return {ok: false, errorCode: 'TOOL_NOT_FOUND', message: toolNotFoundMessage(ghostId, tool, target.manifest.tools?.filter(candidate => pluginToolVisible(target.manifest, candidate.name, 'model')).map(candidate => candidate.name) ?? [])};
    if (grantOnly && !attachments?.length) return {ok: false, errorCode: 'ATTACHMENT_INVALID', message: 'grant_only 调用必须携带 attachments(要预授权的文件地址列表)'};
    if(!grantOnly&&!pageGrant&&!await approveToolUi(target))return {ok:false,errorCode:'UI_APPROVAL_REQUIRED',message:'交互页面尚未获准，请在插件管理中允许后重试'};
    const setup = assessment(id);
    if (setup.state !== 'ready') {
      const ready=await connections.ensure({ghost:target,session,...(!pageGrant?{tool}:{}),signal});
      if(!ready.ok)return {ok:false,errorCode:ready.errorCode??'SETUP_REQUIRED',message:ready.message??'插件配置尚未完成',setup};
    }
    let committedHandoff;
    const preparedHandoff=await handoff.prepare({ghost:target,tool,urls:attachments??[],session,signal});
    attachments=preparedHandoff.media;let merged=preparedHandoff.args(args);
    if (attachments?.length) {
      try {
        const hashes = await grantAttachments(id, [...new Set(attachments)], workdir, approved, sessionId);
        merged = {...merged, attachments: hashes};
      } catch (error) { return {ok: false, errorCode: error.code ?? 'ATTACHMENT_INVALID', message: error.message}; }
    }
    if(grantOnly){
      const batch=await preparedHandoff.commit();batch.release();
      return {ok:true,result:{ok:true,granted_count:(merged.attachments?.length??0)+batch.files.length,attachments:merged.attachments??[],cindy_handoff_files:batch.files,guidance:'整批文件已获授权；可继续逐次调用目标工具。'}};
    }
    const callId = randomUUID();
    for (const [value, field, vault, code] of [[dir, 'dir_deposit', getDirDepositVault(), 'DIR_INVALID'], [saveDir, 'save_deposit', getSaveDepositVault(), 'DIR_INVALID']]) {
      if (value === undefined) continue;
      const inside = workdir && isPathInsideDir(fs.realpathSync.native(workdir), fs.existsSync(value) ? fs.realpathSync.native(value) : value);
      if (!inside && !approved) return {ok: false, errorCode: code, message: '工作目录外的目录需要用户批准'};
      const real = inside ? value : fs.realpathSync.native(value);
      const deposited = vault.deposit({...(field === 'dir_deposit' ? {callId} : {}), ghostId: id, dirAbs: real, workdirAbs: workdir, userGranted: !inside, ...(!inside ? {expectedRealPath: real} : {})});
      if (!deposited.ok) return {ok: false, errorCode: code, message: deposited.message};
      merged = {...merged, [field]: deposited.receipt};
    }
    if ('session_context' in merged) { const {session_context: _dropped, ...rest} = merged; void _dropped; merged = rest; }
    if (target.manifest.sessionContext === true) merged = {...merged, session_context: deriveGhostSessionContext(sessionId, workdir, session ? {workingDir: session.workingDir, remoteHostId: null, workdirIsReadOnly: session.readOnly === true} : null)};
    const again = classifyInstalledGhostVisibility(target, workdir, visibility);
    if(!again.ok){log.warn('Plugin install identity changed before dispatch: '+id);return again;}
    if(pageGrant)await pageGrant.assertCurrent();
    committedHandoff=await preparedHandoff.commit();if(committedHandoff.files.length)merged={...merged,cindy_handoff_files:committedHandoff.files};
    if(pageGrant)pageCalls.set(callId,pageGrant);
    cards.registerCall(callId, {ghostId: id, logicalGhostId: pluginStoragePart(installedGhostLogicalIdentity(target)), tool, toolUseId: agentToolUseId ?? null, sessionId, sessionInstanceId:session?.instanceId, remoteHostId: null});
    let result;
    try {result=await dispatcher.callGhostTool({ghostId:id,tool,args:merged,callId,signal,sessionId:sessionId??undefined,onProgress:pageGrant?.onProgress});if(!result.ok)await committedHandoff?.discard();}
    catch(error){await committedHandoff?.discard();throw error;}
    finally{committedHandoff?.release();pageCalls.delete(callId);}
    const producedMedia = drainGhostCallMedia(id, callId);
    const plain = result.ok && result.images ? {ok: true, result: result.result, producedMedia} : {...result, producedMedia};
    const finalized = withCardToken(plain, cards.finalizeCall(callId), callId, cards.isTurnEndCall?.(callId));
    if (!finalized.ok) return finalized;
    if (sessionId) await mediaStore.deliver(id, sessionId, JSON.stringify([finalized.result, producedMedia]));
    if(!pageGrant&&sessionId){
      await mcpApps.publish(target,sessionId,tool,args,finalized.result,[...(merged.attachments??[]),...producedMedia.map(url=>blobStore.parseBlobUrl(url)?.hash).filter(Boolean)],committedHandoff?.files??[]);
    }
    return {...finalized,result:pageGrant?finalized.result:mcpApps.modelResult(finalized.result),producedMedia, ...(result.ok && result.images?.length ? {images: result.images} : {})};
  }

  const aborts = new Map();
  async function agentCall(params) {
    const key = params.cancelKey;
    const controller = new AbortController();
    if (key) aborts.set(key, controller);
    // A stopped task stops waiting at once. The plugin's own work ends with its
    // call's deadline, as in Cindy: only a Node worker can be told to cancel.
    const cancelled = new Promise(resolve => controller.signal.addEventListener('abort', () => resolve({ok: false, errorCode: 'CANCELLED', message: '任务已停止，插件调用不再等待'}), {once: true}));
    try { return await Promise.race([callTool(params, controller.signal), cancelled]); }
    finally { if (key) aborts.delete(key); }
  }

  // ghost-pipe:send, with the same order of types as Cindy's router.
  async function receive(ghost, message) {
    const id = installedGhostStoragePart(ghost), type = message.type;
    switch (type) {
      case 'recommendations-update': return tasks.recommendations(id, message.items);
      case 'tool-result': dispatcher.handleToolResult(id, message); return {ok: true};
      case 'tool-progress': dispatcher.handleToolProgress(id, message); return {ok: true};
      case 'cindy-request': case 'model-request': return models.request(id, message);
      case 'download-request': return library.download(id, message);
      case 'fetch-request': return network.handleFetchRequest(id, message);
      case 'fs-request': return fsScope.run({id,callId:message.callId},()=>fileSlot.handleFsRequest(id,message));
      case 'library-request': return library.request(id, message);
      case 'agent-request': return agent.handleRequest(id, message);
      case 'agent-errand-request': return errand.handleRequest(id, message);
      case 'observe-request': return tasks.observe(id, message);
      case 'companions-request': return tasks.companions(id, message);
      case 'tasks-request': case 'targets-request': return tasks.tasks(id, message);
      case 'node-request': return library.withNodeDownloads(id, message, request => node.request(id, request));
      case 'pick-request': return pick.handleRequest(id, message, () => current(ghost));
      case 'workspace-request': return tasks.workspace(id, message, () => current(ghost));
      case 'preview-request': return preview.handleRequest(id, message);
      case 'task-view-request': return taskView.handleRequest(id, message);
      case 'routine-request': return tasks.routine(ghost, message);
      case 'schedule-request': return schedule.handleRequest(id, message);
      case 'card-update': cards.handleCardUpdate(id, message); return {ok: true};
      case 'notify': return notify.handleNotify(id, message);
      case 'badge': return badge.handleBadge(id, message);
      case 'confirm-request': return confirm.handleRequest(id, message);
      case 'model-picker-request': return models.picker(id, message);
      case 'browser-extension-request': return browserExtension.request(id,message);
      case 'mail-draft-update': case 'mail-draft-read': return mail.request(ghost,message);
      default: return {ok: false, errorCode: 'CAPABILITY_UNAVAILABLE', message: 'WinOne 没有此插件能力：' + String(type)};
    }
  }

  async function call(method, params) {
    if (params.language) setGhostLanguage(params.language);
    switch (method) {
      case 'agent-list': return manager.list().filter(ghost=>agentVisible(ghost)&&!isGhostDisabledForWorkdir(installedGhostStoragePart(ghost),params.session?.workingDir)).map(info);
      case 'agent-roster': return manager.list().filter(ghost=>agentVisible(ghost)&&!isGhostDisabledForWorkdir(installedGhostStoragePart(ghost),params.session?.workingDir)).map(ghost => ({id: ghost.manifest.id, ...(ghost.namespaceState === 'pending' ? {} : deliveryNamespaceFields(ghost)), name: ghost.manifest.name, ...(ghost.manifest.command ? {command: ghost.manifest.command} : {}), ...(ghost.manifest.recall ? {recall: ghost.manifest.recall} : {})}));
      case 'agent-info': {
        const classified = classifyGhostVisibility(params.ghostId, params.session?.workingDir ?? null, visibility, params.namespace);
        if (!classified.ok) return classified;
        return agentVisible(classified.ghost) ? {ok: true, ghost: info(classified.ghost)} : {ok: false, errorCode: 'GHOST_NOT_FOUND', message: '该插件没有提供 Agent 可用的工具或手册'};
      }
      case 'agent-manual': {
        const classified = classifyGhostVisibility(params.ghostId, params.session?.workingDir ?? null, visibility, params.namespace);
        if (!classified.ok) return {ok: false, manual: [], content: '', errorCode: classified.errorCode, message: classified.message};
        if (!classified.ghost.manifest.manual?.items?.length) return {ok: false, manual: [], content: '', errorCode: 'GHOST_NOT_FOUND', message: '该插件未声明可供读取的手册;不要重试,改用其它方式完成。'};
        return readInstalledGhostManual(classified.ghost, params.path);
      }
      case 'agent-call': return agentCall(params);
      case 'agent-connect-account': return connections.connect(params);
      case 'agent-cancel': aborts.get(params.cancelKey)?.abort();connections.cancel(params.cancelKey); return {ok: true};
      case 'ui-approval': {const ghost=find(params.ghostId);return {ok:!!ghost&&await approveToolUi(ghost,true)};}
      case 'agent-cancel-session': dispatcher.cancelSessionCalls(params.sessionId);connections.cancelSession(params.sessionId);appMessages.cancelSession(params.sessionId);return {ok:true};
      case 'forge-guide': case 'forge-scaffold': case 'forge-pack': case 'forge-install': return forge.call(method, params);
      case 'card-get': {
        const card = cards.getCard?.(params.callId) ?? persistedCard(params.callId);
        if(!card)return null;
        const children=[];
        const rootId=ghostCardRootCallId(card.callId);
        try {
          for(const name of fs.readdirSync(path.join(root,'cards'))){
            if(!name.startsWith(rootId.replace(/[^\w-]/g,'_')+'__sp'))continue;
            const child=JSON.parse(fs.readFileSync(path.join(root,'cards',name),'utf8'));
            if(child.callId!==rootId&&ghostCardRootCallId(child.callId)===rootId)children.push(nativeCard(child));
          }
        }catch(error){if(error.code!=='ENOENT')throw error;}
        return {...nativeCard(card),children:children.sort((a,b)=>a.updatedAt-b.updatedAt)};
      }
      case 'card-link': {
        const card=persistedCard(params.callId),ghost=card?find(card.ghostId):null;
        if(!ghost?.enabled||!ghost.manifest.card?.externalLinks||typeof params.url!=='string'||cardAttribute(card.html,'data-ghost-link','data-ghost-link',params.url)===undefined)throw failure('PERMISSION_DENIED','卡片未声明这个链接');
        if(!ghostCardLinkOpensDirectly(params.url)&&!(await ui('confirm',{ghostName:ghost.manifest.name,body:params.url,confirmText:'打开链接',cancelText:'取消'}))?.confirmed)return {ok:false,reason:'cancelled'};
        if(!current(ghost))throw failure('PLUGIN_CHANGED','插件已变化');
        await host('open-external',{url:params.url});return {ok:true};
      }
      case 'card-action': {
        let selection=params.modelSelection;
        if(params.pickModel){
          const card=persistedCard(params.callId);
          if(!card||!ghostCardDeclaresModelSelect(card.html,params.action))throw failure('INVALID_REQUEST','卡片动作没有模型选择器');
          let initial;try{initial=JSON.parse(cardAttribute(card.html,'data-ghost-model-select','data-ghost-action',params.action)||'null');}catch{}
          const answer=await ui('model-picker',{request:{requestId:randomUUID(),initial,title:find(card.ghostId)?.manifest.name}});
          if(answer?.cancelled||!answer?.route)return {ok:false,reason:'cancelled'};
          selection=answer.route;
        }
        return cardActions.dispatch(params.callId,params.action,params.prompt,undefined,params.formValues,params.change===true,undefined,selection);
      }
      case 'resolve-panel-media': {
        const result=await panelMedia.resolve(params);
        if(params.purpose==='attach'&&params.sessionId){
          if(!await host('session-snapshot',{sessionId:params.sessionId}))throw failure('TASK_NOT_FOUND','任务已不可用');
          await mediaStore.deliver(result.pluginId,params.sessionId,result.url);
        }
        return result;
      }
      case 'panel-media-action': {
        const result=await panelMedia.resolve({...params,purpose:'menu'});
        if(params.action==='reveal')await host('show-item',{path:result.path});
        else if(params.action==='copy')await host('clipboard-file',{path:result.path});
        else throw failure('INVALID_PARAMS','未知媒体动作');
        return {ok:true};
      }
      case 'mail-ui': return nativeMailUi(params,root);
      case 'mail-preview': return mail.preview(params);
      case 'mcp-app-native': return mcpApps.native(params);
      case 'mcp-app-list': return mcpApps.list(params);
      case 'mcp-app-release-owner': await mcpApps.releaseOwner(params.ownerPid);return {released:true};
      case 'mcp-app-message-admit': await appMessages.assert(params);return {accepted:true};
      case 'agent-connection-action': return connections.action(params);
      case 'agent-connection-pending': return connections.pending(params.sessionId);
      case 'handoff-action': return handoff.action(params);
      case 'media-path': return {path: blobStore.resolveSafe(params.url).absPath};
      case 'background-tasks': return host('task', {op: 'plugin-native', method: 'background-list', pluginId: params.pluginId});
      case 'reveal-background-task': {
        const task = await host('task', {op: 'plugin-native', method: 'reveal', pluginId: params.pluginId, taskId: params.taskId});
        push('reveal-session', {sessionId: task.taskId});
        return task;
      }
      case 'workdir-preferences': return {disabled:listDisabledGhostIdsForWorkdir(params.workingDir),plugins:manager.list().map(ghost=>({pluginId:installedGhostStoragePart(ghost),name:ghost.manifest.name,enabled:ghost.enabled}))};
      case 'set-workdir-preference': {
        const ghost=find(params.pluginId);if(!ghost)throw failure('NOT_FOUND','插件不存在');
        return {disabled:setGhostDisabledForWorkdir(params.workingDir,installedGhostStoragePart(ghost),params.disabled===true)};
      }
      case 'badges': return loadGhostUnread();
      case 'badge-clear': {
        const changed=clearGhostUnread(params.pluginId,params.seenAt);
        if(changed)push('badge',{ghostId:params.pluginId,unread:false});return {cleared:changed!==null};
      }
      case 'models': return host('models', {});
      case 'setup': return assessment(params.pluginId);
      case 'account-action': return accounts.action(params);
      case 'settings-session': case 'page-session': {
        const ghost = find(params.pluginId);
        const kind = method === 'settings-session' ? 'settings' : params.page;
        if (!ghost?.enabled || !['settings', 'panel', 'main', 'task'].includes(kind)) throw failure('NOT_FOUND', '插件没有这个页面');
        const started = await runtime.spawn(ghost);
        if (!started.ok || !await runtime.waitUntilRunning(params.pluginId)) throw failure('PLUGIN_UNAVAILABLE', '插件沙箱启动失败');
        if (kind === 'task' && (!/^[A-Za-z0-9-]{1,64}$/.test(params.sessionId ?? '') || !await host('session-snapshot', {sessionId: params.sessionId}))) throw failure('NOT_FOUND', '任务已不可用');
        const declaration=kind==='panel'?ghost.manifest.panel:kind==='main'?ghost.manifest.mainView:kind==='task'?ghost.manifest.taskView:null;
        return {url:adapter.pageUrl(ghost,kind,params.sessionId),title:declaration?.title??ghost.manifest.name,version:ghost.manifest.version,
          ...(kind==='settings'&&ghost.manifest.settingsHeight!==undefined?{settingsHeight:ghost.manifest.settingsHeight}:{})};
      }
      case 'agent-native-event': if(params.kind==='settled')appMessages.settled({sessionId:params.session?.id,requestId:params.requestId});return lifecycle.receive(params);
      case 'reply-overrides': return lifecycle.read(params.sessionId);
      case 'screen-user': return gateway.screenUserMessage({sessionId: params.sessionId, text: params.text});
      case 'screen-assistant': return gateway.screenAssistantMessage({sessionId: params.sessionId, text: params.text});
      case 'turn-event': gateway.publish('turn', params.name, params.data); return {ok: true};
      case 'expand-command': return {text: expandGhostCommand(params.text, manager.list().filter(ghost=>!isGhostDisabledForWorkdir(installedGhostStoragePart(ghost),params.workingDir)))};
      case 'agent-skills': {
        const directories = [];
        for (const ghost of manager.list()) {
          if (!ghost.enabled || !ghost.manifest.skill?.items?.length) continue;
          if (!await manager.verifyApprovedSkillSnapshot(ghost)) {
            log.warn('Plugin Skill snapshot is unavailable: ' + installedGhostStoragePart(ghost));
            continue; // Never fall back to mutable package files after approval.
          }
          if (!current(ghost)) continue;
          for (const item of ghost.manifest.skill.items) directories.push(path.join(ghost.approvedSkillRoot, ...item.dir.split('/')));
        }
        return directories;
      }
      case 'commands': return manager.list().filter(ghost => ghost.enabled && ghost.manifest.command && !isGhostDisabledForWorkdir(installedGhostStoragePart(ghost),params.workingDir)).map(ghost => ({command: ghost.manifest.command, name: ghost.manifest.name, pluginId: installedGhostStoragePart(ghost)}));
      case 'pages': return manager.list().flatMap(ghost=>[
        ['panel',ghost.manifest.panel],['main',ghost.manifest.mainView],['task',ghost.manifest.taskView],['settings',ghost.manifest.settingsHtml?{}:null],
      ].filter(([,decl])=>decl).map(([page,decl])=>({pluginId:installedGhostStoragePart(ghost),page,title:decl.title??ghost.manifest.name,
        html:page==='settings'?ghost.manifest.settingsHtml:decl.html,version:ghost.manifest.version,enabled:ghost.enabled,
        approved:ghost.approval.state==='approved',state:runtime.stateOf(installedGhostStoragePart(ghost)),
        ...(readGhostUnread(installedGhostStoragePart(ghost))?{unread:readGhostUnread(installedGhostStoragePart(ghost))}:{}),
        ...Object.fromEntries(['position','minWidth','defaultFraction','open','side','multiple','icon'].filter(key=>decl[key]!==undefined).map(key=>[key,decl[key]])),
        ...(ghost.manifest.settingsHeight!==undefined?{settingsHeight:ghost.manifest.settingsHeight}:{}),
      })));
      case 'library-action': return library.action(params);
      case 'user-gesture': errand.noteUserGesture(params.pluginId); return {ok: true};
      case 'session-focus': tasks.setFocused(params.sessionId ?? null); return {ok: true};
      case 'tasks-event': return tasks.hostEvent(params);
      case 'targets-review': return tasks.reviewTarget(params);
      case 'tasks-write-retry': return tasks.retryWriteAccess(params);
      case 'teams-operation': return tasks.teamOperation(params);
      case 'page-attach': return pageContext.attach(params);
      case 'page-detach': return pageContext.detach(params);
      case 'page-navigation': return pageContext.navigation(params);
      case 'page-gesture': return pageContext.gesture(params);
      case 'worktree-readiness': return tasks.worktreeReadiness(params);
      case 'targets-admit': return tasks.admitTarget(params);
      case 'tasks-admit': return tasks.admit(params);
      case 'tasks-refs': return tasks.refs(params);
      case 'recommendations': return tasks.recommendationList(params.language);
      case 'has-assistant-hook': return manager.list().some(ghost => ghost.enabled && ghost.manifest.subscribe?.hooks?.includes('will-assistant-message'));
      default: throw failure('UNKNOWN_METHOD', '未知插件操作');
    }
  }

  return {receive, call, dispatcher, cards, unread:readGhostUnread, preferenceDetails: id => tasks.preferenceDetails(id), validatePreferences: (id, value) => tasks.validatePreferences(id, value), writePreferences: (id, value) => tasks.writePreferences(id, value), oauthTokens:accounts.oauthTokens, networkConnections:accounts.networkConnections,
    configurationChanged(id,params) { if(params.parameters!==undefined)changeBus.emit(id,{source:'kv'});for(const key of Object.keys(params.secrets??{}))changeBus.emit(id,{source:'secret',ref:key}); },
    start: id => { browserExtension.reconcile();const ghost = find(id); return ghost ? node.start(ghost).catch(error => log.warn('Plugin Node startup failed: ' + error.message)) : Promise.resolve(); },
    async stop(id) { await browserExtension.stop(id);await mcpApps.stop(id);connections.stop(id); pageContext.stop(id); accounts.invalidate(id); await Promise.all([node.stop(id), library.stop(id)]); },
    endpoint: async (ghost, route, req, res) => await panelMedia.endpoint(ghost,route,req,res) || await accounts.endpoint(ghost, route, req, res) || await library.endpoint(ghost, route, req, res),
    readHandoffFile:handoff.readHandoffFile,touchHandoffFiles:handoff.touchHandoffFiles,
    libraryTransport: {readFile: library.readFile, saveStream: library.saveStream, fileSize: library.fileSize},
    onRuntimeState: (id, state) => { dispatcher.onRuntimeState(id, state); tasks.runtimeStateChanged(); if (state === 'crashed') void node.stop(id).catch(error => log.warn(error.message)); },
    readGhostMedia: (id, hash) => mediaStore.read(id, hash), saveGhostMedia: params => mediaStore.generated(params), isSupportedMediaMime: mime => blobStore.supportedMime(mime),
    uninstall: async (id, remove) => { const ghost = find(id);const result = await tasks.uninstall(id, remove); if (!result.rejection) {await handoff.remove(id); clearGhostUnread(id); await library.remove(id); accounts.remove(id, ghost.manifest); await fs.promises.rm(path.join(root, 'ghost-fs', id), {recursive:true, force:true}); await mediaStore.removeDeposits(id).catch(error => log.warn('Plugin deposit cleanup failed: ' + error.message)); } return result; },
    async close() {await browserExtension.close();await mcpApps.close();await connections.close();appMessages.close();handoff.close();pageContext.close(); lifecycle.close(); node.close?.(); await library.close(); await mediaStore.close(); await tasks.close(); clearWorktreeSecrets(); clearDatabase(); accounts.close(); }};
}
