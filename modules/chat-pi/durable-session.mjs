// WinOne's durable execution adapter. The official Harness owns model turns,
// tool intents, inboxes and recovery. Pi coding-agent is retained only for its
// existing resource/extension/tool adapters and the append-only JSONL view.
import path from 'node:path';
import fs from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import {randomUUID} from 'node:crypto';
import {AsyncLocalStorage} from 'node:async_hooks';
import {BACKGROUND_CONTEXT as context,withAbortSignal} from '@earendil-works/chord/context';
import {Harness,createRegistry,defineExtension,defineTool,section,hook,GenerationTask,ToolTask,LiveDoc,InboxDoc,watchEvents,defineDoc} from '@earendil-works/pi-durable';
import {openDurableStorage} from './durable-storage.mjs';
import {NodeExecutionEnv} from '@earendil-works/pi-durable/env/node';
import {estimateContextTokens} from '@earendil-works/pi-ai/utils/estimate';
import {createWinOneSession} from './session.mjs';
import {cindyRetrySettings} from './cindy/runtime-settings.mjs';
import {buildSystemPrompt} from './node_modules/@earendil-works/pi-coding-agent/dist/core/system-prompt.js';
import {limitChatPayload} from './attachments.mjs';
import {createCollaboration,CollaborationDoc} from './durable-collaboration.mjs';
import {createWorkflow,WorkflowDoc} from './durable-workflow.mjs';
import {createToolSearchExtension} from '@earendil-works/pi-durable/tool-search';
import {createCodemodeExtension} from '@earendil-works/pi-durable/codemode';
import {CodemodeSandbox,loadQuickJSWasm} from '@earendil-works/pi-codemode';

// Explicit delta policy preserves pause/steering state across the 1.4 upgrade.
const AdapterDoc=defineDoc({kind:'winone.adapter',version:1,scope:'conversation',history:'latest',fork:'initial',initial:()=>({imported:false,paused:false,interruptedCalls:[],interruptedRun:null}),checkpointWhen:()=>false});
const readTools=new Set(['read','ls','grep','find']);
const textOf=message=>typeof message?.content==='string'?message.content:(message?.content??[]).filter(part=>part.type==='text').map(part=>part.text).join('\n');
const plain=value=>JSON.parse(JSON.stringify(value));

/** One Host-owned worker opens one SQLite storage. Its JSONL file is a durable
 * entry-ID-addressed projection for existing history, review, export and usage
 * readers. A restart fills missing projection entries without repeating tools. */
export async function createDurableSession(config,{event=()=>{},authorize=async()=>false,question=async()=>{throw Error('Question UI is unavailable');}}={}){
 let currentMessages=[],liveSignal,closed=false,streaming=false,partial,root,harness,currentView,watch,events,notifiedIdle=true;
 let projectedTail,contextStats,usage={models:{},tools:{}},queue=[],settleTask,retrying=false;
 let extra={subagents:[],workflow:{items:[]},backgroundRunning:false,paused:false,taskGraph:{scheduling:'paused',tasks:[],total:0}};
 const calls=new AsyncLocalStorage(),listeners=new Set(),pending=new Set();
 const send=value=>{event(value);for(const listener of listeners)listener(value);};
 const report=error=>send({type:'notice',message:String(error?.message??error)});
 const track=promise=>{pending.add(promise);promise.then(()=>pending.delete(promise),error=>{pending.delete(promise);report(error);});return promise;};
 const native=await createWinOneSession({...config,durableHost:true,intentMessages:()=>currentMessages},{authorize,event:value=>{if(['notice','review_start','review_end'].includes(value.type))send(value);}});
 const sdk=native.session,manager=sdk.sessionManager,registry=createRegistry();
 const basePromptOptions=sdk.extensionRunner.createCommandContext().getSystemPromptOptions();
 let runtimePrompt=sdk.systemPrompt;
 // Reuse the official deterministic sandbox and nested-call journal. Model
 // access remains in WinOne AI services; scripts only call gated local tools.
 function withDiscovery(extension){
  return {...extension,tools:extension.tools.map(tool=>({...tool,execute:async(args,api,ctx)=>{
   // SDK MCP connections register tools lazily. Let its discovery hook finish
   // before Durable snapshots the catalog, then use Durable's own persisted
   // addTools result instead of the SDK's separate active-tool transcript.
   const discovery=await sdk.extensionRunner.emitToolCall({type:'tool_call',toolName:'tool_search',toolCallId:api.callId,input:args});
   if(discovery?.block)throw Error(discovery.reason||'Tool discovery was blocked');
   refreshRegistry();
   // A tool phase keeps its original registry snapshot. MCP may have connected
   // during the readiness hook; resolve this conversation again so discovery
   // sees the newly registered catalog with the same stored tool restrictions.
   const conversation=await harness.conversation(api.conversationId,ctx);
   return tool.execute(args,{...api,registry:registry.snapshot(),agent:signalContext=>conversation.agent(signalContext)},ctx);
  }}))};
 }
 registry.install(withDiscovery(createToolSearchExtension()));
 // A steer must reach a model even when this tool round cannot finish. Keep
 // cancellation intent in the same durable store: sequential calls not started
 // yet must not execute stale instructions after an active call is interrupted.
 registry.install(defineExtension({name:'winone.steering',hooks:[hook(ToolTask,{beforeTool:async(call,api,ctx)=>{
  const state=await api.snapshot(AdapterDoc,api.conversationId,ctx);
  if(!state?.interruptedCalls?.includes(call.id))return;
  const live=await api.snapshot(LiveDoc,api.conversationId,ctx);
  if(state.interruptedRun===live?.run?.taskId)return {block:'Skipped because the user inserted new instructions during this tool round. Read the new input before deciding whether to run this operation.'};
 }}),hook(GenerationTask,{afterTools:async(_assistant,_results,api,ctx)=>{
  const state=await api.snapshot(AdapterDoc,api.conversationId,ctx);
  if(state?.interruptedCalls?.length)await api.commit(async tx=>{const doc=await tx.doc(AdapterDoc,api.conversationId);doc.interruptedCalls=[];doc.interruptedRun=null;},ctx);
 }})]}));
 // The build explicitly selects shipped resources; source execution keeps the
 // official package paths. A missing bundle resource is an error, not a fallback.
 const bundled=typeof WINONE_CHAT_BUNDLE!=='undefined'&&WINONE_CHAT_BUNDLE;
 const codeSandbox=config.runtimeSettings?.codeMode===false?undefined:new CodemodeSandbox({
  memoryLimitBytes:128*1024*1024,...(bundled?{workerUrl:new URL('./codemode-worker.js',import.meta.url)}:{}),
  // Compile only when a script starts, so ordinary chats pay no WASM startup
  // cost and a load failure is returned as a tool error instead of a rejection.
  get wasm(){return bundled?loadQuickJSWasm(fileURLToPath(new URL('./quickjs.wasm',import.meta.url))):undefined;},
 });
 if(codeSandbox)registry.install(withDiscovery(createCodemodeExtension({sandbox:codeSandbox,models:false})));
 const file=manager.getSessionFile();
 if(!file||!path.isAbsolute(file))throw Error('A durable session needs an absolute history file');
 const storageFile=file+'.durable.sqlite';
 const projected=new Map(),projectionWaiters=new Map();
 for(const entry of manager.getEntries()){
  const id=entry.type==='message'?entry.message.durableEntryId:entry.details?.durableEntryId;
  if(id){projected.set(id,entry.id);projectedTail=entry.id;}
 }
 let workflow,collaboration;
 const toolRegistrations=new Map();
 function makeTool(definition){
  return defineTool({name:definition.name,description:definition.description,parameters:definition.parameters,
   replay:readTools.has(definition.name)?'safe':'unsafe',
   executionMode:readTools.has(definition.name)?'parallel':'sequential',
   ...(definition.exposure==='deferred'?{deferred:true}:{}),
   ...(definition.exposure==='codemode'?{callers:['tools']}:{}),
   execute:async(args,api,callContext)=>calls.run({api,context:callContext},async()=>{
    const signal=callContext.abortSignal;
    if(!native.builtinToolNames.includes(definition.name))await native.checkExternalTool({id:api.callId,tool:definition.name,args},signal);
    const intercepted=await sdk.extensionRunner.emitToolCall({type:'tool_call',toolName:definition.name,toolCallId:api.callId,input:args});
    if(intercepted?.block)throw Error(intercepted.reason||'The extension blocked this operation');
    const ctx={...sdk.extensionRunner.createToolContext(api.callId,signal),cwd:(await api.agent(callContext)).cwd??config.cwd,signal,
     executeTool:async(name,input)=>{
      const result=await api.executeTool(name,input,callContext);
      const output=result.structuredOutput;
      return {toolCallId:result.taskId,toolName:name,result:{content:typeof output==='string'?[{type:'text',text:output}]:Array.isArray(output)?output:[{type:'text',text:JSON.stringify(output??result.diagnostics)}],details:result.details},isError:result.isError};
     }};
    const result=await definition.execute(api.callId,args,signal,update=>{
     if(update?.details)void api.details(plain(update.details),callContext).catch(report);
    },ctx);
    const changed=await sdk.extensionRunner.emitToolResult({type:'tool_result',toolName:definition.name,toolCallId:api.callId,input:args,content:result.content,details:result.details,isError:!!result.isError});
    return {output:changed?.content??result.content,details:plain(changed?.details??result.details??{}),isError:changed?.isError??result.isError??false};
   })});
 }
 function refreshRegistry(){
  toolRegistrations.clear();
  for(const info of sdk.getAllTools()){
   // These are owned by Durable; importing SDK versions loses its journal and
   // tool_search's persisted selection of tools for the following request.
   if(info.exposure==='hidden'||['tool_search','codemode'].includes(info.name))continue;
   const definition=sdk.getToolDefinition(info.name);if(!definition)continue;
   toolRegistrations.set(info.name,makeTool({...definition,exposure:info.exposure}));
  }
  registry.install(defineExtension({name:'winone.resources',tools:[...toolRegistrations.values()],sections:[section('winone',()=>runtimePrompt,{tag:false})],hooks:[hook(GenerationTask,{beforeRequest:async request=>{currentMessages=[...request.messages];return {messages:await sdk.extensionRunner.emitContext(currentMessages)};}})]}));
 }
 refreshRegistry();
 workflow=createWorkflow({question,settings:config.runtimeSettings??{},onChange:()=>void updateExtras()});registry.install(workflow.extension);
 collaboration=createCollaboration({config,toolRegistrations,onChange:()=>void updateExtras()});registry.install(collaboration.extension);
 const models=new Proxy(native.modelRuntime,{get(target,key){
  if(key==='streamSimple'||key==='completeSimple')return (model,modelContext,options={})=>target[key](model,modelContext,{...options,onPayload:async(payload,selectedModel)=>{
   let value=(await options.onPayload?.(payload,selectedModel))??payload;
   value=(await sdk.extensionRunner.emitBeforeProviderRequest(value))??value;
   const effort=thinkingLevel();if(effort!=='off')value.reasoning_effort=effort;else if(config.thinkingLevel)value.reasoning_effort='none';
   if(config.fastMode)value.service_tier='priority';
   return limitChatPayload(value);
  }});
  const value=Reflect.get(target,key,target);return typeof value==='function'?value.bind(target):value;
 }});
 function thinkingLevel(){return config.thinkingLevel||config.runtimeSettings?.thinkingLevel||sdk.thinkingLevel||'off';}
 const settings={
  get retry(){return cindyRetrySettings(config.runtimeSettings);},
  // Cindy disables provider-internal retries; the official Harness is the
  // single retry owner, with visible progress and an immediately usable Stop.
  stream:{transport:'sse',maxRetries:0},
  get compaction(){const saved=sdk.settingsManager.getCompactionSettings(sdk.model);return {...saved,enabled:config.runtimeSettings?.autoCompaction!==false,backgroundTokens:config.runtimeSettings?.backgroundCompaction?Math.min(8192,Math.floor((sdk.model?.contextWindow??32768)/10)):0};},
  toolExecution:'parallel',contextRetentionMs:60000,documentRetentionMs:60000,
  steeringMode:config.runtimeSettings?.steeringMode??'one-at-a-time',followUpMode:config.runtimeSettings?.followUpMode??'one-at-a-time',
 };
 try{
  harness=await Harness.open(await openDurableStorage(storageFile),{models,registry,settings,env:({cwd})=>new NodeExecutionEnv({cwd:cwd??config.cwd}),onReport:report},context);
  root=await harness.root(context,{agent:{model:{provider:'winone',modelId:config.model.id},thinkingLevel:thinkingLevel(),cwd:config.cwd}});
  collaboration.setHarness(harness);
  // Current Host choices always win over last run's model. No credential is
  // stored in the agent document; only an enabled WinOne route is persisted.
  await root.configure({model:{provider:'winone',modelId:config.model.id},thinkingLevel:thinkingLevel(),cwd:config.cwd,...(config.model.tools===false||config.runtimeSettings?.toolPreset==='chat-only'?{tools:[]}:config.runtimeSettings?.toolPreset==='read-only'?{tools:[...toolRegistrations.values()].filter(t=>readTools.has(t.name)).concat(workflow.extension.tools??[])}:{})},context);
  const state=await harness.snapshot(AdapterDoc,root.id,context);
  extra.paused=!!state?.paused;extra.queuePaused=!!state?.queuePaused;
  if(!state?.imported){
   const messages=manager.buildSessionProjection().messages.filter(message=>['user','assistant','toolResult'].includes(message.role));
   await root.commit(async tx=>{for(const message of messages)await tx.appendEntry(root.id,{kind:'winone.import',model:[plain(message)],data:{legacy:true}});(await tx.doc(AdapterDoc,root.id)).imported=true;},context);
  }
  // Do not schedule while opening history. Attach the snapshot before resume,
  // and never project imported legacy messages a second time.
  let cursor;
  do{const page=await root.entries({order:'ascending'},500,cursor,context);for(const entry of page.items)project(entry,false);cursor=page.cursor;}while(cursor);
  watch=await root.watch(context);currentView=watch.value;
  watch.start(async value=>{currentView=value;usage=value.docs['pi.usage']??usage;queue=value.docs['pi.inbox']?.items??[];await updateExtras();});
  events=await watchEvents(harness,root.id,context);usage=events.snapshot.usage;partial=events.snapshot.generation?.message;
  streaming=!!events.snapshot.run;notifiedIdle=!streaming;
  events.start(async batch=>{
   // watchEvents pairs a committed message_start with message_end using the
   // same entry.model message. Branch imports already exist in the JSONL/UI
   // snapshot: suppress their starts as well as project()'s skipped ends.
   // Checking the batch's provenance (not text, timestamps or a reset flag)
   // also covers legacy history and cannot suppress a new identical reply.
   const imported=new Set(batch.filter(value=>value.type==='message_end'&&value.entry.data?.legacy).map(value=>value.entry.model?.[0]));
   for(const value of batch){
    if(value.type==='message_start'&&imported.has(value.message))continue;
    await receive(value);
   }
  });
  currentMessages=(await root.context(context)).messages;
  await updateExtras();
 }catch(error){await harness?.close(context).catch(()=>{});await codeSandbox?.close().catch(()=>{});await native.close().catch(()=>{});throw error;}

 function rememberProjection(entryId,id){
  projected.set(entryId,id);projectedTail=id;
  projectionWaiters.get(entryId)?.();projectionWaiters.delete(entryId);
 }
 async function waitForProjection(){
  // waitForIdle covers execution, not the independent committed-event reader.
  // Wait for its final visible entry before announcing completion to the UI.
  let cursor;
  do{
   const page=await root.entries({order:'descending'},500,cursor,context);
   const last=page.items.find(entry=>!entry.data?.legacy&&(entry.model?.[0]||entry.kind==='pi.compaction'));
   if(last){if(!closed&&!projected.has(last.id))await new Promise(resolve=>projectionWaiters.set(last.id,resolve));return;}
   cursor=page.cursor;
  }while(cursor&&!closed);
 }
 function project(entry,notify=true){
  if(entry.data?.legacy||projected.has(entry.id))return;
  if(entry.kind==='pi.compaction'){
   const summary=textOf(entry.model?.[0]),first=projected.get(entry.head)??manager.getLeafId();
   const id=manager.appendCompaction(summary,first,contextStats?.tokens??0,{durableEntryId:entry.id});rememberProjection(entry.id,id);return;
  }
  const message=entry.model?.[0];if(!message)return;
  const receipt=entry.data?.receipt;
  if(message.role==='user'&&receipt){manager.appendCustomEntry('winone.input',receipt);}
  const decorated={...message,durableEntryId:entry.id,...(message.role==='toolResult'?{details:entry.data?.details,durationMs:entry.data?.durationMs}:{}),...(receipt?{requestId:receipt.id}:{}),...(message.role==='assistant'?native.modelIdentity():{})};
  const id=manager.appendMessage(decorated);
  sdk.refreshContext();
  if(notify){
   // The raw user-start event has no receipt. Emit it from its committed entry
   // instead, so steer/followUp reordering cannot borrow a FIFO neighbour's ID.
   if(message.role==='user')send({type:'message_start',message:{...decorated,id}});
   send({type:'message_end',message:{...decorated,id}});
  }
  rememberProjection(entry.id,id);
 }
 async function receive(value){
  if(closed)return;
  switch(value.type){
   case 'run_start':streaming=true;notifiedIdle=false;send({type:'agent_start'});break;
   // Do not block this reader at an intermediate run end: queued follow-ups
   // still need their events delivered while the harness finishes those runs.
   case 'run_end':partial=undefined;void settleIfIdle().catch(report);break;
   case 'message_start':if(value.message.role==='user')break;if(value.message.role==='assistant')partial=value.message;send(value);break;
   case 'message_update':
    for(const change of value.changes){
     if(change.type==='message'){partial=change.message;continue;}
     if(change.type==='block'&&partial){partial={...partial,content:partial.content.map((p,i)=>i===change.contentIndex?change.block:p)};continue;}
     if(change.type.endsWith('_start')&&partial){const content=[...partial.content];content[change.contentIndex]=change.block;partial={...partial,content};}
     if(['text_delta','thinking_delta'].includes(change.type)&&partial){const content=partial.content.map(p=>({...p})),index=change.contentIndex,key=change.type==='text_delta'?'text':'thinking';content[index]={...(content[index]??{type:key}),[key]:(content[index]?.[key]??'')+change.delta};partial={...partial,content};}
     send({type:'message_update',assistantMessageEvent:change});
    }break;
   case 'message_end':{
    project(value.entry);const message=value.entry.model?.[0];
    if(message?.role==='assistant'){
     partial=undefined;
     if(retrying&&message.stopReason!=='error'&&message.stopReason!=='aborted'){retrying=false;send({type:'auto_retry_end',success:true});}
    }
    break;
   }
   case 'entry_appended':project(value.entry,false);break;
   case 'tool_execution_start':send(value);break;
   case 'tool_execution_update':send({...value,partialResult:{details:value.details,content:[]}});break;
   case 'tool_execution_end':{const message=value.entry?.model?.[0];send({...value,result:{content:message?.content??[],details:value.entry?.data?.details??{}},isError:message?.isError??value.result?.isError});break;}
   case 'usage_changed':usage=value.usage;await refreshMetrics();break;
   case 'auto_retry_start':retrying=true;send({...value,delayMs:Math.max(0,value.at-Date.now()),maxAttempts:settings.retry.maxRetries});break;
   // Durable's event means the backoff ended, not that inference succeeded.
   // Cindy keeps recovery active until a successful message or final settlement.
   case 'auto_retry_end':break;
   case 'compaction_start':send(value);break;
   case 'compaction_end':send({...value,errorMessage:value.error});await refreshMetrics();break;
   case 'task_failed':report(Error(value.message));break;
   case 'snapshot':
    for(const entry of value.entries)project(entry,false);partial=value.generation?.message;usage=value.usage;streaming=!!value.run;
    send({type:'durable_snapshot'});break;
  }
 }
 async function refreshMetrics(){
  const active=await root.context(context);currentMessages=[...active.messages];
  const tokens=estimateContextTokens(active.messages).tokens,window=sdk.model?.contextWindow??0;
  contextStats={tokens,contextWindow:window,percent:window?tokens/window*100:null};
  send({type:'usage',usage:usageSummary(),contextUsage:contextStats});
 }
 function usageSummary(){
  const sum={input:0,output:0,cacheRead:0,cacheWrite:0,totalTokens:0,cost:0,costKnown:!!config.model.price};
  for(const value of [...Object.values(usage.models??{}),...Object.values(usage.tools??{})]){for(const key of ['input','output','cacheRead','cacheWrite','totalTokens'])sum[key]+=value[key]??0;sum.cost+=value.cost?.total??0;}
  return sum;
 }
 async function updateExtras(){
  if(!root||!harness||closed)return;
  extra={...extra,subagents:await collaboration.list(harness,root.id),workflow:(await harness.snapshot(WorkflowDoc,root.id,context))??{items:[]},backgroundRunning:false,taskGraph:await taskGraph()};
  const live=await harness.snapshot(LiveDoc,root.id,context);
  extra.backgroundRunning=extra.subagents.some(agent=>agent.status==='running')||!!live?.compactions?.length;
  send({type:'durable_state',...extra});
 }
 async function taskGraph(){
  const inspection=await harness.inspect(context);
  // Inspector records contain tool arguments and model inputs. Send only task
  // identity, ownership and scheduler state, bounded with an explicit total.
  return {scheduling:inspection.scheduling,total:inspection.tasks.length,tasks:inspection.tasks.slice(0,512).map(({record,state})=>({id:record.id,kind:record.kind,conversationId:record.conversationId,owner:record.owner,background:record.background,state:state.kind}))};
 }
 function settleIfIdle(){
  // run_end and a finished submission may both observe the idle edge. One
  // shared attempt keeps the settled event and its metrics single.
  if(!settleTask)settleTask=(async()=>{
   try{
    await root.waitForIdle(context);
    await waitForProjection();
    if(closed||notifiedIdle)return;
    const snapshot=await root.context(context);currentMessages=[...snapshot.messages];streaming=false;notifiedIdle=true;
    if(retrying){retrying=false;const last=currentMessages.findLast(message=>message.role==='assistant');send({type:'auto_retry_end',success:last?.stopReason!=='error'&&last?.stopReason!=='aborted',finalError:last?.errorMessage});}
    await refreshMetrics();send({type:'agent_settled'});
   }finally{settleTask=undefined;}
  })();
  return settleTask;
 }
 async function syncBranch(){
  if(!projectedTail||manager.getBranch().some(entry=>entry.id===projectedTail))return;
  // Retry/fork on the existing JSONL projection is an explicit context change.
  // Durable keeps the abandoned work in SQLite; neither history is truncated.
  await root.reset(undefined,context);
  const messages=manager.buildSessionProjection().messages.filter(m=>['user','assistant','toolResult'].includes(m.role));
  await root.commit(async tx=>{for(const message of messages)await tx.appendEntry(root.id,{kind:'winone.import',model:[plain(message)],data:{legacy:true}});},context);
  projectedTail=manager.getLeafId();
 }
 function interruptToolRound(interrupted){
  if(interrupted.some(tool=>tool.status!=='done')){
   // Control work cannot reject an already-admitted input. Abort only this
   // tool round, never root.abort(), which would end the run and its inbox.
   track((async()=>{
    let tasks=[];
    await root.commit(async tx=>{
     const live=await tx.doc(LiveDoc,root.id);
     if(!live.tools?.some(tool=>tool.taskId!==undefined&&interrupted.some(prior=>prior.taskId===tool.taskId)))return;
     const pending=live.tools.filter(tool=>tool.status!=='done');
     const adapter=await tx.doc(AdapterDoc,root.id);
     adapter.interruptedCalls=pending.map(tool=>tool.callId);
     adapter.interruptedRun=live.run?.taskId??null;
     tasks=pending.flatMap(tool=>tool.taskId===undefined?[]:[tool.taskId]);
    },context);
    await Promise.all(tasks.map(task=>harness.abortTask(task,context)));
   })());
  }
 }
 async function prompt(text,options={}){
  if(closed)throw Error('Durable session is closed');
  if(extra.paused)throw Error('Resume the paused session before sending a message');
  const command=/^\/([^\s]+)(?:\s+([\s\S]*))?$/.exec(text.trim());
  const registered=command&&sdk.extensionRunner.getCommand(command[1]);
  if(registered){await registered.handler(command[2]??'',sdk.extensionRunner.createCommandContext());options.preflightResult?.('handled');return;}
  const transformed=await sdk.extensionRunner.emitInput(text,options.images,'rpc',options.streamingBehavior);
  if(transformed?.action==='handled'){options.preflightResult?.('handled');return;}
  if(transformed?.action==='transform'){text=transformed.text;options={...options,images:transformed.images??options.images};}
  if(command?.[1].startsWith('skill:')){
   const name=command[1].slice(6),skill=native.resourceLoader.getSkills().skills.find(s=>s.name===name);
   if(!skill)throw Error('Skill not found: '+name);
   text=`<skill name=${JSON.stringify(name)}>\n${await fs.readFile(skill.filePath??skill.path,'utf8')}\n</skill>\n${command[2]??''}`;
  }
  // Cindy/Pi handles busy input before before_agent_start. Re-running that
  // startup path waits on MCP and extension work belonging to the active run,
  // which turns an immediate steer into an accidental application-level queue.
  if(!streaming){
   const prepared=await sdk.extensionRunner.emitBeforeAgentStart({messageType:'user',prompt:text,images:options.images},basePromptOptions);
   runtimePrompt=buildSystemPrompt(prepared.systemPromptOptions);refreshRegistry();
   for(const message of prepared.messages)await root.submit({type:'write',entry:{kind:'winone.extension',data:plain(message),model:[{role:'user',content:message.content,timestamp:Date.now()}]}},context);
   await syncBranch();
  }
  const wasRunning=streaming;
  const id=options.requestId??randomUUID();
  const content=options.images?.length?[{type:'text',text},...options.images]:text;
  // Capture this round before admitting the input. Looking up tools afterward
  // without comparing task identities could interrupt the corrected response.
  const interrupted=wasRunning&&options.streamingBehavior==='steer'?(await harness.snapshot(LiveDoc,root.id,context))?.tools??[]:[];
  const submission=await root.submit({type:'input',content,requestId:id,whenBusy:options.streamingBehavior??'followUp',...(options.inputReceipt?{entry:{kind:'winone.input',data:{receipt:plain(options.inputReceipt)}}}:{})},context);
  streaming=true;notifiedIdle=false;
  interruptToolRound(interrupted);
  if(wasRunning){options.preflightResult?.(options.streamingBehavior==='steer'?'steered':'queued');return;}
  options.preflightResult?.('started');
  const settled=await submission.wait(context);
  await settleIfIdle();
  if(settled.status==='unanswered'&&settled.reason!=='aborted')throw Error(`Durable input failed: ${settled.reason}${settled.detail?': '+JSON.stringify(settled.detail):''}`);
 }
 async function compact(instructions){
  const task=await root.compact(instructions,context),result=await harness.waitForTask(task,context);
  if(result.state.outcome.status!=='completed')throw Error('Context compaction did not complete');
  if(result.state.outcome.result?.submissionId){const submission=await harness.submission(result.state.outcome.result.submissionId,context);await submission.wait(context);}
  await refreshMetrics();
 }
 // The inbox and these held items share one transaction. An edit locks its
 // FIFO tail; earlier items may drain. No editor operation aborts the live run.
 async function changeQueue(action,{id,locked,text,images,receipt,targetIndex}={}){
  const interrupted=action==='steer'?(await harness.snapshot(LiveDoc,root.id,context))?.tools??[]:[];
  let result;
  await root.commit(async tx=>{
   const inbox=await tx.doc(InboxDoc,root.id),adapter=await tx.doc(AdapterDoc,root.id);
   const held=adapter.heldInputs??[],order=adapter.queueOrder??[];
   const all=[...inbox.items,...held].map(plain).sort((a,b)=>{
    const ai=order.indexOf(a.id),bi=order.indexOf(b.id);
    return ai<0&&bi<0?a.id-b.id:ai<0?1:bi<0?-1:ai-bi;
   });
   const key=item=>item.entry?.data?.receipt?.id;
   const index=all.findIndex(item=>key(item)===id),item=all[index];
   if(['update','cancel','steer','reorder'].includes(action)||action==='lock'&&locked){
    if(!item)throw Error('这条消息已开始处理，无法再修改排队内容');
   }
   const locks=new Set(adapter.queueLocks??[]);
   if(action==='lock'){if(locked)locks.add(id);else locks.delete(id);}
   else if(action==='interaction')adapter.queueInteractionLocked=locked===true;
   else if(action==='pause')adapter.queuePaused=true;
   else if(action==='resume')adapter.queuePaused=false;
   else if(action==='resetLocks'){locks.clear();adapter.queueInteractionLocked=false;}
   else if(action==='reconcile'){}
   else if(action==='update'){
    item.content=images?.length?[{type:'text',text},...images]:text;
    item.entry.data.receipt=plain(receipt);
   }else if(action==='cancel'){
    // Same settlement as Harness.abortSubmission, while also removing a held
    // item in this atomic commit. A placed message is never a cancellation.
    tx.settleSubmission(item.id,{status:'unanswered',reason:'aborted'});
    all.splice(index,1);locks.delete(id);
   }else if(action==='steer'){
    item.mode='steer';item.entry.data.receipt={...item.entry.data.receipt,...receipt,queued:false,deliveryMode:'steer'};
    locks.delete(id);
   }else if(action==='reorder'){
    if(!Number.isInteger(targetIndex)||targetIndex<0||targetIndex>all.length)throw Error('无效的排队位置');
    all.splice(index,1);all.splice(targetIndex>index?targetIndex-1:targetIndex,0,item);
   }else throw Error('不支持的排队操作');
   adapter.queueLocks=[...locks];adapter.queueOrder=all.map(item=>item.id);
   let blocked=adapter.queuePaused===true||adapter.queueInteractionLocked===true;
   const active=[],waiting=[];
   for(const input of all){
    if(locks.has(key(input)))blocked=true;
    // An explicit steer retains its same-turn semantics even across a held
    // follow-up. It is never implemented as stop + restart.
    (blocked&&input.mode!=='steer'?waiting:active).push(input);
   }
   inbox.items=active;adapter.heldInputs=waiting;
   result={receipts:all.flatMap(input=>input.entry?.data?.receipt?[input.entry.data.receipt]:[]),paused:adapter.queuePaused===true};
  },context);
  interruptToolRound(interrupted);
  extra.queuePaused=result.paused;
  if(!['lock','interaction','pause','update','reconcile'].includes(action)||locked===false){
   // An empty context write runs the official final boundary when idle. It
   // introduces no model message; busy sessions keep their existing run.
   await root.submit({type:'write',entry:{kind:'winone.queue',model:[]}},context);
  }
  return result;
 }
 async function setThinkingLevel(value){
  // '' and 'auto' restore the provider default: no reasoning_effort is injected.
 const level=value==='auto'?'':value;
 if(level!==''&&!['off','minimal','low','medium','high','xhigh','max'].includes(level))throw Error('Invalid thinking level');
  if(streaming)throw Error('Stop the run before changing thinking level');
  config.thinkingLevel=level;await root.configure({thinkingLevel:level||'off'},context);sdk.setThinkingLevel(level||'off');
 }
 const facade={
  sessionManager:manager,settingsManager:sdk.settingsManager,extensionRunner:sdk.extensionRunner,agent:sdk.agent,
  get messages(){return manager.buildSessionProjection().messages;},get model(){return sdk.model;},get thinkingLevel(){return thinkingLevel();},
  get sessionId(){return manager.getSessionId();},get sessionFile(){return file;},get isStreaming(){return streaming;},
  subscribe(listener){listeners.add(listener);return()=>listeners.delete(listener);},
  prompt,compact,changeQueue,setThinkingLevel,setFastMode:value=>{config.fastMode=!!value;},
  inputStatus:id=>root.commit(async tx=>(await tx.submissionByRequest(root.id,id))?.status,context),
  async abort(){await root.abort(context,{background:true});await settleIfIdle();},
  steer:(text,options={})=>prompt(text,{...options,streamingBehavior:'steer'}),
  followUp:(text)=>prompt(text,{streamingBehavior:'followUp'}),waitForIdle:()=>root.waitForIdle(context),
  setSessionName:name=>sdk.setSessionName(name),getActiveToolNames:()=>sdk.getActiveToolNames(),
  getSessionStats:()=>({tokens:usageSummary(),cost:usageSummary().cost,contextUsage:contextStats}),getContextUsage:()=>contextStats,
  dispose(){void close();},
 };
 // Rebind extension actions so an installed skill/plugin cannot accidentally
 // start the old SDK agent loop beside the durable one.
 const actions=native.resourceLoader.getExtensions().runtime;
 const refreshNativeTools=actions.refreshTools;
 sdk.extensionRunner.bindCore({...actions,
  sendUserMessage:(content,options)=>track(prompt(typeof content==='string'?content:textOf({content}),options)),
  sendMessage:(message,options)=>track(options?.triggerTurn?prompt(textOf(message),{streamingBehavior:options.deliverAs==='steer'?'steer':'followUp'}):root.submit({type:'write',entry:{kind:'winone.extension',data:plain(message),...(message.content?{model:[{role:'user',content:message.content,timestamp:Date.now()}]}:{})}},context)),
  getActiveTools:()=>sdk.getActiveToolNames(),setActiveTools:names=>{sdk.setActiveToolsByName(names);track(root.configure({tools:names.map(name=>toolRegistrations.get(name)).filter(Boolean)},context));},
  refreshTools:()=>{refreshNativeTools();refreshRegistry();},
  setThinkingLevel:value=>{track(setThinkingLevel(value));},getThinkingLevel:thinkingLevel,
  setModel:async model=>{if(model.provider!=='winone')throw Error('Choose a model in WinOne AI services');await sdk.setModel(model);await root.configure({model:{provider:model.provider,modelId:model.id}},context);return true;},
 },{getModel:()=>sdk.model,getScopedModels:()=>sdk.scopedModels,isIdle:()=>!streaming,isProjectTrusted:()=>false,getSignal:()=>calls.getStore()?.context.abortSignal??liveSignal,getSystemPromptOptions:()=>basePromptOptions,
  abort:()=>{track(facade.abort());},hasPendingMessages:()=>queue.length>0,shutdown:()=>{void close().catch(report);},getContextUsage:()=>contextStats,compact:options=>{track(compact(options?.customInstructions));},getSystemPrompt:()=>runtimePrompt,
  getCallableTools:()=>sdk.getAllTools().map(t=>sdk.getToolDefinition(t.name)).filter(Boolean),
  executeTool:async(_caller,name,args)=>{const current=calls.getStore();if(!current)throw Error('Nested tools require an active durable tool call');return current.api.executeTool(name,args,current.context);},
 });
 sdk.extensionRunner.bindCommandContext({waitForIdle:facade.waitForIdle,reload:async()=>{await native.resourceLoader.reload();refreshRegistry();},
  navigateTree:async id=>{if(streaming)throw Error('Stop the current run first');if(!manager.getEntry(id))throw Error('Unknown history entry');manager.branch(id);await syncBranch();return {cancelled:false};},
  newSession:async()=>{await root.reset(undefined,context);manager.resetLeaf();projectedTail=undefined;return {cancelled:false};},
  fork:async()=>{throw Error('Use the Chat branch action to keep the new conversation in the sidebar');},switchSession:async()=>{throw Error('Use the Chat sidebar to switch conversations');},
 });
 async function close(){
  if(closed)return;closed=true;for(const resolve of projectionWaiters.values())resolve();projectionWaiters.clear();
  // Even a failed Harness shutdown must release VM workers and native resources.
  try{await harness.close(context);await events.stop();await watch.stop();await Promise.allSettled([...pending]);}
  finally{try{await codeSandbox?.close();}finally{await native.close();}}
 }
 async function pause(){
  // Commit the user's scheduling choice before releasing execution resources.
  // Harness close preserves safe checkpoints and marks unsafe calls interrupted.
  await root.commit(async tx=>{(await tx.doc(AdapterDoc,root.id)).paused=true;},context);
  extra.paused=true;await updateExtras();streaming=false;await close();
  extra.taskGraph={...extra.taskGraph,scheduling:'paused'};
 }
 async function resume(explicit=false){
  if(extra.paused&&!explicit)return;
  if(explicit){await root.commit(async tx=>{(await tx.doc(AdapterDoc,root.id)).paused=false;},context);extra.paused=false;}
  await changeQueue('resetLocks');harness.resume();await updateExtras();
  if(streaming){send({type:'agent_start'});void settleIfIdle().catch(report);}
 }
 return {
  session:facade,durable:true,harness,root,storageFile,
  state:()=>({...native.state(),messages:facade.messages,streaming:!extra.paused&&streaming,thinkingLevel:thinkingLevel(),fastMode:!!config.fastMode,runtime:'pi-durable',usage:usageSummary(),contextUsage:contextStats,...extra}),
  liveMessages:()=>partial?[{...partial,...native.modelIdentity()}]:[],
  modelIdentity:native.modelIdentity,supportsImages:native.supportsImages,setPermission:native.setPermission,
  async setModel(model,models){await native.setModel(model,models);config.model=model;await root.configure({model:{provider:'winone',modelId:model.id}},context);await refreshMetrics();},
  async control(action,params={}){if(action==='tasks')return taskGraph();if(action==='list')return collaboration.list(harness,root.id);if(action==='workflow')return (await harness.snapshot(WorkflowDoc,root.id,context))??{items:[]};return collaboration.control(harness,root.id,action,params);},
  pause,resume,close,
 };
}
