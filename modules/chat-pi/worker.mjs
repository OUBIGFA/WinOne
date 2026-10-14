// Versioned JSONL boundary for WinOne's Go Host. Pi retains ownership of agent
// execution and session files; stdout contains protocol records only.
import {once} from "node:events";
import {createHash} from "node:crypto";
import path from "node:path";
import fs from "node:fs/promises";
import {SessionManager} from "@earendil-works/pi-coding-agent";
import {createWinOneSession} from "./session.mjs";
import {createOperationRecorder,readOperations} from "./operations.mjs";
import {prepareAttachments,displayMessage} from './attachments.mjs';
import packageInfo from "./package.json" with {type:"json"};
import {exportConversationArchive} from './archive.mjs';

const maximumLine=1024*1024,maximumQueued=4*1024*1024;
let live,initializing=false,closing=false,writes=Promise.resolve(),queued=0,running=false,abortWanted=false,epoch=0,settledEpoch=0,startedEpoch=0,completion;
let pendingInput,activeTurn,pendingPrompt,sessionDirectory,preflightGeneration=0;
const submittedInputs=[];
let archiveTask,archiveAbort;
const approvals=new Map();
const operations=createOperationRecorder({write:value=>live.session.sessionManager.appendCustomEntry("winone.operation",value),emit:operation=>{void emit({type:"operation",operation}).catch(stopOnOutputError);}});
let thinkingMessage=0;
const thinkingBlocks=new Map();
function thinkingBlock(index){
 let block=thinkingBlocks.get(index);
 if(!block){block={index,startedAt:Date.now(),clock:performance.now()};thinkingBlocks.set(index,block);void emit({type:"thinking_timing",timestamp:thinkingMessage,...block,clock:undefined}).catch(stopOnOutputError);}
 return block;
}
function finishThinking(index){
 const block=thinkingBlocks.get(index);if(!block||block.durationMs!==undefined)return;
 block.durationMs=Math.max(0,Math.round(performance.now()-block.clock));
 void emit({type:"thinking_timing",timestamp:thinkingMessage,...block,clock:undefined}).catch(stopOnOutputError);
}
function withThinking(message,blocks){
 if(!Array.isArray(message.content)||!blocks)return message;
 return {...message,content:message.content.map((part,index)=>{const timing=blocks.find(block=>block.index===index);return part.type==="thinking"&&timing?{...part,thinkingStartedAt:timing.startedAt,thinkingDurationMs:timing.durationMs}:part;})};
}
function emit(value){
 const line=Buffer.from(JSON.stringify(value)+"\n");
 if(line.length>maximumLine||queued+line.length>maximumQueued)throw Error("Chat output exceeded the protocol buffer");
 queued+=line.length;
 const sent=writes.then(async()=>{if(!process.stdout.write(line))await once(process.stdout,"drain");}).finally(()=>{queued-=line.length;});
 writes=sent;return sent;
}
const reply=(id,value)=>emit({type:"response",id,ok:true,value});
const failure=(id,error)=>emit({type:"response",id,ok:false,error:String(error?.message??error)});
function messagePage(manager,{before,limit=30}={}){
 const all=[];const savedInputs=[];const timing=new Map(),identities=new Map();let input,turn,receipt;
 const branch=manager.getBranch();
 for(const entry of branch){
  if(entry.type==="custom"&&entry.customType==="winone.thinking")timing.set(entry.data?.timestamp,entry.data?.blocks);
  if(entry.type==="custom"&&entry.customType==="winone.model")identities.set(entry.data?.timestamp,entry.data?.identity);
  if(entry.type==="custom"&&entry.customType==="winone.input"){input=entry.data?.id;receipt=entry.data;}
  if(entry.type==="message"){
   const message=displayMessage({...entry.message});
   if(message.role==="user")turn=input??entry.id;
   if(turn)message.turnId=turn;
   if(message.role==="user"&&input){message.requestId=input;if(typeof receipt?.text==='string'){message.content=receipt.text;message.attachments=receipt.attachments;}savedInputs.push(input);input=undefined;}
   all.push({...entry,message});
  }
 }
 let end=before?all.findIndex(entry=>entry.id===before):all.length;if(end<0)throw Error("Message cursor is no longer on this branch");
 let start=end,bytes=0,turnCount=0;const messages=[];
 // Tool calls/results belong to their user turn and do not spend the history
 // limit. Keep an ordinary turn on one page; only a turn larger than the wire
 // budget needs cursor-addressable fragments so all history remains readable.
 page:while(start>0&&turnCount<Math.max(1,Math.min(limit,50))){
  const turnId=all[start-1].message.turnId,group=[];let cursor=start,groupBytes=0;
  while(cursor>0&&all[cursor-1].message.turnId===turnId){
   const entry=all[cursor-1],message={...withThinking(entry.message,timing.get(entry.message.timestamp)??(entry.message.timestamp===thinkingMessage?[...thinkingBlocks.values()]:undefined)),...identities.get(entry.message.timestamp),id:entry.id};
   const size=Buffer.byteLength(JSON.stringify(message));
   if(size>600*1024)throw Error("A saved message is too large to display; keep the Pi session file");
   if(bytes+groupBytes+size>600*1024){if(messages.length)break page;break;}
   groupBytes+=size;group.push(message);cursor--;
  }
  bytes+=groupBytes;messages.unshift(...group.reverse());start=cursor;turnCount++;
  if(start>0&&all[start-1].message.turnId===turnId)break;
 }
 const turns=new Set(messages.map(message=>message.turnId));if(running&&activeTurn)turns.add(activeTurn);
 const work=readOperations(branch,turns,running?activeTurn:undefined);
 if(Buffer.byteLength(JSON.stringify({messages,operations:work}))>800*1024)throw Error("Saved process history is too large to display; keep the Pi session file");
 return {messages,operations:work,hasOlder:start>0,before:messages[0]?.id??null,total:all.length,savedInputs:savedInputs.slice(-128)};
}
const inputHash=(text,attachments=[])=>createHash("sha256").update(attachments?.length?JSON.stringify({text,attachments}):text,"utf8").digest("hex");
function inputStatus(id,text,attachments=[]){
 // Accepted follow-ups are durable before Pi consumes them. A reconnect must
 // reconcile this receipt rather than submitting the same input twice.
 const queuedReceipt=pendingReceipts().find(receipt=>receipt.id===id);
 if(queuedReceipt){if(queuedReceipt.textHash!==inputHash(text,attachments))throw Error("This request identifier was already used for a different message");return {saved:true};}
 let receipt;
 for(const entry of live.session.sessionManager.getBranch()){
  if(entry.type==="custom"&&entry.customType==="winone.input")receipt=entry.data;
  if(entry.type==="message"&&entry.message.role==="user"&&receipt){
   if(receipt.id===id){
    // Earlier WinOne receipts had no hash. Compare the user message itself;
    // never accept a reused request id merely because it appears in history.
    const original=typeof entry.message.content==="string"?entry.message.content:entry.message.content.filter(part=>part.type==="text").map(part=>part.text).join("\n");
    if((receipt.textHash??inputHash(original))!==inputHash(text,attachments))throw Error("Request identifier was already used for a different message");
    return {saved:true};
   }
   receipt=undefined;
  }
 }
 return {saved:false};
}
function pendingReceipts(){
 const pending=new Map();let receipt;
 for(const entry of live.session.sessionManager.getBranch()){
  if(entry.type==="custom"&&entry.customType==="winone.queued")pending.set(entry.data.id,entry.data);
  if(entry.type==="custom"&&entry.customType==="winone.input")receipt=entry.data?.id;
  if(entry.type==="custom"&&entry.customType==="winone.cancelled")pending.delete(entry.data?.id);
  if(entry.type==="message"&&entry.message.role==="user"&&receipt){pending.delete(receipt);receipt=undefined;}
 }
 return [...pending.values()];
}
function state(options){
 const value=live.state(),{messages,...summary}=value;
 return {...summary,...messagePage(live.session.sessionManager,options),streaming:running||value.streaming};
}
function event(value){
 if(value.type==="message_start"&&value.message?.role==="user"&&submittedInputs.length){
  const receipt=submittedInputs.shift();pendingInput=receipt.id;pendingPrompt=receipt;activeTurn=receipt.id;
  // Pi's public start event precedes persistence. Place the receipt beside
  // the user message it identifies, even when several messages were queued.
  live.session.sessionManager.appendCustomEntry("winone.input",receipt);
 }
 if(value.type==='notice'){void emit({type:'notice',text:String(value.message??'')}).catch(stopOnOutputError);return;}
 if(value.message){value={...value,message:displayMessage(value.message)};if(value.message.role==='assistant')value.message={...value.message,...live?.modelIdentity()};}
 // The request receipt is the stable identity shared with the immediate user
 // row and messagePage(). Pi's timestamp/history ID alone cannot reconcile it.
 if(value.message?.role==='user'&&pendingPrompt)value={...value,message:{...value.message,requestId:pendingPrompt.id,content:pendingPrompt.text,attachments:pendingPrompt.attachments}};
 if(activeTurn)value={...value,turnId:activeTurn};
 if(value.message&&activeTurn)value={...value,message:{...value.message,turnId:activeTurn}};
 if(value.type==="message_start"&&value.message?.role==="assistant"){thinkingMessage=value.message.timestamp;thinkingBlocks.clear();}
 if(value.type==="message_update"){
  const update=value.assistantMessageEvent;
  if(update?.type==="thinking_start"||update?.type==="thinking_delta")thinkingBlock(update.contentIndex);
  if(update?.type==="thinking_end")finishThinking(update.contentIndex);
  if(update?.type==="text_start")for(const index of thinkingBlocks.keys())finishThinking(index);
 }
 if(value.type==="message_end"&&value.message?.role==="assistant"&&thinkingBlocks.size){
  for(const index of thinkingBlocks.keys())finishThinking(index);
  const blocks=[...thinkingBlocks.values()].map(({clock,...block})=>block);
  live.session.sessionManager.appendCustomEntry("winone.thinking",{timestamp:thinkingMessage,blocks});
  value={...value,message:withThinking(value.message,blocks)};
 }
 if(value.type==="review_start"||value.type==="review_end"){
  if(value.type==="review_start")operations.begin(`review:${value.id}`,"review",activeTurn);else operations.end(`review:${value.id}`);
  void emit({type:"review",id:value.id,running:value.type==="review_start"}).catch(stopOnOutputError);return;
 }
 if(value.type==="compaction_start")operations.begin("compaction","compaction",activeTurn);
 if(value.type==="compaction_end")operations.end("compaction",value.aborted?"stopped":value.errorMessage?"failed":"completed");
 if(value.type==="auto_retry_start"||value.type==="summarization_retry_scheduled")operations.begin(value.type==="auto_retry_start"?"retry":"summary-retry","retry",activeTurn,{attempt:value.attempt,maxAttempts:value.maxAttempts,delayMs:value.delayMs});
 if(value.type==="auto_retry_end")operations.end("retry",value.success?"completed":abortWanted?"stopped":"failed");
 if(value.type==="summarization_retry_finished")operations.end("summary-retry","completed");
 if(value.type==="message_end"&&value.message?.role==="user"&&pendingInput){
  if(pendingPrompt)value={...value,message:{...value.message,content:pendingPrompt.text,attachments:pendingPrompt.attachments}};
  const id=pendingInput;pendingInput=undefined;void emit({type:"input_saved",id}).catch(stopOnOutputError);
 }
 if(value.type==='message_end'&&value.message?.role==='assistant')live.session.sessionManager.appendCustomEntry('winone.model',{timestamp:value.message.timestamp,identity:live.modelIdentity()});
 if(value.type==="agent_start"){startedEpoch=epoch;running=true;}
 if(value.type==="agent_settled"){
  running=false;settledEpoch=epoch;
  const last=live?.session.messages.findLast(message=>message.role==="assistant");
  const status=abortWanted||value.aborted||last?.stopReason==="aborted"?"stopped":last?.stopReason==="error"?"failed":"completed";
  operations.end("turn",status);operations.endAll("stopped");
  void emit({type:"settled",status,error:last?.errorMessage??"",state:state()}).catch(stopOnOutputError);
  return;
 }
 // Deltas are sufficient for live display. Do not repeat the entire growing
 // assistant message for every token or create an unbounded stdout backlog.
 if(value.type==="message_update"){
  const update=value.assistantMessageEvent;
  if(["text_delta","thinking_delta"].includes(update?.type))void emit({type:"delta",kind:update.type,text:update.delta,index:update.contentIndex,turnId:activeTurn,timestamp:thinkingMessage}).catch(stopOnOutputError);
  return;
 }
 if(["message_start","message_end","tool_execution_start","tool_execution_update","tool_execution_end"].includes(value.type)){
  void emit({...value,type:"pi_event",eventType:value.type}).catch(stopOnOutputError);
 }
}
function stopOnOutputError(error){console.error("Pi protocol output:",error.message);void shutdown(1);}
function authorize(request,signal){
 return new Promise((resolve,reject)=>{
  if(signal?.aborted){resolve(false);return;}
  operations.begin(`approval:${request.id}`,"approval",activeTurn);
  const finish=value=>{approvals.delete(request.id);signal?.removeEventListener("abort",abort);operations.end(`approval:${request.id}`,abortWanted||signal?.aborted?"stopped":(typeof value==="object"?value.allow:value)?"allowed":"denied");resolve(value);};
  const abort=()=>{finish(false);void emit({type:"approval_cancelled",id:request.id}).catch(stopOnOutputError);};
  approvals.set(request.id,finish);signal?.addEventListener("abort",abort,{once:true});
  try{void emit({type:"approval",request}).catch(reject);}catch(error){finish(false);reject(error);}
 });
}
async function shutdown(code=0){
 if(closing)return;closing=true;preflightGeneration++;
 for(const resolve of approvals.values())resolve(false);
 try{abortWanted=true;archiveAbort?.abort();if(archiveTask)await archiveTask.catch(()=>{});if(live)await live.close();await completion;await writes;}catch(error){console.error("Pi shutdown:",error.message);code=1;}
 process.exit(code);
}
async function handle(request){
 if(closing)throw Error("Chat is shutting down");
 if(!request||typeof request.id!=="string"||request.id.length>128||typeof request.method!=="string")throw Error("Invalid command envelope");
 const params=request.params??{};
 if(request.method==="export"){
  if(archiveTask)throw Error('An export is already in progress');
  archiveAbort=new AbortController();archiveTask=exportConversationArchive({...params,signal:archiveAbort.signal});
  try{return await reply(request.id,await archiveTask);}finally{archiveTask=undefined;archiveAbort=undefined;}
 }
 if(request.method==="inspect"){
  if(!path.isAbsolute(params.file)||!path.isAbsolute(params.cwd)||!path.isAbsolute(params.directory))throw Error("Invalid session location");
  const manager=SessionManager.open(params.file,params.directory,params.cwd);
  return reply(request.id,{id:manager.getSessionId(),file:manager.getSessionFile(),name:manager.getSessionName()??"",...messagePage(manager,params)});
 }
 if(request.method==="fork"){
  if(running)throw Error("Stop the agent before creating a branch");
  if(!path.isAbsolute(params.file)||!path.isAbsolute(params.cwd)||!path.isAbsolute(params.directory))throw Error("Invalid branch location");
  await fs.mkdir(params.directory,{recursive:true});
  const source=SessionManager.open(params.file,params.directory,params.cwd);
  const leaf=params.entry||source.getLeafId();if(!leaf||!source.getEntry(leaf))throw Error("Branch entry is unavailable");
  const file=source.createBranchedSession(leaf);if(!file)throw Error("Branch was not saved");
  const manager=SessionManager.open(file,params.directory,params.cwd);
  if(params.name)manager.appendSessionInfo(params.name);
  return reply(request.id,{id:manager.getSessionId(),file:manager.getSessionFile(),name:manager.getSessionName()??"",...messagePage(manager)});
 }
 if(request.method==="init"){
  if(live||initializing)throw Error("Session is already initialized or starting");
  initializing=true;try{sessionDirectory=params.sessionDirectory;live=await createWinOneSession(params,{event,authorize});activeTurn=messagePage(live.session.sessionManager).messages.at(-1)?.turnId;
   // Accepted but unconsumed inputs survive worker shutdown and navigation.
   // Re-enter Pi's normal submission path using their original request IDs.
   for(const receipt of pendingReceipts())await handle({id:receipt.id,method:"prompt",params:{text:receipt.text,attachments:receipt.attachments?.map(file=>file.path)}});
   return reply(request.id,state());
  }finally{initializing=false;}
 }
 if(!live)throw Error("Session is not initialized");
 switch(request.method){
  case "state":return reply(request.id,state(params));
  case "resources":return reply(request.id,live.state().resources);
  case "permission":{
   const resolve=approvals.get(params.id);if(!resolve)throw Error("Approval is no longer pending");
   if(typeof params.allow!=="boolean"||![undefined,"","once","session"].includes(params.scope))throw Error("Invalid approval decision");resolve({allow:params.allow,scope:params.scope});return reply(request.id,{resolved:true});
  }
  case "set_permission":live.setPermission(params.value);return reply(request.id,{permission:params.value});
  case "rename":if(running)throw Error("Wait for the current run before renaming");live.session.setSessionName(params.name);return reply(request.id,state());
  case "set_model":if(running)throw Error("Stop the current run before changing models");await live.setModel(params.model,params.models);return reply(request.id,state());
  case "prompt":{
   if(typeof params.text!=="string"||(!params.text.trim()&&!params.attachments?.length)||params.text.length>100000)throw Error("Message is empty or too long");
   const wasRunning=running,generation=wasRunning?preflightGeneration:++preflightGeneration;running=true;if(!wasRunning)abortWanted=false;let attached;
   try{attached=await prepareAttachments(sessionDirectory,params.attachments,live.supportsImages());}
   catch(error){if(!wasRunning&&generation===preflightGeneration)running=false;throw error;}
   // Abort and a later prompt can both arrive while native image work awaits.
   // A cancelled preflight cannot start a turn or reset the newer turn's state.
   if(generation!==preflightGeneration||abortWanted||closing)throw Error('The message was cancelled before submission');
   const receipt={id:request.id,textHash:inputHash(params.text,params.attachments),text:params.text,attachments:attached.attachments};
   live.session.sessionManager.appendCustomEntry("winone.queued",receipt);submittedInputs.push(receipt);
   const turn=wasRunning?epoch:++epoch;if(!wasRunning){activeTurn=request.id;operations.begin("turn","turn",activeTurn);}
   let disposition;const discard=()=>{const index=submittedInputs.indexOf(receipt);if(index>=0)submittedInputs.splice(index,1);live.session.sessionManager.appendCustomEntry("winone.cancelled",{id:receipt.id});};
   const task=live.session.prompt((params.text||'Please inspect the attached files.')+attached.context,{images:attached.images,streamingBehavior:"followUp",preflightResult:value=>{disposition=value;}}).then(async()=>{
    // A handled extension/template command may start no run. For normal
    // runs, only Pi's agent_settled event declares completion after retries.
    if(disposition==="handled")discard();
    if(disposition!=="queued"&&startedEpoch!==turn&&settledEpoch!==turn){running=false;settledEpoch=turn;operations.end("turn");await emit({type:"settled",status:"completed",error:"",state:state()});}
   },async error=>{discard();if(!wasRunning&&settledEpoch!==turn){running=false;settledEpoch=turn;operations.endAll("failed");await emit({type:"settled",status:"failed",error:String(error.message??error),state:state()});}throw error;});
   if(wasRunning)await task;else completion=task.catch(()=>{});
   return reply(request.id,{accepted:true});
  }
  case "retry":{
   if(running)throw Error("Stop the agent before retrying");
   const user=live.session.sessionManager.getEntry(params.entry);
   if(user?.type!=="message"||user.message.role!=="user"||!user.parentId)throw Error("Select a saved user message to retry");
   // Pi owns tree navigation. Its append-only branch keeps the old answer
   // available while the replacement run sends the same user input once.
   const preceding=live.session.sessionManager.getEntry(user.parentId),receipt=preceding?.type==="custom"&&preceding.customType==="winone.input"?preceding.data:undefined;
   if(Object.hasOwn(params,'text')&&(typeof params.text!=='string'||params.text.length>100000))throw Error('Invalid edited message');
   const text=params.text??receipt?.text??(typeof user.message.content==="string"?user.message.content:user.message.content.filter(part=>part.type==="text").map(part=>part.text).join("\n"));
   // Branch directly from the saved user parent. This is Pi's synchronous
   // SessionManager primitive and avoids the interactive tree-navigation hook
   // path, which can reject RPC retries as an unsupported module operation.
   live.session.sessionManager.branch(user.parentId);
   // The earlier input's queued receipt is now before the new branch leaf.
   // Retire it so reopening cannot restore both old and replacement inputs.
   if(receipt?.id)live.session.sessionManager.appendCustomEntry("winone.cancelled",{id:receipt.id});
   return handle({...request,method:"prompt",params:{text,attachments:receipt?.attachments?.map(file=>file.path)}});
  }
  case "input_status":
   if(typeof params.id!=="string"||typeof params.text!=="string"||params.text.length>100000)throw Error("Invalid input receipt query");
   return reply(request.id,inputStatus(params.id,params.text,params.attachments));
  case "steer":if(typeof params.text!=="string"||params.text.length>100000)throw Error("Invalid steering message");return reply(request.id,{disposition:await live.session.steer(params.text)});
  case "abort":{const generation=++preflightGeneration;abortWanted=true;for(const resolve of approvals.values())resolve(false);await live.session.abort();if(generation===preflightGeneration)running=false;return reply(request.id,state());}
  case "compact":{
   if(running)throw Error("Stop the current run before compacting");running=true;abortWanted=false;activeTurn??=live.session.sessionId;
   completion=live.session.compact(params.instructions).then(async()=>{running=false;await emit({type:"settled",status:"completed",error:"",state:state()});},async error=>{running=false;await emit({type:"settled",status:abortWanted?"stopped":"failed",error:String(error.message??error),state:state()});}).catch(stopOnOutputError);
   return reply(request.id,{accepted:true});
  }
  case "close":await reply(request.id,{closing:true});return shutdown();
  default:throw Error("Unsupported Chat command");
 }
}

// Split bytes only on LF. U+2028/U+2029 are legal inside JSON strings and
// must never split a prompt or an agent event into multiple commands.
let buffer=Buffer.alloc(0);
process.stdin.on("data",chunk=>{
 buffer=Buffer.concat([buffer,chunk]);
 for(;;){const newline=buffer.indexOf(10);if(newline<0)break;const line=buffer.subarray(0,newline);buffer=buffer.subarray(newline+1);
  if(line.length>maximumLine){stopOnOutputError(Error("Chat command exceeded the protocol limit"));return;}
  let request;try{request=JSON.parse(line.toString("utf8"));}catch(error){void failure("parse",error).catch(stopOnOutputError);continue;}
  void handle(request).catch(error=>failure(request?.id??"invalid",error)).catch(stopOnOutputError);
 }
 if(buffer.length>maximumLine)stopOnOutputError(Error("Unterminated Chat command exceeded the protocol limit"));
});
process.stdin.on("end",()=>{void shutdown();});
process.stdin.on("error",error=>stopOnOutputError(error));
process.stdout.on("error",error=>stopOnOutputError(error));
void emit({type:"ready",protocol:1,piVersion:packageInfo.dependencies["@earendil-works/pi-coding-agent"]}).catch(stopOnOutputError);
