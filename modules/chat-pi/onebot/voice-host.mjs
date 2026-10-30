// Electron/database/network facts for Cindy's voice services. The renderer has
// a fixed local sender identity; only the private Host bridge supplies secrets.
import {randomUUID} from 'node:crypto';
import {oneBotOwnerScope} from './im-scope.mjs';
import {oneBotError} from './content-service.mjs';
export {activeOwnerScopeKey,isAppSessionBoundaryPending,ownerScopedUserDataPath} from './im-scope.mjs';
export const getActiveAppSession=()=>({dataOwnerId:oneBotOwnerScope().key});
export const app={getPath:()=>oneBotOwnerScope().paths.root,isPackaged:true};
export const BrowserWindow={getAllWindows:()=>{const owner=oneBotOwnerScope();return [{isDestroyed:()=>owner.closed,webContents:{send:(channel,payload)=>owner.event({type:channel==='voice-input:data-changed'?'voice-data':'voice-input',payload})}}];}};
export const ipcMain={on(){throw Error('Use the WinOne voice API');},handle(){throw Error('Use the WinOne voice API');}};
export function assertTrustedAppRendererEvent(){throw Error('Electron IPC is not a WinOne entry point');}
export function throwIpcError(code,message){throw oneBotError(code,message);}
export function requireObject(value,name='input'){if(!value||typeof value!=='object'||Array.isArray(value))throwIpcError('INVALID_PARAMS',name+' must be an object');return value;}
export function requireString(value,name='input'){if(typeof value!=='string')throwIpcError('INVALID_PARAMS',name+' must be text');return value;}
export const outboundFetch=(url,options={})=>{const signal=oneBotOwnerScope().voiceInputAbort?.signal;return fetch(url,{...options,...(signal?{signal:options.signal?AbortSignal.any([signal,options.signal]):signal}:{})});};
// BYOK failures never sign the user out of the Cindy application.
export function reportCindyGatewayKeyRejected(){oneBotOwnerScope().event({type:'voice-input',payload:{type:'error',runId:'',message:'语音服务拒绝了当前凭据'}});}
export async function requestExplicitUtilityText(prompt,options={}){
 if(options.signal?.aborted)return {ok:false,reason:'aborted'};
 if(options.beforeDispatch&&await options.beforeDispatch()===false)return {ok:false,reason:'route_changed'};
 const owner=oneBotOwnerScope();
 const input={prompt,responseInstructions:options.systemPrompt??options.responseInstructions,providerId:options.providerId,model:options.model,maxTokens:options.maxTokens,timeoutMs:options.timeoutMs,signal:options.signal};
 const result=options.onText?await owner.host.auxiliaryTextStream({...input,onText:options.onText}):await owner.host.auxiliaryText(input);
 return result;
}
export function extractText(value){if(typeof value==='string'){try{return extractText(JSON.parse(value));}catch{return value;}}if(value&&typeof value.text==='string')return value.text;return Array.isArray(value)?value.filter(part=>part?.type==='text').map(part=>part.text??'').join(String.fromCharCode(10)):'';}
export async function readVoiceCallMemory(botId){const value=await oneBotOwnerScope().content.call('memory.index',{botId});return typeof value==='string'?value:value.index??JSON.stringify(value);}
export async function dispatchVoiceTask(input){const owner=oneBotOwnerScope();await input.authorizationGuard.validate();input.authorizationGuard.assertCurrent();const result=await owner.tasksNative.dispatch(input);return result.ok;}
export async function createMessage(sessionId,input,options){
 const owner=oneBotOwnerScope(), db=owner.taskStore;
 if(options?.shouldBroadcast&&!options.shouldBroadcast())throw Error('call_owner_changed');
 if(!db.queryOne('SELECT id FROM messages WHERE session_id=? AND client_id=?',[sessionId,input.clientId])){
  await owner.host.voiceRecord({sessionId,clientId:input.clientId,record:input.agentMeta.voiceCall,createdAt:input.createdAt});
  db.exec('INSERT INTO messages (id,session_id,client_id,role,content,agent_meta,created_at) VALUES (?,?,?,?,?,?,?)',[randomUUID(),sessionId,input.clientId,input.role,JSON.stringify(input.content),JSON.stringify(input.agentMeta),input.createdAt]);
 }
}
export async function broadcastMessageAgentMetaUpdate(sessionId,clientId){
 const owner=oneBotOwnerScope(),row=owner.taskStore.queryOne('SELECT agent_meta AS meta FROM messages WHERE session_id=? AND client_id=?',[sessionId,clientId]);
 if(!row)return;
 const record=JSON.parse(row.meta).voiceCall;
 await owner.host.voiceRecord({sessionId,clientId,record,update:true});
}
// Platform mute I/O is the Host's Windows Core Audio operation. Ownership and
// restore ordering stay in Cindy's SystemAudioMuteGuard.
export const getMuted=()=>oneBotOwnerScope().host.voiceSystemAudio({}).then(result=>result.muted);
export const setMuted=muted=>oneBotOwnerScope().host.voiceSystemAudio({muted}).then(()=>undefined);
