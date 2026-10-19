import {
 projectWorkActivities as projectAll,projectRecentWorkActivities as projectRecent,
 type WorkActivityMessageLike,type ProjectableWorkChild,
 type ProjectedToolActivity as SharedTool,type ProjectedWorkActivity as SharedActivity,
} from "./shared/workActivityProjection";
export type {ProjectedThinkingActivity} from "./shared/workActivityProjection";

/** Pi data is normalized at this boundary. Cindy owns activity grouping and
 * command interpretation; the adapter never executes a described command. */
export interface WorkMessage extends WorkActivityMessageLike {isError?:boolean;role?:string}
export type ProjectedToolActivity=SharedTool<WorkMessage>;
export type ProjectedWorkActivity=SharedActivity<WorkMessage>;
export const projectWorkActivities=(items:readonly ProjectableWorkChild<WorkMessage>[],running:boolean)=>projectAll(items,running);
export const projectRecentWorkActivities=(items:readonly ProjectableWorkChild<WorkMessage>[],running:boolean,limit:number)=>projectRecent(items,running,limit);

// Retain Cindy's deferred-history hooks. WinOne currently pages through Pi's
// history at the conversation boundary, so no synthetic deferred data is used.
// Display-only running lines follow Cindy db24db2a's HistoryWorkLine contract.
export type HistoryWorkLine =
 | {kind:'tool';key:string;toolName:string;descriptor:import('./shared/toolUseDescriptor').ToolUseDescriptor;status:'running'|'done';hasError?:true;ghost?:{id:string;tool:string}}
 | {kind:'thinking';key:string;text:string;truncated?:true};
export interface DeferredHistoryWork {
 lines?:HistoryWorkLine[];awaitingDetails?:boolean;
 owner:unknown;key:string;expanded?:boolean;previewComplete?:boolean;loading?:boolean;failed?:boolean;
 setVisible?:(expanded:boolean,preview:boolean)=>void;toggle:()=>void;retry:()=>void;
}
