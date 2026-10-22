import type {ChatWorkflow,ToolActivity} from './chat-state';
import type {ProcessResponse} from './chat-process';
import {toolResultText} from './chat-tool-content';

/** Only a completed, successful tool result can publish a plan. A streamed
 * argument, rejected call or stopped call is not durable workflow evidence. */
export function workflowFromTool(tool:ToolActivity):ChatWorkflow|undefined{
 if(tool.name!=='update_plan'||tool.running||tool.isError||tool.result===undefined)return;
 let value:unknown;
 try{value=JSON.parse(toolResultText(tool.result)??'');}catch{/* Older tools reply with a short success string. */}
 const saved=value&&typeof value==='object'?value as Record<string,unknown>:undefined;
 const items=saved?.items??tool.args.plan;
 if(!Array.isArray(items)||items.some(item=>!item||typeof item.step!=='string'||!['pending','in_progress','completed'].includes(item.status)))return;
 return {items,explanation:typeof saved?.explanation==='string'?saved.explanation:typeof tool.args.explanation==='string'?tool.args.explanation:undefined};
}
export function planForResponse(response:ProcessResponse){
 const plans=response.entries.flatMap(entry=>{if(entry.kind!=='tool')return [];const workflow=workflowFromTool(entry.tool);return workflow?[{entry,workflow}]:[];});
 const latest=plans.at(-1);
 return latest?{anchor:plans[0].entry.key,latest:latest.entry,workflow:latest.workflow}:undefined;
}
