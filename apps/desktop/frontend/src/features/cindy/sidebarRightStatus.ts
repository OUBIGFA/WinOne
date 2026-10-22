// Extracted from Cindy sessionActivity.ts (Apache-2.0, Copyright 2026 XD Inc.).
// Only the display projection is needed: WinOne owns lifecycle evidence and does
// not infer activity from a task title or start runtimes while listing sessions.
export type SidebarRightStatusKind='error'|'awaiting'|'running'|'interrupted'|'done'|'time'|'paused';
export function resolveSidebarRightStatus(activity:{phase:string;attention:boolean;paused?:boolean}):SidebarRightStatusKind{
 if(activity.phase==='error'&&activity.attention)return 'error';
 if(activity.paused)return 'paused';
 if(activity.phase==='needs-interaction')return 'awaiting';
 if(activity.phase==='running')return 'running';
 if(activity.phase==='interrupted')return 'interrupted';
 if(activity.phase==='completed'&&activity.attention)return 'done';
 return 'time';
}
