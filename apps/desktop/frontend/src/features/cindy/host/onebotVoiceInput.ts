// OneBot's microphone and question dictation use Cindy's capture/controller
// components. This seam adapts their synchronous snapshot reads to the native
// asynchronous bridge; every mutation still persists in the original store.
import {chatCall} from '../../../bridge';
import {subscribeOneBotChanges} from './onebotAPI';
import {encodeBytes} from './onebotGroupFiles';
import {toast} from '../components/ui/toast';
import {createVoiceInputHistoryEntry, normalizeVoiceInputDataSnapshot, type VoiceInputDataSnapshot} from '../group-shared/shared/voiceInputData';
import type {AudioTrace, VoiceInputRendererEvent} from './onebotVoice';

const call=<T>(method:string,params:unknown={})=>chatCall<T>('onebot.voice.input.'+method,params);
let snapshot=normalizeVoiceInputDataSnapshot(null,'win32'), runId='', epoch=0, audioTail=Promise.resolve();
const dataListeners=new Set<(value:VoiceInputDataSnapshot)=>void>(), inputListeners=new Set<(event:VoiceInputRendererEvent)=>void>();
const historyIds=new Map<string,Promise<string|null>>();
type ActionResult={ok:true;error?:undefined}|{ok:false;error:string};
type PermissionResult=ActionResult&{status?:string};
type StartResult={ok:true;runId:string}|{ok:false;error:string};
let microphoneVerified=false;
const permission=():PermissionResult=>microphoneVerified?{ok:true,status:'granted'}:{ok:false,status:'not-determined',error:'需要麦克风权限'};
const systemPermissions=()=>({microphone:permission(),accessibility:{ok:true as const,status:'not-required'}});
let unsubscribe:(()=>void)|undefined;
function publish(value:VoiceInputDataSnapshot){snapshot=value;for(const listener of dataListeners)listener(value);}
function observe(){if(unsubscribe)return;unsubscribe=subscribeOneBotChanges(event=>{if(event.type==='voice-data'&&event.payload)publish(event.payload as VoiceInputDataSnapshot);if(event.type==='voice-input'&&event.payload)for(const listener of inputListeners)listener(event.payload as VoiceInputRendererEvent);});}
function release(){if(!dataListeners.size&&!inputListeners.size){unsubscribe?.();unsubscribe=undefined;}}
function fail(error:unknown){toast.error(error instanceof Error?error.message:String(error));}
export async function hydrateVoiceData(){const value=await call<VoiceInputDataSnapshot>('data');if(!value?.settings||!Array.isArray(value.history))throw Error('语音设置未能读取');publish(value);}
export function getVoiceInputSettings(){return snapshot.settings;}
export function getVoiceInputHistoryForRefinement(){return snapshot.history;}
export function subscribeVoiceInputSettings(listener:(settings:VoiceInputDataSnapshot['settings'])=>void){const receive=(value:VoiceInputDataSnapshot)=>listener(value.settings);dataListeners.add(receive);observe();return()=>{dataListeners.delete(receive);release();};}
export function recordVoiceInputHistory(text:string):string|null {
 const entry=createVoiceInputHistoryEntry(text);if(!entry)return null;
 publish({...snapshot,history:[entry,...snapshot.history]});
 const pending=call<string|null>('recordHistory',{text});historyIds.set(entry.id,pending);
 void pending.then(()=>hydrateVoiceData()).catch(fail);return entry.id;
}
export function updateVoiceInputHistoryEntry(id:string,text:string){void (async()=>{const nativeId=await (historyIds.get(id)??Promise.resolve(id));if(nativeId){await call('updateHistory',{id:nativeId,text});await hydrateVoiceData();}})().catch(fail);}

export const voiceInputPreload={
 // Benchmark fixtures belong to the explicit test harness, never production.
 getBenchmarkFixtureAudio:async():Promise<{ok:true;wav:ArrayBuffer;path:string}|{ok:false}>=>({ok:false}),
 async updateSettings(patch:unknown) {const settings=await call<VoiceInputDataSnapshot['settings']>('settings',{patch});publish({...snapshot,settings});return settings;},
 getDataSnapshot:()=>snapshot,
 getHistory:(limit?:number)=>typeof limit==='number'?snapshot.history.slice(0,limit):snapshot.history,
 getHistoryForRefinement:()=>snapshot.history,
 recordHistory:recordVoiceInputHistory,updateHistoryEntry:updateVoiceInputHistoryEntry,
 onDataChanged(callback:(value:VoiceInputDataSnapshot)=>void){dataListeners.add(callback);observe();return()=>{dataListeners.delete(callback);release();};},
 onEvent(callback:(event:VoiceInputRendererEvent)=>void){inputListeners.add(callback);observe();return()=>{inputListeners.delete(callback);release();};},
 // OneBot uses the foreground keyboard handler in the original components.
 // There is no Cindy global overlay or system-wide shortcut producer here.
 onGlobalShortcutTrigger:undefined as undefined|((callback:(event:{id?:string;phase?:'start'|'end'|'tap'})=>void)=>()=>void),
 claimGlobalShortcutTrigger:async(_id:string)=>false,
 onPowerStateChange(callback:(event:{reason:'system_suspend'|'screen_locked'})=>void){
  const suspend=()=>callback({reason:'system_suspend'}),lock=()=>callback({reason:'screen_locked'});
  window.addEventListener('winone:voice-system-suspend',suspend);window.addEventListener('winone:voice-screen-locked',lock);
  return()=>{window.removeEventListener('winone:voice-system-suspend',suspend);window.removeEventListener('winone:voice-screen-locked',lock);};
 },
 openMicrophoneSettings:()=>chatCall<ActionResult>('onebot.voice.input.microphoneSettings'),
 getMicrophonePermissionCached:permission,getSystemPermissionsCached:systemPermissions,getSystemPermissions:async()=>systemPermissions(),
 getReadinessCached:():ActionResult|null=>null,
 getReadiness:()=>call<ActionResult>('readiness'),
 setRendererMicrophonePermissionVerified:async(verified:boolean)=>{microphoneVerified=verified;return {ok:true as const};},
 // The capture engine performs the real permission request. A failed capture
 // must not open another probe stream or claim that permission was granted.
 requestMicrophonePermission:async():Promise<PermissionResult>=>permission(),
 async start(params:unknown):Promise<StartResult> {const current=++epoch;audioTail=Promise.resolve();const result=await call<StartResult>('start',params);if(current===epoch&&result.ok)runId=result.runId;return result;},
 appendAudio(chunk:{pcm16k:ArrayBuffer;trace:AudioTrace}) {const current=epoch,active=runId;audioTail=audioTail.then(async()=>{if(current!==epoch||!active)return;await call('audio',{runId:active,pcm:encodeBytes(new Uint8Array(chunk.pcm16k)),trace:chunk.trace});}).catch(error=>{fail(error);for(const listener of inputListeners)listener({type:'error',runId:active,message:String(error)});});},
 drainAudioQueue:()=>audioTail,
 async stop():Promise<ActionResult> {await audioTail;return call('stop',{runId});},
 async cancel(input?:{runId?:string}):Promise<ActionResult> {if(!input?.runId||input.runId===runId){epoch++;runId='';}return call('cancel',input??{});},
 muteSystemAudio:()=>call<{ok:boolean;error?:string}>('mute'),
 restoreSystemAudio:()=>call<{ok:boolean;error?:string}>('restore'),
};
