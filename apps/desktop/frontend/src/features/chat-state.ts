import type {AISelection} from "./ai-configuration";
export type ChatPermission="ask"|"auto"|"bypassPermissions";
export interface ChatProject{id:string;name:string;path:string;permission?:ChatPermission}
export interface Conversation{id:string;projectId?:string;title:string;directory:string;sessionDirectory:string;sessionFile?:string;model:AISelection;permission:ChatPermission;thinkingLevel?:string;fastMode?:boolean;archived?:boolean;pinned?:boolean;hasMessages?:boolean;createdAt:string;updatedAt:string}
export interface ChatActivity{phase:'idle'|'running'|'interrupted'|'needs-interaction'|'completed'|'error';attention:boolean;paused?:boolean}
export type ChatActivities=Record<string,ChatActivity>;
export interface ChatCatalog{projects:ChatProject[];conversations:Conversation[]}
export interface MessagePart{type:string;text?:string;thinking?:string;id?:string;name?:string;arguments?:Record<string,unknown>;thinkingStartedAt?:number;thinkingDurationMs?:number}
export interface ChatChangedFile{path:string;name:string;status:string;modified?:number;size?:number;patch?:string}
export type ChatDeliveryMode='queue'|'steer';
export interface ChatMessage{id?:string;presentationOnly?:boolean;agentMeta?:{voiceCall?:import('../../../../../modules/chat-pi/cindy/voice/voice-call').VoiceCallRecord;botCollaboration?:Record<string,unknown>;botDirectMessage?:Record<string,unknown>;botStarterOptions?:unknown;botLearning?:unknown};hiddenInput?:boolean;
 /** A OneBot owner message's quote of an earlier message in the same chat (Cindy ChatReplyQuote). */
 replyQuote?:import('./cindy/shared/chatReplyQuote').ChatReplyQuote;pending?:boolean;queued?:boolean;deliveryMode?:ChatDeliveryMode;role:string;content:MessagePart[]|string;timestamp?:number;durationMs?:number;turnId?:string;toolCallId?:string;toolName?:string;isError?:boolean;errorMessage?:string;model?:string;provider?:string;providerName?:string;modelName?:string;requestId?:string;stopReason?:string;details?:{files?:ChatChangedFile[]};attachments?:import('./chat-types').ChatAttachment[]}
export interface Approval{kind?:"question";questions?:{question:string;options?:string[]}[];id:string;tool:string;args:Record<string,unknown>;cwd?:string;outside?:boolean;protectedFile?:boolean;scopeLabel?:string;autoReviewUnavailable?:boolean;reason?:string}
export interface ProcessOperation{id:string;turnId:string;kind:"turn"|"compaction"|"retry"|"review"|"approval";startedAt:number;durationMs?:number;status:"running"|"completed"|"failed"|"stopped"|"allowed"|"denied";attempt?:number;maxAttempts?:number;delayMs?:number}
export interface ChatUsage{input:number;output:number;cacheRead:number;cacheWrite:number;totalTokens:number;cost:number;costKnown:boolean}
export interface ChatContextUsage{tokens:number;contextWindow:number;percent:number|null}
export interface ChatSubagent{name:string;role?:string;status:string;task?:string;model?:string;background?:boolean;conversationId?:string;result?:{status?:string;answer?:string}|null;createdAt?:number;finishedAt?:number}
export interface ChatWorkflow{items:{step:string;status:"pending"|"in_progress"|"completed"}[];explanation?:string}
export interface ChatTask{id:number;kind:string;conversationId:number;owner?:number;background:boolean;state:string}
export interface ChatTaskGraph{scheduling:string;tasks:ChatTask[];total:number}
export interface ChatSnapshot{messages:ChatMessage[];operations?:ProcessOperation[];hasOlder:boolean;before?:string;modelError?:string;streaming?:boolean;notice?:string;usage?:ChatUsage;contextUsage?:ChatContextUsage;subagents?:ChatSubagent[];workflow?:ChatWorkflow;backgroundRunning?:boolean;paused?:boolean;queuePaused?:boolean;taskGraph?:ChatTaskGraph}
export interface ToolActivity{id:string;name:string;args:Record<string,unknown>;running:boolean;result?:unknown;isError?:boolean;turnId?:string;files?:ChatChangedFile[]}
export interface ChatState extends ChatSnapshot{running:boolean;status:string;approvals:Approval[];tools:ToolActivity[];reviewing:boolean}
export interface ChatEvent{type:string;id?:string;requestId?:string;turnId?:string;operation?:ProcessOperation;request?:Approval;eventType?:string;message?:ChatMessage;kind?:string;index?:number;text?:string;toolCallId?:string;toolName?:string;args?:Record<string,unknown>;result?:unknown;isError?:boolean;status?:string;error?:string;state?:ChatSnapshot;running?:boolean;timestamp?:number;startedAt?:number;durationMs?:number;usage?:ChatUsage;contextUsage?:ChatContextUsage;subagents?:ChatSubagent[];workflow?:ChatWorkflow;backgroundRunning?:boolean;paused?:boolean;queuePaused?:boolean;taskGraph?:ChatTaskGraph;before?:number;after?:number}
export interface ChatPoll{events:{sequence:number;data:ChatEvent}[];cursor:number;reset?:boolean;more?:boolean;running:boolean;status:string;error?:string;approvals:Approval[]}
export const emptyChatState=():ChatState=>({messages:[],operations:[],hasOlder:false,running:false,status:"idle",approvals:[],tools:[],reviewing:false});
export function mergeChatOperations(older:ProcessOperation[]=[],newer:ProcessOperation[]=[]){return [...new Map([...older,...newer].map(operation=>[operation.id,operation])).values()];}
export function messageText(message:ChatMessage){return typeof message.content==="string"?message.content:message.content.filter(part=>part.type==="text").map(part=>part.text??"").join("\n");}
// Committed input receipts, not arrival order or text, identify a message.
// A steer can overtake queued input, and a saved assistant ID may arrive after
// its timestamp-only partial. Pending rows must never split a live response.
function sameMessage(a:ChatMessage,b:ChatMessage){
 if(a.role!==b.role)return false;
 if(a.id&&b.id&&a.id===b.id)return true;
 if(a.role==='user'&&a.requestId&&b.requestId)return a.requestId===b.requestId;
 return !a.pending&&!b.pending&&a.timestamp!==undefined&&a.timestamp===b.timestamp&&a.toolCallId===b.toolCallId&&(!a.turnId||!b.turnId||a.turnId===b.turnId);
}
export function upsertChatMessage(current:ChatMessage[],incoming:ChatMessage):ChatMessage[]{
 const messages=[...current],index=messages.findIndex(message=>sameMessage(message,incoming));
 if(index>=0&&!messages[index].pending){messages[index]={...incoming,id:incoming.id??messages[index].id};return messages;}
 if(index>=0)messages.splice(index,1);
 let before=incoming.pending?-1:messages.findIndex(message=>message.pending);
 if(incoming.role==='user'&&incoming.turnId){const answer=messages.findIndex(message=>message.role!=='user'&&message.turnId===incoming.turnId);if(answer>=0&&(before<0||answer<before))before=answer;}
 messages.splice(before<0?messages.length:before,0,incoming);return messages;
}
export function applyChatEvent(state:ChatState,event:ChatEvent):ChatState{
 if(event.type==='queue_changed'&&event.state){
  const saved=state.messages.filter(message=>!message.pending),ids=new Set(saved.map(message=>message.requestId));
  return {...state,queuePaused:event.state.queuePaused,messages:[...saved,...event.state.messages.filter(message=>message.pending&&!ids.has(message.requestId))]};
 }
 if(event.type==="notice")return {...state,notice:event.text??""};
 if(event.type==="usage")return {...state,usage:event.usage??state.usage,contextUsage:event.contextUsage??state.contextUsage};
 if(event.type==="durable_state")return {...state,subagents:event.subagents??[],workflow:event.workflow??state.workflow,backgroundRunning:!!event.backgroundRunning,paused:event.paused??state.paused,taskGraph:event.taskGraph??state.taskGraph};
 if((event.type==="resumed"||event.type==="branch_reset")&&event.state)return {...emptyChatState(),...event.state,running:!!event.state.streaming,status:event.state.streaming?'running':'idle'};
 if(event.type==="operation"&&event.operation)return {...state,operations:mergeChatOperations(state.operations,[event.operation])};
 if(event.type==="approval"&&event.request)return {...state,approvals:[...state.approvals.filter(value=>value.id!==event.request!.id),event.request]};
 if(event.type==="approval_cancelled")return {...state,approvals:state.approvals.filter(value=>value.id!==event.id)};
 if(event.type==="review")return {...state,reviewing:!!event.running};
 if(event.type==="thinking_timing")return {...state,messages:state.messages.map(message=>{
  if(message.role!=="assistant"||message.timestamp!==event.timestamp)return message;
  const content=typeof message.content==="string"?[{type:"text",text:message.content}]:message.content.map(part=>({...part}));
  const index=event.index??0;while(content.length<=index)content.push({type:"thinking",thinking:""});
  content[index]={...content[index],thinkingStartedAt:event.startedAt,thinkingDurationMs:event.durationMs};return {...message,content};
 })};
 if(event.type==="settled"&&event.state)return {...emptyChatState(),...event.state,running:false,status:event.status??"completed"};
 if(event.type==="delta"){
  const messages=[...state.messages];let index=messages.length-1;while(index>=0&&(messages[index].role!=="assistant"||(event.turnId&&messages[index].turnId!==event.turnId)||(event.timestamp!==undefined&&messages[index].timestamp!==event.timestamp)))index--;
  if(index<0){const pending=messages.findIndex(message=>message.pending);index=pending<0?messages.length:pending;messages.splice(index,0,{role:"assistant",content:[],turnId:event.turnId,timestamp:event.timestamp});}
  const current=messages[index],content=typeof current.content==="string"?[{type:"text",text:current.content}]:current.content.map(part=>({...part}));
  const part=event.index??0;while(content.length<=part)content.push({type:"text",text:""});
  if(event.kind==="thinking_delta"){content[part]={...content[part],type:"thinking",thinking:(content[part].thinking??"")+(event.text??"")};delete content[part].text;}
  else content[part]={type:"text",text:(content[part].text??"")+(event.text??"")};
  messages[index]={...current,content};return {...state,messages,running:true};
 }
 if(event.type==="pi_event"){
  if((event.eventType==="message_start"||event.eventType==="message_end")&&event.message){
   return {...state,messages:upsertChatMessage(state.messages,event.message)};
  }
  if(event.eventType?.startsWith("tool_execution_")){
   const tools=[...state.tools],id=event.toolCallId??"",index=tools.findIndex(tool=>tool.id===id);
   const tool:ToolActivity={...(index<0?{id,name:event.toolName??"tool",args:event.args??{}}:tools[index]),turnId:event.turnId??(index>=0?tools[index].turnId:undefined),running:event.eventType!=="tool_execution_end",result:event.result,isError:event.isError};
   if(index<0)tools.push(tool);else tools[index]=tool;return {...state,tools};
  }
 }
 return state;
}
