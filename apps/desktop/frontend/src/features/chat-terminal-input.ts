/** Cindy's pending-input batching adapted to an asynchronous Host transport.
 * Buffer typing during startup, then serialize writes. A failed write is never
 * replayed: the shell may already have consumed part of a command. */
export function createTerminalInput(write:(data:string)=>Promise<unknown>,onError:(error:unknown)=>void){
 let ready=false,closed=false,writing=false,pending='';
 async function flush(){
  if(!ready||closed||writing||!pending)return;
  const data=pending;pending='';writing=true;
  try{await write(data);}catch(error){closed=true;pending='';onError(error);}
  finally{writing=false;if(!closed&&pending)queueMicrotask(()=>void flush());}
 }
 return {
  enqueue(data:string){if(closed)return;pending+=data;queueMicrotask(()=>void flush());},
  attach(){ready=true;void flush();},
  close(){closed=true;pending='';},
 };
}
