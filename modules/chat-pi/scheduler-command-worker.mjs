import {ScriptScheduleRunner} from './scheduler-script.ts';
import {executePreRunHook} from './scheduler-pre-run-hook.ts';
import {createScriptBroker} from './scheduler-capabilities.mjs';

// One process and one Host-owned Job per invocation. User code never receives
// the private bridge credential or the Host's inherited service credentials.
let initialized=false,running=false,closing=false,context;
const abort=new AbortController();
const emit=value=>process.stdout.write(JSON.stringify(value)+'\n');
const post=async(route,value)=>{
 const response=await fetch(context.baseUrl+'/'+route,{method:'POST',headers:{Authorization:'Bearer '+context.key,'Content-Type':'application/json'},body:JSON.stringify(value),signal:abort.signal});
 if(!response.ok){const error=Error((await response.text()).slice(0,2048));error.code='HOST_CALL_FAILED';throw error;}return response.json();
};
async function handle(request){
 if(request.method==='init'){
  if(initialized)throw Error('Command worker already initialized');
  const endpoint=new URL(request.params.baseUrl);if(endpoint.protocol!=='http:'||endpoint.hostname!=='127.0.0.1'||request.params.key?.length<16)throw Error('Invalid private command bridge');
  context=request.params;initialized=true;return {ready:true};
 }
 if(!initialized||request.method!=='execute'||running||closing)throw Error('Invalid command worker state');
 running=true;
 const {schedule,runId,firedAt,mode,workingDir}=request.params;
 if(mode==='hook')return executePreRunHook({command:schedule.preRunHook.command,timeoutMs:schedule.preRunHook.timeoutMs,cwd:workingDir,signal:abort.signal,stdinPayload:{event:'schedule-pre-run',scheduleId:schedule.id,scheduleName:schedule.name,runId,firedAt,workingDir,lastFinishedAt:schedule.lastFinishedAt}});
 const broker=createScriptBroker({...context,cwd:workingDir,dispatch:params=>post('dispatch',{schedule,runId,call:params})});
 if(mode==='notification'){try{return await broker.notify(schedule.notificationChannel,schedule.notificationText,abort.signal);}finally{await broker.close();}}
 const runner=new ScriptScheduleRunner({
  logger:{warn:(message,detail)=>console.error(message,detail??''),error:(message,detail)=>console.error(message,detail??'')},
  getConversation:id=>post('conversation',{id}),
  scheduler:{pause:(id,options)=>post('retire',{id,runId:options.exemptRunId})},
  broker:{call:(request,granted,callContext)=>broker.call(request,granted,callContext,abort.signal)},
 });
 try{return await runner.fire(schedule,{runId,firedAt,signal:abort.signal,onPreRunHookCompleted:result=>emit({type:'pre_run_hook',result})});}finally{await broker.close();}
}
let buffer='';process.stdin.setEncoding('utf8');
process.stdin.on('data',chunk=>{buffer+=chunk;if(buffer.length>1024*1024){console.error('Command input exceeded limit');process.exit(1);}for(;;){const at=buffer.indexOf('\n');if(at<0)break;const line=buffer.slice(0,at);buffer=buffer.slice(at+1);let request;try{request=JSON.parse(line);}catch{process.exit(1);}void handle(request).then(value=>emit({type:'response',id:request.id,ok:true,value}),error=>emit({type:'response',id:request.id,ok:false,error:String(error.message??error)}));}});
process.stdin.on('end',()=>{closing=true;abort.abort();process.exit(0);});
process.stdout.on('error',()=>process.exit(1));
emit({type:'ready',protocol:1});
