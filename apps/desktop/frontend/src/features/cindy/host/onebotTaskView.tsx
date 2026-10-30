// Local data/identity seam for Cindy's original task card. One subscription is
// shared by every card in the same parent conversation; unmount releases it.
import {useCallback,useEffect,useState,useSyncExternalStore} from 'react';
import {chatCall} from '../../../bridge';
import type {BotCollaborationMeta} from '../../../../../../../modules/chat-pi/cindy/tasks/shared/botCollaboration';
import type {BotDelegationView} from '../../../../../../../modules/chat-pi/cindy/tasks/shared/botDelegation';
import type {ChatActivities,ChatCatalog} from '../../chat-state';
import {conversationToSession} from './sessionService';
import {subscribeOneBotChanges} from './onebotAPI';
import {cindyNavigation} from './store';

type Snapshot={rows:BotDelegationView[];catalog:ChatCatalog|null;activity:ChatActivities;resolved:boolean;stale:boolean};
const empty:Snapshot={rows:[],catalog:null,activity:{},resolved:false,stale:false};
const views=new Map<string,ReturnType<typeof createView>>();
export const isActiveDelegationStatus=(status:string)=>['queued','running','waiting'].includes(status);
function createView(parentId:string){
 let value=empty,loading:Promise<void>|undefined,again=false,timer:ReturnType<typeof setTimeout>|undefined,off:undefined|(()=>void);
 const listeners=new Set<()=>void>();
 const publish=(next:Snapshot)=>{value=next;for(const listener of listeners)listener();};
 const load=()=>{
  if(loading){again=true;return loading;}
  if(timer)clearTimeout(timer);
  loading=(async()=>{
   try{
    const [tasks,catalog,activity]=await Promise.all([chatCall<{delegations:BotDelegationView[]}>('onebot.tasks.list',{callerSessionId:parentId}),chatCall<ChatCatalog>('catalog'),chatCall<ChatActivities>('activity')]);
    publish({rows:tasks.delegations,catalog,activity,resolved:true,stale:false});
   }catch{publish({...value,resolved:true,stale:true});}
   finally{loading=undefined;if(listeners.size){if(again){again=false;void load();}else if(value.stale||value.rows.some(row=>isActiveDelegationStatus(row.status)))timer=setTimeout(()=>void load(),1500);}}
  })();return loading;
 };
 return {read:()=>value,subscribe(listener:()=>void){
  listeners.add(listener);
  if(listeners.size===1){off=subscribeOneBotChanges(event=>{if(event.reset||['task-changed','lifecycle-changed'].includes(event.type??''))void load();});void load();}
  return()=>{listeners.delete(listener);if(!listeners.size){off?.();if(timer)clearTimeout(timer);views.delete(parentId);}};
 }};
}
function useView(parentId:string|null|undefined){
 const subscribe=useCallback((listener:()=>void)=>{
  if(!parentId)return()=>{};
  let view=views.get(parentId);if(!view){view=createView(parentId);views.set(parentId,view);}
  return view.subscribe(listener);
 },[parentId]);
 return useSyncExternalStore(subscribe,()=>parentId?views.get(parentId)?.read()??empty:empty,()=>empty);
}
/** Every delegation started from one conversation (Cindy botDelegationLive.useBotDelegations). */
export function useBotDelegations(sessionId:string|null):readonly BotDelegationView[]{return useView(sessionId).rows;}
export function useBotDelegation(parentId:string|null|undefined,id:string){const view=useView(parentId);return {row:view.rows.find(row=>row.id===id)??null,resolved:view.resolved,stale:view.stale};}
export function useBotTaskSession(card:BotCollaborationMeta,sessionId?:string){
 const view=useView(sessionId??card.parentSessionId),row=view.rows.find(row=>row.id===card.delegationId)??null;
 const targetId=row?.childSessionId??card.childSessionId??null,conversation=view.catalog?.conversations.find(item=>item.id===targetId);
 const state=targetId?view.activity[targetId]:undefined;
 const activity:{workingPhase:string;compactDetail?:string}|null=state?.phase==='running'?{workingPhase:'processing'}:null;
 return {row,resolved:view.resolved,stale:view.stale,targetId,session:conversation?conversationToSession(conversation):null,
  sourceDeviceId:undefined,activity,
  running:state?.phase==='running'||state?.phase==='needs-interaction',originConflict:false,
  open:()=>{if(targetId)cindyNavigation().openConversation(targetId);},
 };
}
// Remote/Cindy-cloud companions are explicitly outside WinOne's product scope.
const noRemoteBots:readonly {deviceId:string;online:boolean}[]=[];
export const useRemoteBots=()=>noRemoteBots;
export function useElementVisible(){
 const [element,ref]=useState<HTMLElement|null>(null),[visible,setVisible]=useState(()=>typeof IntersectionObserver==='undefined');
 useEffect(()=>{if(!element||typeof IntersectionObserver==='undefined')return;const observer=new IntersectionObserver(entries=>setVisible(entries.some(entry=>entry.isIntersecting)));observer.observe(element);return()=>observer.disconnect();},[element]);
 return {ref,visible};
}
