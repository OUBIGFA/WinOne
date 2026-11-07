// Use Cindy's target admission, idempotency and terminal receipts unchanged.
// Native Host operations keep ordinary tasks, projects and companions in their
// existing stores; this module never substitutes a capability list for a send.
import {setTimeout as delay} from 'node:timers/promises';
import {PluginTargetSlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/pluginTargetSlot.ts';
import {isPluginTaskPermissionAllowed} from '../cindy/ghost/apps/desktop/src/main/maker-ipc/pluginTaskService.ts';
import {hasPluginTargetApproval,hasPluginDelegationApproval} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/taskCapability.ts';
import {ghostInstallApprovalToken} from '../cindy/ghost/apps/desktop/src/shared/ghost.ts';
import {createPluginTaskReviewResolver} from '../cindy/ghost/apps/desktop/src/main/maker-ipc/pluginTaskReviewContext.ts';
import {newPluginTaskId} from './platform-task-service.mjs';

const known=new Set(['PERMISSION_DENIED','HOST_NOT_READY','INVALID_PARAMS','INVALID_REQUEST','TARGET_UNAVAILABLE','TASK_NOT_FOUND','SUPERSEDED','REQUEST_KEY_CONFLICT','DELIVERY_UNCERTAIN','TASK_BUSY','INVALID_ATTACHMENTS','UNSUPPORTED_CAPABILITY','STALE_REVISION','REVISION_CONFLICT','ROUTE_UNAVAILABLE']);
const fail=code=>{throw Object.assign(Error(code),{code});};
export function createGhostTargets({find,current,host,taskService,log}){
 const slot=new PluginTargetSlot(),stop=new AbortController(),watches=new Map(),operations=new Set();
 const native=(method,params={})=>host('task',{op:'plugin-target',method,...params},stop.signal);
 const guardFor=id=>{const captured=find(id);return()=>{if(stop.signal.aborted||!captured||!current(captured)||!hasPluginTargetApproval(find(id)))fail('PERMISSION_DENIED');};};
 const store=()=>taskService.targetStore();
 function watch(receipt){
  if(!receipt?.taskId||receipt.terminal||watches.has(receipt.id)||stop.signal.aborted)return;
  const work=(async()=>{
   let cursor=0;
   while(!stop.signal.aborted){
    try{
     const result=await native('wait',{pluginId:receipt.pluginId,taskId:receipt.taskId,requestId:receipt.id,cursor});cursor=result.cursor;
     if(!result.terminal)continue;
     const terminal=result.terminal;
     if(!terminal.execution)fail('DELIVERY_UNCERTAIN');
     await store().settle(receipt.id,receipt.taskId,{...terminal.execution,outcome:terminal.status==='stopped'?'cancelled':terminal.status,...(terminal.outputMessageId?{outputMessageId:terminal.outputMessageId}:{})});
     return;
    }catch(error){
     if(stop.signal.aborted)return;
     log.warn('Plugin target receipt remains unconfirmed: '+String(error.code??error.name));
     if(['TARGET_UNAVAILABLE','TASK_NOT_FOUND'].includes(error.code))return;
     await delay(1000,undefined,{signal:stop.signal});
    }
   }
  })().catch(error=>{if(!stop.signal.aborted)log.warn(error.message);}).finally(()=>watches.delete(receipt.id));
  watches.set(receipt.id,work);
 }
 slot.adapter={
  store,allocateTaskId:newPluginTaskId,
  approvalRevision:id=>hasPluginTargetApproval(find(id))?ghostInstallApprovalToken(find(id).approval):null,
  delegationRevision:id=>hasPluginDelegationApproval(find(id))?ghostInstallApprovalToken(find(id).approval):null,
  // Every Auto action resolves the live Host receipt through the original
  // delegated-scope resolver below; capabilities alone grant no execution.
  delegationReviewAvailable:()=>true,
  candidates:kind=>native('list',{targetKind:kind}),
  session:(kind,id)=>native('session',{targetKind:kind,targetId:id}),
  assertCreatePermission:(id,mode)=>{const limit=taskService.permissionLimit(id);if(limit!==undefined&&!isPluginTaskPermissionAllowed(mode,limit))fail('PERMISSION_DENIED');},
  assertProjectDispatch:(id,taskId)=>taskService.assertProjectDispatch(id,taskId),
  create:(...args)=>taskService.createTarget(...args),
  send:async(pluginId,taskId,clientId,text,guard,validateTarget,delivery,images=[],options={})=>{
   await validateTarget();guard();
   const receipt=await store().getById(clientId);
   const result=await native('send',{pluginId,taskId,clientId,text,delivery,context:options.context,delegated:!!receipt?.delegation,
    images:images.map(image=>({name:image.name,mimeType:image.mimeType,hash:image.hash,data:image.buffer.toString('base64')}))});
   if(!result.accepted||!['queue','steer'].includes(result.delivery))fail('DELIVERY_UNCERTAIN');
   watch(await store().getById(clientId));
   return result.delivery;
  },
  withdraw:async(taskId,clientId,guard)=>{guard();await native('withdraw',{taskId,requestId:clientId});},
  deliverySettled:async(taskId,clientId,guard)=>{guard();const result=await native('settled',{taskId,requestId:clientId});guard();return result.settled===true;},
  scheduleAutoTitle:(pluginId,taskId,text)=>{void host('task',{op:'plugin-native',method:'auto-title',pluginId,taskId,text},stop.signal).catch(error=>log.warn(error.message));},
 };
 return {
  handle(id,request){
   const work=(async()=>{
    try{
     const data=await slot.handle(id,request,guardFor(id));
     if(request.kind==='get'&&data.found)watch(await store().get(id,request.requestKey));
     return {ok:true,data};
    }catch(error){const code=known.has(error.code)?error.code:'HOST_NOT_READY';return {ok:false,error:{code,message:code,retryable:['HOST_NOT_READY','TASK_BUSY','DELIVERY_UNCERTAIN'].includes(code)}};}
   })();
   operations.add(work);void work.finally(()=>operations.delete(work));return work;
  },
  async review(params){
   const receipt=await store().getById(params.requestId);
   const resolve=createPluginTaskReviewResolver(async()=>{
    if(!receipt?.delegation)return null;
    const guard=guardFor(receipt.pluginId);guard();
    await slot.assertQueuedDelivery(params.sessionId,params.requestId);guard();
    const value=await native('review',{taskId:params.sessionId,requestId:params.requestId,execution:params.execution});guard();
    const latest=await store().getById(params.requestId);
    if(JSON.stringify(latest)!==JSON.stringify(receipt))fail('PERMISSION_DENIED');
    const revision=slot.adapter.delegationRevision(receipt.pluginId),ceiling=taskService.permissionLimit(receipt.pluginId);
    if(!revision||revision!==receipt.approvalRevision)fail('PERMISSION_DENIED');
    return {pluginId:receipt.pluginId,authorized:ceiling===undefined||isPluginTaskPermissionAllowed('auto',ceiling),
     revision:[revision,receipt.id,receipt.hash,params.execution,value.revision,ceiling],registeredRoute:value.session.route,
     plan:{task:receipt.delegation.task,concurrency:1,items:[]},session:value.session,lead:value.session,
     projection:{...value.projection,revision:value.revision},history:[],sessionHistory:[],historyComplete:true};
   });
   return resolve({...params.request,sessionId:params.sessionId});
  },
  async admit(params){await slot.assertQueuedDelivery(params.sessionId,params.requestId);return {accepted:true};},
  async close(){stop.abort();await Promise.allSettled([...operations,...watches.values()]);},
 };
}
