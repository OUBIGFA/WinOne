// Host-only seams for the reused Cindy UI. Pi remains WinOne's only harness.
import {useCallback,type CSSProperties,type ReactElement,type ReactNode} from "react";
import {useTranslation as useWinOneTranslation} from "../../i18n";
import {Tooltip,TooltipContent,TooltipTrigger} from "../../components/ui/tooltip";
import {LoaderCircle} from "lucide-react";
export {cn} from "cn";
export type PermissionMode="ask"|"auto"|"bypassPermissions";
export type AgentKind="pi"|"codex"|"claude-code";
export interface PermissionModeDescriptor{id:PermissionMode;displayName:string;description:string}
export const WINDOW_NO_DRAG_STYLE={"--wails-draggable":"no-drag"} as CSSProperties;
const stableTranslationOptions={};
export function useTranslation(){
 const {t}=useWinOneTranslation(undefined,stableTranslationOptions);
 const translate=useCallback((key:string,options?:Record<string,unknown>)=>{
  if(key==="newChat.permissionSelector.triggerAria")return t("chat.permissions")+" · "+String(options?.label??"");
  if(key==="newChat.permissionSelector.listAria")return t("chat.permissions");
  if(key==="newChat.sendButton.send")return t("chat.send");
  if(key==="newChat.sendButton.stop")return t("chat.stop");
  if(key==="chat.thinking.completed")return t("chat.thoughtFor",options);
  if(key==="chat.thinking.aborted")return t("chat.thoughtStoppedFor",options);
  const workKeys:Record<string,string>={"chat.workGroup.workDetails":"chat.processDetails","chat.workGroup.working":"chat.processWorking","chat.workGroup.worked":"chat.processWorked","chat.workGroup.exploration.read":"chat.processRead","chat.workGroup.exploration.search":"chat.processSearch","chat.workGroup.exploration.list":"chat.processList","chat.errorBanner.retry":"chat.retry"};
  if(workKeys[key])return t(workKeys[key],options);
  return String(options?.defaultValue??t(key,options));
 },[t]);
 return {t:translate};
}
export function Spinner({size=14,className=""}:{size?:number;className?:string}){return <LoaderCircle aria-hidden="true" size={size} className={`animate-spin motion-reduce:animate-none ${className}`}/>;}
export function createLogger(name:string){return {warn:(...values:unknown[])=>console.warn(name,...values)};}
export function formatSessionDuration(ms:number,_t?:unknown){
 const seconds=Math.max(0,Math.floor(ms/1000));
 if(seconds<60)return `${seconds}s`;
 const minutes=Math.floor(seconds/60);if(minutes<60)return `${minutes}m ${seconds%60}s`;
 const hours=Math.floor(minutes/60);if(hours<24)return `${hours}h ${minutes%60}m`;
 return `${Math.floor(hours/24)}d ${hours%24}h ${minutes%60}m`;
}
export function useAgentCapabilities(_agent:AgentKind,_device?:string){
 const {t}=useWinOneTranslation();
 return {capabilities:{permissionModes:(["ask","auto","bypassPermissions"] as const).map(id=>{const key=id==="bypassPermissions"?"full":id;return {id,displayName:t("chat."+key),description:t("chat."+key+"Hint")};})}};
}
export function Tip({text,children,side="top",contentClassName}:{text:ReactNode;children:ReactNode;side?:"top"|"bottom"|"left"|"right";contentClassName?:string}){
 return <Tooltip><TooltipTrigger render={children as ReactElement}/><TooltipContent side={side} className={contentClassName}>{text}</TooltipContent></Tooltip>;
}
