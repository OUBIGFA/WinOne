import fs from 'node:fs/promises';
import path from 'node:path';
import {once} from 'node:events';
import {createAutomationRuntime} from './automations.mjs';
import {createScheduleDispatcher} from './scheduler-dispatch.mjs';

let runtime,initializing=false,closing=false,writes=Promise.resolve(),bytes=0,storageDirectory,stateTail=Promise.resolve(),lastActive;
function emit(value){const line=Buffer.from(JSON.stringify(value)+'\n');if(line.length>1024*1024||bytes+line.length>4*1024*1024)throw Error('Automation protocol buffer exceeded');bytes+=line.length;const result=writes.then(async()=>{if(!process.stdout.write(line))await once(process.stdout,'drain');}).finally(()=>bytes-=line.length);writes=result;return result;}
function updateActivation(){stateTail=stateTail.catch(()=>{}).then(async()=>{if(!runtime)return;const schedules=await runtime.call('list');const active=schedules.some(item=>item.status==='active'&&!item.manual);if(active===lastActive)return;const file=path.join(storageDirectory,'automations-active.json');await fs.writeFile(file+'.tmp',JSON.stringify({active}));await fs.rename(file+'.tmp',file);lastActive=active;await emit({type:'scheduler_activation',active});});void stateTail.catch(error=>{void emit({type:'scheduler_event',event:{type:'diagnostic',message:error.message}});});}
function event(value){void emit({type:'scheduler_event',event:value}).catch(error=>shutdown(error));if(['changed','completed','failed','skipped'].includes(value.type))updateActivation();}
async function shutdown(error){if(closing)return;closing=true;try{await stateTail;if(runtime)await runtime.close();await writes;}catch(failure){error??=failure;}if(error)console.error('Automation shutdown:',String(error.message??error));process.exit(error?1:0);}
async function handle(request){
 if(!request||typeof request.id!=='string'||typeof request.method!=='string'||request.id.length>128)throw Error('Invalid automation request');
 const params=request.params??{};
 if(request.method==='init'){
  if(runtime||initializing)throw Error('Automation scheduler already initialized');initializing=true;
  try{storageDirectory=params.directory;const dispatch=createScheduleDispatcher(params);runtime=await createAutomationRuntime({directory:params.directory,projectDirectories:params.projectDirectories,dispatch,notify:dispatch.notify,event});updateActivation();await stateTail;return {ready:true};}finally{initializing=false;}
 }
 if(!runtime||closing)throw Error('Automation scheduler is not ready');
 const result=await runtime.call(request.method,params);
 if(['create','update','pause','resume','delete'].includes(request.method)){updateActivation();await stateTail;}
 return result;
}
let buffer=Buffer.alloc(0);
process.stdin.on('data',chunk=>{buffer=Buffer.concat([buffer,chunk]);for(;;){const newline=buffer.indexOf(10);if(newline<0)break;const line=buffer.subarray(0,newline);buffer=buffer.subarray(newline+1);if(line.length>1024*1024){void shutdown(Error('Automation input too large'));return;}let request;try{request=JSON.parse(line.toString('utf8'));}catch(error){void shutdown(error);return;}void handle(request).then(value=>emit({type:'response',id:request.id,ok:true,value}),error=>emit({type:'response',id:request.id,ok:false,error:String(error.message??error)})).catch(shutdown);}if(buffer.length>1024*1024)void shutdown(Error('Automation input too large'));});
process.stdin.on('end',()=>{void shutdown();});process.stdin.on('error',shutdown);process.stdout.on('error',shutdown);
void emit({type:'ready',protocol:1}).catch(shutdown);
