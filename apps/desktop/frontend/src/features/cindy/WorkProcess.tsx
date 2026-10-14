// WorkGroupBlock/AgentActionsBlock presentation adapted from Cindy (Apache-2.0).
// WinOne provides Pi's work entries; the disclosure, rail, timing and remembered
// expansion retain Cindy's component patterns. Lobe UI supplies long scrolling.
import {Layers,LoaderCircle,RotateCcw,ShieldCheck} from "lucide-react";
import {useTranslation} from "../../i18n";
import {WorkGroupBlock,type WorkGroupChild} from "./WorkGroupBlock";
import {workGroupsForResponse,type WorkItem} from "./workGroupAdapter";
import {formatSessionDuration} from "./adapter";
import {ChatMarkdown} from "../chat-messages";
import {ThinkingCard} from "./ThinkingCard";
import type {ProcessResponse} from "../chat-process";
import type {ProcessOperation} from "../chat-state";

function Operation({operation}:{operation:ProcessOperation}){
 const {t}=useTranslation();
 const Icon=operation.status==="running"?LoaderCircle:operation.kind==="retry"?RotateCcw:operation.kind==="review"||operation.kind==="approval"?ShieldCheck:Layers;
 const label={turn:"processWorking",compaction:"operationCompaction",retry:"operationRetry",review:"operationReview",approval:"operationApproval"}[operation.kind];
 return <div className="flex items-start gap-2 text-sm text-muted-foreground"><Icon size={14} className={"mt-1 shrink-0"+(operation.status==="running"?" animate-spin motion-reduce:animate-none":"")}/><div className="min-w-0 flex-1"><span>{t(`chat.${label}`)}</span>{operation.attempt!==undefined&&<span> · {operation.attempt}{operation.maxAttempts!==undefined?` / ${operation.maxAttempts}`:""}</span>}<span className={"ml-2"+(operation.status==="failed"||operation.status==="denied"?" text-destructive":"")}>{t(`chat.operationStatus_${operation.status}`)}</span></div>{operation.durationMs!==undefined&&<span className="shrink-0 font-mono text-xs">{formatSessionDuration(operation.durationMs)}</span>}</div>;
}

export function WorkProcess({group,conversationId,onCopy}:{group:ProcessResponse;conversationId:string;onCopy:(text:string)=>void}){
 function child(item:WorkItem):WorkGroupChild{
  const key=`${conversationId}:${group.key}:${item.key}`;
  if(item.type==="group")return {kind:"group",key,blockId:key,durationMs:item.durationMs,isStreaming:item.running,startedAtMs:item.startedAt,childItems:item.children.map(child)};
  if(item.type!=="entry")return {kind:"rendered",key,renderNode:()=>null};
  const entry=item.entry;
  if(entry.kind==="thinking")return {kind:"thinking",key,message:{clientId:key,content:entry.part.thinking??""}};
  if(entry.kind==="tool"){
   const tool=entry.tool,result=typeof tool.result==="string"?tool.result:tool.result===undefined?undefined:JSON.stringify(tool.result,null,2);
   return {kind:"tools",key,toolCalls:[{clientId:key,content:"",toolName:tool.name,toolInput:tool.args,isError:tool.isError}],resultMap:new Map(result===undefined?[]:[[key,result]]),settledIds:new Set(tool.running?[]:[key])};
  }
  return {kind:"rendered",key,renderNode:()=>entry.kind==="operation"?<Operation operation={entry.operation}/>:<ChatMarkdown text={entry.text} onCopy={onCopy}/>};
 }
 // Cindy keeps a compact live-thinking row visible as soon as a turn starts,
 // including the short interval before Pi has emitted its first event.
 // Keeping this fallback here makes the loading state use the same disclosure,
 // animation and remembered expansion behavior as ordinary thinking blocks.
 if(!group.entries.length)return group.running?<div className="cindy-chat-ui chat-work-process"><ThinkingCard content="" isStreaming startedAt={group.startedAt} blockKey={`${conversationId}:${group.key}:live`}/></div>:null;
 return <div className="cindy-chat-ui chat-work-process">{workGroupsForResponse(group).map(item=>{
  const entry=child(item);return entry.kind==="group"?<WorkGroupBlock key={entry.key} forceExpanded={group.running} blockId={entry.blockId} durationMs={entry.durationMs} isStreaming={entry.isStreaming} startedAtMs={entry.startedAtMs} childItems={entry.childItems}/>:entry.kind==="rendered"?<div key={entry.key} className="chat-work-commentary">{entry.renderNode()}</div>:entry.kind==="thinking"?<ThinkingCard key={entry.key} content={entry.message.content} isStreaming={group.running} startedAt={group.startedAt} durationMs={group.running?undefined:group.durationMs} blockKey={entry.key}/>:null;
 })}</div>;
}
