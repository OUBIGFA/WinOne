// IM turns reuse WinOne's existing durable Chat dispatcher and streamed event
// protocol. Channel transports own delivery, not a second model/session engine.
import {createHash} from 'node:crypto';
import {createScheduleDispatcher} from '../scheduler-dispatch.mjs';
import {companionLaneRouteKey} from '../cindy/bots/imBotBindingRoute.ts';
import {decodeLaneUserId as telegramLane} from '../cindy/im/telegram/codec.ts';
import {decodeLaneUserId as feishuLane} from '../cindy/im/feishu/codec.ts';
import {decodeLaneUserId as dingtalkLane} from '../cindy/im/dingtalk/codec.ts';
import {decodeWecomLane} from '../cindy/im/wecom/codec.ts';

export function oneBotIMTarget(connection,message,ownerUserId){
 let group;
 if(connection.channel==='telegram')group=telegramLane(message.senderId);
 if(connection.channel==='feishu')group=feishuLane(message.senderId);
 if(connection.channel==='dingtalk'){const lane=dingtalkLane(message.senderId);if(lane)group={chatId:lane.conversationId,threadId:''};}
 if(connection.channel==='wecom'){const lane=decodeWecomLane(message.senderId);if(lane.kind==='group')group={chatId:lane.targetId,threadId:''};}
 const ownerAuthorized=message.speaker?message.speaker.isOwner===true:!!ownerUserId&&message.senderId===ownerUserId;
 if(!group&&ownerAuthorized)return {routeKey:'canonical',title:'',ownerAuthorized,group:false};
 const lane=group?{kind:'group',...group}:{kind:'guest-dm',userId:message.senderId};
 return {routeKey:companionLaneRouteKey(connection.channel,message.contextId||connection.botContextId||connection.id,lane),
  title:`${connection.channel} · ${message.interactionSource?.chatName||group?.chatId||message.senderId}`.slice(0,200),ownerAuthorized,group:!!group};
}
function textOf(message){return typeof message?.content==='string'?message.content:(message?.content??[]).filter(part=>part.type==='text').map(part=>part.text??'').join('\n');}
export function createOneBotIMDispatcher({baseUrl,key}){
 const execute=createScheduleDispatcher({baseUrl,key});
 const post=async(route,value,signal)=>{
  const response=await fetch(baseUrl+'/'+route,{method:'POST',headers:{Authorization:'Bearer '+key,'Content-Type':'application/json'},body:JSON.stringify(value),signal});
  if(!response.ok)throw Error((await response.text()).slice(0,2048));return response.json();
 };
 const requestIdFor=(connection,message)=>'im-'+createHash('sha256').update(JSON.stringify([connection.id,message.contextId,message.senderId,message.messageId])).digest('hex');
 const dispatch=async({connection,message,ownerUserId,signal,onText,requestInteraction})=>{
  const target=oneBotIMTarget(connection,message,ownerUserId);
  const requestId=requestIdFor(connection,message);
  if(message.protectedContent===true){
   const response=await fetch(baseUrl+'/im/protected',{method:'POST',headers:{Authorization:'Bearer '+key,'Content-Type':'application/json'},body:JSON.stringify({connection,message,ownerUserId,requestId,...target}),signal});
   if(!response.ok)throw Error((await response.text()).slice(0,2048));const decoder=new TextDecoder();let buffer='',result;
   for await(const bytes of response.body){buffer+=decoder.decode(bytes,{stream:true});let end;while((end=buffer.indexOf('\n'))>=0){const line=buffer.slice(0,end);buffer=buffer.slice(end+1);if(!line)continue;const item=JSON.parse(line);if(item.approval){const answer=await requestInteraction(item.approval);await post('im/interaction',{sessionId:item.sessionId,requestId,id:item.approval.id,kind:item.approval.kind,allow:answer.allow===true,answers:answer.answers,cancel:answer.cancel===true},signal);}if(item.result)result=item.result;if(item.error)throw Error(item.error);}}
   if(!result)throw Error('受保护消息未返回完整结果');return result;
  }
  const routed=await post('im/route',{botId:connection.botId,channel:connection.channel,connectionId:connection.id,requestId,...target,prompt:message.text??'',protectedContent:message.protectedContent===true,
   attachments:(message.attachments??[]).map(attachment=>attachment.absPath)},signal);
  const handled=new Set();let streamedText='';
  const answer=await execute({id:requestId,name:connection.channel,prompt:message.text??'',targetSessionId:routed.sessionId},
   {runId:requestId,signal,userPrompt:message.text??'',attachments:routed.attachments,fullResult:true,
    onEvents:async page=>{
     for(const {data:event} of page.events??[]){
      if(event.type==='delta'&&event.kind==='text_delta'){streamedText+=event.text??'';onText?.(streamedText);}
      if((event.type==='message_update'||event.type==='message_end'||event.type==='pi_event'&&event.eventType==='message_end')&&event.message?.role==='assistant'){streamedText=textOf(event.message);onText?.(streamedText);}
     }
     for(const request of page.approvals??[]){
      if(request.requestId!==requestId||handled.has(request.id))continue;handled.add(request.id);
      const result=await requestInteraction(request);
      await post('im/interaction',{sessionId:routed.sessionId,requestId,id:request.id,kind:request.kind,allow:result.allow===true,answers:result.answers,cancel:result.cancel===true},signal);
     }
    }});
  return {text:answer.resultText,sessionId:routed.sessionId,allowedFileRoots:routed.allowedFileRoots};
 };
 dispatch.requestId=requestIdFor;
 dispatch.resolve=({connection,message,ownerUserId})=>post('im/resolve',{botId:connection.botId,channel:connection.channel,connectionId:connection.id,...oneBotIMTarget(connection,message,ownerUserId)});
 dispatch.stop=(sessionId,requestId)=>post('stop',{id:sessionId,runId:requestId});
 return dispatch;
}
