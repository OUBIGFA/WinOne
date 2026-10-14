import type {AISelection} from "./ai-configuration";
export type ChatPermission="ask"|"auto"|"bypassPermissions";
export interface ChatProject{id:string;name:string;path:string;permission?:ChatPermission}
export interface Conversation{id:string;projectId?:string;title:string;directory:string;sessionDirectory:string;sessionFile?:string;model:AISelection;permission:ChatPermission;archived?:boolean;pinned?:boolean;createdAt:string;updatedAt:string}
export interface ChatCatalog{projects:ChatProject[];conversations:Conversation[]}
export interface MessagePart{type:string;text?:string;thinking?:string;id?:string;name?:string;arguments?:Record<string,unknown>;thinkingStartedAt?:number;thinkingDurationMs?:number}
export interface ChatChangedFile{path:string;name:string;status:string;modified?:number;size?:number;patch?:string}
export interface ChatMessage{id?:string;role:string;content:MessagePart[]|string;timestamp?:number;durationMs?:number;turnId?:string;toolCallId?:string;toolName?:string;isError?:boolean;errorMessage?:string;model?:string;provider?:string;providerName?:string;modelName?:string;requestId?:string;stopReason?:string;details?:{files?:ChatChangedFile[]};attachments?:import('./chat-types').ChatAttachment[]}
export interface Approval{id:string;tool:string;args:Record<string,unknown>;cwd?:string;outside?:boolean;protectedFile?:boolean;scopeLabel?:string;autoReviewUnavailable?:boolean;reason?:string}
export interface ProcessOperation{id:string;turnId:string;kind:"turn"|"compaction"|"retry"|"review"|"approval";startedAt:number;durationMs?:number;status:"running"|"completed"|"failed"|"stopped"|"allowed"|"denied";attempt?:number;maxAttempts?:number;delayMs?:number}
export interface ChatSnapshot{messages:ChatMessage[];operations?:ProcessOperation[];hasOlder:boolean;before?:string;modelError?:string;streaming?:boolean;notice?:string}
export interface ToolActivity{id:string;name:string;args:Record<string,unknown>;running:boolean;result?:unknown;isError?:boolean;turnId?:string;files?:ChatChangedFile[]}
export interface ChatState extends ChatSnapshot{running:boolean;status:string;approvals:Approval[];tools:ToolActivity[];reviewing:boolean}
export interface ChatEvent{type:string;id?:string;turnId?:string;operation?:ProcessOperation;request?:Approval;eventType?:string;message?:ChatMessage;kind?:string;index?:number;text?:string;toolCallId?:string;toolName?:string;args?:Record<string,unknown>;result?:unknown;isError?:boolean;status?:string;error?:string;state?:ChatSnapshot;running?:boolean;timestamp?:number;startedAt?:number;durationMs?:number}
export interface ChatPoll{events:{sequence:number;data:ChatEvent}[];cursor:number;reset?:boolean;more?:boolean;running:boolean;status:string;error?:string;approvals:Approval[]}
export const emptyChatState=():ChatState=>({messages:[],operations:[],hasOlder:false,running:false,status:"idle",approvals:[],tools:[],reviewing:false});
export function mergeChatOperations(older:ProcessOperation[]=[],newer:ProcessOperation[]=[]){return [...new Map([...older,...newer].map(operation=>[operation.id,operation])).values()];}
export function messageText(message:ChatMessage){return typeof message.content==="string"?message.content:message.content.filter(part=>part.type==="text").map(part=>part.text??"").join("\n");}
const messageKey=(message:ChatMessage)=>message.id??`${message.role}-${message.timestamp??0}-${message.toolCallId??""}`;
export function applyChatEvent(state:ChatState,event:ChatEvent):ChatState{
 if(event.type==="notice")return {...state,notice:event.text??""};
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
  if(index<0){index=messages.length;messages.push({role:"assistant",content:[],turnId:event.turnId,timestamp:event.timestamp});}
  const current=messages[index],content=typeof current.content==="string"?[{type:"text",text:current.content}]:current.content.map(part=>({...part}));
  const part=event.index??0;while(content.length<=part)content.push({type:"text",text:""});
  if(event.kind==="thinking_delta"){content[part]={...content[part],type:"thinking",thinking:(content[part].thinking??"")+(event.text??"")};delete content[part].text;}
  else content[part]={type:"text",text:(content[part].text??"")+(event.text??"")};
  messages[index]={...current,content};return {...state,messages,running:true};
 }
 if(event.type==="pi_event"){
  if((event.eventType==="message_start"||event.eventType==="message_end")&&event.message){
   // Request receipts identify a user input before Pi assigns its history ID.
   // Text is not an identity: users may deliberately send the same prompt twice.
   const incoming=event.message,messages=[...state.messages],key=messageKey(incoming),index=messages.findIndex(message=>
    messageKey(message)===key||(incoming.role==='user'&&message.role==='user'&&!!incoming.requestId&&message.requestId===incoming.requestId));
   if(index<0)messages.push(event.message);else messages[index]=event.message;
   return {...state,messages};
  }
  if(event.eventType?.startsWith("tool_execution_")){
   const tools=[...state.tools],id=event.toolCallId??"",index=tools.findIndex(tool=>tool.id===id);
   const tool:ToolActivity={...(index<0?{id,name:event.toolName??"tool",args:event.args??{}}:tools[index]),turnId:event.turnId??(index>=0?tools[index].turnId:undefined),running:event.eventType!=="tool_execution_end",result:event.result,isError:event.isError};
   if(index<0)tools.push(tool);else tools[index]=tool;return {...state,tools};
  }
 }
 return state;
}
