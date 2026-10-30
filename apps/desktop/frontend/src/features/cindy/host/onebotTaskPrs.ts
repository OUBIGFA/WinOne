// Visibility-owned PR reads for the original task cards. No hidden polling or
// anonymous GitHub fallback; native Cindy services keep gh auth and TTL policy.
import {useCallback,useSyncExternalStore} from 'react';
import {chatCall} from '../../../bridge';
import {subscribeOneBotChanges} from './onebotAPI';
import {PR_STATUS_REFRESH_INTERVAL_MS,prStatusKey,type SessionPrRef,type PrStatusResult} from './onebotTaskShared';
type Snapshot={refs:SessionPrRef[];statuses:Map<string,PrStatusResult>;successfulStatuses:Map<string,PrStatusResult>;refreshError:boolean};
const initial:Snapshot={refs:[],statuses:new Map(),successfulStatuses:new Map(),refreshError:false};
const stores=new Map<string,ReturnType<typeof createStore>>();
function createStore(id:string){
 let state=initial,consumers=0,working:Promise<void>|undefined,again=false,timer:ReturnType<typeof setTimeout>|undefined,off:undefined|(()=>void);
 const listeners=new Set<()=>void>();
 const publish=(next:Snapshot)=>{state=next;for(const listener of listeners)listener();};
 const refresh=()=>{
  if(!consumers)return;
  if(working){again=true;return;}
  if(timer)clearTimeout(timer);
  working=(async()=>{
   try{
    const refs=await chatCall<SessionPrRef[]>('onebot.tasks.pr.refs',{childSessionId:id});
    const results=refs.length?await chatCall<PrStatusResult[]>('onebot.tasks.pr.statuses',{childSessionId:id}):[];
    const statuses=new Map(results.map(result=>[prStatusKey(result),result]));
    const successfulStatuses=new Map(state.successfulStatuses);for(const result of results)if(result.ok)successfulStatuses.set(prStatusKey(result),result);
    publish({refs,statuses,successfulStatuses,refreshError:false});
   }catch{publish({...state,refreshError:true});}
   finally{working=undefined;if(consumers){if(again){again=false;refresh();}else timer=setTimeout(refresh,PR_STATUS_REFRESH_INTERVAL_MS);}}
  })();
 };
 return {read:()=>state,subscribe(listener:()=>void){listeners.add(listener);return()=>{listeners.delete(listener);if(!listeners.size&&!consumers)stores.delete(id);};},refresh,
  register(){consumers++;if(consumers===1){off=subscribeOneBotChanges(event=>{if(event.reset||event.type==='task-changed')refresh();});refresh();}
   return()=>{consumers--;if(!consumers){off?.();if(timer)clearTimeout(timer);if(!listeners.size)stores.delete(id);}};
  },
 };
}
function store(id:string){let value=stores.get(id);if(!value){value=createStore(id);stores.set(id,value);}return value;}
function useSnapshot(id:string){return useSyncExternalStore(useCallback((listener:()=>void)=>id?store(id).subscribe(listener):()=>{},[id]),()=>id?store(id).read():initial,()=>initial);}
export function usePrRefsForSession(id:string){return useSnapshot(id).refs;}
export function usePrStatuses(id:string){return useSnapshot(id);}
const actions={registerPrConsumer:(id:string,_device?:string)=>store(id).register(),invalidateRemotePrRefs:(id:string)=>store(id).refresh()};
export function usePrActions(){return actions;}
