import {cronToConfig,configToCron,DEFAULT_CONFIG,type CodexScheduleConfig} from "./cindy/cronCodexPreset";
import type {AISelection} from "./ai-configuration";
import type {ChatPermission} from "./chat-state";
import type {WinOneSchedule,WinOneCreateScheduleInput,WinOneUpdateScheduleInput} from "./chat-automations-types";
import type {ScriptCapability} from "../../../../../third_party/cindy/scheduler/types";

export interface AutomationDraft{
 name:string;prompt:string;timing:"cron"|"interval"|"once"|"manual";cron:CodexScheduleConfig;timezone:string;
 intervalValue:string;intervalUnit:"minutes"|"hours";onceLocal:string;
 sessionMode:"fresh"|"persistent"|"bound";targetSessionId:string;workspaceKind:"project"|"dialogue";workingDir:string;
 model:AISelection;followModel:boolean;permissionMode:ChatPermission;notifyDesktop:boolean;silentWhenIdle:boolean;
 effort:string;fastMode:boolean;executionMode:"agent"|"script";scriptCommand:string;scriptTimeout:string;capabilities:ScriptCapability[];
 hookEnabled:boolean;hookCommand:string;hookTimeout:string;useWorktree:boolean;notifyFeishu:boolean;notifyWecom:boolean;
}
function wallParts(epoch:number,timezone:string){
 const parts=new Intl.DateTimeFormat("en-CA",{timeZone:timezone,year:"numeric",month:"2-digit",day:"2-digit",hour:"2-digit",minute:"2-digit",hourCycle:"h23"}).formatToParts(epoch);
 const value=(type:string)=>parts.find(part=>part.type===type)?.value??"";
 return `${value("year")}-${value("month")}-${value("day")}T${value("hour")}:${value("minute")}`;
}
/** Resolve wall time in the selected zone, including DST gaps and half-hour
 * offsets. Ambiguous fall-back times consistently choose the earlier instant. */
export function zonedDateTimeToEpoch(local:string,timezone:string){
 if(!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(local))throw Error("timeInvalid");
 const nominal=Date.parse(local+":00Z");if(!Number.isFinite(nominal))throw Error("timeInvalid");
 try{
  const offsets=new Set([-86400000,0,86400000].map(delta=>{const instant=nominal+delta;return Date.parse(wallParts(instant,timezone)+":00Z")-instant;}));
  const matches=[...offsets].map(offset=>nominal-offset).filter(epoch=>wallParts(epoch,timezone)===local);
  if(matches.length)return Math.min(...matches);
 }catch{throw Error("timeInvalid");}
 throw Error("timeInvalid");
}
export function createAutomationDraft(schedule:WinOneSchedule|undefined,model:AISelection):AutomationDraft{
 const timezone=schedule?.timezone||Intl.DateTimeFormat().resolvedOptions().timeZone||"UTC";
 const interval=schedule?.intervalMs??300000,wholeHours=interval%3600000===0;
 return {name:schedule?.name??"",prompt:schedule?.prompt??"",timing:schedule?.onceAt!==undefined?"once":schedule?.manual?"manual":schedule?.intervalMs!==undefined?"interval":"cron",cron:schedule?cronToConfig(schedule.cronExpr):{...DEFAULT_CONFIG},timezone,
  intervalValue:String(interval/(wholeHours?3600000:60000)),intervalUnit:wholeHours?"hours":"minutes",onceLocal:wallParts(schedule?.onceAt??Date.now()+3600000,timezone),
  sessionMode:schedule?.persistentSession?"persistent":schedule?.targetSessionId?"bound":"fresh",targetSessionId:schedule?.targetSessionId??"",workspaceKind:schedule?.workspaceKind??"dialogue",workingDir:schedule?.workingDir??"",
  effort:schedule?.effort??"",fastMode:schedule?.fastMode??false,executionMode:schedule?.executionMode??"agent",scriptCommand:schedule?.scriptConfig?.command??"",scriptTimeout:schedule?.scriptConfig?.timeoutMs?String(schedule.scriptConfig.timeoutMs/1000):"",capabilities:schedule?.scriptConfig?.capabilities??[],hookEnabled:!!schedule?.preRunHook,hookCommand:schedule?.preRunHook?.command??"",hookTimeout:schedule?.preRunHook?.timeoutMs?String(schedule.preRunHook.timeoutMs/1000):"",useWorktree:schedule?.useWorktree??false,notifyFeishu:schedule?.notify.feishu??false,notifyWecom:schedule?.notify.wecomGroup??false,
  model:schedule?{provider:schedule.providerId??"",model:schedule.model??""}:model,followModel:!!schedule?.targetSessionId&&!schedule.model&&!schedule.providerId,permissionMode:schedule?.permissionMode??"ask",notifyDesktop:schedule?.notify.desktop??true,silentWhenIdle:schedule?.silentWhenIdle??false};
}
/** Cindy's fresh/persistent/bound semantics are preserved. Only fields exposed
 * by this agent UI are written; unrelated upstream script options stay intact. */
export function buildAutomationInput(draft:AutomationDraft,now=Date.now(),original?:WinOneSchedule):WinOneCreateScheduleInput{
 const script=draft.executionMode==="script";
 if(!draft.name.trim())throw Error("nameRequired");if(!script&&!draft.prompt.trim())throw Error("promptRequired");
 if(script&&!draft.scriptCommand.trim())throw Error("commandRequired");
 if(draft.hookEnabled&&!draft.hookCommand.trim())throw Error("hookRequired");
 const timeout=(value:string)=>{if(!value.trim())return undefined;const ms=Number(value)*1000;if(!Number.isSafeInteger(ms)||ms<=0)throw Error("timeoutInvalid");return ms;};
 try{new Intl.DateTimeFormat("en",{timeZone:draft.timezone}).format(now);}catch{throw Error("timezoneInvalid");}
 const target=draft.sessionMode==="fresh"?undefined:draft.targetSessionId.trim()||undefined;
 if(draft.sessionMode==="bound"&&!target)throw Error("targetRequired");
 const follows=!!target&&draft.followModel;
 if(!follows&&(!script||draft.capabilities.includes("sessions.dispatch"))&&(!draft.model.provider||!draft.model.model))throw Error("modelRequired");
 if((script||!target&&draft.workspaceKind==="project")&&!draft.workingDir.trim())throw Error("directoryRequired");
 const cronExpr=configToCron(draft.cron);if(!cronExpr)throw Error("cronRequired");
 let onceAt:number|undefined,intervalMs:number|undefined;
 if(draft.timing==="once"){
  const retained=original?.onceAt!==undefined&&draft.timezone===original.timezone&&draft.onceLocal===wallParts(original.onceAt,original.timezone);
  onceAt=retained?original!.onceAt:zonedDateTimeToEpoch(draft.onceLocal,draft.timezone);
  // Editing metadata on an expired one-time task must not reschedule it.
  if(!retained&&onceAt!==undefined&&onceAt<=now)throw Error("timePast");
 }
 if(draft.timing==="interval"){intervalMs=Number(draft.intervalValue)*(draft.intervalUnit==="hours"?3600000:60000);if(!Number.isSafeInteger(intervalMs)||intervalMs<=0)throw Error("intervalInvalid");}
 return {name:draft.name.trim(),prompt:draft.prompt,kind:"cron",cronExpr,timezone:draft.timezone,recurring:draft.timing!=="once",manual:draft.timing==="manual",intervalMs,onceAt,
  agentKind:"pi",modelAgentKind:follows||!draft.model.model?undefined:"pi",providerId:follows?undefined:draft.model.provider||undefined,model:follows?undefined:draft.model.model||undefined,permissionMode:draft.permissionMode,
  effort:follows?undefined:draft.effort||undefined,fastMode:follows?undefined:draft.fastMode,executionMode:draft.executionMode,scriptConfig:script?{command:draft.scriptCommand,timeoutMs:timeout(draft.scriptTimeout),capabilities:draft.capabilities}:undefined,preRunHook:draft.hookEnabled?{command:draft.hookCommand,timeoutMs:timeout(draft.hookTimeout)}:undefined,
  workspaceKind:script?"project":draft.workspaceKind,workingDir:!script&&(target||draft.workspaceKind==="dialogue")?undefined:draft.workingDir.trim(),useWorktree:!script&&!target&&draft.workspaceKind==="project"&&draft.useWorktree,targetSessionId:target,persistentSession:!script&&draft.sessionMode==="persistent",silentWhenIdle:!script&&draft.silentWhenIdle,notify:{desktop:draft.notifyDesktop,feishu:draft.notifyFeishu,wecomGroup:draft.notifyWecom}};
}
/** JSON transport needs explicit null to clear optional persisted columns. */
export function buildAutomationPatch(input:WinOneCreateScheduleInput,original?:WinOneSchedule):WinOneUpdateScheduleInput{
 const patch:WinOneUpdateScheduleInput={...input,onceAt:input.onceAt??null,intervalMs:input.intervalMs??null,model:input.model??null,fastMode:input.fastMode??null,effort:input.effort??null,modelAgentKind:input.modelAgentKind??null,providerId:input.providerId??null,targetSessionId:input.targetSessionId??null,workingDir:input.workingDir??null,scriptConfig:input.scriptConfig??null,preRunHook:input.preRunHook??null};
 const sameTiming=original&&input.timezone===original.timezone&&(original.onceAt!==undefined?input.onceAt===original.onceAt:input.onceAt===undefined&&input.manual===original.manual&&input.intervalMs===original.intervalMs&&input.cronExpr===configToCron(cronToConfig(original.cronExpr))&&input.recurring===original.recurring);
 if(sameTiming){delete patch.onceAt;delete patch.intervalMs;delete patch.cronExpr;delete patch.timezone;delete patch.recurring;delete patch.manual;}
 if(original)patch.notify={...original.notify,...input.notify};
 return patch;
}
