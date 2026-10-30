// Versioned JSONL boundary for WinOne's Go Host. Pi retains ownership of agent
// execution and session files; stdout contains protocol records only.
import {once} from "node:events";
import {createHash,randomUUID} from "node:crypto";
import {readInputExecution,recordInputExecution} from './input-execution.mjs';
import path from "node:path";
import fs from "node:fs/promises";
import {SessionManager} from "@earendil-works/pi-coding-agent";
import {createDurableSession} from "./durable-session.mjs";
import {createOperationRecorder,readOperations} from "./operations.mjs";
import {prepareAttachments,displayMessage} from './attachments.mjs';
import packageInfo from "./package.json" with {type:"json"};
import {exportConversationArchive} from './archive.mjs';
import {inspectResources} from './resources.mjs';
import {extractPrRefs,messageContentToText} from './cindy/tasks/git/prRefExtractor.ts';

const maximumLine=1024*1024,maximumQueued=4*1024*1024;
let live,initializing=false,closing=false,writes=Promise.resolve(),queued=0,running=false,abortWanted=false,epoch=0,settledEpoch=0,startedEpoch=0,completion;
let pendingInput,activeTurn,pendingPrompt,sessionDirectory,preflightGeneration=0;
let sessionConfig,transition,pausing=false,suspended=false,taskHeld=false;
const submittedInputs=[];
const executionInstance=randomUUID();let executionGeneration=0;
let prWrites=Promise.resolve();const publishedPrs=new Set(),pendingPrs=new Set();
function recordTaskPrs(message){
 if(!sessionConfig?.taskBridge||!['user','assistant'].includes(message?.role))return;
 const urls=extractPrRefs(messageContentToText(message.content)).map(ref=>ref.url).filter(url=>!publishedPrs.has(url)&&!pendingPrs.has(url));
 for(const url of urls)pendingPrs.add(url);
 for(let offset=0;offset<urls.length;offset+=32){const batch=urls.slice(offset,offset+32);
  prWrites=prWrites.then(async()=>{
   const bridge=sessionConfig.taskBridge;
   const response=await fetch(bridge.baseUrl+'/refs',{method:'POST',headers:{Authorization:'Bearer '+bridge.key,'Content-Type':'application/json'},body:JSON.stringify({urls:batch}),signal:AbortSignal.timeout(5000)});
   if(!response.ok)throw Error('PR reference persistence failed');
   for(const url of batch)publishedPrs.add(url);
  }).catch(()=>{void emit({type:'notice',text:'任务 PR 引用暂未保存，原消息已保留，重开任务会重试。'}).catch(stopOnOutputError);}).finally(()=>{for(const url of batch)pendingPrs.delete(url);});
 }
}
function executionState(){
 const receipt=activeTurn?readInputExecution(live.session.sessionManager,activeTurn):null;
 return {requestId:activeTurn??null,execution:receipt?.execution??null,running,paused:live.state().paused,queuePaused:live.state().queuePaused,held:taskHeld,pending:pendingReceipts().map(input=>({id:input.id,text:input.presentation?.text??input.text,originSessionId:input.presentation?.originSessionId??null})),approvals:[...approvals.entries()].map(([id,finish])=>({...finish.request,id,requestId:finish.requestId})),questions:[...questions.values()].map(value=>value.request)};
}
function settled(status,error=''){
 const terminal=recordInputExecution(live.session.sessionManager,activeTurn,status,error);
 return emit({type:'settled',requestId:activeTurn,status,error,...(terminal?{execution:terminal.execution}:{}),state:state()});
}
let archiveTask,archiveAbort;
const approvals=new Map();
const questions=new Map();
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
 const voiceRecords=new Map(branch.filter(entry=>entry.type==='custom'&&entry.customType==='winone.voice').map(entry=>[entry.data.clientId,entry.data.record]));
 for(const entry of branch){
  if(entry.type==="custom"&&entry.customType==="winone.thinking")timing.set(entry.data?.timestamp,entry.data?.blocks);
  if(entry.type==="custom"&&entry.customType==="winone.model")identities.set(entry.data?.timestamp,entry.data?.identity);
  if(entry.type==="custom"&&entry.customType==="winone.input"){input=entry.data?.id;receipt=entry.data;}
  if(entry.type==='custom'&&entry.customType==='winone.timeline'){
   // Host-authored task cards are presentation-only. They survive reopening
   // without entering Pi's prompt context or borrowing a user-input receipt.
   const voiceCall=voiceRecords.get(entry.data.clientId);
   all.push({...entry,message:{...entry.data,...(voiceCall?{agentMeta:{...entry.data.agentMeta,voiceCall}}:{}),id:entry.id,timestamp:entry.data.createdAt,turnId:'timeline:'+entry.data.clientId,presentationOnly:true}});
  }
  if(entry.type==="message"){
   const message=displayMessage({...entry.message});
   if(message.role==="user")turn=input??entry.id;
   if(turn)message.turnId=turn;
   if(message.role==="user"&&input){message.requestId=input;if(typeof receipt?.text==='string'){message.content=receipt.presentation?.text??receipt.text;message.hiddenInput=receipt.presentation?.hidden===true;message.attachments=receipt.attachments;if(receipt.presentation?.replyQuote)message.replyQuote=receipt.presentation.replyQuote;if(receipt.presentation?.agentReferences)message.agentReferences=receipt.presentation.agentReferences;}savedInputs.push(input);input=undefined;}
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
// A companion reply quote (Cindy ChatReplyQuote) and @teammate references are
// display metadata of the owner's input; the Host has already normalised them
// and built the model text.
const validReplyQuote=value=>!!value&&typeof value==='object'&&!Array.isArray(value)&&typeof value.clientId==='string'&&value.clientId.length>0&&value.clientId.length<=256&&(value.role==='user'||value.role==='assistant')&&typeof value.excerpt==='string'&&value.excerpt.length<=1000;
const inputHash=(text,attachments=[],deliveryMode='queue')=>createHash("sha256").update(deliveryMode==='steer'?JSON.stringify({text,attachments:attachments??[],deliveryMode}):attachments?.length?JSON.stringify({text,attachments}):text,"utf8").digest("hex");
function inputStatus(id,text,attachments=[],deliveryMode='queue'){
 // Accepted follow-ups are durable before Pi consumes them. A reconnect must
 // reconcile this receipt rather than submitting the same input twice.
 const queuedReceipt=pendingReceipts().find(receipt=>receipt.id===id);
 if(queuedReceipt){if(queuedReceipt.textHash!==inputHash(text,attachments,deliveryMode))throw Error("This request identifier was already used for a different message");return {saved:true,queued:true,...readInputExecution(live.session.sessionManager,id)};}
 let receipt;
 for(const entry of live.session.sessionManager.getBranch()){
  if(entry.type==="custom"&&entry.customType==="winone.input")receipt=entry.data;
  if(entry.type==="message"&&entry.message.role==="user"&&receipt){
   if(receipt.id===id){
    // Earlier WinOne receipts had no hash. Compare the user message itself;
    // never accept a reused request id merely because it appears in history.
    const original=typeof entry.message.content==="string"?entry.message.content:entry.message.content.filter(part=>part.type==="text").map(part=>part.text).join("\n");
    if((receipt.textHash??inputHash(original))!==inputHash(text,attachments,deliveryMode))throw Error("Request identifier was already used for a different message");
    return {saved:true,...readInputExecution(live.session.sessionManager,id)};
   }
   receipt=undefined;
  }
 }
 return {saved:false};
}
function pendingReceipts(){
 const pending=new Map(),consumed=new Set();let receipt;
 for(const entry of live.session.sessionManager.getBranch()){
  if(entry.type==="custom"&&entry.customType==="winone.queued"){pending.delete(entry.data.id);pending.set(entry.data.id,entry.data);}
  if(entry.type==="custom"&&entry.customType==="winone.input")receipt=entry.data?.id;
  if(entry.type==="custom"&&entry.customType==="winone.cancelled")pending.delete(entry.data?.id);
  if(entry.type==="message"&&entry.message.role==="user"&&receipt){consumed.add(receipt);pending.delete(receipt);receipt=undefined;}
 }
 return [...pending.values()].filter(receipt=>!consumed.has(receipt.id));
}
function state(options){
 const value=live.state(),{messages,...summary}=value,page=messagePage(live.session.sessionManager,options);
 // Queued inputs remain visible on reopen, but are not yet history/branch
 // targets. Acknowledged receipts must not rehydrate as an unsent draft.
 if(!options?.before){
  const pending=pendingReceipts(),saved=new Set(page.messages.map(message=>message.requestId));
  page.messages.push(...pending.filter(receipt=>!saved.has(receipt.id)).map(receipt=>({id:receipt.id,requestId:receipt.id,turnId:receipt.id,role:'user',content:receipt.presentation?.text??receipt.text,hiddenInput:receipt.presentation?.hidden===true,attachments:receipt.attachments,...(receipt.presentation?.replyQuote?{replyQuote:receipt.presentation.replyQuote}:{}),...(receipt.presentation?.agentReferences?{agentReferences:receipt.presentation.agentReferences}:{}),pending:true,queued:receipt.queued===true,deliveryMode:receipt.deliveryMode??'queue',timestamp:receipt.timestamp})));
  page.savedInputs=[...new Set([...page.savedInputs,...pending.map(receipt=>receipt.id)])].slice(-128);
 }
 return {...summary,...page,streaming:!value.paused&&(running||value.streaming)};
}
function event(value){
 if(value.type==='message_end')recordTaskPrs(value.message);
 if(value.type==='input_settled'){
  const terminal=recordInputExecution(live.session.sessionManager,value.requestId,value.status,value.error,value.resultText);
  void emit({...value,...(terminal?{execution:terminal.execution}:{})}).catch(stopOnOutputError);
  return;
 }
 if(value.type==="message_start"&&value.message?.role==="user"){
  const id=value.message.requestId,index=submittedInputs.findIndex(receipt=>receipt.id===id);
  const receipt=index>=0?submittedInputs.splice(index,1)[0]:live.session.sessionManager.getBranch().findLast(entry=>entry.type==='custom'&&entry.customType==='winone.input'&&entry.data?.id===id)?.data;
  pendingInput=receipt?.id;pendingPrompt=receipt;
  const turn=receipt?.id??value.message.id;
  if(turn&&turn!==activeTurn){operations.end('turn');activeTurn=turn;operations.begin('turn','turn',activeTurn);}
 }
 if(value.type==='notice'){void emit({type:'notice',text:String(value.message??'')}).catch(stopOnOutputError);return;}
 if(value.type==='usage'){void emit({type:'usage',usage:value.usage,contextUsage:value.contextUsage}).catch(stopOnOutputError);return;}
 if(value.type==='durable_state'){void emit({type:'durable_state',subagents:value.subagents,workflow:value.workflow,backgroundRunning:!!value.backgroundRunning,paused:!!value.paused,taskGraph:value.taskGraph}).catch(stopOnOutputError);return;}
 if(value.type==='durable_snapshot')return;
 if(value.message){value={...value,message:displayMessage(value.message)};if(value.message.role==='assistant')value.message={...value.message,...live?.modelIdentity()};}
 // The request receipt is the stable identity shared with the immediate user
 // row and messagePage(). Pi's timestamp/history ID alone cannot reconcile it.
 if(value.message?.role==='user'&&pendingPrompt)value={...value,message:{...value.message,requestId:pendingPrompt.id,content:pendingPrompt.presentation?.text??pendingPrompt.text,hiddenInput:pendingPrompt.presentation?.hidden===true,attachments:pendingPrompt.attachments,...(pendingPrompt.presentation?.replyQuote?{replyQuote:pendingPrompt.presentation.replyQuote}:{}),...(pendingPrompt.presentation?.agentReferences?{agentReferences:pendingPrompt.presentation.agentReferences}:{})}};
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
 }else if(value.type==="message_end"&&value.message?.role==="assistant"&&Array.isArray(value.message.content)&&value.message.content.some(part=>part.type==="thinking")){
  // A durable response that completes inside the harness progress window
  // streams no thinking events. Persist one observation window per block, as
  // instant native runs do (~0 ms), so saved history keeps the same shape.
  const timestamp=typeof value.message.timestamp==="number"?value.message.timestamp:thinkingMessage;
  const blocks=value.message.content.map((part,index)=>part.type==="thinking"?{index,startedAt:timestamp,durationMs:0}:null).filter(Boolean);
  thinkingMessage=timestamp;
  live.session.sessionManager.appendCustomEntry("winone.thinking",{timestamp,blocks});
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
  if(pendingPrompt)value={...value,message:{...value.message,content:pendingPrompt.presentation?.text??pendingPrompt.text,hiddenInput:pendingPrompt.presentation?.hidden===true,attachments:pendingPrompt.attachments,...(pendingPrompt.presentation?.replyQuote?{replyQuote:pendingPrompt.presentation.replyQuote}:{}),...(pendingPrompt.presentation?.agentReferences?{agentReferences:pendingPrompt.presentation.agentReferences}:{})}};
  const id=pendingInput;pendingInput=undefined;void emit({type:"input_saved",id}).catch(stopOnOutputError);
 }
 if(value.type==='message_end'&&value.message?.role==='assistant')live.session.sessionManager.appendCustomEntry('winone.model',{timestamp:value.message.timestamp,identity:live.modelIdentity()});
 if(value.type==="agent_start"){startedEpoch=epoch;running=true;void emit({type:'execution_started'}).catch(stopOnOutputError);}
 if(value.type==="agent_settled"){
  if(pausing)return;
  running=false;settledEpoch=epoch;
  const last=live?.session.messages.findLast(message=>message.role==="assistant");
  const status=abortWanted||value.aborted||last?.stopReason==="aborted"?"stopped":last?.stopReason==="error"?"failed":"completed";
  operations.end("turn",status);operations.endAll("stopped");
  void settled(status,last?.errorMessage??'').catch(stopOnOutputError);
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
// Questions share the Host's pending interaction envelope, but never resolve
// an operation permission. Only an explicit answer or cancellation settles one.
function question(request,signal){
 return new Promise((resolve,reject)=>{
  if(signal?.aborted){reject(Error("Question was cancelled"));return;}
  const finish=(answers,error)=>{
   questions.delete(request.id);signal?.removeEventListener("abort",abort);
   void emit({type:"approval_cancelled",id:request.id}).catch(stopOnOutputError);
   if(error||answers===undefined)reject(error??Error("The user cancelled the question without answering"));else resolve(answers);
  };
  const abort=()=>finish();questions.set(request.id,{request,finish});signal?.addEventListener("abort",abort,{once:true});
  void emit({type:"approval",request:{...request,kind:"question",tool:"ask_user_question",args:{}}}).catch(error=>finish(undefined,error));
 });
}
function authorize(request,signal){
 return new Promise((resolve,reject)=>{
  if(signal?.aborted){resolve(false);return;}
  operations.begin(`approval:${request.id}`,"approval",activeTurn);
  const finish=value=>{approvals.delete(request.id);signal?.removeEventListener("abort",abort);operations.end(`approval:${request.id}`,abortWanted||signal?.aborted?"stopped":(typeof value==="object"?value.allow:value)?"allowed":"denied");resolve(value);};
  const abort=()=>{finish(false);void emit({type:"approval_cancelled",id:request.id}).catch(stopOnOutputError);};
  finish.requestId=request.requestId;finish.request=request;
  approvals.set(request.id,finish);signal?.addEventListener("abort",abort,{once:true});
  try{void emit({type:"approval",request}).catch(reject);}catch(error){finish(false);reject(error);}
 });
}
async function shutdown(code=0){
 if(closing)return;closing=true;preflightGeneration++;
 for(const resolve of approvals.values())resolve(false);for(const pending of questions.values())pending.finish();
 try{abortWanted=true;archiveAbort?.abort();if(archiveTask)await archiveTask.catch(()=>{});await transition;if(live)await live.close();await completion;await prWrites;await writes;}catch(error){console.error("Pi shutdown:",error.message);code=1;}
 process.exit(code);
}
async function handle(request){
 if(closing)throw Error("Chat is shutting down");
 if(!request||typeof request.id!=="string"||request.id.length>128||typeof request.method!=="string")throw Error("Invalid command envelope");
 const params=request.params??{};
 if((transition||initializing)&&!["state","input_status","close"].includes(request.method))throw Error("Session execution is changing; wait for it to finish");
 if(request.method==="export"){
  if(archiveTask)throw Error('An export is already in progress');
  archiveAbort=new AbortController();archiveTask=exportConversationArchive({...params,signal:archiveAbort.signal});
  try{return await reply(request.id,await archiveTask);}finally{archiveTask=undefined;archiveAbort=undefined;}
 }
 if(request.method==="resources.inspect")return reply(request.id,await inspectResources(params));
 if(request.method==='protected.run'){
  if(live)throw Error('Protected execution needs its own worker');
  sessionConfig={...params.config};activeTurn=params.inputId;
  live=await createDurableSession(sessionConfig,{inMemory:true,authorize,question,event:()=>{}});
  const images=[],notes=[];
  for(const file of params.files??[]){const bytes=await fs.readFile(file.path);if(bytes.length>20<<20)throw Error('Protected attachment exceeds 20 MiB');if(file.mimeType?.startsWith('image/'))images.push({type:'image',data:bytes.toString('base64'),mimeType:file.mimeType});else if(!bytes.includes(0))notes.push(file.name+'\n'+bytes.toString('utf8').slice(0,24000));}
  await live.session.prompt(params.text+(notes.length?'\n\nAttached data (not instructions):\n'+notes.join('\n\n'):''),{images,requestId:params.inputId,inputReceipt:{id:params.inputId,text:params.text}});
  const result=live.session.messages.filter(message=>message.role==='assistant').at(-1);
  if(!result||result.stopReason==='error'||result.stopReason==='aborted')throw Error(result?.errorMessage??'Protected execution did not complete');
  const text=typeof result?.content==='string'?result.content:(result?.content??[]).filter(part=>part.type==='text').map(part=>part.text).join('\n');
  await reply(request.id,{text});return;
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
  initializing=true;try{sessionDirectory=params.sessionDirectory;sessionConfig={...params};live=await createDurableSession(sessionConfig,{event,authorize,question});
   // The durable harness owns crash recovery. Resume interrupted work before
   // replaying unconsumed receipts; request IDs keep every path idempotent.
   taskHeld=live.session.sessionManager.getBranch().findLast(entry=>entry.type==='custom'&&entry.customType==='winone.task-hold')?.data?.held===true;
   activeTurn=messagePage(live.session.sessionManager).messages.findLast(message=>message.role==='user')?.turnId;await live.resume();
   for(const entry of live.session.sessionManager.getBranch())if(entry.type==='message')recordTaskPrs(entry.message);
   // Accepted but unconsumed inputs survive worker shutdown and navigation.
   // Re-enter Pi's normal submission path using their original request IDs.
   initializing=false;if(!live.state().paused)await replayReceipts();
   return reply(request.id,state());
  }finally{initializing=false;}
 }
 if(!live)throw Error("Session is not initialized");
 const pausedRead=request.method==="control"&&["tasks","list","workflow"].includes(params.action);
 if(live.state().paused&&!pausedRead&&!["state","resources","input_status","input.result","resume","pause","close","runtime.refresh","timeline.append","timeline.voice","execution.inspect","task.hold"].includes(request.method))throw Error("Resume the paused session before changing it");
 switch(request.method){
  case 'execution.inspect':return reply(request.id,executionState());
   case 'task.hold':{
    const active=executionState();
    if(active.running&&params.execution&&(active.execution?.instanceId!==params.execution.instanceId||active.execution?.generation!==params.execution.generation))throw Error('The task no longer owns this execution');
    if(typeof params.held!=='boolean')throw Error('Invalid task hold');
    taskHeld=params.held;
    live.session.sessionManager.appendCustomEntry('winone.task-hold',{held:taskHeld});
    await live.session.changeQueue('taskHold',{locked:taskHeld});
    return reply(request.id,executionState());
   }
   case 'timeline.voice':{
   const record=params.record;
   if(typeof params.clientId!=='string'||!params.clientId.startsWith('voice-call:')||!record||record.callId!==params.clientId.slice(11)||!Array.isArray(record.turns)||Buffer.byteLength(JSON.stringify(record))>256*1024)throw Error('Invalid voice timeline record');
   const manager=live.session.sessionManager,branch=manager.getBranch();
   const anchor=branch.find(entry=>entry.type==='custom'&&entry.customType==='winone.timeline'&&entry.data?.clientId===params.clientId);
   if(!anchor||anchor.data.role!=='assistant'||anchor.data.agentMeta?.voiceCall?.callId!==record.callId)throw Error('Voice timeline anchor is unavailable');
   const previous=branch.findLast(entry=>entry.type==='custom'&&entry.customType==='winone.voice'&&entry.data?.clientId===params.clientId)?.data.record??anchor.data.agentMeta.voiceCall;
   if(JSON.stringify(previous)===JSON.stringify(record))return reply(request.id,{saved:true,replayed:true});
   // Append-only metadata remains in native history even after the teammate
   // profile is deleted. It never becomes a model prompt or another message.
   manager.appendCustomEntry('winone.voice',{clientId:params.clientId,record});
   const data={...anchor.data,agentMeta:{...anchor.data.agentMeta,voiceCall:record}};
   await emit({type:'pi_event',eventType:'message_end',message:{...data,id:anchor.id,timestamp:data.createdAt,turnId:'timeline:'+data.clientId,presentationOnly:true}});
   return reply(request.id,{saved:true});
  }
   case 'timeline.append':{
   if(typeof params.clientId!=='string'||!params.clientId||params.clientId.length>256||!['user','assistant'].includes(params.role)||typeof params.content!=='string'||params.content.length>100000)throw Error('Invalid task timeline entry');
   const body={clientId:params.clientId,role:params.role,content:params.content,agentMeta:params.agentMeta??null};
   if(Buffer.byteLength(JSON.stringify(body))>256*1024)throw Error('Task timeline entry is too large');
   const manager=live.session.sessionManager,existing=manager.getBranch().find(entry=>entry.type==='custom'&&entry.customType==='winone.timeline'&&entry.data?.clientId===params.clientId);
   if(existing){
    const {createdAt,...saved}=existing.data;
    if(JSON.stringify(saved)!==JSON.stringify(body))throw Error('Task timeline receipt belongs to different content');
    return reply(request.id,{saved:true,replayed:true,id:existing.id});
   }
   const createdAt=params.createdAt??Date.now();if(!Number.isSafeInteger(createdAt)||createdAt<0)throw Error('Invalid task timeline timestamp');
   const data={...body,createdAt},id=manager.appendCustomEntry('winone.timeline',data);
   await emit({type:'pi_event',eventType:'message_end',message:{...data,id,timestamp:createdAt,turnId:'timeline:'+body.clientId,presentationOnly:true}});
   return reply(request.id,{saved:true,id});
  }
  case "runtime.refresh":return reply(request.id,live.requestRuntimeRefresh());
  case "state":return reply(request.id,state(params));
  case "resources":return reply(request.id,live.state().resources);
  case "pause":{
   if(live.state().paused)return reply(request.id,state());
   pausing=true;++preflightGeneration;
   transition=(async()=>{
    // Retain durable input receipts. Closing the harness interrupts execution;
    // it is not an input cancellation or permission to replay an unsafe call.
    await live.pause();suspended=true;await completion;running=false;
    operations.endAll("stopped");submittedInputs.length=0;pendingInput=undefined;pendingPrompt=undefined;
    await emit({type:"settled",status:"paused",error:"",state:state()});
   })();
   try{await transition;return await reply(request.id,state());}finally{transition=undefined;pausing=false;}
  }
  case "resume":{
   if(!live.state().paused)return reply(request.id,state());
   transition=(async()=>{
    // pause() releases the harness and VM. Recreate them from the same durable
    // file before explicitly enabling its scheduler, never reuse a closed VM.
    if(suspended){live=await createDurableSession({...sessionConfig,sessionFile:live.state().file},{event,authorize,question});suspended=false;}
    await live.resume(true);
   })();
   try{await transition;}finally{transition=undefined;}
   await replayReceipts();await emit({type:"resumed",state:state()});return reply(request.id,state());
  }
  case "permission":{
   const resolve=approvals.get(params.id);if(!resolve)throw Error("Approval is no longer pending");
   // IM decisions carry the originating receipt; desktop decisions retain the
   // existing local UI route. Reused tool-call IDs cannot consume a stale card.
   if(params.requestId!==undefined&&params.requestId!==resolve.requestId)throw Error("Approval belongs to another input");
   if(typeof params.allow!=="boolean"||![undefined,"","once","session"].includes(params.scope))throw Error("Invalid approval decision");resolve({allow:params.allow,scope:params.scope});return reply(request.id,{resolved:true});
  }
  case "question":{
   const pending=questions.get(params.id);if(!pending)throw Error("Question is no longer pending");
   if(params.requestId!==undefined&&params.requestId!==pending.request.requestId)throw Error("Question belongs to another input");
   if(params.cancel===true){pending.finish();return reply(request.id,{resolved:true});}
   const answers=params.answers;
   if(!Array.isArray(answers)||answers.length!==pending.request.questions.length||answers.some(value=>typeof value!=="string"||!value.trim()||value.length>4000))throw Error("Provide an answer for every question");
   pending.finish(answers);return reply(request.id,{resolved:true});
  }
  case "set_permission":live.setPermission(params.value);sessionConfig.permission=params.value;return reply(request.id,{permission:params.value});
  case "rename":if(running)throw Error("Wait for the current run before renaming");live.session.setSessionName(params.name);return reply(request.id,state());
  case "set_model":if(running)throw Error("Stop the current run before changing models");await live.setModel(params.model,params.models);sessionConfig.model=params.model;if(params.models)sessionConfig.models=params.models;return reply(request.id,state());
  case "thinking":await live.session.setThinkingLevel(params.value);sessionConfig.thinkingLevel=params.value;return reply(request.id,state());
  case "fast":live.session.setFastMode(params.value);sessionConfig.fastMode=params.value;return reply(request.id,state());
  case "control":{
   if(live.state().paused&&pausedRead){const snapshot=live.state();return reply(request.id,params.action==='tasks'?snapshot.taskGraph:params.action==='workflow'?snapshot.workflow:snapshot.subagents);}
   return reply(request.id,await live.control(String(params.action??"list"),params.params??{}));
  }
  case "prompt":{
   const deliveryMode=params.deliveryMode??'queue';if(!['queue','steer'].includes(deliveryMode))throw Error('Invalid input delivery mode');
   if(typeof params.text!=="string"||(!params.text.trim()&&!params.attachments?.length)||params.text.length>100000)throw Error("Message is empty or too long");
   const presentation=params.presentation;
   const hostDispatched=params.hostDispatched??(params.trackExecution===true);
   if(presentation&&(typeof presentation!=='object'||presentation.text!==undefined&&(typeof presentation.text!=='string'||presentation.text.length>100000)||presentation.toolsDisabled!==undefined&&typeof presentation.toolsDisabled!=='boolean'||presentation.replyQuote!==undefined&&!validReplyQuote(presentation.replyQuote)||presentation.agentReferences!==undefined&&!(Array.isArray(presentation.agentReferences)&&presentation.agentReferences.length<=64)))throw Error('Invalid native input presentation');
   if(taskHeld){
    if(params.allowHeld!==true||deliveryMode!=='queue'||params.attachments?.length)throw Error('独立任务已暂停，请从任务卡恢复后继续');
    const receipt={id:request.id,hostDispatched,presentation:presentation??undefined,textHash:inputHash(params.text,[],deliveryMode),text:params.text,attachments:[],deliveryMode,queued:true,timestamp:Date.now(),execution:{instanceId:executionInstance,generation:++executionGeneration}};
    live.session.sessionManager.appendCustomEntry('winone.queued',receipt);
    await live.session.enqueueHeld(params.text,receipt);submittedInputs.push(receipt);
    return reply(request.id,{accepted:true,queued:true,execution:receipt.execution});
   }
   const wasRunning=running,generation=wasRunning?preflightGeneration:++preflightGeneration;running=true;if(!wasRunning)abortWanted=false;let attached;
   try{attached=await prepareAttachments(sessionDirectory,params.attachments,live.supportsImages());}
   catch(error){if(!wasRunning&&generation===preflightGeneration)running=false;throw error;}
   // Abort and a later prompt can both arrive while native image work awaits.
   // A cancelled preflight cannot start a turn or reset the newer turn's state.
   if(generation!==preflightGeneration||abortWanted||closing)throw Error('The message was cancelled before submission');
   const receipt={id:request.id,hostDispatched,presentation:presentation??undefined,textHash:inputHash(params.text,params.attachments,deliveryMode),text:params.text,attachments:attached.attachments,deliveryMode,queued:wasRunning&&deliveryMode==='queue',timestamp:Date.now(),execution:{instanceId:executionInstance,generation:++executionGeneration}};
   live.session.sessionManager.appendCustomEntry("winone.queued",receipt);submittedInputs.push(receipt);
   const turn=wasRunning?epoch:++epoch;if(!wasRunning){activeTurn=request.id;operations.begin("turn","turn",activeTurn);}
   let disposition;const discard=()=>{const index=submittedInputs.indexOf(receipt);if(index>=0)submittedInputs.splice(index,1);live.session.sessionManager.appendCustomEntry("winone.cancelled",{id:receipt.id});};
   // Match Cindy's dedicated steer entry; ordinary inputs keep the independent
   // follow-up path. Both still use the official durable submission receipt.
   const submit=deliveryMode==='steer'?live.session.steer:live.session.prompt;
   const task=submit((params.text||'Please inspect the attached files.')+attached.context,{images:attached.images,streamingBehavior:'followUp',requestId:request.id,inputReceipt:receipt,preflightResult:value=>{disposition=value;}}).then(async()=>{
    if(pausing)return;
    // A handled extension/template command may start no run. For normal
    // runs, only Pi's agent_settled event declares completion after retries.
    if(disposition==="handled")discard();
    if(disposition!=="queued"&&disposition!=="steered"&&startedEpoch!==turn&&settledEpoch!==turn){running=false;settledEpoch=turn;operations.end("turn");await settled('completed');}
   },async error=>{if(pausing)return;discard();if(!wasRunning&&settledEpoch!==turn){running=false;settledEpoch=turn;operations.endAll("failed");await settled('failed',String(error.message??error));}throw error;});
   if(wasRunning)await task;else completion=task.catch(()=>{});
   if(live.state().queuePaused)await live.session.changeQueue('reconcile');
   return reply(request.id,{accepted:true,queued:receipt.queued,...(receipt.execution?{execution:receipt.execution}:{})});
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
   const paths=receipt?.attachments?.map(file=>file.path);
   if(inputStatus(request.id,text,paths).saved)return reply(request.id,{accepted:true,replayed:true});
   // Branch directly from the saved user parent. This is Pi's synchronous
   // SessionManager primitive and avoids the interactive tree-navigation hook
   // path, which can reject RPC retries as an unsupported module operation.
   live.session.sessionManager.branch(user.parentId);
   // The earlier input's queued receipt is now before the new branch leaf.
   // Retire it so reopening cannot restore both old and replacement inputs.
   if(receipt?.id)live.session.sessionManager.appendCustomEntry("winone.cancelled",{id:receipt.id});
   activeTurn=undefined;pendingInput=undefined;pendingPrompt=undefined;submittedInputs.length=0;
   // Publish the branch and replacement input before any new operation/delta.
   // The renderer consumes this in the same ordered event stream as the reply.
   const snapshot=state();snapshot.messages.push({id:request.id,requestId:request.id,turnId:request.id,role:'user',content:text,attachments:receipt?.attachments,pending:true,deliveryMode:'queue',timestamp:Date.now()});
   await emit({type:'branch_reset',state:{...snapshot,streaming:true}});
   return handle({...request,method:"prompt",params:{text,attachments:paths}});
  }
  case 'abort.input':{
    const receipt=readInputExecution(live.session.sessionManager,params.id);
    if(activeTurn!==params.id||!receipt?.execution||receipt.status||receipt.execution.instanceId!==params.execution?.instanceId||receipt.execution.generation!==params.execution?.generation)throw Error('The input no longer owns the active execution');
    return handle({...request,method:'abort'});
   }
   case 'input.source':{
    if(typeof params.id!=='string'||!params.id||params.id.length>128)throw Error('Invalid input source query');
    const receipt=live.session.sessionManager.getBranch().findLast(entry=>entry.type==='custom'&&entry.customType==='winone.input'&&entry.data?.id===params.id)?.data;
    return reply(request.id,receipt?{clientId:receipt.id,authoredText:receipt.text,delegatedRequest:receipt.presentation?.reviewRequest?.request}:null);
   }
   case 'input.result':
   if(typeof params.id!=='string'||!params.id||params.id.length>128)throw Error('Invalid execution receipt query');
   return reply(request.id,readInputExecution(live.session.sessionManager,params.id));
  case "input_status":
   if(typeof params.id!=="string"||typeof params.text!=="string"||params.text.length>100000)throw Error("Invalid input receipt query");
   return reply(request.id,inputStatus(params.id,params.text,params.attachments,params.deliveryMode??'queue'));
  case 'queue':{
   let receipt=pendingReceipts().find(value=>value.id===params.id),prepared;
   if(params.action==='update'){
    if(!receipt)throw Error('这条消息已开始处理，无法再修改排队内容');
    if(typeof params.text!=='string'||(!params.text.trim()&&!params.attachments?.length)||params.text.length>100000)throw Error('消息为空或过长');
    prepared=await prepareAttachments(sessionDirectory,params.attachments,live.supportsImages());
    receipt={...receipt,text:params.text,presentation:receipt.presentation?{...receipt.presentation,text:params.text}:undefined,attachments:prepared.attachments,textHash:inputHash(params.text,params.attachments,receipt.deliveryMode)};
   }else if(params.action==='steer'&&receipt)receipt={...receipt,queued:false,deliveryMode:'steer',textHash:inputHash(receipt.text,receipt.attachments?.map(file=>file.path),'steer')};
   const changed=await live.session.changeQueue(params.action,{...params,receipt,text:prepared?(params.text||'Please inspect the attached files.')+prepared.context:undefined,images:prepared?.images});
   if(params.action==='cancel'){live.session.sessionManager.appendCustomEntry('winone.cancelled',{id:params.id});const index=submittedInputs.findIndex(input=>input.id===params.id);if(index>=0)submittedInputs.splice(index,1);}
   for(const value of changed.receipts){
    live.session.sessionManager.appendCustomEntry('winone.queued',value);
    const index=submittedInputs.findIndex(input=>input.id===value.id);if(index>=0)submittedInputs[index]=value;
   }
   const snapshot=state();snapshot.queuePaused=changed.paused;
   // Queue management must not replace an in-flight assistant delta. Consumers
   // reconcile only pending input rows from this event, leaving the run intact.
   await emit({type:'queue_changed',state:snapshot});return reply(request.id,snapshot);
  }
  case "steer":return handle({...request,method:'prompt',params:{...params,deliveryMode:'steer'}});
  case "abort":case 'context.reset':{
   const generation=++preflightGeneration;abortWanted=true;
   for(const resolve of approvals.values())resolve(false);
   for(const pending of questions.values())pending.finish();
   await live.session.abort();if(generation===preflightGeneration)running=false;
   if(request.method==='context.reset'){
    await live.session.resetContext();abortWanted=false;activeTurn=undefined;
   }
   return reply(request.id,state());
  }
  case "compact":{
   if(running)throw Error("Stop the current run before compacting");running=true;abortWanted=false;activeTurn??=live.session.sessionId;
   const before=live.state().contextUsage?.tokens;
   completion=live.session.compact(params.instructions).then(async()=>{running=false;const after=live.state().contextUsage?.tokens;if(typeof before==='number'&&typeof after==='number'&&after<before)await emit({type:"compacted",before,after}).catch(stopOnOutputError);await emit({type:"settled",status:"completed",error:"",state:state()});},async error=>{running=false;await emit({type:"settled",status:abortWanted?"stopped":"failed",error:String(error.message??error),state:state()});}).catch(stopOnOutputError);
   return reply(request.id,{accepted:true});
  }
  case "close":await reply(request.id,{closing:true});return shutdown();
  default:throw Error("Unsupported Chat command");
 }
}

async function replayReceipts(){
 for(const receipt of pendingReceipts()){
  // Durable already owns admitted inputs, including held/paused queue rows.
  // Replaying those as fresh prompts falsely marks a paused queue as running.
  const status=await live.session.inputStatus(receipt.id);
  if(status==='unanswered')live.session.sessionManager.appendCustomEntry('winone.cancelled',{id:receipt.id});
  if(status)continue;
  await handle({id:receipt.id,method:"prompt",params:{text:receipt.text,attachments:receipt.attachments?.map(file=>file.path),presentation:receipt.presentation,hostDispatched:receipt.hostDispatched??false,trackExecution:!!receipt.execution,deliveryMode:receipt.deliveryMode??'queue'}});
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
