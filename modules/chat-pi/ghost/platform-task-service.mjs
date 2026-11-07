// Cindy's complete ordinary-task service owns idempotency, revisions, runs and
// uninstall tombstones. WinOne supplies only real session/model/queue operations.
import {randomBytes} from 'node:crypto';
import {setTimeout as delay} from 'node:timers/promises';
import {createPluginTaskService,PluginTaskError} from '../cindy/ghost/apps/desktop/src/main/maker-ipc/pluginTaskService.ts';
import {hasPluginTaskApproval} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/taskCapability.ts';
import {PluginWriteAccessGate} from '../cindy/ghost/apps/desktop/src/main/maker-ipc/pluginWriteAccessGate.ts';
import {isPluginTaskPermissionAllowed} from '../cindy/ghost/apps/desktop/src/main/maker-ipc/pluginTaskService.ts';
import {t} from './seams/i18n.ts';
import {resolvePluginWorkerDirectory} from '../cindy/ghost/apps/desktop/src/main/maker-ipc/pluginWorkerDirectory.ts';
import {isGhostPickedDir} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/pickGrantsStore.ts';
import {ghostDeclaresHiddenTasks,ghostInstallApprovalToken} from '../cindy/ghost/apps/desktop/src/shared/ghost.ts';
import {createPluginTargetStore} from '../cindy/ghost/apps/desktop/src/main/maker-ipc/pluginTargetStore.ts';
import {createGhostPreferences} from './platform-preferences.mjs';
import {createGhostTeams} from './platform-teams.mjs';
import {createGhostWorktrees} from './worktrees.mjs';
import {openGhostTaskLedger} from './task-ledger.mjs';

// Match the Host's existing 26-character base32 conversation identifiers.
export const newPluginTaskId=()=>[...randomBytes(26)].map(byte=>'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'[byte&31]).join('');
export function createGhostTaskService({root,find,current,host,ui,cards,log,preferences,worktrees:providedWorktrees}){
 let ledger,service,targetStore,teams,worktrees=providedWorktrees,closed=false;
 const live=new Map(),states=new Map(),watches=new Map(),stop=new AbortController();
 const writeAccessGate=new PluginWriteAccessGate();let permissionRequestOpen=false;
 const settings=preferences??createGhostPreferences({root,find,host});
 const preference=id=>settings.read(id)??{};
 const permissionLimit=id=>{const config=settings.read(id);return config===undefined?undefined:config.permissionMode??'ask';};
 const savePreference=(id,value)=>settings.write(id,value);
 const assertOpen=()=>{if(closed)throw new PluginTaskError('HOST_NOT_READY','Plugin task service is closing',true);};
 const authorized=id=>{assertOpen();if(!hasPluginTaskApproval(find(id)))throw new PluginTaskError('PERMISSION_DENIED','Plugin task capability is unavailable');};
 const native=(method,params={})=>host('task',{op:'plugin-native',method,...params},stop.signal);
 const remember=(id,state)=>{
  states.set(id,state);
  if(state.instanceId){
   let entry=live.get(id);
   if(entry?.instanceId!==state.instanceId){entry={instanceId:state.instanceId,stablePlanModeState:{enabled:false,generation:0}};live.set(id,entry);}
   const enabled=state.planModeEnabled===true;
   if(entry.stablePlanModeState.enabled!==enabled)entry.stablePlanModeState={enabled,generation:entry.stablePlanModeState.generation+1};
  }else live.delete(id);
  return state;
 };
 const inspect=async(taskId,pluginId)=>remember(taskId,await native('inspect',{taskId,pluginId}));
 const ownerOf=async id=>(await ledger.store.get(id))?.pluginId;
 function ensure(){
  assertOpen();if(service)return service;
  ledger=openGhostTaskLedger(root);
  service=createPluginTaskService({store:ledger.store,id:newPluginTaskId,assertCurrent:assertOpen,assertAuthorized:authorized,
   readPermissionMode:permissionLimit,readModelPreference:preference,
   resolveRoute:async(id,route,callId)=>{
    authorized(id);
    const info=callId?cards.inFlightCallInfoOf(callId):null;
    if(callId&&info?.ghostId!==id)throw new PluginTaskError('PERMISSION_DENIED','Source call is not owned by this plugin');
    const value=route===undefined?await settings.resolve(preference(id),info?.sessionId):await native('resolve-route',{pluginId:id,route,sourceSessionId:info?.sessionId});
    authorized(id);return value;
   },
   createSession:async(pluginId,id,title,route,isolatedWorkspace,_requested,onPersistenceStarted,callId,hidden,targetContext,permissionMode)=>{
    authorized(pluginId);const captured=find(pluginId),config=preference(pluginId),before=JSON.stringify(config);
    const guard=()=>{authorized(pluginId);if(!current(captured)||before!==JSON.stringify(preference(pluginId)))throw new PluginTaskError('PERMISSION_DENIED','Plugin task settings changed during creation');};
    let workingDir=targetContext?.workingDir??config.workingDir;
    if(workingDir&&!isolatedWorkspace)workingDir=await resolvePluginWorkerDirectory({requested:workingDir,configuredDirectory:workingDir,isPickedDirectory:dir=>isGhostPickedDir(pluginId,dir),assertCurrent:guard});
    const creation=await ledger.store.get(id),autoTitle=creation?JSON.parse(creation.payload).autoTitle===true:false;
    guard();onPersistenceStarted();
    await native('create',{pluginId,taskId:id,title,autoTitle,route,isolatedWorkspace,hidden:!!hidden,
     workingDir,project:!!targetContext,sourceSessionId:callId?cards.inFlightCallInfoOf(callId)?.sessionId:undefined,
     permissionMode:permissionMode??config.permissionMode??'ask'});
    ledger.created(id);
   },
   readSession:async id=>native('get',{taskId:id,pluginId:await ownerOf(id)}),
   readLiveSession:id=>live.get(id),
   assertHiddenTasksAllowed:id=>{authorized(id);if(!ghostDeclaresHiddenTasks(find(id).manifest))throw new PluginTaskError('PERMISSION_DENIED','Background tasks are not declared');},
   setHidden:async(id,hidden,guard)=>{guard();await native('hidden',{taskId:id,pluginId:await ownerOf(id),hidden});},
   revealPluginTasks:async pluginId=>{if((await ledger.store.list(pluginId,'create',null,'',1)).length)await native('reveal-all',{pluginId});},
   archive:async(id,guard)=>{guard();await native('archive',{taskId:id,pluginId:await ownerOf(id)});},
   isBusy:id=>{const state=states.get(id);return !!(state?.running||state?.pending?.length||state?.approvals?.length||state?.questions?.length);},
   setModel:async(id,route,guard)=>{
    // This callback runs inside Cindy's receipt queue. Never call service.get
    // recursively here; read the Host view, then use the supplied guard and CAS.
    const pluginId=await ownerOf(id),before=await native('get',{taskId:id,pluginId});
    await guard();return native('set-model',{taskId:id,pluginId,route,expectedRevision:before.revision});
   },
   assertTeamPlanUnstarted:async id=>native('assert-unstarted',{taskId:id,pluginId:await ownerOf(id)}),
   dispatch:async(pluginId,id,clientId,text,images=[],context)=>native('send',{pluginId,taskId:id,clientId,text,context,
    images:images.map(image=>({name:image.name,mimeType:image.mimeType,hash:image.hash,data:image.buffer.toString('base64')}))}),
   inspect:async id=>{const state=await inspect(id,await ownerOf(id));return {execution:state.running?(state.execution??null):null,pending:(state.pending??[]).map(item=>({clientId:item.id}))};},
   cancel:async(pluginId,id,inputClientIds,execution)=>native('cancel',{pluginId,taskId:id,inputClientIds,execution}),
   scheduleAutoTitle:(pluginId,id,text)=>{void native('auto-title',{pluginId,taskId:id,text}).catch(error=>log.warn(error.message));},
  });
  return service;
 }
 const teamService=()=>{
  ensure();
  worktrees??=createGhostWorktrees({root,ledger,host,log});
  return teams??=createGhostTeams({ledger,host,id:newPluginTaskId,log,worktrees});
 };
 // Restore only receipt observation, never a model send. The real per-input
 // terminal wins even if another user message has since completed in the task.
 function watch(pluginId,run){
  if(!run||watches.has(run.runId)||['completed','failed','cancelled','interrupted'].includes(run.status)||closed)return;
  const work=(async()=>{
   let cursor=0;
   while(!stop.signal.aborted){
    try{
     const result=await native('wait',{pluginId,taskId:run.taskId,requestId:run.inputMessageId,cursor});
     cursor=result.cursor; if(result.state)remember(run.taskId,result.state);
     if(!result.terminal)continue;
     const receipt=await ledger.store.get(run.runId),saved=receipt?JSON.parse(receipt.payload):null;
     if(saved?.execution)await service.settle(run.taskId,result.terminal.execution,result.terminal.status==='stopped'?'cancelled':result.terminal.status,result.terminal.outputMessageId);
     else await service.discard(run.taskId,{clientId:run.inputMessageId},result.terminal.status==='cancelled'||result.terminal.status==='stopped'?'cancelled':'failed');
     return;
    }catch(error){
     if(stop.signal.aborted)return;
     log.warn('Plugin task receipt read will retry without resending: '+error.message);
     await delay(1000,undefined,{signal:stop.signal});
    }
   }
  })().catch(error=>{if(!stop.signal.aborted)log.warn(error.message);}).finally(()=>watches.delete(run.runId));
  watches.set(run.runId,work);
 }
 async function requestAccess(pluginId,request,explicit=false){
  const api=ensure(),captured=find(pluginId);
  const guard=()=>{authorized(pluginId);if(!current(captured))throw new PluginTaskError('PERMISSION_DENIED','Plugin installation changed');};
  guard();
  const task=await api.get(pluginId,request.taskId),mode=request.mode??'acceptEdits',config=settings.read(pluginId);
  if(task.status!=='active')throw new PluginTaskError('TASK_BUSY','Archived tasks cannot request write access');
  if(task.planModeEnabled)throw new PluginTaskError('PERMISSION_DENIED','Exit Plan Mode before requesting write access');
  if((!config||isPluginTaskPermissionAllowed(task.permissionMode,config.permissionMode??'ask'))&&isPluginTaskPermissionAllowed(mode,task.permissionMode))return {granted:true,task};
  if(permissionRequestOpen)throw new PluginTaskError('TASK_BUSY','A permission request is already open');
  // Busy validation has not asked the owner anything and consumes no refusal
  // attempt. The original gate keeps a refusal across reload and mode changes.
  if(mode==='acceptEdits'&&(await api.listRuns(pluginId,task.taskId)).items.length)throw new PluginTaskError('TASK_BUSY','Only an unstarted task can request edit access');
  await native(mode==='acceptEdits'?'assert-unstarted':'assert-idle',{pluginId,taskId:task.taskId});guard();
  const key=JSON.stringify([pluginId,task.taskId]),identity=JSON.stringify([root,ghostInstallApprovalToken(captured.approval)]),before=JSON.stringify(config);
  permissionRequestOpen=true;
  try{return await api.completeOperation(()=>writeAccessGate.request(key,identity,mode,explicit,async()=>{
   const auto=mode==='auto',prefix='pluginTaskWriteAccess.';
   const answer=await ui('confirm',{ghostId:pluginId,ghostName:captured.manifest.name,sessionId:task.taskId,
    body:t(prefix+(auto?'autoMessage':'message'),{name:captured.manifest.name})+'\n\n'+task.title+'\n\n'+t(prefix+(auto?(config?'autoDetail':'autoTaskDetail'):(config?'detail':'taskDetail'))).replaceAll('Cindy','WinOne'),confirmText:t(prefix+'allow'),cancelText:t(prefix+'cancel')});
   if(!answer?.confirmed)return {granted:false};
   guard();if(before!==JSON.stringify(settings.read(pluginId)))throw new PluginTaskError('STALE_REVISION','Plugin task settings changed');
   await native(mode==='acceptEdits'?'assert-unstarted':'assert-idle',{pluginId,taskId:task.taskId});guard();
   const updated=await native('permission',{pluginId,taskId:task.taskId,expectedRevision:task.revision,mode});
   try{if(config&&!isPluginTaskPermissionAllowed(mode,config.permissionMode??'ask'))savePreference(pluginId,{...config,permissionMode:mode});}
   catch(error){await native('permission-restore',{pluginId,taskId:task.taskId,expectedRevision:updated.revision,mode:task.permissionMode});throw error;}
   return {granted:true,task:updated};
  }));}finally{permissionRequestOpen=false;}
 }
 async function handle(pluginId,request){
  const api=ensure();authorized(pluginId);
  const captured=find(pluginId),guard=()=>{authorized(pluginId);if(!current(captured))throw new PluginTaskError('PERMISSION_DENIED','Plugin installation changed');};
  switch(request.kind){
   case 'models': return native('models',{pluginId});
   case 'create': return api.create(pluginId,request);
   case 'list': return api.list(pluginId,request.after,request.limit);
   case 'get': return api.get(pluginId,request.taskId);
   case 'setModel': return api.setModel(pluginId,request);
   case 'setHidden': return api.setHidden(pluginId,request);
   case 'archive': await inspect(request.taskId,pluginId);guard();return api.archive(pluginId,request);
   case 'send': {
    guard();const run=await api.send(pluginId,request);watch(pluginId,run);return run;
   }
   case 'getRun': {const run=await api.getRun(pluginId,request.runId);watch(pluginId,run);return run;}
   case 'listRuns': {const page=await api.listRuns(pluginId,request.taskId,request.after,request.limit);for(const run of page.items)watch(pluginId,run);return page;}
   case 'cancel': return api.cancel(pluginId,request.runId);
   case 'readMessages': await api.get(pluginId,request.taskId);return native('messages',{pluginId,taskId:request.taskId,after:request.after,limit:request.limit});
   case 'requestWriteAccess': return requestAccess(pluginId,request);
   case 'setTeamPlan': {
    const task=await api.get(pluginId,request.taskId),config=settings.read(pluginId),before=JSON.stringify(config),items=[];
    const currentPlan=()=>{guard();if(before!==JSON.stringify(settings.read(pluginId)))throw new PluginTaskError('PERMISSION_DENIED','Plugin directory authorization changed');};
    for(const item of request.plan.items){
     const workingDir=await resolvePluginWorkerDirectory({requested:item.workingDir,leadDirectory:task.workingDir,configuredDirectory:config?.workingDir,isPickedDirectory:dir=>isGhostPickedDir(pluginId,dir),assertCurrent:currentPlan});
     await native('resolve-route',{pluginId,route:item.route});currentPlan();items.push({...item,workingDir});
    }
    const fresh=await api.get(pluginId,task.taskId);currentPlan();
    if(fresh.revision!==task.revision)throw new PluginTaskError('STALE_REVISION','Task changed during directory validation');
    return api.setTeamPlan(pluginId,request.taskId,{...request.plan,items});
   }
   case 'startTeam': {
    const task=await api.get(pluginId,request.taskId),config=settings.read(pluginId);
    const admission=value=>JSON.stringify([value.resolvedConfig,value.permissionMode,value.planModeEnabled,value.workingDir]);
    const expected=admission(task),before=JSON.stringify(config);
    const allowed=value=>{guard();if(value.status!=='active')throw new PluginTaskError('TASK_BUSY','Archived tasks cannot start a team');if(value.planModeEnabled||value.permissionMode!=='auto'||config&&!isPluginTaskPermissionAllowed('auto',config.permissionMode??'ask'))throw new PluginTaskError('PERMISSION_DENIED','Authorize Auto for the coordinator before starting a team');};
    allowed(task);
    const current=async()=>{const fresh=await api.get(pluginId,task.taskId);allowed(fresh);if(before!==JSON.stringify(settings.read(pluginId))||admission(fresh)!==expected)throw new PluginTaskError('STALE_REVISION','Task changed during team activation');};
    const result=await api.completeOperation(()=>teamService().startPlugin(task.taskId,current));
    if(!result.ok)throw new PluginTaskError(result.errorCode??'HOST_NOT_READY','Collaboration could not be started');
    return result;
   }
   case 'getTeam': await api.get(pluginId,request.taskId);return teamService().get(request.taskId);
   case 'releaseWorker': await api.get(pluginId,request.taskId);return api.completeOperation(()=>teamService().release(request.taskId,request.workerId,request.completedAt));
   default: throw new PluginTaskError('INVALID_REQUEST','Unknown task operation');
  }
 }
 return {handle,preferences:preference,permissionLimit,
  async teamOperation(params){
   try{return await ensure().completeOperation(()=>teamService().operation(params.sessionId,params.method,params.params));}
   catch(error){return {ok:false,errorCode:error.code??'INTERNAL',message:error.message};}
  },
  async retryWriteAccess(pluginId,taskId){
   authorized(pluginId);const captured=find(pluginId),identity=JSON.stringify([root,ghostInstallApprovalToken(captured.approval)]);
   const mode=writeAccessGate.recoverable(JSON.stringify([pluginId,taskId]),identity);
   if(!mode)throw new PluginTaskError('REQUEST_EXPIRED','当前任务没有待重试的插件写权限请求');
   return requestAccess(pluginId,{taskId,mode},true);
  },
  preferenceDetails:id=>({showTaskPreferences:settings.show(id),taskPreferences:preference(id)}),
  validatePreferences:(id,value)=>settings.validate(id,value),
  writePreferences:(id,value)=>settings.write(id,value),
  database(){ensure();return ledger.db;},
  targetStore(){ensure();return targetStore??=createPluginTargetStore(ledger.db,assertOpen);},
  assertProjectDispatch:(pluginId,id)=>ensure().assertDispatch(pluginId,id),
  createTarget:(pluginId,project,id,title,route,guard,autoTitle,permissionMode)=>ensure().create(pluginId,{requestKey:'target:'+id,...(autoTitle?{autoTitle:true}:{title}),route,permissionMode},{taskId:id,workingDir:project,assertCurrent:guard}),
  async admit(params){
   const api=ensure(),pluginId=await ownerOf(params.sessionId);
   if(!pluginId)throw new PluginTaskError('TASK_NOT_FOUND','Task receipt not found');
   await inspect(params.sessionId,pluginId);
   await api.accept(params.sessionId,{clientId:params.requestId},params.execution);
   return {accepted:true};
  },
  async worktreeReadiness(params){
   ensure();worktrees??=createGhostWorktrees({root,ledger,host,log});
   return worktrees.restore(params.sessionId,params.workingDir);
  },
  async refs(params){ensure();return ledger.prs.record(params.sessionId,params.urls);},
  async uninstall(pluginId,remove){let result;await ensure().withUninstall(pluginId,async()=>{result=await remove();});return result;},
  async close(){stop.abort();await teams?.close();await Promise.allSettled([...watches.values()]);if(service)await service.drain();worktrees?.close?.();closed=true;ledger?.close();},
 };
}
