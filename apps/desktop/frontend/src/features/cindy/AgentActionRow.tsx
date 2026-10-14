// AgentActionRow row and inline-detail JSX adapted from Cindy a152497a,
// Copyright 2026 XD Inc., Apache-2.0. File payloads use WinOne's existing dialog
// and Lobe ScrollArea; all Pi input and output is retained in the detail view.
import {Check,ChevronDown,ChevronRight,Copy,FileText,TriangleAlert} from "lucide-react";
import {useState} from "react";
import {Button} from "../../ui/button";
import {Dialog,DialogContent,DialogTitle,DialogDescription} from "../../components/ui/dialog";
import {ScrollArea} from "../lobe/scroll-area/ScrollArea";
import {cn,Spinner,useTranslation} from "./adapter";
import {useExpandedBlockMemory} from "./useExpandedBlockMemory";
import {describeToolUse} from "./shared/toolUseDescriptor";
import {ChatMarkdown} from "../chat-markdown";
import type {CommandIntent} from "./shared/commandIntent";
import type {WorkMessage} from "./workActivityAdapter";
import {ACTIVITY_ROW_CHEVRON_SLOT_CLASS,ACTIVITY_ROW_COLOR_TRANSITION_CLASS,ACTIVITY_ROW_HOVER_SURFACE_CLASS,ACTIVITY_ROW_RADIUS_CLASS} from "./activityRowChrome";

export function AgentActionRow({message,toolResult,status,intentOverride,showRawCommand=false}:{message:WorkMessage;toolResult?:string;status:"running"|"done";intentOverride?:CommandIntent;showRawCommand?:boolean}){
 const {t}=useTranslation(),{expanded,setExpanded}=useExpandedBlockMemory(`tool:${message.clientId}`),[dialog,setDialog]=useState(false),[copyError,setCopyError]=useState("");
 const descriptor=describeToolUse(message.toolName??"tool",message.toolInput),args=(message.toolInput??{}) as Record<string,unknown>;
 const file=descriptor.kind==="file"?descriptor.filePath:undefined;
 const isFile=!!file&&["Read","Write","Edit","MultiEdit","read","write","edit"].includes(message.toolName??"");
 const verb=descriptor.kind==="file"?t(`chat.toolVerb_${descriptor.action}`):message.toolName??t("chat.tool");
 const intent=intentOverride??(descriptor.kind==="command"?descriptor.intent:undefined);
 const displayParam=descriptor.kind==="file"?descriptor.fileName:descriptor.kind==="command"?descriptor.description??intent?.target??descriptor.command:descriptor.kind==="search"?descriptor.pattern:descriptor.kind==="web"?descriptor.target:descriptor.kind==="mcp"?descriptor.detail??descriptor.toolLabel:String(args.path??args.command??args.query??"");
 const command=descriptor.kind==="command"?descriptor.command:typeof args.command==="string"?args.command:undefined;
 const inputText=command&&Object.keys(args).every(key=>["command","cwd","description"].includes(key))?command:JSON.stringify(message.toolInput??{},null,2);
 const copy=async(text:string)=>{try{await navigator.clipboard.writeText(text);setCopyError("");}catch(error){setCopyError(String(error));}};
 const details=<div className="chat-tool-payload">
  <div className="chat-tool-payload-label"><span>{t("chat.toolInput")}</span><Button variant="ghost" size="icon" icon={Copy} aria-label={t("chat.copy")} onClick={()=>void copy(inputText)}/></div>
  <ChatMarkdown text={inputText}/>
  {toolResult!==undefined&&<><div className="my-2 h-px bg-[var(--msg-tool-card-chevron)]/20"/><div className="chat-tool-payload-label"><span>{t("chat.toolOutput")}</span><Button variant="ghost" size="icon" icon={Copy} aria-label={t("chat.copy")} onClick={()=>void copy(toolResult)}/></div><ChatMarkdown text={toolResult}/></>}
  {copyError&&<p role="alert">{copyError}</p>}
 </div>;
 return <div className="flex min-w-0 flex-col chat-work-entry" data-message-client-id={message.clientId}>
  <button type="button" onClick={()=>isFile?setDialog(true):setExpanded(value=>!value)} aria-expanded={!isFile?expanded:undefined} aria-label={`${verb}${displayParam?" "+displayParam:""}`} className={cn("group flex w-full min-w-0 items-center gap-1.5",ACTIVITY_ROW_RADIUS_CLASS,"px-2 py-[3px]",ACTIVITY_ROW_HOVER_SURFACE_CLASS,ACTIVITY_ROW_COLOR_TRANSITION_CLASS,"cursor-pointer select-none outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus-ring)] text-left")}>
   <span role="img" aria-label={t(message.isError?"chat.failed":status==="running"?"chat.working":"chat.toolDone")} className="inline-flex h-[18px] w-4 items-center justify-center shrink-0 text-[var(--msg-tool-card-chevron)]">{status==="running"?<Spinner size={13}/>:message.isError?<TriangleAlert size={13}/>:<Check size={13}/>}</span>
   <span className="text-14 text-[var(--msg-tool-card-chevron)] shrink-0">{verb}</span>
   {isFile&&<FileText size={14} className="shrink-0 text-[var(--msg-tool-card-chevron)]"/>}<span className="min-w-0 truncate text-14 text-[var(--msg-tool-card-chevron)]" title={file??displayParam}>{displayParam}</span>
   <span className="flex-1"/><span aria-hidden="true" className={ACTIVITY_ROW_CHEVRON_SLOT_CLASS}>{!isFile&&expanded?<ChevronDown size={13}/>:<ChevronRight size={13}/>}</span>
  </button>
  {showRawCommand&&command&&command!==displayParam&&!expanded&&<div data-agent-action-raw-command="true" title={command} className="min-w-0 truncate px-2 pb-[3px] pl-[30px] font-mono text-12 leading-[1.5] text-[var(--msg-tool-card-chevron)]">{command}</div>}
  {!isFile&&expanded&&<div className="mx-2 mt-1 mb-1 rounded-[6px] px-[10px] py-2 bg-[var(--msg-user-bg)] border border-[var(--msg-user-border)] font-mono text-[length:calc(var(--app-code-font-size)_-_1px)] leading-[calc(var(--app-code-font-size)_+_4px)] text-[var(--foreground)] select-text cursor-text">{details}</div>}
  <Dialog open={dialog} onOpenChange={setDialog}><DialogContent className="chat-tool-dialog cindy-chat-ui"><DialogTitle>{verb} · {file}</DialogTitle><DialogDescription>{t("chat.toolDetailHint")}</DialogDescription><ScrollArea disableContentFit className="chat-tool-dialog-scroll">{details}</ScrollArea></DialogContent></Dialog>
 </div>;
}
