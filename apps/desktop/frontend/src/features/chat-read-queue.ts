// Chat renders many independent file/tool rows. Bound read admission before
// crossing the native 16-job boundary, while leaving stop, approval and writes
// independent. Identical in-flight reads share work; failures are not cached.
export function createChatReadQueue(limit=6,capacity=512){
 if(limit<1||capacity<1)throw Error('Invalid chat read queue capacity');
 let active=0;
 const queued:Array<()=>void>=[];
 const inflight=new Map<string,Promise<unknown>>();
 function pump(){while(active<limit&&queued.length)queued.shift()!();}
 return function read<T>(key:string,operation:()=>Promise<T>):Promise<T>{
  const existing=inflight.get(key);if(existing)return existing as Promise<T>;
  if(queued.length>=capacity)return Promise.reject(Error('文件读取队列已满，请关闭未使用的面板后重试'));
  let resolve!:(value:T)=>void,reject!:(reason:unknown)=>void;
  const result=new Promise<T>((yes,no)=>{resolve=yes;reject=no;});inflight.set(key,result);
  const finish=()=>{active--;if(inflight.get(key)===result)inflight.delete(key);pump();};
  const start=()=>{active++;void Promise.resolve().then(operation).then(value=>{finish();resolve(value);},error=>{finish();reject(error);});};
  queued.push(start);pump();return result;
 };
}
const readMethods=new Set(['file.preview','file.stat','files','files.list','files.index','files.search','resources','review.turn','review.snapshot','review.metadata','review.diff','review.diff.preview','review.file.preview','review.branches','review.commits','project.git']);
const read=createChatReadQueue();
export function scheduleChatCall<T>(method:string,payload:string,call:()=>Promise<T>):Promise<T>{return readMethods.has(method)?read(method+'\0'+payload,call):call();}
