// OneBot host adapter for Cindy's actual IM transport classes and connection
// record store. No replacement Bot API/WebSocket protocol implementation.
import fs from 'node:fs';
import path from 'node:path';
import {createHash,randomUUID} from 'node:crypto';
import {createFeishuIM,createTelegramIM,createDiscordIM,createWecomIM,createDingTalkIM,TELEGRAM_DEFAULT_BEHAVIOR} from '../cindy/im/index.ts';
import {addBotImConnectionRecord,removeBotImConnectionRecord,listBotImConnections,readBotImConnection,secretKeyPrefixFor,setBotImConnectionGroupActivation,updateBotImConnectionRecord} from '../cindy/bots/botImConnectionStore.ts';
import {verifiedInboundMediaMime} from '../cindy/bots/inboundMediaVerify.ts';
import {createOneBotPaths,requireBotId,oneBotError} from './content-service.mjs';
import {createIMSecrets} from './im-secrets.mjs';
import {withIMScope} from './im-scope.mjs';
import {createLogger} from './logger.ts';
import {createOneBotInteractions} from './im-interactions.mjs';
import {createOneBotIMQueue} from './im-queue.mjs';

const secretKeys={
 telegram:['telegram-bot-token','telegram-owner-user-id','telegram-bot-runtime-active','telegram-updates-offset','telegram-bot-offline'],
 feishu:['feishu_bot_app_id','feishu_bot_app_secret','feishu_bot_service','feishu_bot_owner_open_id','feishu_bot_lifecycle_announcement'],
 wecom:['wecom-bot-id','wecom-bot-secret','wecom-owner-user-id'],
 dingtalk:['dingtalk-bot-app-key','dingtalk-bot-app-secret','dingtalk-bot-owner-user-id'],
 discord:['discord-bot-token','discord-owner-user-id','discord-bot-runtime-active','discord-bot-lifecycle-announcement'],
};
const identityKey={telegram:'telegram-bot-token',feishu:'feishu_bot_app_id',wecom:'wecom-bot-id',dingtalk:'dingtalk-bot-app-key',discord:'discord-bot-token'};
function publicIdentity(im,channel){
 const state=im.getConfigState?.()??im.getPublicState?.()??{};
 return {username:state.botUsername??null,ownerUserId:state.ownerUserId??im.getOwnerUserId?.()??null};
}
function factories(host,record){
 const options={secretKeyPrefix:secretKeyPrefixFor(record.id),ipc:false};
 switch(record.channel){
  case 'telegram':return createTelegramIM(host,{...options,guests:true,resolveImageUrl:host.media.resolveMediaUrl,behavior:()=>({...TELEGRAM_DEFAULT_BEHAVIOR,groupActivation:readBotImConnection(record.id)?.groupActivation})});
  case 'feishu':return createFeishuIM(host,{...options,guests:true});
  case 'discord':return createDiscordIM(host,{...options,resolveImageUrl:host.media.resolveMediaUrl});
  case 'wecom':return createWecomIM(host,options);
  case 'dingtalk':return createDingTalkIM(host,{...options,fetcher:fetch});
  default:throw oneBotError('INVALID_PARAMS','未知 IM 渠道');
 }
}

export function createOneBotIMService({directory,masterKey,readBot,dispatch,lanes,event=()=>{},factory=factories}){
 if(typeof dispatch!=='function'||typeof lanes!=='function')throw new TypeError('Real IM dispatch and lane adapters are required');
 const paths=createOneBotPaths(directory),log=createLogger('im'),secrets=createIMSecrets(paths,masterKey,log);
 const queue=createOneBotIMQueue(paths,masterKey),working=new Set();
 const scope={paths,key:randomUUID(),closed:false};
 const live=new Map(),pending=new Set(),tails=new Map(),media=new Map(),staged=new Map();
 const interactions=createOneBotInteractions();
 const ownerId=item=>publicIdentity(item.im,item.record.channel).ownerUserId;
 const isOwner=(item,message)=>message.speaker?message.speaker.isOwner===true:!!ownerId(item)&&message.senderId===ownerId(item);
 const scoped=operation=>withIMScope(scope,operation);
 let closing=false,mutation=Promise.resolve();
 const statusOf=record=>live.get(record.id)?.im.getStatus()??{kind:'idle'};
 const view=record=>({...record,status:statusOf(record),identity:publicIdentity(live.get(record.id)?.im??{},record.channel)});
 function list(botId){return scoped(()=>({connections:listBotImConnections().filter(record=>!botId||record.botId===botId).map(view)}));}
 function changed(){event({type:'im-changed',payload:list()});}
 function stage(buffer,fileName){
  const bytes=Buffer.from(buffer);if(!bytes.length||bytes.length>64*1024*1024)throw Error('IM attachment is empty or too large');
  const basename=path.basename(String(fileName).replaceAll('\\','/')).replace(/[<>:"/\\|?*\x00-\x1f]/g,'-').slice(0,120)||'attachment';
  const folder=path.join(paths.root,'im-media',randomUUID());paths.assertPath(folder);fs.mkdirSync(folder,{recursive:true});
  const absPath=path.join(folder,basename);fs.writeFileSync(absPath,bytes,{flag:'wx',mode:0o600});
  const saved={absPath,discard:async()=>{paths.assertPath(absPath);try{fs.unlinkSync(absPath);fs.rmdirSync(folder);}catch(error){if(error.code!=='ENOENT')throw error;}staged.delete(absPath);for(const [key,value] of media)if(value.absPath===absPath)media.delete(key);}};
  staged.set(absPath,saved);return saved;
 }
 const host={
  secrets,createLogger,
  ipc:{throwIpcError:(code,message)=>{throw oneBotError(code,message);},handle:()=>{throw Error('Companion transports must not register global settings IPC');},broadcast:()=>changed()},
  accountScope:{capture:()=>closing?null:scope.key,isCurrent:token=>!closing&&token===scope.key,run:async(token,operation)=>{if(closing||token!==scope.key)throw Error('IM owner changed');const result=await scoped(operation);if(closing)throw Error('IM owner changed');return result;}},
  paths:{feishuMediaDir:path.join(paths.root,'im-media'),discordMediaDir:path.join(paths.root,'im-media'),telegramMediaDir:path.join(paths.root,'im-media'),wecomMediaDir:path.join(paths.root,'im-media')},
  httpPostForm:async(url,form)=>{const response=await fetch(url,{method:'POST',body:form});return {status:response.status,body:await response.json()};},
  files:{stage:async({buffer,fileName})=>stage(buffer,fileName)},
  media:{
   cacheImage:async({integration,token,buffer,mimeType})=>cacheMedia(integration,token,buffer,verifiedInboundMediaMime(buffer,mimeType,'image')),
   cacheMedia:async({integration,token,buffer,mimeType})=>cacheMedia(integration,token,buffer,verifiedInboundMediaMime(buffer,mimeType,'av')),
   getCachedImage:async(integration,token,options)=>options?.shouldReuse?.()===false?null:media.get(integration+':'+token)??null,
   resolveMediaUrl:url=>{const file=String(url).startsWith('winone-im:')?String(url).slice('winone-im:'.length):null;if(file)paths.assertPath(file);return file;},
  },
 };
 function cacheMedia(integration,token,buffer,mimeType){const saved=stage(buffer,createHash('sha256').update(String(token)).digest('hex')+'.'+mimeType.split('/')[1]);const value={...saved,url:'winone-im:'+saved.absPath,mimeType};media.set(integration+':'+token,value);return value;}
 function track(promise){pending.add(promise);promise.then(()=>pending.delete(promise),error=>{pending.delete(promise);log.warn('IM 处理失败',{message:String(error.message??error)});});return promise;}
 async function receive(item,message){
  // Confirmations must bypass the lane's awaiting turn, otherwise a text-only
  // channel deadlocks behind the very request it is trying to approve.
  if(interactions.consume(item,message))return;
  const queueId=queue.accept(item.record,message);
  if(queueId&&(working.has(queueId)||queue.result(queueId)?.delivered))return;
  if(queueId)working.add(queueId);
  const lane=item.record.id+':'+message.senderId;
  const previous=tails.get(lane)??Promise.resolve();
  const work=previous.catch(()=>{}).then(async()=>{
   if(closing||!live.has(item.record.id))return;
   const owner=await readBot(item.record.botId);if(!owner||owner.status!=='active')return;
   const im=item.im;
   let stream;
   try{
    if(message.unsupported?.length)await im.sendText(message.senderId,'部分附件未能读取：'+message.unsupported.map(value=>value.label).join('、'));
    if(!message.text?.trim()&&!message.attachments?.length)return;
    await im.beginReply?.(message.senderId);
    if(im.startStreamingText)stream=await im.startStreamingText(message.senderId,'');
    const saved=queueId?queue.result(queueId):null;
    const result=saved?.value??await dispatch({connection:item.record,message:{...message,raw:undefined},ownerUserId:ownerId(item),signal:item.abort.signal,onText:text=>stream?.replace(text),
     // The transport's own card-action verification remains authoritative.
     requestInteraction:request=>interactions.request(item,message,request,next=>isOwner(item,next))});
    if(queueId&&!saved?.value)queue.saveResult(queueId,result);
    const text=result.text??'';
    if(message.ambient&&text.trim()==='NO_REPLY'){stream?.close();if(queueId)queue.delivered(queueId);return;}
    if(stream)await stream.finalize(text);
    else if(im.commitFinal)await im.commitFinal({userId:message.senderId,text,terminal:'done',threadTs:message.threadTs,mediaAbsPaths:result.mediaAbsPaths,allowedFileRoots:result.allowedFileRoots});
    else await im.sendMarkdownText(message.senderId,text,{threadTs:message.threadTs});
    for(const file of result.files??[]){const sent=await im.sendFile(message.senderId,file.path,file.name,{threadTs:message.threadTs});if(!sent.ok)throw Error('IM file delivery failed: '+sent.reason);}
    if(queueId)queue.delivered(queueId);
   }catch(error){stream?.close();if(!closing&&!item.abort.signal.aborted)await im.sendText(message.senderId,'本次处理未完成：'+String(error.message??error));throw error;}
   finally{
    // A protected upload is a one-turn staging file, not a persistent media
    // cache. Normal accepted uploads are already copied by the Chat store.
    if(message.protectedContent||queueId&&queue.result(queueId)?.delivered){
     for(const attachment of message.attachments??[])await staged.get(attachment.absPath)?.discard();
    }
   }
  });
  tails.set(lane,work);track(work).finally(()=>{if(queueId)working.delete(queueId);if(tails.get(lane)===work)tails.delete(lane);}).catch(()=>{});
 }
 function instantiate(record){
  const item={record,im:scoped(()=>factory(host,record)),abort:new AbortController(),off:[]};
  item.off.push(item.im.onMessage(message=>{void scoped(()=>receive(item,message));}));
  item.off.push(item.im.onStatusChange(status=>scoped(()=>{
   const identity=item.im.botContextId||status.appId;
   if(status.kind==='connected'&&item.record.botContextId!==identity)track(updateBotImConnectionRecord(record.id,{botContextId:identity}).then(next=>{if(next)item.record=next;changed();}));
   else changed();
  })));
  live.set(record.id,item);return item;
 }
 async function release(id){const item=live.get(id);if(!item)return;live.delete(id);item.abort.abort();for(const off of item.off)off();await scoped(()=>item.im.dispose());}
 function purge(record){for(const name of secretKeys[record.channel])secrets.remove(secretKeyPrefixFor(record.id)+name);}
 async function add(input){
  if(closing)throw Error('IM 服务正在关闭');
  requireBotId(input.botId);if(!Object.hasOwn(secretKeys,input.channel))throw oneBotError('INVALID_PARAMS','未知 IM 渠道');
  const bot=await readBot(input.botId);if(!bot||bot.status!=='active')throw oneBotError('INACTIVE','OneBot 未启用');
  if(!secrets.isAvailable())throw oneBotError('SECRET_UNAVAILABLE','Windows 安全存储不可用');
  const identity=String(input.token??input.appId??input.wecomBotId??input.appKey??'').trim();
  if(!identity)throw oneBotError('INVALID_PARAMS','缺少平台身份');
  for(const record of listBotImConnections())if(record.channel===input.channel&&secrets.read(secretKeyPrefixFor(record.id)+identityKey[input.channel])===identity)throw oneBotError('ALREADY_EXISTS','这个机器人已连接到本机的 OneBot');
  const record=await addBotImConnectionRecord(input);let item;
  try{
   item=instantiate(record);
   let result;
   if(input.channel==='telegram'||input.channel==='discord')result=await item.im.setConfig({token:input.token,ownerUserId:input.ownerUserId});
   if(input.channel==='feishu')result=await item.im.saveCredentials({appId:input.appId,appSecret:input.appSecret,service:input.service??'feishu'});
   if(input.channel==='wecom')result=await item.im.setConfig({botId:input.wecomBotId,secret:input.secret});
   if(input.channel==='dingtalk')result=await item.im.saveCredentials({appKey:input.appKey,appSecret:input.appSecret});
   const status=result?.saveErrorStatus??result?.status??item.im.getStatus();
   if(status.kind==='error'||status.kind==='conflict'||['error','conflict'].includes(result?.verdict)||['telegram','discord'].includes(input.channel)&&status.kind!=='connected')throw Error(status.reason??'平台连接失败');
   if(input.channel==='wecom'&&status.kind==='connecting'){
    const settled=await new Promise((resolve,reject)=>{
     const done=value=>{clearTimeout(timer);off();item.abort.signal.removeEventListener('abort',abort);resolve(value);};
     const timer=setTimeout(()=>done(item.im.getStatus()),15000);
     const off=item.im.onStatusChange(value=>{if(value.kind!=='connecting')done(value);});
     const abort=()=>{clearTimeout(timer);off();reject(Error('IM connection cancelled'));};item.abort.signal.addEventListener('abort',abort,{once:true});
    });
    if(settled.kind==='error'||settled.kind==='conflict')throw Error(settled.reason??'平台连接失败');
   }
   if(closing||(await readBot(input.botId))?.status!=='active')throw Error('OneBot 已停止');
   changed();return {connection:view(item.record),connections:list().connections};
  }catch(error){await release(record.id);purge(record);await removeBotImConnectionRecord(record.id);changed();throw error;}
 }
 function call(method,params={}){
  if(closing)return Promise.reject(Error('IM 服务正在关闭'));
  if(method==='list')return Promise.resolve(list(params.botId));
  const result=mutation.catch(()=>{}).then(()=>scoped(async()=>{
   if(method==='startSaved'){await startSaved();return list();}
   if(method==='add')return add(params.input??params);
   // WinOne has no separate, teammate-less personal-bot configuration to adopt.
   if(method==='listAdoptable')return {bots:[]};
   if(method==='adopt')throw oneBotError('NOT_FOUND','没有可移入的个人机器人，请直接添加平台连接');
   const record=readBotImConnection(params.id);if(!record)throw oneBotError('NOT_FOUND','IM 连接不存在');
   if(method==='remove'){
    const retired=await lanes('retire',{id:record.id});
    const item=live.get(record.id);
    if(item)try{
     if(record.channel==='telegram'||record.channel==='discord')await item.im.disconnect();
     else await item.im.clearCredentials();
    }catch(error){log.warn('IM 平台解绑未确认',{message:String(error.message??error)});}
    await release(record.id);purge(record);queue.removeConnection(record.id);await removeBotImConnectionRecord(record.id);changed();
    return {...list(),archivedSessionIds:retired.archivedSessionIds};
   }
   if(method==='groups.list')return lanes('groups',{id:record.id});
   if(['lanes.list','lanes.reset','lanes.archive'].includes(method)){
    const value=await lanes(method.slice('lanes.'.length),{id:record.id,sessionId:params.sessionId});
    if(method!=='lanes.list')changed();
    return value;
   }
   if(method==='groups.setMode'||method==='groups.setActivation'){
    if(record.channel!=='telegram'||!['always','mention'].includes(params.mode))throw oneBotError('INVALID_PARAMS','群参与模式无效');
    const updated=await setBotImConnectionGroupActivation(record.id,params.chatId,params.mode);
    const active=live.get(record.id);if(active&&updated)active.record=updated;
    changed();return method==='groups.setMode'?list():lanes('groups',{id:record.id});
   }
   const item=live.get(record.id)??instantiate(record);
   if(method==='reconnect'){await item.im.init();return list();}
   if(method==='offline'&&record.channel==='telegram'){await item.im.goOffline();return list();}
   if(method==='online'&&record.channel==='telegram'){await item.im.goOnline();return list();}
   throw oneBotError('UNKNOWN_METHOD','不支持的 IM 操作');
  }));mutation=result;return result;
 }
 async function startSaved(){
  for(const record of scoped(listBotImConnections)){
   if(closing)return;
   const bot=await readBot(record.botId);
   if(bot?.status==='active'&&!live.has(record.id)){
    const item=instantiate(record);
    try{await scoped(()=>item.im.init());for(const pending of queue.pending(record.id))void scoped(()=>receive(item,pending.message));}
    catch(error){log.warn('IM 连接恢复失败',{message:String(error.message??error)});}
   }
  }
 }
 async function close(){if(closing)return;closing=true;await Promise.all([...live.keys()].map(release));await mutation.catch(()=>{});await Promise.allSettled([...pending]);scope.closed=true;queue.close();secrets.close();}
 // These are private lifecycle hooks, not renderer methods. The profile has
 // already changed before resume; do not call back into its locked writer.
 async function pauseBot(botId){requireBotId(botId);for(const record of scoped(listBotImConnections))if(record.botId===botId)await release(record.id);changed();}
 async function resumeBot(botId){requireBotId(botId);for(const record of scoped(listBotImConnections))if(record.botId===botId&&!live.has(record.id)){const item=instantiate(record);await scoped(()=>item.im.init());}changed();}
 async function removeBot(botId){await pauseBot(botId);for(const record of scoped(listBotImConnections))if(record.botId===botId){purge(record);queue.removeConnection(record.id);await scoped(()=>removeBotImConnectionRecord(record.id));}changed();}
 function findIdentity(channel,identity){return scoped(()=>listBotImConnections().find(record=>record.channel===channel&&secrets.read(secretKeyPrefixFor(record.id)+identityKey[channel])===identity));}
 return {call,startSaved,close,pauseBot,resumeBot,removeBot,findIdentity};
}
