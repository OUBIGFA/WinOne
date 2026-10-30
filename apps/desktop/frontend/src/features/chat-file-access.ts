import {chatCall} from '../bridge';
export interface ChatFileStat {path:string;name:string;directory:boolean;size:number;modified:number}
export interface ChatFileStatResult {requested:string;file?:ChatFileStat;missing?:boolean;error?:string}
type Waiter={path:string;resolve:(value:ChatFileStatResult)=>void;reject:(error:unknown)=>void};
const pending=new Map<string,Map<string,Waiter[]>>();
const active=new Map<string,Promise<ChatFileStatResult>>();
const cache=new Map<string,{at:number;value:ChatFileStatResult}>();
const key=(id:string,path:string)=>id+'\0'+path.replace(/\\/g,'/');
let scheduled=false;
export function invalidateChatFileStats(id?:string){for(const item of cache.keys())if(!id||item.startsWith(id+'\0'))cache.delete(item);}
if(typeof window!=='undefined')window.addEventListener('winone:chat-files-changed',event=>invalidateChatFileStats((event as CustomEvent<{id?:string}>).detail?.id));
async function flush(){
 scheduled=false;const work=[...pending];pending.clear();
 for(const [id,paths] of work){
  const entries=[...paths];
  for(let offset=0;offset<entries.length;offset+=64){
   const batch=entries.slice(offset,offset+64);
   void chatCall<{files:ChatFileStatResult[]}>('file.stat',{id,paths:batch.map(([path])=>path)}).then(response=>{
    if(!response||!Array.isArray(response.files))throw Error('文件检查响应无效，请重新打开此会话');
    const results=new Map(response.files.map(value=>[value.requested,value]));
    for(const [path,waiters] of batch){
     const value=results.get(path);if(!value)throw Error('文件检查结果不完整');
     const identity=key(id,path);cache.delete(identity);cache.set(identity,{at:Date.now(),value});
     while(cache.size>512)cache.delete(cache.keys().next().value!);
     for(const waiter of waiters)waiter.resolve(value);
    }
   }).catch(error=>{for(const [,waiters] of batch)for(const waiter of waiters)waiter.reject(error);});
  }
 }
}
// Cindy-style existence checks: coalesce the visible render's paths into stat
// batches and share in-flight lookups. Preview bytes/tokens are requested only
// when a viewer opens. A short cache avoids duplicate work across sibling rows.
export function inspectChatPath(id:string,path:string,fresh=false):Promise<ChatFileStatResult>{
 const identity=key(id,path),cached=cache.get(identity);
 if(!fresh&&cached&&Date.now()-cached.at<1500)return Promise.resolve(cached.value);
 const prior=active.get(identity);if(prior)return prior;
 const request=new Promise<ChatFileStatResult>((resolve,reject)=>{
  let group=pending.get(id);if(!group){group=new Map();pending.set(id,group);}
  const waiters=group.get(path)??[];waiters.push({path,resolve,reject});group.set(path,waiters);
 });active.set(identity,request);
 void request.finally(()=>{if(active.get(identity)===request)active.delete(identity);}).catch(()=>{});
 if(!scheduled){scheduled=true;queueMicrotask(()=>void flush());}
 return request;
}
export function inspectChatPaths(id:string,paths:string[]){return Promise.all([...new Set(paths)].map(path=>inspectChatPath(id,path)));}
