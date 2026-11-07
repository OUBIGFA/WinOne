// The actual Cindy Orca lifecycle and Worker-creation kernels. WinOne adapts
// only their storage, live-session and queue I/O to its existing Pi Host. Links
// and receipts survive restart; reading a team never invents a completed run.
import {AsyncLocalStorage} from 'node:async_hooks';
import {randomUUID} from 'node:crypto';
import {setTimeout as delay} from 'node:timers/promises';
import {createOrcaLifecycleService,ORCA_WORKER_READY_MESSAGE} from '../cindy/ghost/apps/desktop/src/main/maker-ipc/orcaLifecycleService.ts';
import {createOrcaWorkerCreationService} from '../cindy/ghost/apps/desktop/src/main/maker-ipc/orcaWorkerCreationService.ts';
import {normalizeOrcaWorkerLabel,normalizeOrcaWorkerRole,orcaWorkerSessionTitle} from '../cindy/ghost/packages/maker-shared/src/orcaTeam.ts';
import {PluginTaskError} from '../cindy/ghost/apps/desktop/src/main/maker-ipc/pluginTaskService.ts';
import {createGhostTeamStore} from './team-store.mjs';
import {readBranch} from '../onebot/history.mjs';
import {readInputExecution} from '../input-execution.mjs';

const failure=(code,message)=>{throw new PluginTaskError(code,message);};
export function createGhostTeams({ledger,host,id,log,worktrees}){
 const store=createGhostTeamStore(ledger),scope=new AsyncLocalStorage(),stop=new AbortController(),watchers=new Map();
 let catalog=[],runtime={subagents:true,maxSubagents:4};
 const native=(method,params={})=>host('task',{op:'team-native',method,...params},stop.signal);
 const teamWorkers=lead=>{const active=store.active(lead);return active?store.workers(lead).filter(row=>row.team_id===active.id):[];};
 const changed=lead=>{void native('changed',{taskId:lead}).catch(error=>log.warn(error.message));};
 const session=id=>native('get',{taskId:id});
 const snapshot=row=>row?{id:row.id,leadSessionId:row.lead_session_id}:null;
 const mark=(lead,role,teamId,permissionMode)=>native('role',{taskId:lead,role,teamId,permissionMode});
 const requireScope=()=>{const value=scope.getStore();if(!value)failure('PERMISSION_DENIED','Orca call has no captured session');return value;};
 async function refresh(){catalog=await native('models');runtime=await native('settings');}
 function models(agent){
  if(agent!=='pi')return [];
  const found=new Map();
  for(const provider of catalog)for(const model of provider.models){
   const metadata=provider.modelMetadata?.[model]??{},efforts=metadata.efforts?.length?metadata.efforts:metadata.levels??[];
   // Fast is the existing WinOne Pi composer request flag, not a guarantee
   // about a vendor endpoint; the same native route/gateway validates execution.
   if(!found.has(model))found.set(model,{id:model,efforts,defaultEffort:null,effortsUnknown:!efforts.length,supportsFastMode:true});
  }
  return [...found.values()];
 }
 async function facts(worker){
  const value=await native('facts',{taskId:worker.session_id}),row=value.row,branch=await readBranch(row),report=worker.pending_remote_report?JSON.parse(worker.pending_remote_report):null;
  const input=branch.findLast(entry=>entry.type==='custom'&&entry.customType==='winone.input')?.data;
  const terminal=report?.clientId?readInputExecution({getBranch:()=>branch},report.clientId):null;
  const pending=value.state?.pending??[],working=value.state?.running===true||value.activity?.phase==='running';
  const waiting=(value.state?.approvals?.length??0)+(value.state?.questions?.length??0)>0;
  const matches=report&&input?.id===report.clientId&&(!value.live||value.state.requestId===report.clientId);
  const completedAt=matches&&!working&&!waiting&&!pending.length&&terminal?.status==='completed'?terminal.completedAt??null:null;
  let status=working?'running':completedAt?'done':terminal?.status==='failed'?'error':worker.status==='done'?'idle':worker.status;
  if(report&&terminal?.status&&matches){store.report(worker.id,{...report,terminal},status);worker=store.worker(worker.id);}
  return {worker,row,report,terminal,completedAt,status,working,waiting,pending,branch,activity:value.activity,state:value.state};
 }
 async function bridge(worker,report){
  if(report.bridged||report.notify===false||report.terminal?.status!=='completed')return;
  const text=`Worker ${worker.label} completed:\n${report.terminal.resultText??''}`;
  const result=await native('send',{taskId:worker.leadSessionId,originSessionId:worker.session_id,clientId:'orca:report:'+report.clientId.slice(5),text,delivery:'queue',hidden:false});
  if(result.accepted!==true)failure('DELIVERY_UNCERTAIN','Worker report was not confirmed');
  const current=store.worker(worker.id),saved=current?.pending_remote_report?JSON.parse(current.pending_remote_report):null;
  if(saved?.clientId===report.clientId)store.report(worker.id,{...saved,bridged:true});
 }
 function watch(workerId){
  if(watchers.has(workerId)||stop.signal.aborted)return;
  const work=(async()=>{
   let cursor=0;
   while(!stop.signal.aborted){
    const worker=store.worker(workerId),report=worker?.pending_remote_report?JSON.parse(worker.pending_remote_report):null;
    if(!report?.clientId)return;
    try{
     if(report.terminal){await bridge(worker,report);return;}
     const result=await native('wait',{taskId:worker.session_id,requestId:report.clientId,cursor});cursor=result.cursor;
     if(!result.terminal)continue;
     const fresh=store.worker(workerId),current=JSON.parse(fresh.pending_remote_report);
     if(current.clientId!==report.clientId)continue;
     store.report(workerId,{...current,terminal:result.terminal},result.terminal.status==='completed'?'done':result.terminal.status==='failed'?'error':'idle');
     // The result is task output, not a new owner command. Delivery has its
     // own stable native ID, so recovery cannot repeat a model turn here.
     await bridge(worker,{...current,terminal:result.terminal});changed(worker.leadSessionId);
     return;
    }catch(error){if(stop.signal.aborted)return;log.warn('Orca receipt observation will retry without resending: '+error.code);await delay(1000,undefined,{signal:stop.signal});}
   }
  })().catch(error=>{if(!stop.signal.aborted)log.warn(error.message);}).finally(()=>watchers.delete(workerId));
  watchers.set(workerId,work);
 }
 async function dispatch({targetSessionId,message,imagePaths,dispatchMeta,delivery='queue'},guard){
  await guard?.();const worker=store.session(targetSessionId);if(!worker||worker.teamStatus!=='active')failure('WORKER_NOT_FOUND','Worker is not in an active team');
  const clientId='orca:'+randomUUID(),before=await native('facts',{taskId:targetSessionId});
  store.report(worker.id,{clientId,text:message,acceptedAt:Date.now(),notify:true},'running');
  const value=await native('send',{taskId:targetSessionId,originSessionId:worker.leadSessionId,clientId,text:message,delivery,images:imagePaths});
  await guard?.();if(value.accepted!==true)failure('DELIVERY_UNCERTAIN','Worker input was not confirmed');
  const current=JSON.parse(store.worker(worker.id).pending_remote_report);store.report(worker.id,{...current,execution:value.execution},'running');watch(worker.id);
  const common={agentKind:'pi',targetTitle:before.row.title,targetLastUserSendAt:null};
  return value.queued?{...common,dispatched:false,queued:true,wakeKind:'queued',queuedMessageId:clientId,dispatchOutcome:{kind:'queued',clientId}}:{...common,dispatched:true,wakeKind:'already-active',dispatchOutcome:{kind:'session-dispatch',source:dispatchMeta?.source??'orca',dispatched:true}};
 }
 const creation=createOrcaWorkerCreationService({
  getActiveTeamByLead:async lead=>snapshot(store.active(lead)),
  listWorkersByLead:async lead=>teamWorkers(lead).map(row=>({id:row.id,label:row.label,status:row.status})),
  isActiveWorkerStatus:status=>status==='idle'||status==='running',
  readCollaborationSettings:()=>({workerSoftLimit:runtime.maxSubagents,workerHardLimit:runtime.maxSubagents}),
  getLeadSessionRow:async lead=>{const row=await session(lead),route=row.resolvedConfig;return {id:lead,agentKind:'pi',workspaceKind:row.projectId?'project':'dialogue',workingDir:row.directory,model:route.model,effort:route.effort,permissionMode:row.permissionMode,fastMode:route.fastMode,providerId:route.providerId,remoteHostId:null};},
  getWorkerDefaults:()=>({}),getWorkerPermissionMode:()=>requireScope().permissionMode,
  resolveWorkerWorkingDir:async(directory,lead)=>{const value=await native('directory',{taskId:lead.id,params:{directory}});return value.directory;},
  ...(worktrees?{resolveWorkerWorktreeSource:(...args)=>worktrees.resolve(...args),getWorkerWorktree:session=>worktrees.get(session),createWorkerWorktree:(...args)=>worktrees.create(...args),discardWorkerWorktree:session=>worktrees.discard(session)}:{}),
  getAvailableModels:models,
  getProviderRoutingContext:async(agent,_remote,device)=>{
   if(device)failure('UNSUPPORTED_CAPABILITY','WinOne Orca uses this computer');await refresh();
   const providers=catalog.map(provider=>({id:provider.id,name:provider.name,models:provider.models,requiresExplicitRoute:true,
    effortMetaByModel:Object.fromEntries(provider.models.map(model=>{const metadata=provider.modelMetadata?.[model]??{},efforts=metadata.efforts?.length?metadata.efforts:metadata.levels??[];return [model,{efforts,defaultEffort:null,effortsUnknown:!efforts.length}];}))}));
   return {availability:{pi:providers,codex:[],'claude-code':[]},resolveDefaultProviderIdForModel:(_agent,model)=>providers.find(row=>row.models.includes(model))?.id??null};
  },
  readClaudeApiKey:()=>null,reserveWorkerCreation:async input=>store.reserve(input),renewWorkerCreationReservation:async(id,lease)=>store.renew(id,lease),releaseWorkerCreationReservation:async id=>store.release(id),
  createId:randomUUID,createSessionId:id,buildCreateOptsWithStderr:opts=>opts,
  bootstrapSession:async options=>{const row=await native('create',{options});return {session:{id:row.id,agentKind:'pi'},didInjectOrcaInstructions:true,didInjectProjectContext:true};},
  addOrUpdateWorker:async worker=>store.put(worker),markOrcaRoleIfNeeded:async()=>{},
  closeWorkerSession:async taskId=>native('stop',{taskId}),archiveWorkerSession:async taskId=>native('archive',{taskId}),forgetWorkerSession:()=>{},removeWorker:async id=>store.remove(id),
  dispatchWorkerTask:dispatch,broadcastSessionCreated:taskId=>{const worker=store.session(taskId);if(worker)changed(worker.leadSessionId);},broadcastOrcaWorkerChanged:changed,
 });
 const lifecycle=createOrcaLifecycleService({
  getActiveTeamByLead:async lead=>snapshot(store.active(lead)),createActiveTeam:async lead=>snapshot(store.create(lead)),isOrphanedTeamInit:async id=>store.orphaned(id),
  getWorkerPermissionMode:()=>requireScope().permissionMode,setWorkerPermissionMode:value=>{requireScope().permissionMode=value;},
  getWorkerPermissionModeOverride:async()=>({permissionMode:requireScope().permissionMode,assertCurrent:async()=>requireScope().guard?.()}),
  createWorkerInTeam:(...args)=>creation.createWorkerInTeam(...args),dispatchWorkerTask:dispatch,markTeamEnded:async(id,status)=>store.end(id,status),
  setSessionOrcaRole:async(lead,role)=>{const ctx=requireScope();await mark(lead,role??'',store.active(lead)?.id,ctx.permissionMode);},
  clearKnownNonOrcaSession:()=>{},setLeadVendorOptions:async()=>{},clearLeadVendorOptions:async lead=>mark(lead,''),
  sendWorkerReadyPlaceholder:async({workerSessionId})=>{const worker=store.session(workerSessionId);await native('send',{taskId:workerSessionId,originSessionId:worker.leadSessionId,clientId:'orca:ready:'+worker.id,text:ORCA_WORKER_READY_MESSAGE,delivery:'queue',hidden:true,toolsDisabled:true});},
  rollbackCreatedWorker:async({workerId,workerSessionId})=>{await native('archive',{taskId:workerSessionId});store.remove(workerId);},broadcastSessionCreated:taskId=>{const worker=store.session(taskId);if(worker)changed(worker.leadSessionId);},broadcastOrcaWorkerChanged:changed,
 });
 async function withLead(lead,operation,{guard,plugin=false,permissionMode}={}){
  await refresh();if(!runtime.subagents)failure('UNSUPPORTED_CAPABILITY','协作已在 Chat 设置中关闭');
  const row=await session(lead);if(row.status!=='active'||row.orcaRole==='worker')failure('LEAD_NOT_SUPPORTED','当前任务不能作为 Lead');
  const mode=plugin?'auto':permissionMode??(row.workerPermissionMode||'auto');
  return scope.run({lead,guard,permissionMode:mode},operation);
 }
 async function resolveWorker(lead,reference){
  const worker=store.workers(lead).find(row=>row.id===reference||row.session_id===reference||row.label===reference);
  if(!worker)failure('WORKER_NOT_FOUND','Worker not found');return worker;
 }
 async function team(lead){
  const active=store.active(lead),workers=[];
  for(const worker of teamWorkers(lead)){
   const value=await facts(worker);if(value.report&&(!value.terminal?.status||!value.report.bridged))watch(worker.id);
   const usages=value.branch.filter(entry=>entry.type==='message'&&entry.message.role==='assistant').map(entry=>entry.message.usage).filter(Boolean);
   const tokens=usages.length?usages.reduce((sum,row)=>sum+(row.totalTokens??((row.input??0)+(row.output??0)+(row.cacheRead??0)+(row.cacheWrite??0))),0):null;
   const worktree=worktrees?await worktrees.get(worker.session_id):null;
   workers.push({worker_id:worker.id,session_id:worker.session_id,label:worker.label,role:worker.role,focused:!!worker.focused,status:value.status,session_status:value.row.status,agent_kind:'pi',model:value.row.resolvedConfig.model,providerId:value.row.resolvedConfig.providerId,permissionMode:value.row.permissionMode,fastMode:value.row.resolvedConfig.fastMode,working_dir:value.row.directory,completedAt:value.completedAt,createdAt:value.row.createdAt,acceptedAt:value.report?.acceptedAt,startedAt:value.activity?.startedAtMs,waitingForUser:value.waiting,timingBasis:'host-input-receipt',usage:{scope:'session-total',tokens,costUSD:null,approximate:false,reason:'No confirmed USD cost; estimates are not a bill'},...(worktree?{owns_worktree:true,worktree:{path:worktree.path,branch:worktree.branch}}:{})});
  }
  return {ok:true,workflow:active?{workflow_id:active.id,lead_session_id:lead,status:active.status}:null,worker_count:workers.length,ui_capacity:runtime.maxSubagents,workers};
 }
 async function archive(lead,reference,completedAt){
  const worker=await resolveWorker(lead,reference),value=await facts(worker);
  if(completedAt!==undefined&&(value.completedAt!==completedAt||!value.report?.clientId))failure('STALE_REVISION','Worker has no matching successful completion');
  if(value.working||value.waiting||value.pending.length)failure('WORKER_STATE_CHANGED','Worker has active or queued input');
  if(value.row.status!=='archived'){
   if(completedAt!==undefined)await native('release',{taskId:worker.session_id,requestId:value.report.clientId,expectedCompletedAt:completedAt,expectedRevision:value.row.revision});
   else await native('archive',{taskId:worker.session_id});
  }
  store.status(worker.id,'done');
  const worktreeDisposition=worktrees?await worktrees.archive(worker.session_id):null;
  return {ok:true,workerId:worker.id,...(worktreeDisposition?{worktreeDisposition}:{})};
 }
 async function queueTarget(lead,reference){return reference?(await resolveWorker(lead,reference)).session_id:lead;}
 async function queueAction(lead,method,params){
  const taskId=await queueTarget(lead,params.workerRef),owner=await native('facts',{taskId}),rows=owner.state?.pending??[];
  const selected=rows.find(row=>row.id===params.queuedMessageId);
  if(method!=='listWorkerQueuedMessages'&&method!=='mergeWorkerQueuedMessages'&&(!selected||selected.originSessionId!==lead))failure('NOT_LEAD_MESSAGE','Only this Lead’s queued input can be changed');
  if(method==='listWorkerQueuedMessages')return {ok:true,workerId:store.session(taskId)?.id??null,workerSessionId:taskId,status:store.session(taskId)?.status??'lead',isWorking:owner.state?.running??false,willQueue:owner.state?.running??false,queuePaused:owner.state?.queuePaused??false,messages:rows.map((row,position)=>({queuedMessageId:row.id,position,source:row.originSessionId===lead?'lead':row.originSessionId?'worker':'user',content:row.text,consuming:false}))};
  const actions={updateWorkerQueuedMessage:'update',cancelWorkerQueuedMessage:'cancel',steerWorkerQueuedMessage:'steer',moveWorkerQueuedMessage:'reorder',mergeWorkerQueuedMessages:'merge'};
  const changed=await native('queue',{taskId,originSessionId:lead,action:actions[method],params:{id:params.queuedMessageId,text:params.message,ids:params.queuedMessageIds,targetIndex:params.position}});
  return {ok:true,workerId:store.session(taskId)?.id??null,queuedMessageId:params.queuedMessageId??params.queuedMessageIds?.[0],delivery:method==='steerWorkerQueuedMessage'?'steered':undefined,position:params.position,messages:changed.messages?.filter(row=>row.pending).map((row,index)=>({queuedMessageId:row.requestId,position:index,source:'lead',content:typeof row.content==='string'?row.content:'',consuming:false}))};
 }
 const operation=async(lead,method,params={})=>{
  if(params.leadSessionId&&params.leadSessionId!==lead||params.callerLeadSessionId&&params.callerLeadSessionId!==lead)failure('PERMISSION_DENIED','Orca caller identity differs');
  if(method==='context')return session(lead);
  if(method==='getTeam'||method==='getWorkspaceInfo'){await refresh();return team(lead);}
  if(method==='releaseWorker')return archive(lead,params.workerId,params.completedAt);
  if(method==='startTeam')return withLead(lead,()=>lifecycle.startTeam({leadSessionId:lead,workerPermissionMode:params.workerPermissionMode}),{permissionMode:params.workerPermissionMode});
  if(method==='createWorker')return withLead(lead,()=>lifecycle.createWorker({...params,leadSessionId:lead,agent:params.agent??'pi'}));
  if(method==='listAvailableModels'){
   if(params.executionDeviceId||params.agentDeviceId)failure('UNSUPPORTED_CAPABILITY','WinOne 协作模型在本机管理');
   await refresh();
   const list=models('pi').map(model=>{const providers=catalog.filter(provider=>provider.models.includes(model.id)).map(provider=>({id:provider.id,name:provider.name}));return {...model,label:model.id,providers,defaultProviderId:providers[0]?.id??null};});
   return {ok:true,agentDeviceId:null,locations:[],pi:params.agent&&params.agent!=='pi'?[]:list,codex:[],claude_code:[]};
  }
  if(method==='listWorkers'){const value=await team(lead);return {ok:true,workers:value.workers.map(row=>({workerId:row.worker_id,sessionId:row.session_id,label:row.label,role:row.role,status:row.status,focused:row.focused,agent:'pi',agentKind:'pi',model:row.model,effort:row.effort??null,idleSince:null,...(row.owns_worktree?{ownsWorktree:true,worktree:row.worktree}: {})}))};}
  if(method==='switchFocus'){const worker=await resolveWorker(lead,params.workerIdOrLabel);store.focus(worker.id);return {ok:true,workerId:worker.id};}
  if(method==='sendToWorker'||method==='interruptWorker'){
   const worker=await resolveWorker(lead,params.targetSessionId),before=await facts(worker);
   const interrupt=method==='interruptWorker'&&before.working;
   if(interrupt)await native('queue',{taskId:worker.session_id,originSessionId:lead,action:'pause',params:{}});
   const value=await dispatch({targetSessionId:worker.session_id,message:params.message,imagePaths:params.images,delivery:params.delivery??'queue',dispatchMeta:{source:'cindy_orca',context:lead}});
   let stopOutcome=method==='interruptWorker'?'no-active-turn':undefined;
   if(interrupt){try{await native('stop',{taskId:worker.session_id});stopOutcome='requested';}catch{stopOutcome='unconfirmed';}}
   return {ok:true,agentKind:'pi',wakeKind:value.queued?'queued':params.delivery==='steer'?'steered':'already-active',targetTitle:value.targetTitle,targetLastUserSendAt:null,queuedMessageId:value.queuedMessageId??(method==='interruptWorker'?JSON.parse(store.worker(worker.id).pending_remote_report).clientId:undefined),stopOutcome,queuePaused:interrupt};
  }
  if(method==='updateWorkerQueuedMessage'||method==='cancelWorkerQueuedMessage'||method==='steerWorkerQueuedMessage'||method==='moveWorkerQueuedMessage'||method==='mergeWorkerQueuedMessages'||method==='listWorkerQueuedMessages')return queueAction(lead,method,params);
  if(method==='archiveWorker')return archive(lead,params.workerId);
  if(method==='idleWorker'){const worker=await resolveWorker(lead,params.workerId),value=await facts(worker);if(value.working||value.pending.length)failure('WORKER_STATE_CHANGED','Worker is busy');store.status(worker.id,'idle');return {ok:true,workerId:worker.id};}
  if(method==='updateWorker'){
   const worker=await resolveWorker(lead,params.workerId),role=normalizeOrcaWorkerRole(params.role??worker.role),label=normalizeOrcaWorkerLabel(params.label??worker.label);
   if(!role.ok||!label.ok)failure('INVALID_PARAMS',role.message??label.message);
   store.identity(worker.id,role.value,label.value);await native('identity',{taskId:worker.session_id,params:{title:orcaWorkerSessionTitle(role.value,label.value)}});return {ok:true,workerId:worker.id,role:role.value,label:label.value};
  }
  if(method==='endTeam'){
   for(const worker of teamWorkers(lead)){const value=await facts(worker);if(value.row.status!=='archived')await native('stop',{taskId:worker.session_id});await archive(lead,worker.id);}
   const current=store.active(lead);if(current)store.end(current.id,'completed');await mark(lead,'');return {ok:true};
  }
  if(method==='getWorkerStatus'||method==='readWorker'){
   const worker=await resolveWorker(lead,params.workerId),value=await facts(worker);
   return {ok:true,worker_id:worker.id,session_id:worker.session_id,status:value.status,session_status:value.row.status,idle_ms:null,last_active_ms:value.activity?.lastActivityAtMs??null,isWorking:value.working,result:value.terminal?.resultText??'',turnError:value.terminal?.status==='failed'?{message:value.terminal.error??'Worker failed'}:null};
  }
  failure('INVALID_REQUEST','Unknown Orca operation');
 };
 return {operation,
  startPlugin(lead,guard){return withLead(lead,()=>lifecycle.startTeam({leadSessionId:lead}),{plugin:true,guard});},
  get:team,release:archive,
  async close(){stop.abort();await Promise.allSettled([...watchers.values()]);},
 };
}
