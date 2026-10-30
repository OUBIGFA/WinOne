// Native seams for Cindy's unchanged independent-task coordinator. The Host
// owns the enabled model catalog and Pi instances, never the model's arguments.
import {randomBytes,randomUUID} from 'node:crypto';
import {and,eq,inArray} from 'drizzle-orm';
import {oneBotOwnerScope,withIMScope} from './im-scope.mjs';
import {oneBotError} from './content-service.mjs';
import {createOneBotTaskStore} from './task-store.mjs';
import {createOneBotPrService} from './pr-service.mjs';
import {createBotDelegationService} from '../cindy/tasks/botDelegationService.ts';
import {createBotDirectMessageService} from '../cindy/tasks/botDirectMessageService.ts';
import {hydrateQueuedAgentReferences} from '../cindy/tasks/agentInputReferences.ts';
import {projectAgentFacingText,readAgentInputReferences} from '../cindy/tasks/core/agentInputProjection.ts';
import {readReplyQuoteNote} from '../cindy/bots/chatReplyQuoteNote.ts';
import {readChatReplyQuote} from '../cindy/bots/shared/chatReplyQuote.ts';
import {admitTaskAction,NEEDS_OWNER_REQUEST,TaskActionAuthorizationError} from '../cindy/tasks/taskActionAuthorization.ts';
import {formatPrStatuses} from '../cindy/bots/taskDigest.ts';
import {buildWorkingMemoryRefreshNote,pruneWorkingMemoryForMessages,readBotWorkingMemory,renderWorkingMemory,WORKING_MEMORY_REFRESH_TERMINATOR} from '../cindy/bots/botWorkingMemory.ts';
import {readBranch} from './history.mjs';
import {learningTranscript} from './learning-data.mjs';
import {createOneBotGroups} from './groups-host.mjs';
import {sessions,messages,botDelegations,botDirectMessages} from '../cindy/tasks/db/schema.ts';
import zh from '../cindy/tasks/locales/zh-CN.json';
import en from '../cindy/tasks/locales/en.json';

/** Cindy listOpenTasks: at most this many background tasks still running. */
const OPEN_TASKS_LIMIT=8;
const messageText=content=>typeof content==='string'?content:(content??[]).filter(part=>part.type==='text').map(part=>part.text).join('\n');
/**
 * Cindy's working memory refresh (makerSendTransaction peek/consume): after a
 * native compaction in a teammate's main chat, the owner's next message
 * carries the host ledger once, without entries whose source left the branch.
 * It counts as delivered once a message carrying it was accepted after that
 * compaction, so a send that never arrived does not consume it. A missing or
 * unreadable ledger never blocks the message (Cindy reads it as no ledger).
 */
async function workingMemoryRefresh(owner,taskStore,sessionId,botId){
 try{
  const session=await owner.host.conversation(sessionId),branch=await readBranch(session);
  const compaction=branch.findLastIndex(entry=>entry.type==='compaction');
  if(compaction<0||branch.slice(compaction+1).some(entry=>entry.type==='message'&&entry.message?.role==='user'&&messageText(entry.message.content).includes(WORKING_MEMORY_REFRESH_TERMINATOR)))return null;
  const stored=await readBotWorkingMemory(owner.paths.root,botId);
  if(!stored)return null;
  const live=new Set(learningTranscript(branch).map(row=>row.id));
  const sources=[...stored.threads,...stored.commitments,...stored.waiting,...stored.corrections,...stored.recentlyDone].map(item=>item.messageId).filter(Boolean);
  const withdrawn=new Set(sources.filter(id=>!live.has(id)));
  const memory=withdrawn.size?pruneWorkingMemoryForMessages(stored,withdrawn).memory:stored;
  // Cindy ACTIVE_DELEGATION_STATUSES: queued, running and waiting tasks.
  const openTasks=taskStore.drizzle.select({id:botDelegations.id,objective:botDelegations.objective,status:botDelegations.status}).from(botDelegations)
   .where(and(eq(botDelegations.requestingBotId,botId),inArray(botDelegations.status,['queued','running','waiting']))).limit(OPEN_TASKS_LIMIT).all()
   .map(row=>({id:row.id,objective:row.objective,status:String(row.status)}));
  const rendered=renderWorkingMemory(memory,{openTasks});
  return rendered?buildWorkingMemoryRefreshNote(rendered):null;
 }catch{return null;}
}

export const getDbClient=()=>oneBotOwnerScope().taskStore;
export const captureDataOwnerBroadcastScope=()=>oneBotOwnerScope();
export const getCurrentDbClientSnapshot=()=>{const owner=oneBotOwnerScope();return owner.taskStore?(owner.dbSnapshot??={client:{...owner.taskStore,
 query:async(sql,args)=>{await owner.refreshVoiceProjection?.();return owner.taskStore.query(sql,args);},
 queryOne:async(sql,args)=>{await owner.refreshVoiceProjection?.();return owner.taskStore.queryOne(sql,args);},
}}):null;};
// WinOne has no Cindy cloud homes: a local companion is never a demoted copy
// that runs on another computer.
export const isCompanionShadowed=()=>false;
export const getResolvedMainLocale=()=>oneBotOwnerScope().tasksNative.language==='en-US'?'en':'zh-CN';
export function t(key,values={}){
 const tree=getResolvedMainLocale()==='en'?en:zh;
 const text=key.split('.').reduce((node,part)=>node?.[part],tree);
 if(typeof text!=='string')throw oneBotError('MISSING_TRANSLATION','OneBot 任务文案缺失：'+key);
 return text.replace(/{{([A-Za-z0-9_]+)}}/g,(_match,name)=>String(values[name]??''));
}
export function resolveBusinessSessionId(id){
 if(id)return id;
 // Use WinOne's existing 26-character base32 identity contract for native Chat.
 const bytes=randomBytes(26);return [...bytes].map(byte=>'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'[byte&31]).join('');
}
export function sessionCreateToRow(id,body={},now=Date.now()){
 if(body.agentKind!=='pi'||body.remoteHostId||body.agentDeviceId)throw oneBotError('UNSUPPORTED_ROUTE','OneBot 任务只使用本机 Pi');
 return {id,title:body.title,workingDir:body.workingDir,workspaceKind:body.workspaceKind??'dialogue',model:body.model,providerId:body.providerId??null,agentKind:'pi',
  effort:body.effort??'',fastMode:body.fastMode??false,permissionMode:body.permissionMode??'ask',parentSessionId:body.parentSessionId??null,
  remoteHostId:null,agentDeviceId:null,extraDirs:JSON.stringify(body.extraDirs??[]),source:body.source??'desktop',status:'active',createdAt:now,updatedAt:now};
}
export async function validateTaskModel(route){return oneBotOwnerScope().tasksNative.validateTaskModel(route);}
export async function resolveTaskModelSelection(selection){return oneBotOwnerScope().tasksNative.resolveTaskModelSelection(selection);}
export async function openSession(input,commit){
 const owner=oneBotOwnerScope();
 const route={harness:input.body.agentKind,model:input.body.model,providerId:input.body.providerId,effort:input.body.effort,fastMode:input.body.fastMode};
 if(!await validateTaskModel(route))throw oneBotError('TASK_MODEL_UNAVAILABLE','任务模型不可用，请重新选择');
 const assertCurrent=()=>{if(owner.closed||oneBotOwnerScope()!==owner)throw oneBotError('NOT_READY','OneBot 任务归属已变化');input.assertCurrent?.();};
 assertCurrent();
 const row=sessionCreateToRow(input.id,{...input.body,...input.finalize?.(),source:input.source},input.now);
 const value=await commit(row,assertCurrent);assertCurrent();return {row,value};
}
export async function setWorktreePathInDb(id,worktreePath){getDbClient().drizzle.update(sessions).set({worktreePath}).where(eq(sessions.id,id)).run();}
export async function resolveBotCanonicalSession(botId){const row=oneBotOwnerScope().profiles.findSession(botId);return row?{status:'resolved',sessionId:row.sessionId}:{status:'missing'};}
export async function createBotCanonicalSession(input,beforeCommit){
 const owner=oneBotOwnerScope();
 await beforeCommit?.();
 const result=await owner.tasksNative.createCanonicalSession(input);
 // The native store owns the new chat; refresh the coordinator's projections
 // before its ensureCanonicalSession loop reads them again.
 owner.taskStore.syncProfile(owner.profiles.get(input.botId),owner.profiles.sessions(input.botId));
 const row=await owner.tasksNative.readSession(result.canonicalSessionId);
 owner.taskStore.syncSession({...sessionCreateToRow(row.id,row),...row});
 return result;
}
export async function createMessage(sessionId,input){
 const owner=oneBotOwnerScope();
 // The visible native timeline must accept the anchor before the task starts.
 if(!owner.tasksNative.persistTimelineMessage)throw oneBotError('NOT_READY','OneBot 任务时间线尚未就绪');
 await owner.tasksNative.persistTimelineMessage({sessionId,...input});
 getDbClient().drizzle.insert(messages).values({id:randomUUID(),sessionId,clientId:input.clientId,role:input.role,content:JSON.stringify(input.content),agentMeta:input.agentMeta?JSON.stringify(input.agentMeta):null,createdAt:input.createdAt??Date.now()}).onConflictDoNothing().run();
 // Cindy createMessage returns the saved row (group notes read its id).
 return getDbClient().drizzle.select().from(messages).where(and(eq(messages.sessionId,sessionId),eq(messages.clientId,input.clientId))).get();
}

// Cindy's send_to_agent tool result (lizi xdt-helper okPayload / errorPayload).
function teammateMessagePayload(result){
 if(!result.ok)return {ok:false,errorCode:result.errorCode,data:{...(result.messageId?{message_id:result.messageId}:{}),...(result.availableBots?{available_agents:result.availableBots}:{}),hint:result.message}};
 return {ok:true,action:'send_to_agent',accepted:true,delivered:result.delivered??result.wakeKind!=='queued',replied:false,
  ...(result.transport?{transport:result.transport}:{}),...(result.messageId?{message_id:result.messageId}:{}),
  target_id:result.targetBotId,target_name:result.targetBotName,wake_kind:result.wakeKind,
  guidance:'The host accepted this message. Queued acceptance is not delivery or a reply. End this turn; only an actual incoming message proves a reply.'};
}

export function createOneBotTaskService(owner,native){
 // A group transaction that archives lanes archives their profile links in the same tick.
 let groups;
 const taskStore=createOneBotTaskStore(owner.paths.root,{onArchive:(ids,at)=>groups?.archive(ids,at)}),scope={...owner,taskStore,tasksNative:native};
 // Voice uses the same coordinator projection and native dispatcher. A voice
 // settings read may precede the first task, so expose them when created.
 owner.taskStore=taskStore;owner.tasksNative=native;
 const prs=createOneBotPrService(taskStore);
 const active=new Set();let closing=false;
 const service=withIMScope(scope,()=>createBotDelegationService({...native,
  validateTaskModel,resolveTaskModelSelection,
  readCallerRuntime:id=>taskStore.session(id),
  readCallerPermission:id=>taskStore.session(id)?.permissionMode??null,
  persistTimelineMessage:input=>createMessage(input.sessionId,input),
  onChanged:payload=>{const task=taskStore.drizzle.select().from(botDelegations).where(eq(botDelegations.id,payload.delegationId)).get();owner.event({type:'task-changed',botId:task?.requestingBotId,...payload});},
 }));
 // Cindy send_to_agent: a bounded teammate conversation over each companion's
 // own canonical chat. Delivery uses the native task queue; this computer has
 // no device-link transport, so only local teammates are addressable.
 const directMessages=withIMScope(scope,()=>createBotDirectMessageService({
  dispatch:input=>native.dispatch(input),
  ensureCanonicalSession:(botId,beforeRecovery)=>service.ensureCanonicalSession(botId,beforeRecovery),
  hasQueuedDelivery:(sessionId,clientId)=>native.hasQueuedInput(sessionId,clientId),
  discardQueuedDelivery:async(sessionId,clientId,assertOwner)=>{assertOwner();await native.withdrawQueuedInput(sessionId,clientId);},
  onChanged:payload=>owner.event({type:'direct-message-changed',botId:payload.participantBotIds[0],payload}),
 }));
 const refreshProfile=botId=>{const profile=owner.profiles.get(botId);taskStore.syncProfile(profile,owner.profiles.sessions(botId));};
 // Cindy's local group chats share this store, the native client and the profile mirror.
 groups=withIMScope(scope,()=>createOneBotGroups({scope,native,refreshProfile}));
 const syncSession=row=>taskStore.syncSession({...sessionCreateToRow(row.id,row),...row});
 owner.refreshTaskSession=async sessionId=>{const link=owner.profiles.sessionOwnerOrNull(sessionId);if(link)refreshProfile(link.botId);syncSession(await native.readSession(sessionId));};
 // Teammate messages read every local companion and its canonical chat from
 // this database; mirror the Host-owned rows before each call.
 async function syncRoster(callerSessionId){
  for(const profile of owner.profiles.list()){
   refreshProfile(profile.id);
   const canonical=owner.profiles.sessions(profile.id).find(link=>link.role==='canonical');
   // A missing canonical chat is not mirrored; ensureCanonicalSession repairs it
   // through the Host, which never replaces a conversation that still exists.
   if(canonical&&profile.status==='active'){try{syncSession(await native.readSession(canonical.id));}catch{}}
  }
  if(callerSessionId)syncSession(await native.readSession(callerSessionId));
 }
 async function execute(method,params){
  if(closing)throw oneBotError('NOT_READY','OneBot 任务服务正在关闭');
  if(method.startsWith('tasks.pr.')){
   const task=taskStore.drizzle.select().from(botDelegations).where(eq(botDelegations.childSessionId,params.childSessionId)).get();
   if(!task)throw oneBotError('NOT_FOUND','独立任务不存在');
   if(method==='tasks.pr.record'){
    if(!Array.isArray(params.urls)||params.urls.some(url=>typeof url!=='string'))throw oneBotError('INVALID_PARAMS','PR 引用无效');
    const result=prs.record(params.childSessionId,params.urls);owner.event({type:'task-changed',botId:task.requestingBotId});return result;
   }
   if(method==='tasks.pr.refs')return prs.refs(params.childSessionId);
   if(method==='tasks.pr.statuses')return prs.statuses(params.childSessionId);
   throw oneBotError('UNKNOWN_METHOD','不支持的 PR 操作');
  }
  if(method==='tasks.interaction'){
   if(params.phase==='start')await service.handleInteractionStart(params.childSessionId,params.request);
   else if(params.phase==='end')await service.handleInteractionEnd(params.childSessionId,params.request);
   else throw oneBotError('INVALID_PARAMS','任务交互回执无效');
   return {recorded:true};
  }
  if(method==='tasks.accept'){
   const row=taskStore.drizzle.select().from(botDelegations).where(eq(botDelegations.id,params.delegationId)).get();
   if(!row||row.childSessionId!==params.childSessionId)throw oneBotError('NOT_FOUND','独立任务归属不存在');
   const owned=['start','resume','unpause','interject'].some(kind=>params.requestId===`bot-delegation-${kind}:${row.id}`||params.requestId.startsWith(`bot-delegation-${kind}:${row.id}:`));
   if(!owned)return {accepted:true,owned:false};
   const execution=params.execution;
   if(!execution?.instanceId||!Number.isSafeInteger(execution.generation))throw oneBotError('INVALID_PARAMS','任务输入缺少原生执行身份');
   if(owner.profiles.get(row.requestingBotId).status!=='active')throw oneBotError('INACTIVE','伙伴已暂停或删除，任务输入未放行');
   native.adoptExecution(params.childSessionId,params.requestId,execution);
   await service.acceptQueuedSessionInput(params.childSessionId,params.requestId,undefined,true);
   service.confirmQueuedSessionInputDispatched(params.childSessionId,params.requestId);
   return {accepted:true,owned:true};
  }
  if(method==='tasks.recover'){
   // Reattach durable native receipts after only the OneBot worker restarted.
   // This passive pass never turns an uncertain/unaccepted task into a new run.
   const rows=taskStore.drizzle.select().from(botDelegations).all().filter(row=>['queued','running','waiting'].includes(row.status)||row.completionDeliveredAt===null);
   const botIds=new Set(rows.map(row=>row.requestingBotId));
   for(const botId of botIds)refreshProfile(botId);
   let tracked=0;
   for(const row of rows){
    if(!row.childSessionId||!['running','waiting'].includes(row.status))continue;
    const execution=JSON.parse(row.permissionSnapshotJson).taskExecution;
    if(!execution?.clientId||execution.runSequence!==row.runSequence)continue;
    native.watchAccepted({sessionId:row.childSessionId,requestId:execution.clientId,execution});tracked++;
   }
   for(const botId of botIds)await service.resumeCompletionDelivery(botId);
   // Cindy reconciles teammate-message receipts after a restart without replaying them.
   if(taskStore.drizzle.select({id:botDirectMessages.id}).from(botDirectMessages).where(eq(botDirectMessages.deliveryStatus,'pending')).limit(1).all().length){await syncRoster();await directMessages.restore();}
   return {tracked};
  }
  if(method==='tasks.settle'){
   // Only the private native bridge supplies terminal receipts. Never fall
   // back to the upstream legacy path that lacks an execution identity.
   if(!params.execution||typeof params.execution.instanceId!=='string'||!params.execution.instanceId||!Number.isSafeInteger(params.execution.generation)||!['done','error'].includes(params.outcome))throw oneBotError('INVALID_PARAMS','任务完成回执缺少原生执行身份');
   await service.settleSession(params);return {settled:true};
  }
  if(method==='tasks.input.prepare'){
   // An owner's message in a companion chat (Cindy makerSendTransaction): the
   // reply-quote note and @teammate references become the model text; the
   // owner's text, quote and persistent references are saved for display.
   const inputLink=owner.profiles.sessionOwner(params.sessionId);
   if(typeof params.text!=='string'||params.text.length>100000)throw oneBotError('INVALID_PARAMS','消息内容无效');
   let quote=null,note=null,references=[],modelText=params.text;
   if(params.quote!==undefined&&params.quote!==null){
    quote=readChatReplyQuote(params.quote);
    if(!quote)throw oneBotError('INVALID_PARAMS','引用的消息无效');
    note=await readReplyQuoteNote(params.sessionId,JSON.stringify({replyQuote:quote}));
   }
   if(params.agentReferences!==undefined&&params.agentReferences!==null){
    references=readAgentInputReferences(params.agentReferences,params.text);
    // The composer only references teammates; every span must match its link.
    if(!Array.isArray(params.agentReferences)||references.length!==params.agentReferences.length||references.some(reference=>reference.kind!=='bot'))throw oneBotError('INVALID_PARAMS','伙伴引用无效');
    await syncRoster();
    const hydrated=await hydrateQueuedAgentReferences({text:params.text,persistedContent:'{}',agentReferences:references});
    modelText=projectAgentFacingText({text:params.text,agentReferences:hydrated.agentReferences});
    references=JSON.parse(hydrated.persistedContent).agentReferences??[];
   }
   const refresh=inputLink.role==='canonical'?await workingMemoryRefresh(owner,taskStore,params.sessionId,inputLink.botId):null;
   return {modelText:[refresh,note,modelText].filter(Boolean).join('\n\n'),quote,agentReferences:references};
  }
  if(method==='tasks.directMessage.get'){
   if(typeof params.threadId!=='string'||typeof params.viewerBotId!=='string')throw oneBotError('INVALID_PARAMS','伙伴对话参数无效');
   await syncRoster();
   return directMessages.getThread(params.threadId,params.viewerBotId);
  }
  if(method==='tasks.directMessage.restore'){await syncRoster();await directMessages.restore();return {restored:true};}
  if(method==='tasks.agents.list'||method==='tasks.agents.send'){
   owner.profiles.sessionOwner(params.callerSessionId);
   await syncRoster(params.callerSessionId);
   if(method==='tasks.agents.list'){
    const result=await directMessages.listAgents(params.callerSessionId);
    return result.ok?result:{ok:false,errorCode:result.errorCode,data:{hint:result.message}};
   }
   if(typeof params.target_id!=='string'||typeof params.message!=='string')throw oneBotError('INVALID_PARAMS','伙伴消息参数无效');
   return teammateMessagePayload(await directMessages.messageAgent({callerSessionId:params.callerSessionId,targetBotId:params.target_id,message:params.message.trim()}));
  }
  const linked=owner.profiles.sessionOwner(params.callerSessionId);
  refreshProfile(linked.botId);
  const row=await native.readSession(params.callerSessionId);
  taskStore.syncSession({...sessionCreateToRow(row.id,row),...row});
  if(['tasks.start','tasks.message','tasks.stop'].includes(method)){
   const ownerInput=params.ownerAuthorized===true;
   if(!ownerInput){
    // Cindy taskActionHost.autonomousActionAllowed: on its own initiative a
    // teammate may advance or stop only the background tasks it started;
    // new tasks, permission decisions and resuming a pause need the owner.
    if(params.principal!=='autonomous')throw oneBotError('OWNER_ONLY','创建独立任务需要当前轮次的主人授权');
    const own=method!=='tasks.start'&&taskStore.drizzle.select({botId:botDelegations.requestingBotId}).from(botDelegations).where(eq(botDelegations.id,String(params.delegationId??''))).get()?.botId===linked.botId;
    if(!own||method==='tasks.message'&&['approve','deny','resume'].includes(params.input?.kind))return NEEDS_OWNER_REQUEST;
   }
   const profile=owner.profiles.get(linked.botId),permission=profile.capabilities.permissions;
   const assertCurrentSync=()=>{
    const current=owner.profiles.get(linked.botId),link=owner.profiles.sessionOwner(params.callerSessionId);
    if(closing||owner.closed||current.status!=='active'||current.capabilities.permissions!==permission||link.botId!==linked.botId||link.role!==linked.role)throw new TaskActionAuthorizationError();
   };
   const assertCurrent=async()=>{
    assertCurrentSync();
    // The native lease owns per-turn authorization; a worker/model boolean is
    // insufficient after model admission, worktree preparation or another await.
    if(!native.assertTaskAuthority||await native.assertTaskAuthority({sessionId:params.callerSessionId,requestId:params.requestId,permissionMode:row.permissionMode,delegationId:params.delegationId,action:method,replyKind:params.input?.kind})!==true)throw new TaskActionAuthorizationError();
    assertCurrentSync();
   };
   const tool=method==='tasks.start'?'start_session_task':method==='tasks.message'?'message_session_task':'stop_session_task';
   const admission=await admitTaskAction({server:'cindy_helper',tool,sessionId:params.callerSessionId,args:params},
    {key:owner.key+':'+params.callerSessionId,ownerInput,principal:['owner','entrusted','autonomous'].includes(params.principal)?params.principal:ownerInput?'owner':'autonomous',delegated:false,assertCurrent,assertCurrentSync});
   if(!admission.ok)return admission;
   return admission.run(async()=>{
    if(method==='tasks.start')return service.startSessionTask(params);
    const task=taskStore.drizzle.select().from(botDelegations).where(eq(botDelegations.id,params.delegationId)).get();
    if(task?.requestingBotId===linked.botId&&task.childSessionId)await native.refreshSession?.(task.childSessionId);
    return method==='tasks.message'?service.messageSessionTask(params.callerSessionId,params.delegationId,params.input):service.stopSessionTask(params.callerSessionId,params.delegationId,params.mode);
   });
  }
  if(method==='tasks.trace'){
   if(params.ownerAuthorized!==true||await native.assertTaskAuthority({sessionId:params.callerSessionId,requestId:params.requestId,permissionMode:row.permissionMode})!==true)throw oneBotError('OWNER_ONLY','仅主人可以记录任务补充');
   return service.traceOwnerNote(params.callerSessionId,params.delegationId,params.text);
  }
  if(method==='tasks.get')return service.getSessionTask(params.callerSessionId,params.delegationId,params.queuedMessageId);
  if(method==='tasks.list')return service.listDelegations(params.callerSessionId);
  throw oneBotError('UNKNOWN_METHOD','不支持的 OneBot 任务操作');
 }
 return {
  // Admission can arrive while the native queue-release command is in flight.
  // It must not re-enter that same native command gate to flush its own release.
  call(method,params){const work=withIMScope({...scope,taskRequest:params},async()=>{const result=await execute(method,params);if(method!=='tasks.accept'&&method!=='tasks.interaction')await native.flushHolds?.();return result;});active.add(work);work.then(()=>active.delete(work),()=>active.delete(work));return work;},
  refreshProfile,
  /** Cindy bot-group IPC (local groups): the group service in this owner scope. */
  groups(method,params){const work=withIMScope(scope,()=>groups.call(method,params));active.add(work);work.then(()=>active.delete(work),()=>active.delete(work));return work;},
  /** One-line PR state of a task's recorded pull requests (Cindy get_task_digest pr_status). */
  prStatuses:async sessionId=>formatPrStatuses(await prs.statuses(sessionId)),
  /** Cindy register.ts: a follow-up 'finished' event is skipped when the delegation already delivers it. */
  isCompletionHandledByTeammate:(sessionId,botId,execution)=>withIMScope(scope,()=>service.isCompletionHandledByTeammate(sessionId,{pendingForBotId:botId,execution})),
  async close(){closing=true;await Promise.allSettled([...active]);withIMScope(scope,()=>{groups.dispose();service.dispose();});scope.closed=true;prs.close();taskStore.close();},
 };
}
