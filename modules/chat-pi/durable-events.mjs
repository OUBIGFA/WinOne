// Pi Durable 1.5 removed watchEvents(): a client renders the conversation
// view instead. WinOne's worker protocol, its Host and both Chat front ends
// still speak those events, so this module derives them from one view watch
// call (its value before and after, and the folded operations between), as
// pi-durable 1.4.0's harness/events.js derived them from a commit. Event
// shapes, fields and their order follow that file; what the view does not
// hold is reported elsewhere: task failures through onReport, submission
// settlements by waiting on the submissions themselves.

const PARTIAL_PATH=['docs','pi.live','generation','message'];
const same=(a,b)=>a===b||JSON.stringify(a)===JSON.stringify(b);

export function viewParts(view){
 return {live:view?.docs?.['pi.live']??{},inbox:view?.docs?.['pi.inbox'],usage:view?.docs?.['pi.usage']};
}

const startsWith=(path,prefix)=>Array.isArray(path)&&prefix.length<=path.length&&prefix.every((segment,index)=>path[index]===segment);

/** The view operations on the in-flight message as message changes. */
function messageChanges(ops,message){
 const changes=[],whole=new Set();
 for(const op of ops){
  // A replacement or a reorder carries no delta of its own.
  if(op[0]==='r'||op[0]==='m')return [{type:'message',message}];
  const path=op[1];
  if(!startsWith(path,PARTIAL_PATH)){
   if(startsWith(PARTIAL_PATH,path))return [{type:'message',message}];
   continue;
  }
  const rest=path.slice(PARTIAL_PATH.length);
  if(rest[0]==='usage')continue;
  if(rest[0]!=='content')return [{type:'message',message}];
  if(rest.length===1){
   if(op[0]!=='p'||op[3]!==0)return [{type:'message',message}];
   for(const [offset,block] of op[4].entries()){
    const type=block.type==='text'?'text_start':block.type==='thinking'?'thinking_start':'toolcall_start';
    changes.push({type,contentIndex:op[2]+offset,block});
   }
   continue;
  }
  const contentIndex=rest[1],field=rest[2];
  if(whole.has(contentIndex))continue;
  if(op[0]==='a'&&rest.length===3&&(field==='text'||field==='thinking'))changes.push({type:field==='text'?'text_delta':'thinking_delta',contentIndex,delta:op[2]});
  else if(op[0]==='a'&&field==='arguments')changes.push({type:'toolcall_delta',contentIndex,path:rest.slice(3),delta:op[2]});
  else{whole.add(contentIndex);changes.push({type:'block',contentIndex,block:message.content[contentIndex]});}
 }
 return changes;
}

function callOf(slot){
 return {toolCallId:slot.callId,toolName:slot.name,...(slot.taskId===undefined?{}:{taskId:slot.taskId}),
  ...('parentCallId' in slot?{parentToolCallId:slot.parentCallId,parentTaskId:slot.parentTaskId}:{})};
}

/** Output, details and diagnostics changes of a running slot at `at` in pi.live. */
function toolUpdate(ops,at,slot,previous){
 const outputPath=['docs','pi.live',...at,'output'];
 let trimStart=0,append='',set=false;
 for(const op of ops){
  if(op[0]==='r'||op[0]==='m'){set=true;continue;}
  if(!startsWith(op[1],outputPath))continue;
  if(op[0]==='t')trimStart+=op[2];else if(op[0]==='a')append+=op[2];else set=true;
 }
 let output;
 if(set||(slot.output!==previous.output&&trimStart===0&&append===''))output={set:slot.output??''};
 else if(trimStart>0||append!=='')output={...(trimStart>0?{trimStart}:{}),...(append===''?{}:{append})};
 if(output?.set!==undefined&&slot.output===previous.output)output=undefined;
 const update={...(output===undefined?{}:{output}),
  ...(same(slot.details,previous.details)?{}:{details:slot.details??null}),
  ...(same(slot.diagnostics,previous.diagnostics)?{}:{diagnostics:slot.diagnostics??[]})};
 return Object.keys(update).length===0?undefined:update;
}

/** The arguments a model-issued call was made with: in its answer's toolCall part. */
function argumentsOf(callId,view,entries){
 const messages=[viewParts(view).live.generation?.message,...[...entries].reverse().map(entry=>entry.model?.[0]),...[...(view?.entries??[])].reverse().map(entry=>entry.model?.[0])];
 for(const message of messages){
  if(message?.role!=='assistant'||!Array.isArray(message.content))continue;
  const part=message.content.find(part=>part.type==='toolCall'&&part.id===callId);
  if(part)return part.arguments??{};
 }
 return {};
}

/**
 * Every event one watch call causes. `entries` are the entries appended since
 * `before`, in ID order (a window overflow may hold more than the view shows).
 */
export function translateView(before,after,ops,entries){
 const was=viewParts(before),now=viewParts(after),events=[];
 const slotsBefore=new Map((was.live.tools??[]).map(slot=>[slot.callId,slot]));
 const slots=now.live.tools??[];
 const nestedBefore=new Map((was.live.nestedTools??[]).map(slot=>[slot.taskId,slot]));
 const nested=now.live.nestedTools??[];
 for(const slot of [...slots,...nested]){
  const previous='parentCallId' in slot?nestedBefore.get(slot.taskId):slotsBefore.get(slot.callId);
  if(slot.status!=='running'||previous?.status==='running')continue;
  events.push({type:'tool_execution_start',...callOf(slot),args:'parentCallId' in slot?slot.arguments:argumentsOf(slot.callId,after,entries)});
 }
 const partialBefore=was.live.generation?.message,partial=now.live.generation?.message;
 if(partial!==undefined&&partialBefore===undefined)events.push({type:'message_start',message:partial});
 else if(partial!==undefined&&!same(partial,partialBefore))events.push({type:'message_update',usage:partial.usage,changes:messageChanges(ops,partial)});
 for(const [list,previousOf,key] of [[slots,slot=>slotsBefore.get(slot.callId),'tools'],[nested,slot=>nestedBefore.get(slot.taskId),'nestedTools']]){
  for(const [index,slot] of list.entries()){
   const previous=previousOf(slot);
   if(slot.status!=='running'||previous?.status!=='running')continue;
   const update=toolUpdate(ops,[key,index],slot,previous);
   if(update)events.push({type:'tool_execution_update',...callOf(slot),...update});
  }
 }
 const generation=now.live.generation,generationBefore=was.live.generation;
 if(generation?.retry!==undefined&&generationBefore?.retry===undefined)events.push({type:'auto_retry_start',attempt:generation.attempt,at:generation.retry.at,errorMessage:generation.retry.error});
 if(generationBefore?.retry!==undefined&&generation?.retry===undefined)events.push({type:'auto_retry_end',attempt:generationBefore.attempt});
 // Tools that end here: a slot that becomes done, one created done, or an
 // unfinished one that vanishes with its run. A done slot that vanishes ended earlier.
 const toolEnds=[];
 const resultOf=callId=>entries.find(entry=>{const message=entry.model?.[0];return message?.role==='toolResult'&&message.toolCallId===callId;});
 const endTool=(slot,entryId)=>{
  const entry=entries.find(candidate=>candidate.id===entryId)??(after?.entries??[]).find(candidate=>candidate.id===entryId);
  toolEnds.push({type:'tool_execution_end',...callOf(slot),...(entry===undefined?{}:{entry})});
 };
 for(const previous of slotsBefore.values()){
  if(previous.status==='done')continue;
  const slot=slots.find(candidate=>candidate.callId===previous.callId);
  if(slot?.status==='done')endTool(slot,slot.entry);
  else if(slot===undefined)endTool(previous,resultOf(previous.callId)?.id);
 }
 for(const slot of slots)if(slot.status==='done'&&!slotsBefore.has(slot.callId))endTool(slot,slot.entry);
 const nestedNow=new Map(nested.map(slot=>[slot.taskId,slot]));
 for(const previous of [...nestedBefore.values()].reverse()){
  if(previous.status==='done')continue;
  const slot=nestedNow.get(previous.taskId);
  if(slot?.status==='done'||slot===undefined)events.push({type:'tool_execution_end',...callOf(slot??previous)});
 }
 for(const slot of [...nested].reverse())if(slot.status==='done'&&!nestedBefore.has(slot.taskId))events.push({type:'tool_execution_end',...callOf(slot)});
 // Entries in append order; a tool's end directly precedes its result's message.
 let assistantAppended=false;
 const placed=new Set();
 for(const entry of entries){
  for(const end of toolEnds)if(end.entry?.id===entry.id){events.push(end);placed.add(end);}
  const message=entry.model?.[0];
  if(message===undefined){events.push({type:'entry_appended',entry});continue;}
  // A streamed answer already started with its first partial.
  const streamed=message.role==='assistant'&&partialBefore!==undefined&&!assistantAppended;
  if(message.role==='assistant')assistantAppended=true;
  if(!streamed)events.push({type:'message_start',message});
  events.push({type:'message_end',entry});
 }
 for(const end of toolEnds)if(!placed.has(end))events.push(end);
 const compactionsBefore=was.live.compactions??[],compactions=now.live.compactions??[],failure=now.live.compactionFailure;
 for(const {taskId,trigger} of compactionsBefore){
  if(compactions.some(status=>status.taskId===taskId))continue;
  const error=failure?.taskId===taskId?failure.error:undefined;
  events.push({type:'compaction_end',taskId,reason:trigger,...(error===undefined?{}:{error})});
 }
 const run=now.live.run,runBefore=was.live.run,runChanged=run?.inputs?.[0]!==runBefore?.inputs?.[0];
 if(runBefore!==undefined&&runChanged)events.push({type:'run_end',inputs:runBefore.inputs});
 if(!same(now.usage,was.usage))events.push({type:'usage_changed',usage:now.usage??{models:{},tools:{}}});
 for(const {taskId,trigger,blocking} of compactions){
  if(!compactionsBefore.some(status=>status.taskId===taskId))events.push({type:'compaction_start',taskId,reason:trigger,blocking});
 }
 if(run!==undefined&&runChanged)events.push({type:'run_start',inputs:run.inputs});
 return events;
}

/** The submissions a view shows: queued in the inbox, placed in the run, held by WinOne. */
export function viewSubmissions(view){
 const {live,inbox}=viewParts(view);
 return [...(inbox?.items??[]).map(item=>item.id),...(live.run?.inputs??[]),...(view?.docs?.['winone.adapter']?.heldInputs??[]).map(item=>item.id)];
}
