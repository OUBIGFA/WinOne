import {messageText,type ChatMessage,type MessagePart,type ToolActivity,type ProcessOperation} from "./chat-state";

export type ProcessEntry={at?:number;endedAt?:number}&({kind:"thinking";key:string;part:MessagePart}|{kind:"commentary";key:string;text:string;message?:ChatMessage;delivered?:boolean}|{kind:"tool";key:string;tool:ToolActivity}|{kind:"operation";key:string;operation:ProcessOperation});
export type ProcessResponse={kind:"response";key:string;messages:ChatMessage[];entries:ProcessEntry[];answer?:ChatMessage;running:boolean;startedAt?:number;durationMs?:number;hasTools:boolean};
export type ProcessRenderItem={kind:"user";key:string;message:ChatMessage}|ProcessResponse;

// Pi stores one assistant/tool sequence per user turn. Project it into Cindy's
// work-process grouping. Every public text part is a normal reply; only thinking
// and actions belong to disclosures, even when text shares a tool-call message.
export function projectChatProcess(messages:ChatMessage[],liveTools:ToolActivity[],running:boolean,operations:ProcessOperation[]=[]):ProcessRenderItem[]{
 const output:ProcessRenderItem[]=[],groups=new Map<string,ProcessResponse>();let turn="history";
 function response(key:string){let group=groups.get(key);if(!group){group={kind:"response",key,messages:[],entries:[],running:false,hasTools:false};groups.set(key,group);const user=output.findIndex(item=>item.kind==="user"&&item.key===key);if(user>=0)output.splice(user+1,0,group);else output.push(group);}return group;}
 for(const [index,message] of messages.entries()){
  if(message.role==="user"){
   turn=message.turnId??message.requestId??message.id??`user-${message.timestamp??index}`;
   // A progress batch can expose a partial before the committed user entry.
   // Re-anchor that response when its owner arrives; never leave it above input.
   const existing=groups.get(turn);if(existing)output.splice(output.indexOf(existing),1);
   output.push({kind:"user",key:turn,message});if(existing)output.push(existing);continue;
  }
  if(message.role!=="assistant"&&message.role!=="toolResult")continue;
  const key=message.turnId??turn;
  const group=response(key);
  group.messages.push(message);
 }
 const activeTurn=running?([...operations].reverse().find(operation=>operation.status==="running")?.turnId??[...messages].reverse().find(message=>message.role==='user'&&!message.pending)?.turnId??turn):undefined;
 if(activeTurn)response(activeTurn);
 for(const operation of operations)response(operation.turnId);
 const responses=output.filter((item):item is ProcessResponse=>item.kind==="response"),live=new Map(liveTools.map(tool=>[tool.id,tool]));
 const accounted=new Set<string>();
 for(const group of responses){
  group.running=running&&group.key===activeTurn;
  const assistants=group.messages.filter(message=>message.role==="assistant"),last=assistants.at(-1);
  // A started/thinking-only message is not a reply boundary. Cindy's answer
  // predicate requires actual prose; otherwise every model step folds/reopens
  // the previous latest-five preview before its next tool even starts.
  if(last&&(messageText(last).trim()||last.errorMessage)&&(typeof last.content==="string"||!last.content.some(part=>part.type==="toolCall")))group.answer=last;
  const results=new Map(group.messages.filter(message=>message.role==="toolResult").map(message=>[message.toolCallId,message]));
  for(const [messageIndex,message] of group.messages.entries()){
   if(message.role!=="assistant")continue;
   if(message.timestamp!==undefined)group.startedAt=group.startedAt===undefined?message.timestamp:Math.min(group.startedAt,message.timestamp);
   const parts=typeof message.content==="string"?[{type:"text",text:message.content}]:message.content;
   for(const [partIndex,part] of parts.entries()){
    // Pi's persisted entry ID arrives after streaming. The request timestamp
    // already anchors both start and end; switching to the late ID remounts
    // Cindy's disclosure and discards its remembered expansion.
    const key=`${message.timestamp??message.id??messageIndex}:${partIndex}`;
    if(part.type==="thinking"&&part.thinking?.trim())group.entries.push({kind:"thinking",key,part,at:part.thinkingStartedAt??message.timestamp,endedAt:part.thinkingStartedAt!==undefined&&part.thinkingDurationMs!==undefined?part.thinkingStartedAt+part.thinkingDurationMs:undefined});
    if(part.type==="text"&&message!==group.answer&&part.text)group.entries.push({kind:"commentary",key,text:part.text,message:{...message,content:part.text},delivered:true,at:message.timestamp});
    if(part.type==="toolCall"&&part.id){
     const result=results.get(part.id),current=live.get(part.id);accounted.add(part.id);group.hasTools=true;
     group.entries.push({kind:"tool",key:part.id,at:message.timestamp,endedAt:result?.timestamp,tool:{id:part.id,name:part.name??current?.name??"tool",args:part.arguments??current?.args??{},running:!result&&group.running&&(current?.running??true),result:result?messageText(result):current?.result,isError:result?.isError??current?.isError,files:result?.details?.files??(current?.result as {details?:{files?:import("./chat-state").ChatChangedFile[]}}|undefined)?.details?.files}});
    }
   }
  }
  // A page may start after a tool's call. Keep that persisted result visible.
  for(const result of results.values()){if(!result.toolCallId||accounted.has(result.toolCallId))continue;accounted.add(result.toolCallId);group.hasTools=true;group.entries.push({kind:"tool",key:result.toolCallId,tool:{id:result.toolCallId,name:result.toolName??"tool",args:{},running:false,result:messageText(result),isError:result.isError}});}
  if(!group.running&&group.startedAt!==undefined&&last?.timestamp!==undefined&&last.durationMs!==undefined)group.durationMs=Math.max(0,last.timestamp+last.durationMs-group.startedAt);
 }
 for(const tool of liveTools){if(!accounted.has(tool.id)){const group=response(tool.turnId??activeTurn??turn);group.entries.push({kind:"tool",key:tool.id,tool:{...tool,running:running&&tool.running}});group.hasTools=true;}}
 for(const operation of operations){
  const group=response(operation.turnId);
  if(operation.kind==="turn"){group.startedAt=operation.startedAt;group.durationMs=operation.durationMs;}
  else group.entries.push({kind:"operation",key:operation.id,at:operation.startedAt,operation});
 }
 for(const group of groups.values()){
  const active=operations.find(operation=>operation.turnId===group.key&&operation.status==="running");
  if(group.running&&active)group.startedAt=active.startedAt;
  group.entries.sort((a,b)=>(a.at??Number.MAX_SAFE_INTEGER)-(b.at??Number.MAX_SAFE_INTEGER));
 }
 return output;
}
