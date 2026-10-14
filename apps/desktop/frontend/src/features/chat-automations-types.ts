import type {Schedule,CreateScheduleInput,UpdateScheduleInput} from "../../../../../third_party/cindy/scheduler/types";
import type {ChatPermission} from "./chat-state";
export type {ScheduleRun,SchedulerEvent,SchedulerRuntimeSnapshot} from "../../../../../third_party/cindy/scheduler/types";

/** WinOne Host extensions; the imported Cindy engine contracts stay unchanged. */
export interface WinOneSchedule extends Schedule {permissionMode?:ChatPermission;onceAt?:number;unreadRunCount?:number;unreadFailureCount?:number}
export interface WinOneCreateScheduleInput extends CreateScheduleInput {permissionMode?:ChatPermission;onceAt?:number}
type Clearable="fastMode"|"effort"|"modelAgentKind"|"intervalMs"|"model"|"providerId"|"targetSessionId"|"workingDir"|"scriptConfig"|"preRunHook";
export type WinOneUpdateScheduleInput=Omit<UpdateScheduleInput,Clearable>&{[K in Clearable]?:UpdateScheduleInput[K]|null}&{permissionMode?:ChatPermission;onceAt?:number|null};
