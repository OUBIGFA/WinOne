// Private native adapter for the pinned Cindy coordinator. The worker owns the
// task rows; WinOne owns model admission, Pi receipts and ordinary Chat history.
import {eq} from 'drizzle-orm';
import {setTimeout as delay} from 'node:timers/promises';
import {oneBotOwnerScope} from './im-scope.mjs';
import {botDelegations,messages} from '../cindy/tasks/db/schema.ts';
import {randomUUID,createHash} from 'node:crypto';
import {captureAgentRequest} from '../cindy/tasks/forwardedRequest.ts';

export function createOneBotTaskClient({baseUrl,key,language,settle,event,interaction}){
 const stop=new AbortController(),executions=new Map(),inputs=new Map(),watchers=new Map(),states=new Map(),held=new Set(),dirtyHolds=new Set(),interactions=new Map();
 const remember=(sessionId,state)=>{if(state){states.set(sessionId,state);if(state.execution){executions.set(sessionId,state.execution);inputs.set(sessionId,state.requestId);}else{executions.delete(sessionId);inputs.delete(sessionId);}}return state;};
 const inspect=async sessionId=>remember(sessionId,await call('inspect',{sessionId}));
 const flushHold=async sessionId=>{
  const wanted=held.has(sessionId);
  const state=remember(sessionId,await call('hold',{sessionId,held:wanted,execution:executions.get(sessionId)}));
  if(held.has(sessionId)===wanted)dirtyHolds.delete(sessionId);
  return state;
 };
 async function abortSession(sessionId){
  const expected=executions.get(sessionId),requestId=inputs.get(sessionId);
  const state=await inspect(sessionId);
  if(!state.running)return;
  if(!expected||state.execution?.instanceId!==expected.instanceId||state.execution?.generation!==expected.generation)throw Error('任务执行归属已变化，未停止另一条输入');
  await call('abort',{sessionId,requestId,execution:expected});await inspect(sessionId);
 }
 async function queueControl(action,params){
  await params.beforeMutation?.();
  const state=await inspect(params.targetSessionId),owner=oneBotOwnerScope();
  const task=owner.taskStore.drizzle.select().from(botDelegations).where(eq(botDelegations.childSessionId,params.targetSessionId)).get();
  const pending=state.pending.find(input=>input.id===params.queuedMessageId);
  if(!task||!pending||!pending.id.startsWith('bot-delegation-')||!pending.id.includes(':'+task.id))return {ok:false,errorCode:'NOT_OWNED',message:'这条排队消息不属于此伙伴任务'};
  await params.beforeMutation?.();
  remember(params.targetSessionId,await call('queue',{sessionId:params.targetSessionId,requestId:params.queuedMessageId,action,text:params.message}));
  return {ok:true,queuedMessageId:params.queuedMessageId};
 }
 async function call(method,params={}){
  const response=await fetch(baseUrl+'/tasks',{method:'POST',headers:{Authorization:'Bearer '+key,'Content-Type':'application/json'},body:JSON.stringify({method,params}),signal:AbortSignal.any([stop.signal,AbortSignal.timeout(30000)])});
  const result=await response.json();
  if(!response.ok||result.error)throw Object.assign(Error(result.error??'OneBot 任务原生接口失败'),{code:result.errorCode??'NATIVE_TASK_FAILED'});
  return result.value;
 }
 async function observeInteractions(sessionId,state){
  if(!state||!interaction)return;
  const current=new Set();
  for(const [kind,rows] of [['permission',state.approvals??[]],['ask_user_question',state.questions??[]]]){
   for(const native of rows){
    const id='task-interaction:'+createHash('sha256').update(JSON.stringify([sessionId,kind,native.id,native.requestId])).digest('hex').slice(0,24);
    current.add(id);if(interactions.has(id))continue;
    const request={requestId:id,kind,...(kind==='permission'?{toolName:native.tool,description:JSON.stringify(native.args)}:{questions:native.questions.map(question=>({...question,options:question.options?.map(label=>({label}))}))})};
    interactions.set(id,{sessionId,native,request});
    try{await interaction({phase:'start',childSessionId:sessionId,request});}catch(error){interactions.delete(id);throw error;}
   }
  }
  for(const [id,value] of interactions){if(value.sessionId===sessionId&&!current.has(id)){await interaction({phase:'end',childSessionId:sessionId,request:value.request});interactions.delete(id);}}
 }
 // One accepted input's durable terminal: a delegated task settles its
 // coordinator; a group member's turn settles the group (onTerminal).
 function watch(sessionId,requestId,execution,onTerminal){
  const id=sessionId+':'+requestId;if(watchers.has(id)||stop.signal.aborted)return;
  const work=(async()=>{
   let cursor=0,attempt=0;
   while(!stop.signal.aborted){
    try{
     const value=await call('wait',{sessionId,requestId,cursor});cursor=value.cursor;remember(sessionId,value.state);await observeInteractions(sessionId,value.state);
     if(!value.terminal){attempt=0;continue;}
     const actual=value.terminal.execution;
     if(actual?.instanceId!==execution.instanceId||actual?.generation!==execution.generation)throw Error('OneBot 原生结果与任务执行身份不一致');
     if(onTerminal)await onTerminal(value.terminal);
     else await settle({childSessionId:sessionId,execution:actual,outcome:value.terminal.status==='completed'?'done':'error',resultText:value.terminal.resultText,error:value.terminal.error,pendingInputClientIds:[]});
     return;
    }catch(error){
     if(stop.signal.aborted)return;
     if(attempt===0)event({type:'diagnostic',level:'error',scope:'task-receipt',message:'任务回执暂未读取，保留原执行等待重连：'+String(error.message??error)});
     // Retry only receipt reads/settlement, never resubmit the model input.
     await delay(Math.min(30000,1000*2**Math.min(attempt++,5)),undefined,{signal:stop.signal});
    }
   }
  })().catch(error=>{if(!stop.signal.aborted)event({type:'diagnostic',level:'error',scope:'task-receipt',message:String(error.message??error)});}).finally(()=>watchers.delete(id));
  watchers.set(id,work);
 }
 const api={
  language,
  adoptExecution(sessionId,requestId,execution){executions.set(sessionId,execution);inputs.set(sessionId,requestId);states.set(sessionId,{...states.get(sessionId),requestId,execution,running:true,pending:(states.get(sessionId)?.pending??[]).filter(input=>input.id!==requestId)});},
  watchAccepted({sessionId,requestId,execution}){
   executions.set(sessionId,execution);inputs.set(sessionId,requestId);watch(sessionId,requestId,execution);
  },
  /** A group member's turn in its lane: its own terminal receipt settles the group. */
  watchTurn:(sessionId,requestId,execution,onTerminal)=>watch(sessionId,requestId,execution,onTerminal),
  /** Cindy abortLane: stop the lane's current turn and drop the group inputs still queued there. */
  async abortGroupLane(sessionId,isGroupInput){
   const state=await inspect(sessionId);
   for(const input of state.pending)if(isGroupInput(input.id))remember(sessionId,await call('queue',{sessionId,requestId:input.id,action:'cancel'}));
   if(state.running&&state.execution&&state.requestId)remember(sessionId,await call('abort',{sessionId,requestId:state.requestId,execution:state.execution}));
  },
  /** A lane turn waiting on the owner's approval or answer is not late (Cindy hasPendingInteraction). */
  hasPendingInteraction:sessionId=>{const state=states.get(sessionId);return !!(state?.approvals?.length||state?.questions?.length);},
  readSession:sessionId=>call('session',{sessionId}),
  createCanonicalSession:input=>call('canonical',input),
  // Teammate-message deliveries in a canonical chat's durable native queue.
  hasQueuedInput:async(sessionId,clientId)=>(await inspect(sessionId)).pending.some(input=>input.id===clientId),
  withdrawQueuedInput:async(sessionId,clientId)=>{remember(sessionId,await call('queue',{sessionId,requestId:clientId,action:'cancel'}));},
  refreshSession:inspect,
  async flushHolds(){for(const sessionId of [...dirtyHolds])await flushHold(sessionId);},
  readPendingInputClientIds:sessionId=>(states.get(sessionId)?.pending??[]).map(input=>input.id),
  hasPendingInput:sessionId=>(states.get(sessionId)?.pending??[]).length>0,
  taskControl:{
   isActive:sessionId=>{const state=states.get(sessionId);return !!(state?.running||state?.approvals?.length||state?.questions?.length);},
   holdInput(sessionId,value){if(value)held.add(sessionId);else held.delete(sessionId);dirtyHolds.add(sessionId);return [];},
   waitForInputBoundary:flushHold,
   flushInput:flushHold,
   restoreInput:inspect,
   preparePause:flushHold,
   resumeInput:async sessionId=>{await flushHold(sessionId);return remember(sessionId,await call('resume',{sessionId}));},
   async stop(params){await params.beforeMutation?.();await abortSession(params.targetSessionId);return {ok:true,status:'stopped',turnGeneration:executions.get(params.targetSessionId)?.generation};},
   async steer(params){
    await params.beforeMutation?.();
    const task=oneBotOwnerScope().taskStore.drizzle.select().from(botDelegations).where(eq(botDelegations.childSessionId,params.targetSessionId)).get();
    if(!task)return {ok:false,errorCode:'NOT_FOUND',message:'任务不存在'};
    const token=createHash('sha256').update(params.queuedMessageId??randomUUID()).digest('hex').slice(0,32);
    const id=`bot-delegation-interject:${task.id}:steer:${token}`;
    const request=await api.captureRequest(params.callerSessionId,params.message);
    const result=await api.dispatch({targetSessionId:params.targetSessionId,clientId:id,...request,persistedContent:params.message,dispatcherSessionId:params.callerSessionId,deliveryMode:'steer'});
    return result.ok?{ok:true,queuedMessageId:id,turnGeneration:executions.get(params.targetSessionId)?.generation}:result;
   },
  },
  taskQueue:{
   async inspect(sessionId){return (await inspect(sessionId)).pending.map(input=>({queuedMessageId:input.id,consuming:false,message:input.text}));},
   update:params=>queueControl('update',params),cancel:params=>queueControl('cancel',params),
  },
  async discardDelegationQueuedInputs(sessionId,delegationId){
   for(const input of (await inspect(sessionId)).pending){if(input.id.startsWith('bot-delegation-')&&input.id.includes(':'+delegationId))await queueControl('cancel',{targetSessionId:sessionId,queuedMessageId:input.id});}
  },
  validateTaskModel:route=>call('validateModel',{route}),
  resolveTaskModelSelection:selection=>call('resolveModel',{selection}),
  assertTaskAuthority:params=>call('authority',params),
  async captureRequest(sessionId,supplement){
   const requestId=oneBotOwnerScope().taskRequest?.requestId;
   const source=requestId?await call('source',{sessionId,requestId}):null;
   return captureAgentRequest(sessionId,supplement,source,async()=>[],true,source!==null);
  },
  async resolveInteraction(requestId,decision){
   const pending=interactions.get(requestId);if(!pending)return false;
   const state=await inspect(pending.sessionId),kind=pending.request.kind;
   const rows=kind==='permission'?state.approvals:state.questions;
   if(!rows.some(row=>row.id===pending.native.id&&row.requestId===pending.native.requestId))return false;
   let answers;
   if(kind==='ask_user_question')answers=pending.native.questions.map((question,index)=>{
    const answer=decision.answers[question.question]??decision.answers[question.id??String(index)];
    if(typeof answer!=='string'||!answer.trim())throw Error('请回答任务的每个问题');return answer;
   });
   const result=await call('interaction',{sessionId:pending.sessionId,requestId:pending.native.requestId,nativeId:pending.native.id,kind:kind==='permission'?'permission':'question',allow:decision.behavior==='allow',answers});
   return result.resolved===true;
  },
  collectArtifacts:(sessionId,inputClientIds)=>call('artifacts',{sessionId,inputClientIds}),
  persistTimelineMessage:params=>call('timeline',params),
  readSessionExecution:sessionId=>executions.get(sessionId)??null,
  async dispatch(input){
   try{
   const owner=oneBotOwnerScope(),row=owner.taskStore.session(input.targetSessionId);
   const delegation=owner.taskStore.drizzle.select().from(botDelegations).where(eq(botDelegations.childSessionId,input.targetSessionId)).get();
   const {onAccepted,...body}=input;
   if(delegation&&!body.autoReviewUserText)body.autoReviewUserText={kind:'agent-request',request:JSON.parse(delegation.permissionSnapshotJson).taskInput?.requestSource??{userText:null}};
   const interactionTask=input.clientId?.startsWith('bot-delegation-interaction:')?owner.taskStore.drizzle.select().from(botDelegations).where(eq(botDelegations.id,input.clientId.split(':')[1])).get():null;
   const result=await call('dispatch',{...body,...(interactionTask?{replyTaskId:interactionTask.id,replyChildSessionId:interactionTask.childSessionId}:{}),...(delegation?{prepared:{...row,taskId:delegation.id,botId:delegation.requestingBotId}}:{})});
   if(!result.execution)throw Error('OneBot 原生任务缺少执行回执');
   if(!result.queued){executions.set(input.targetSessionId,result.execution);inputs.set(input.targetSessionId,input.clientId);}
   owner.taskStore.drizzle.insert(messages).values({id:randomUUID(),sessionId:input.targetSessionId,clientId:input.clientId,role:'user',content:JSON.stringify(input.persistedContent??input.message),agentMeta:JSON.stringify({autoReviewUserText:input.autoReviewUserText,...(input.dispatcherSessionId?{origin:{kind:'session',senderSessionId:input.dispatcherSessionId}}:{})}),createdAt:Date.now()}).onConflictDoNothing().run();
   if(!result.queued)states.set(input.targetSessionId,{...states.get(input.targetSessionId),running:true});
   // A persisted Pi execution receipt proves native acceptance, even when the
   // JSONL acknowledgement was lost. Do not create the upstream retry alias.
   await onAccepted?.();
   if(delegation)watch(input.targetSessionId,input.clientId,result.execution);
   // A durable queue receipt accepts the message but does not mean the
   // recipient has consumed it. Cindy uses this distinction in its tool result.
   return {ok:true,targetSessionId:input.targetSessionId,wakeKind:result.queued?'queued':result.replayed?'already-active':'created',execution:result.execution};
   }catch(error){
    // The coordinator owns bounded retry/fatal classification. An ambiguous
    // native acknowledgement is retried with the same persisted client ID.
    return {ok:false,errorCode:error.code??'NATIVE_TASK_UNAVAILABLE',message:String(error.message??error)};
   }
  },
  abortSession,
  onChanged:payload=>event({type:'task-changed',...payload}),
  async close(){stop.abort();await Promise.allSettled([...watchers.values()]);},
 };
 return api;
}
