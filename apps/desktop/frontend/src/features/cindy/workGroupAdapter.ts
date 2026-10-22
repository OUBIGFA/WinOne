// Group construction adapts Cindy messageWorkGroups.ts. Turn, answer and gap
// rules themselves come from the unchanged shared implementation.
import {groupWorkRuns} from "./shared/workRunGrouping";
import {planForResponse,workflowFromTool} from '../chat-plan-state';
import type {ProcessEntry,ProcessResponse} from "../chat-process";
export type WorkEntry={type:"entry";key:string;entry:ProcessEntry;at?:number;endedAt?:number};
export type WorkGroup={type:"group";key:string;children:WorkItem[];running:boolean;startedAt?:number;durationMs?:number};
export type WorkItem=WorkEntry|WorkGroup|{type:"boundary"|"answer";key:string;at?:number;sealed?:boolean};
const isActivity=(item:WorkItem):item is WorkEntry=>item.type==="entry"&&item.entry.kind!=="commentary"&&!(item.entry.kind==='tool'&&workflowFromTool(item.entry.tool));
const start=(item:WorkItem|undefined):number|null=>!item?null:item.type==="group"?item.startedAt??null:item.at??null;
const end=(item:WorkItem|undefined):number|null=>item?.type==="entry"?item.endedAt??item.at??null:start(item);
function makeGroup(run:WorkEntry[],next:WorkItem|undefined,running:boolean,boundary:number|null):WorkGroup{
 const first=run.find(isActivity)??run[0],anchor=start(first);
 const startedAt=boundary!==null&&(anchor===null||boundary<=anchor)?boundary:anchor;
 const endedAt=start(next)??run.reduce<number|null>((latest,item)=>{const at=end(item);return at===null?latest:latest===null?at:Math.max(latest,at);},null);
 return {type:"group",key:`work-${first.key}`,children:run,running,...(startedAt!==null?{startedAt}:{}),...(startedAt!==null&&endedAt!==null&&endedAt>=startedAt?{durationMs:endedAt-startedAt}:{})};
}
function completedGroup(run:WorkEntry[],next:WorkItem|undefined,boundary:number|null):WorkGroup{
 if(!run.some(item=>item.entry.kind==="commentary"))return makeGroup(run,next,false,boundary);
 const children:WorkItem[]=[];let activities:WorkEntry[]=[],previous=boundary;
 const flush=(next:WorkItem|undefined)=>{if(activities.length){children.push(makeGroup(activities,next,false,previous));activities=[];}};
 for(const item of run){if(isActivity(item)){activities.push(item);continue;}flush(item);children.push(item);previous=start(item);}
 flush(next);return {...makeGroup(run,next,false,boundary),key:`work-summary-${(run.find(isActivity)??run[0]).key}`,children};
}
export function workGroupsForResponse(response:ProcessResponse):WorkItem[]{
 const plan=planForResponse(response);
 const entries:WorkItem[]=response.entries.flatMap(entry=>{
  if(entry.kind==='tool'&&workflowFromTool(entry.tool)){
   // Keep one stable inline anchor per turn, updating it from the newest
   // committed plan rather than exposing update_plan JSON as an action row.
   return entry.key===plan?.anchor?[{type:'entry' as const,key:entry.key,entry:{...entry,tool:plan.latest.tool},at:entry.at,endedAt:entry.endedAt}]:[];
  }
  return [{type:'entry' as const,key:entry.key,entry,at:entry.at,endedAt:entry.endedAt}];
 });
 entries.unshift({type:"boundary",key:response.key,at:response.startedAt});
 // The actual answer is rendered by ChatMessageView. This boundary is only a
 // grouping input, so working text cannot be mistaken for the final answer.
 // Cindy's answer predicate excludes failed attempts. Their error metadata
 // is not a delivery: retry start/end must leave the existing live group alone.
 const hasAnswer=response.answer&&!response.answer.errorMessage&&response.answer.stopReason!=='error'&&response.answer.stopReason!=='aborted';
 if(hasAnswer||!response.running)entries.push({type:"answer",key:`answer:${response.key}`,at:response.answer?.timestamp??(response.startedAt!==undefined&&response.durationMs!==undefined?response.startedAt+response.durationMs:undefined),sealed:!response.running});
 return groupWorkRuns<WorkItem,WorkEntry>(entries,response.running,{
  isUserBoundary:item=>item.type==="boundary",isAnswer:item=>item.type==="answer"||item.type==="entry"&&item.entry.kind==="commentary",
  isSealedAnswer:item=>item.type==="answer"&&!!item.sealed||item.type==="entry"&&item.entry.kind==="commentary",
  isCompactBoundary:item=>item.type==="entry"&&item.entry.kind==="operation"&&item.entry.operation.kind==="compaction",
  // A complete text reply is a delivery boundary, including before a later tool run.
  isActivity,isArchivable:isActivity,startTimestamp:start,endTimestamp:end,boundaryTimestamp:start,
  userBoundaryEnd:(item,previous)=>end(item)??previous,createGroup:makeGroup,createCompletedGroup:completedGroup,
 }).filter(item=>item.type!=="answer"&&item.type!=="boundary");
}
