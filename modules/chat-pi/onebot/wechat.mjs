// Only the native Pi turn boundary is adapted here. Tencent authorization,
// encrypted cursor/inbox/outbox, text confirmations and retry rules are Cindy's.
import {WechatIM,WECHAT_AUTH_BASE_URL} from '../cindy/wechat/main/im/wechat/WechatIM.ts';
import {TencentIlinkTransport} from '../cindy/wechat-ilink/transport.ts';
import {readWechatChannelSettings,writeWechatCompanionBinding,writeWechatWorkingDir,resetWechatWorkingDir} from '../cindy/wechat/main/im/wechat/channelSettings.ts';
import {createLogger} from './logger.ts';
import {oneBotError} from './content-service.mjs';
import {withIMScope} from './im-scope.mjs';

export function createOneBotWechat(scope,{dispatcher,transportFactory}={}){
 const active=new Map();let closed=false,initializing;
 const settings=()=>readWechatChannelSettings(scope.paths.root);
 const botId=()=>settings().companionBotId;
 const connection=()=>({id:'wechat-local',channel:'wechat',botId:botId()});
 const emitSettings=()=>scope.event({type:'wechat-settings',payload:settings()});
 const host={secrets:scope.importSecrets,createLogger,
  ipc:{broadcast:(channel,payload)=>scope.event({type:channel.includes('state')?'wechat-state':'wechat-settings',payload}),throwIpcError:(code,message)=>{throw oneBotError(code,message);}},
  accountScope:{capture:()=>closed?null:scope.key,isCurrent:key=>!closed&&key===scope.key,run:(_key,run)=>withIMScope(scope,run)},
 };
 const im=new WechatIM({host,getDbClient:()=>scope.taskStore,
  createTransport:transportFactory??(({credentials,onAuthorizationEvent})=>new TencentIlinkTransport({baseUrl:credentials?.baseUrl??WECHAT_AUTH_BASE_URL,...(credentials?{token:credentials.botToken,appId:credentials.ilinkBotId}:{}),botAgent:'WinOne/0.0.1',...(onAuthorizationEvent?{authorizationObserver:{onEvent:onAuthorizationEvent}}:{})})),
  openAuthorizationUrl:url=>scope.host.openWechatAuthorization(url),captureAccountGeneration:()=>closed?null:1,isAccountGenerationCurrent:generation=>!closed&&generation===1,isCompatibilityDisabled:()=>false,
 });
 const message=input=>({contextId:input.botContextId,senderId:input.userId,messageId:input.userMessageId,text:input.text??input.prompt??'',attachments:input.attachments,speaker:{isOwner:im.isOwner(input.userId)}});
 const resolve=async(context,userId)=>{
  const id=botId();if(!id||scope.profiles.get(id).status!=='active')throw oneBotError('INACTIVE','请先把微信连接到已启用的 OneBot');
  const routed=await dispatcher.resolve({connection:connection(),message:{contextId:context,senderId:userId,speaker:{isOwner:im.isOwner(userId)}},ownerUserId:im.isOwner(userId)?userId:''});
  await scope.refreshTaskSession(routed.sessionId);return {row:scope.taskStore.session(routed.sessionId)};
 };
 const stop=async userId=>{const item=active.get(userId);if(item){item.abort.abort();await dispatcher.stop(item.sessionId,item.requestId).catch(error=>{if(!closed)throw error;});}};
 im.attachTurnRuntime({config:{},repo:{prepareNewSession:async()=>({}),findActiveSession:async(context,userId)=>(await resolve(context,userId)).row,createSession:async(context,userId)=>(await resolve(context,userId)).row},
  resetSessionToDefaults:async()=>{throw Error('请在 OneBot 中重启伙伴');},runner:{
   companionBotIdFor:()=>botId(),resolveRouteTarget:resolve,rotateCompanionLane:async()=> 'owner-dm',
   disposeOneSession:async sessionId=>{for(const [userId,item] of active)if(item.sessionId===sessionId)await stop(userId);},
   disposeAllSessions:async()=>{await Promise.allSettled([...active.keys()].map(stop));},
   stopActiveTurn:async input=>stop(input.userId),
   async dispatchAgentTurn(input){
    const route=await resolve(input.botContextId,input.userId);input.onRouteResolved?.(route.row.id);await input.beforeProviderStart();
    const abort=new AbortController(),request=message(input),requestId=dispatcher.requestId(connection(),request),item={abort,sessionId:route.row.id,requestId};active.set(input.userId,item);
    const terminal=(async()=>{
     try{
      const result=await dispatcher({connection:connection(),message:request,ownerUserId:im.isOwner(input.userId)?input.userId:'',signal:abort.signal,
       requestInteraction:async approval=>{
        input.turnPermissionPolicy.onInteractionStateChange?.('waiting');
        try{
         const pending=approval.kind==='question'?{kind:'ask_user_question',requestId:approval.id,questions:approval.questions.map(question=>({question:question.question,options:question.options?.map(label=>({label}))}))}:{kind:'permission',requestId:approval.id,toolName:approval.tool,input:approval.args,autoReviewUnavailable:approval.autoReviewUnavailable};
         const answer=await im.handleTextInteraction(input.userId,pending);
         return approval.kind==='question'?{answers:approval.questions.map(question=>answer.answers?.[question.question]??''),cancel:answer.kind!=='ask_user_question'}:{allow:answer.kind==='permission'&&answer.behavior==='allow'};
        }finally{input.turnPermissionPolicy.onInteractionStateChange?.('resolved');}
       }});
      await im.onUserMessagePersisted({sessionId:route.row.id,userMessageId:input.userMessageId,persisted:true});
      await im.commitFinal({userId:input.userId,text:result.text??'',terminal:'done',allowedFileRoots:result.allowedFileRoots});return {kind:'done'};
     }catch(error){await im.commitFinal({userId:input.userId,text:abort.signal.aborted?'任务已停止。':'本次处理未完成，请在 OneBot 中查看详情。',terminal:abort.signal.aborted?'aborted':'error'});return {kind:'error',errorCode:error.code??'NATIVE_DISPATCH_FAILED'};}
     finally{if(active.get(input.userId)===item)active.delete(input.userId);}
    })();return {kind:'accepted',terminal};
   },
  }});
 const init=()=>initializing??=im.init();
 return {im,async call(method,params={}){
  await init();
  if(method==='state')return im.getState();if(method==='settings')return settings();
  if(method==='authorize'){if(!botId())throw oneBotError('NOT_CANONICAL','请先将微信连接到一位 OneBot');return im.authorize();}
  if(method==='cancelAuthorization'){im.cancelAuthorization();return {ok:true};}
  if(method==='unbind'){await im.unbind();return {ok:true};}
  if(method==='setCompanion'){if(params.botId)scope.profiles.get(params.botId);await im.updateCompanionBinding(()=>writeWechatCompanionBinding(params.botId,scope.paths.root));emitSettings();return settings();}
  if(method==='setWorkingDirectory'){writeWechatWorkingDir(params.path,scope.paths.root);emitSettings();return settings();}
  if(method==='resetWorkingDirectory'){const value=resetWechatWorkingDir(scope.paths.root);emitSettings();return value;}
  throw oneBotError('UNKNOWN_METHOD','不支持的微信操作');
 },async startSaved(){if(!botId()||scope.profiles.get(botId()).status!=='active')return;await init();},async pauseBot(id){if(id===botId()){await im.dispose();initializing=undefined;}},async removeBot(id){if(id===botId()){await im.unbind();writeWechatCompanionBinding(null,scope.paths.root);emitSettings();}},async close(){closed=true;await im.dispose();}};
}
