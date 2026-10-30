// Typed, session-bound tools for the OneBot Home. The model never supplies a
// Bot id or filesystem root; the private Host bridge binds both from the lane.
// Teammate tools follow Cindy's names, schemas and payloads (Pi direct helper
// facade and lizi xdt-helper, Cindy b7e46bc, Apache-2.0): a tool returns
// {ok:true,...}; a refusal is a tool error {ok:false,errorCode,data:{hint}}.
import {z} from 'zod';
import {channelForceConfirmToolCall,channelForceConfirmMutatingToolCall} from '../cindy/bots/im-policy/channelToolPolicy.ts';
import {todoPatch} from '../cindy/helper/botWorkbenchTools.ts';
const string = {type:'string'};
const object = (properties, required=[]) => ({type:'object',properties,required,additionalProperties:false});
const memoryEntry=object({type:{enum:['user','feedback','project','reference']},name:string,title:string,description:string,body:string,mode:{enum:['create','update','append']}},['type','name','title','description','body']);
// Cindy's zod Todo patch, as the JSON schema Pi validates tool arguments with.
const {$schema:_todoSchemaDialect,...todoPatchSchema}=z.toJSONSchema(todoPatch);
const errorPayload=(errorCode,hint,data={})=>({ok:false,errorCode,data:{...data,hint}});
async function request(config, route, value, signal) {
 const target=config.onebot;
 const requestId=await config.onebotInputReceipt?.();
 const response=await fetch(target.baseUrl+route,{method:'POST',headers:{Authorization:'Bearer '+target.key,'Content-Type':'application/json'},body:JSON.stringify({...value,requestId:requestId??null}),signal});
 if(!response.ok)throw Error(await response.text());
 return response.json();
}
// Private Host facts used only by the runtime, never a model-facing tool.
export const readOneBotRuntime=(config,signal)=>request(config,'/runtime',{},signal);
export const acknowledgeOneBotRuntime=(config,revision,resources,signal)=>request(config,'/runtime.applied',{revision,resources},signal);
export const acknowledgeOneBotModel=(config,previousModelId,signal)=>request(config,'/runtime.model',{previousModelId,modelId:config.model.id,modelKey:config.onebot.modelChainKey},signal);

// Per-tool caller scope comes from the immutable Durable input entry, not from
// stdout event timing or a mutable "last received message" variable.
export async function checkOneBotToolScope(config, action, signal) {
 if(!config.onebot?.receiptScoped&&!config.onebot?.channelScoped)return {};
 const policy=await request(config,'/policy',{tool:action.tool,path:action.args?.path,external:action.external===true},signal);
 if(!policy.channel)return {requestId:policy.requestId};
 return {requestId:policy.requestId,forceConfirm:policy.group?channelForceConfirmMutatingToolCall(action.tool,action.args):channelForceConfirmToolCall(action.tool,action.args)};
}

// Import-specific tools come directly from Cindy's companion-connections MCP.
// Their names/schemas/results are unchanged behind a Pi-safe namespace.
export async function oneBotImportedTools(config,check){
 if(!config.onebot?.importedEnvironment||config.model?.tools===false)return [];
 const tools=await request(config,'/imported-tools',{});
 return tools.map(tool=>({name:'imported_'+tool.name,label:tool.name,description:tool.description??tool.name,parameters:tool.inputSchema,
  execute:async(id,args,signal)=>{
   const scope=await checkOneBotToolScope(config,{tool:tool.name==='run_command'?'powershell':'imported_'+tool.name,args,external:tool.name!=='run_command'},signal);
   await check({id,tool:'imported_'+tool.name,args,cwd:config.cwd,external:true,...scope},signal);
   const result=await request(config,'/imported-tool',{args:{name:tool.name,args,callId:id}},signal);
   if(result.isError)throw Error(result.content?.filter(part=>part.type==='text').map(part=>part.text).join('\n')||'Imported tool failed');
   return {content:result.content??[],details:{imported:true}};
  },
 }));
}

// Cindy's start/message/stop_session_task results (lizi xdt-helper session tasks).
function startedTask(result){
 if(!result.ok)return errorPayload(result.errorCode,result.message);
 return {ok:true,action:'start_session_task',...(result.modelRoute?{model_route:result.modelRoute}:{}),task_id:result.delegationId,session_id:result.childSessionId,
  status:result.status,deadline_at:result.deadlineAt,expects_result:true,...(result.completionDestination?{completion_destination:result.completionDestination}:{}),
  guidance:(result.completionDestination==='group'
   ?'This task lives in this group: its card is being posted here, and anything it needs confirmed plus its final result are sent here as your own messages (only if the group can no longer take them do they go to your owner privately). Tell the group briefly what you started; do not claim the card or result is already here. '
   :result.completionDestination==='private'
    ?'This older group cannot carry task messages: anything the task needs confirmed and its final result go to your owner\'s private chat with you, not here. Tell the group briefly what you started and that the result will reach your owner privately. '
    :'')+'The task card tracks progress and the result will return automatically. Do not start it again. Treat model_route and task/session ids as internal bookkeeping; do not echo them or the delegated instruction in ordinary replies unless the user asks for these details.'};
}
/** message_session_task argument rules, then Cindy's SessionTaskMessage. */
function taskReply(args){
 const {message,mode,queued_message_id:queuedMessageId,decision,answers,reason,idempotency_key:idempotencyKey}=args;
 if(mode==='edit'||mode==='withdraw'){
  if(!queuedMessageId||decision||answers||(mode==='edit'?!message?.trim():!!message))return errorPayload('INVALID_ARGS','edit requires queued_message_id and message; withdraw requires only queued_message_id.');
  return mode==='edit'?{kind:'edit',queuedMessageId,text:message.trim()}:{kind:'withdraw',queuedMessageId};
 }
 if(queuedMessageId)return errorPayload('INVALID_ARGS','queued_message_id requires edit or withdraw mode.');
 const choices=Number(Boolean(message?.trim()))+Number(Boolean(decision))+Number(Boolean(answers));
 if((mode==='resume'?Boolean(decision||answers):choices!==1)||(mode==='steer'&&!message?.trim())||(mode&&mode!=='queue'&&Boolean(decision||answers)))return errorPayload('INVALID_ARGS','Provide exactly one of message, decision, or answers.');
 if(mode==='resume')return {kind:'resume',...(message?.trim()?{text:message.trim()}:{})};
 if(message?.trim())return {kind:'message',text:message.trim(),idempotencyKey,...(mode?{mode}:{})};
 if(decision==='approve')return {kind:'approve'};
 if(decision==='deny')return {kind:'deny',reason};
 return {kind:'answer',answers};
}

const routineTrigger={anyOf:[
 object({id:string,kind:{type:'string',enum:['interval']},intervalMs:{type:'integer',minimum:60000,description:'Interval in milliseconds. One minute = 60000.'},anchorMs:{type:'integer',minimum:0,maximum:Number.MAX_SAFE_INTEGER,description:'Original interval anchor in Unix milliseconds; preserve when editing.'}},['id','kind','intervalMs']),
 object({id:string,kind:{type:'string',enum:['cron']},expression:string,timezone:string},['id','kind','expression','timezone']),
 object({id:string,kind:{type:'string',enum:['once']},at:{type:'integer',minimum:0,maximum:Number.MAX_SAFE_INTEGER,description:'One-time trigger timestamp in Unix milliseconds.'}},['id','kind','at']),
 object({id:string,kind:{type:'string',enum:['event']},sourceId:string,eventType:string,filters:{type:'array',items:object({field:string,operator:{type:'string',enum:['equals','contains','not-equals']},value:string},['field','operator','value'])}},['id','kind','sourceId','eventType','filters']),
]};
const taskId={type:'string',minLength:1,description:'Local task ID.'};

export function oneBotTools(config, check) {
 const target=config.onebot;
 if(!target)return [];
 // Cindy's Bot Mode chat: the teammate's main chat and its private topics.
 const ownerChat=target.role==='canonical'||target.role==='thread';
 // [name, Host method, description, parameters, prepare(args) -> worker args or an error payload, present(result, args) -> payload]
 const definitions=[
  ['start_session_task','tasks.start','Start one real independent Cindy Session task in the background. Prefer completing work in the current chat, including status checks, bounded code or document reading, explaining existing results, and simple file work. A repository, multiple files, tools, or a deliverable alone is not a reason to delegate. Use this when the user explicitly requests an independent task or the work needs separate ongoing execution, an isolated workspace, parallel delivery, or independent tracking. Decide by execution needs, not time or file-count thresholds. When warranted, act without asking again merely to start a task; respect an explicit request to work inline. Include the objective, constraints, known facts, relevant files, completed actions, and acceptance criteria in instruction; the task does not automatically inherit this chat. This never selects or wakes a Bot, including a Bot named Cindy. Start it once, do not duplicate its work or poll, and review the result when it returns automatically. Follow up with message_session_task on the same task. Normally omit model_selection: the task uses your task model or your current model; pass an enabled WinOne model id (provider/model) only when the user asks for another model.',
   object({instruction:{type:'string',minLength:1,maxLength:12000},title:{type:'string',minLength:1,maxLength:120},working_dir:{type:'string',minLength:1,maxLength:1024},use_worktree:{type:'boolean'},
    context_refs:{type:'array',items:{type:'string',maxLength:512},maxItems:32},timeout_ms:{type:'integer',minimum:1000,maximum:86400000},
    model_selection:object({id:{type:'string',minLength:1,maxLength:2048},effort:{type:'string',maxLength:64},fast_mode:{anyOf:[{type:'boolean'},{const:'ultrafast'}]}},['id'])},['instruction']),
   args=>({objective:args.instruction.trim(),title:args.title,workingDir:args.working_dir,useWorktree:args.use_worktree,contextRefs:args.context_refs,timeoutMs:args.timeout_ms,
    ...(args.model_selection?{modelSelection:{id:args.model_selection.id,...(args.model_selection.effort!==undefined?{effort:args.model_selection.effort}:{}),...(args.model_selection.fast_mode!==undefined?{fastMode:args.model_selection.fast_mode}:{})}}:{})}),
   startedTask],
  ['check_session_task','tasks.get','Read one Session task state when the user asks for progress or automatic completion appears to be missing. Do not poll.',
   object({task_id:{type:'string',minLength:1,maxLength:128},queued_message_id:{type:'string',minLength:1,maxLength:256}},['task_id']),
   args=>({delegationId:args.task_id,...(args.queued_message_id?{queuedMessageId:args.queued_message_id}:{})}),
   (result,args)=>result.ok?{ok:true,action:'check_session_task',task_id:args.task_id,task:result.task}:errorPayload(result.errorCode,result.message)],
  ['list_session_tasks','tasks.list','List independently tracked tasks started by this companion.',object({})],
  ['message_session_task','tasks.message','Queue instructions (default), steer the running turn with mode=steer, or release a reversible pause with mode=resume. Steer never falls back to queue. If paused, resume before answering pending interactions. A terminal task with ordinary input starts a fresh execution under the same task ID.',
   object({task_id:{type:'string',minLength:1,maxLength:128},mode:{type:'string',enum:['queue','steer','resume','edit','withdraw']},queued_message_id:{type:'string',minLength:1,maxLength:256},
    message:{type:'string',minLength:1,maxLength:4000},decision:{type:'string',enum:['approve','deny']},answers:{type:'object',additionalProperties:{type:'string'}},reason:{type:'string',maxLength:4000},idempotency_key:{type:'string',minLength:1,maxLength:128}},['task_id']),
   args=>{const input=taskReply(args);return input.ok===false?input:{delegationId:args.task_id,input};},
   (result,args)=>result.ok?{ok:true,action:'message_session_task',task_id:args.task_id,session_id:result.childSessionId,resumed:result.resumed,
    ...(result.queued===undefined?{}:{queued:result.queued}),...(result.delivery===undefined?{}:{delivery:result.delivery}),
    ...(result.queuedMessageId===undefined?{}:{queued_message_id:result.queuedMessageId}),...(result.control===undefined?{}:{control:result.control})}:errorPayload(result.errorCode,result.message)],
  ['stop_session_task','tasks.stop','mode=cancel (default) terminates the task; request-stop requests graceful stop only; pause holds the same task and queued input until explicit resume. Check returned control state: pausing/unconfirmed is not stopped.',
   object({task_id:{type:'string',minLength:1,maxLength:128},mode:{type:'string',enum:['cancel','request-stop','pause']}},['task_id']),
   args=>({delegationId:args.task_id,...(args.mode?{mode:args.mode}:{})}),
   (result,args)=>result.ok?{ok:true,action:'stop_session_task',task_id:args.task_id,session_id:result.childSessionId,...(result.control===undefined?{}:{control:result.control})}:errorPayload(result.errorCode,result.message)],
  ['list_teammate_skills','skills.query','Search all personal Skills. Follow nextOffset; disabled Skills remain visible but are never loaded.',object({query:string,offset:{type:'integer',minimum:0},limit:{type:'integer',minimum:1,maximum:50}})],
  ['read_teammate_skill','skills.read','Read one personal Skill before applying or updating it.',object({slug:string},['slug'])],
  ['save_teammate_skill','skills.save','Save a verified reusable Skill when the user explicitly requests it. Existing slugs are updated, not duplicated.',object({name:string,description:string,body:string,slug:string,expectedUpdatedAt:{type:['string','null']}},['name','description','body'])],
  ['delete_teammate_skill','skills.delete','Remove a personal Skill only when requested by its owner.',object({slug:string},['slug'])],
  ['list_agents','tasks.agents.list','Discover teammates on this device. Use the exact returned id with send_to_agent. Do not guess IDs or poll.',object({})],
  ['send_to_agent','tasks.agents.send',['Send one asynchronous message to a named teammate.',
   'Use it for a brief question, discussion, or information transfer. It does not create a task, status, progress, cancellation, or a completion contract.',
   "The message remains visible in both teammates' timelines. The recipient may answer in a later turn. Do not poll or send acknowledgement-only replies.",
   'For independently tracked development or deliverable work, use start_session_task instead.',
   'Use the exact stable id from list_agents or a structured @Bot reference. Never route by name.'].join('\n'),
   object({target_id:{type:'string',minLength:1,maxLength:210},message:{type:'string',minLength:1,maxLength:12000,description:'The message to send to the teammate.'}},['target_id','message'])],
 ];
 if(ownerChat)definitions.push(
  // Cindy botRoutineTools (Pi direct facade): the routine owner is the caller's teammate.
  ['routine_list','teammate.routines.list','List your own persistent Cindy routines. Use before creating to avoid duplicates, and after saving to verify.',object({})],
  ['routine_sources','teammate.routines.sources','List available local event sources, event types, filter fields and listening status. Read before creating event triggers; never guess source IDs.',object({})],
  ['routine_save','teammate.routines.save','Create or fully update your own persistent Cindy routine when the user requests scheduled reminders, recurring work or event-triggered automation. Multiple triggers are OR. Do not use a background Session or shell loop for recurring work. Do not invent an end time. Read back with routine_list before confirming success.',
   object({id:{type:'string',description:'Existing routine ID for updates; omit to create.'},name:{type:'string',minLength:1},prompt:{type:'string',minLength:1,description:'Instructions to execute at each trigger.'},enabled:{type:'boolean'},
    silentWhenIdle:{type:'boolean',description:'Set true for checks with no-change reporting suppressed. Set false for reminders/scheduled delivery. Omitted defaults to false; preserve user choices.'},
    preRunHook:{anyOf:[{type:'null'},object({command:{type:'string',minLength:1,maxLength:32000},timeoutMs:{type:'integer',minimum:1}},['command'])],description:'Install scripts with schedule_set_pre_run_hook; exit 2 skips the model, exit 0 passes stdout, failures stay visible. null removes; omission preserves.'},
    triggers:{type:'array',minItems:1,maxItems:32,items:routineTrigger}},['name','prompt','enabled','triggers'])],
  ['routine_history','teammate.routines.history','Read execution history and results of one of your routines.',object({id:{type:'string',minLength:1}},['id'])],
  ['routine_delete','teammate.routines.delete','Delete one of your routines when requested. To pause instead, save its complete configuration with enabled=false.',object({id:{type:'string',minLength:1}},['id'])],
  ['routine_run_now','teammate.routines.runNow','Run one of your saved routines now when requested.',object({id:{type:'string',minLength:1}},['id'])],
  ['schedule_set_pre_run_hook','teammate.routines.preRunHook','Install and immediately test a Node ESM pre-run check using the existing host installer. exit 0 wakes the model, exit 2 skips it, other errors fail visibly. Attach returned command with routine_save.preRunHook. Only invoke when authorized to install and test the check.',
   object({script:{type:'string',description:'Node ESM source; output a concise change summary. Use CINDY_PRECHECK_OK only after a complete successful check.'},scheduleName:string},['script'])],
  ['create_teammate','teammate.create',"Create a new Cindy Bot teammate directly when the user asks for one. Do not write a template file or tell the user to create it manually. Only a name is required. Follow the user’s default model chain and shared Bot baseline. Leave unspecified identity details open; the new teammate speaks for itself using its own runtime and memory. The returned bot id is immediately usable as send_to_agent target_id; creation alone does not start hidden work.",
   object({name:{type:'string',minLength:1,maxLength:200,description:'Display name for the new teammate.'},description:{type:'string',maxLength:4000,description:'Role or purpose, only if the user supplied it.'},
    identity_source:{type:'string',maxLength:12000,description:'Identity details explicitly requested by the user.'},welcome_message:{type:'string',maxLength:4000,description:'Legacy field; ignored. The new teammate generates its own greeting.'}},['name'])],
  // Follow-up loop: a woken companion reads the short task summary first, then its glance.
  ['get_task_digest','teammate.digest','Read a short (<=2KB) summary of one task: phase, elapsed minutes, last ask and answer, pending question, error, route, suggested next route, PR status and queue. Read this before deciding how to follow up.',object({session_id:taskId},['session_id'])],
  ['get_workbench','teammate.workbench.get','Your workbench at a glance: tasks waiting for the owner and tasks in progress, with the last sweep time.',object({})],
  ['read_workbench_task','teammate.workbench.read','Read the recent bounded transcript of a task you follow (all tasks if you follow every task).',object({task_id:taskId},['task_id'])],
  ['continue_workbench_task','teammate.workbench.continue','Send one message into a task you follow so it continues. Use get_task_digest first; never resend after DELIVERY_UNKNOWN without checking.',object({task_id:taskId,message:{type:'string',minLength:1,maxLength:4000}},['task_id','message'])],
  ['set_workbench_task_responsibility','teammate.workbench.responsibility',"Explicitly take or release responsibility for one Cindy task on this computer. Taking needs the owner's request in this turn; you may release on your own. Tasks you start yourself are recorded by the host. Release only changes your responsibility; it does not stop, move, archive or delete the task, and does not affect Todo. Taking responsibility alone does not start execution; the host then wakes you when the task gets stuck, waits, errors or finishes.",
   object({task_id:{type:'string',minLength:1,maxLength:256},responsible:{type:'boolean'}},['task_id','responsible'])],
  // Cindy bot_workbench Todo tools.
  ['list_teammate_todos','teammate.todos.list','按id/key读取同一事务（含完成/忽略记录），或分页搜索自己的清单，每页25条并返回总数。独立于项目/任务。维护优先精确读；无变化不扫描信息源。',
   object({id:{type:'string',maxLength:128},key:{type:'string',maxLength:512},query:{type:'string',maxLength:300},view:{type:'string',enum:['open','done','hidden']},origin:{type:'string',enum:['all','assigned','discovered']},offset:{type:'integer',minimum:0}})],
  ['update_teammate_todo','teammate.todos.update','新建或部分更新同一事务。key跨来源标识同一问题；已存在时用id+expectedRevision，冲突先读再合并，省略字段保留旧值。发现可入单但不自动授权执行/发送。完成必须携带可核验summary/ref；停止/合并未必满足outcome。删除/静音保留抑制记录，只有用户恢复可解除。来源可指向任务（kind=task，id 为本机任务 id），不需要项目。跟进处理完一件事后更新或完成对应事务。到期保留来源/时区，模糊日期用candidate，用户覆盖不被新来源改掉；稍后不修改期限。',
   object({patch:todoPatchSchema},['patch'])],
  ['preflight_teammate_todo_events','teammate.todos.preflight','现有已授权事件流的便宜增量预检：按来源游标、稳定问题key及忽略/完成记录过滤（project 只作原样回传，不再裁剪范围），只有review事件需要取相关正文/模型判断。本工具不连接、扫描或新建定时器；无新事件不调用。',
   object({events:{type:'array',maxItems:100,items:object({source:{type:'string',maxLength:512},sequence:{type:'integer',minimum:0},key:{type:'string',maxLength:512},project:{type:'string',maxLength:4096}},['source','sequence','key'])}},['events'])],
  ['record_teammate_todo_event','teammate.todos.ingest','原子保存一条成功处理的增量事件与Todo更新或no-action结果。source+sequence幂等，按来源顺序逐条提交，不跳过失败事件；失败不会推进游标。重复、已完成、已忽略的同一key不会重建。不同来源同一反馈沿用key/id，不能擅自绕过用户拒绝。',
   // The patch is update_teammate_todo's; its schema is not repeated here.
   object({source:{type:'string',maxLength:512},sequence:{type:'integer',minimum:0},patch:{type:'object',description:'The same patch fields as update_teammate_todo.patch.'},skip:{type:'string',enum:['duplicate','suppressed','outside-scope','no-action']}},['source','sequence'])],
  // Match Cindy's linked-project tool contract. These are context references,
  // not workspace grants; only canonical/thread sessions may use them.
  ['list_teammate_projects','projects.list','List your linked projects and availability. include_available also lists registered projects you may link. Read project instructions on demand with file tools. This does not read files, assign tasks, grant repository access or start background work.',object({include_available:{type:'boolean'}})],
  ['set_teammate_project','projects.set','Link or unlink one of your own projects when the owner requests it. Use project_id from list_teammate_projects(include_available=true). Files, registration, memory and task responsibility remain unchanged; linking provides context only.',object({project_id:{type:'string',pattern:'^[a-f0-9]{64}$'},linked:{type:'boolean'}},['project_id','linked'])],
 );
 if(target.memoryEnabled)definitions.push(
  ['memory_list','memory.list','List durable memory summaries; query performs full text search.',object({query:string})],
  ['memory_read','memory.read','Read a memory shard by its exact filename.',object({filename:string},['filename'])],
  ['memory_search','memory.search','Search durable memory, including Chinese substrings.',object({query:string,options:{type:'object',properties:{limit:{type:'integer'},type:string},additionalProperties:false}},['query'])],
  ['memory_write','memory.write','Persist reusable knowledge, preferences or corrections. Names are pure slugs, not paths. create refuses duplicates; update/append require an existing shard.',object({type:{enum:['user','feedback','project','reference']},name:string,title:string,description:string,body:string,mode:{enum:['create','update','append']}},['type','name','title','description','body'])],
  ['memory_consolidate','memory.consolidate','Atomically consolidate selected memory shards into one reviewed record.',object({sources:{type:'array',items:string},target:memoryEntry},['sources','target'])],
 );
 return definitions.map(([name,method,description,parameters,prepare,present])=>({name,label:name,description,parameters,
  execute:async(id,args,signal)=>{
   // Teammate tools are Host operations: the Host checks the turn's principal
   // and the Worker admits each action under Cindy's rules.
   const scope=await checkOneBotToolScope(config,{tool:name,args,external:true},signal);
   await check({id,tool:name,args,cwd:config.cwd,external:true,...scope},signal);
   const prepared=prepare?prepare(args):['skills.save','memory.write','memory.consolidate'].includes(method)?{input:args}:args;
   if(prepared?.ok===false)throw Error(JSON.stringify(prepared));
   const value=await request(config,'/tool',{method,args:prepared},signal);
   const payload=present?present(value,args):value;
   // Cindy reports a refused teammate action as a tool error with its code and hint.
   if(payload?.ok===false&&(present||method.startsWith('tasks.agents.')||method.startsWith('teammate.')))throw Error(JSON.stringify(payload));
   return {content:[{type:'text',text:JSON.stringify(payload)}],details:payload};
  },
 }));
}
