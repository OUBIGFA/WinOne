// Native Host seam for the unchanged OneBot project page. Project paths are
// read-only presentation data; mutations send only the Host-listed choice ID.
import {chatCall} from '../../../bridge';
import type {BotProjectOption} from '../shared/botWorkbench';

/** One Host invalidation. `payload` carries the upstream push body where the page needs one. */
export interface OneBotChange {type?: string; botId?: string; reset?: boolean; payload?: unknown}
interface EventPage {cursor: number; events: OneBotChange[]; reset: boolean}
const listeners=new Set<(event:OneBotChange)=>void>();
let polling=false;
const wait=(milliseconds:number)=>new Promise(resolve=>setTimeout(resolve,milliseconds));
function deliver(event:OneBotChange){for(const listener of [...listeners])listener(event);}

async function pollEvents(){
 if(polling)return;
 polling=true;
 let cursor:number|undefined,failures=0;
 try{
  while(listeners.size){
   try{
    const started=Date.now();
    const page=await chatCall<EventPage>('onebot.events',{cursor:cursor??Number.MAX_SAFE_INTEGER});
    if(!Number.isSafeInteger(page.cursor)||page.cursor<0||!Array.isArray(page.events)||typeof page.reset!=='boolean')throw Error('Invalid OneBot event page');
    if(!listeners.size)break;
    if(cursor===undefined||page.reset||page.cursor<cursor)deliver({reset:true});
    for(const event of page.events)deliver(event);
    // A native long poll waits for changes. A faulty instant empty response
    // must not create a busy loop in the UI or consume the native job slots.
    if(cursor===page.cursor&&!page.events.length&&Date.now()-started<200)await wait(1000);
    cursor=page.cursor;failures=0;
   }catch{
    cursor=undefined;
    // Reloading lets the original page show its read failure while retaining
    // the displayed links. Recovery also reloads after the new cursor is set.
    if(listeners.size)deliver({reset:true});
    await wait(Math.min(10000,500*2**Math.min(failures++,4)));
   }
  }
 }finally{
  polling=false;
  if(listeners.size)void pollEvents();
 }
}
export function subscribeOneBotChanges(listener:(event:OneBotChange)=>void){
 listeners.add(listener);void pollEvents();
 return ()=>{listeners.delete(listener);};
}

export const oneBotAPI={
 openExternal:async(url:string)=>{await chatCall('link.open',{url});return {success:true};},
 localDb:{bots:{projects:{
  list:(botId:string)=>chatCall<BotProjectOption[]>('onebot.projects.list',{botId}),
  set:(botId:string,id:string,linked:boolean)=>chatCall<BotProjectOption[]>('onebot.projects.set',{botId,id,linked}),
 }}},
 maker:{
 traceBotDelegationNote:(callerSessionId:string,delegationId:string,text:string)=>chatCall('onebot.tasks.trace',{callerSessionId,delegationId,text}),
 onBotWorkbenchChanged(listener:(event:{botId?:string;reset?:boolean})=>void){
  return subscribeOneBotChanges(event=>{if(event.reset||event.type==='workbench-changed')listener(event);});
 }},
};
