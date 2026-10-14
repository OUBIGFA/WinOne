// SessionShareExportDialog flow adapted from Cindy a152497a (Apache-2.0):
// complete archive, optional password, explicit media-exclusion retry and an
// honest fidelity result. Host adapters own the native save dialog and bytes.
import {useEffect,useId,useState} from "react";
import {Check,Eye,EyeOff,FolderOpen,Download} from "lucide-react";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {Input} from "../components/ui/input";
import {Checkbox} from "../components/ui/checkbox";
import {Dialog,DialogContent,DialogTitle,DialogDescription,DialogFooter} from "../components/ui/dialog";
import {ScrollArea} from "./lobe/scroll-area/ScrollArea";
import type {ChatProject,Conversation} from "./chat-state";
import type {ChatExportOutcome} from "../bridge";

export type ChatArchiveOutcome=ChatExportOutcome;
/** Match the archive's actual fidelity; deliberately excluded media is not
 * reported as a failed read or disguised as a complete archive. */
export function chatArchiveNotice(value:Extract<ChatArchiveOutcome,{status:"ok"}>,t:(key:string,options?:Record<string,unknown>)=>string){
 const key=value.fidelity==="full"?"chat.exportDoneFull":value.fidelity==="db-only"?"chat.exportDoneDBOnly":value.mediaExcluded>0?"chat.exportDoneExcluded":"chat.exportDonePartial";
 return t(key,{path:value.filePath,count:value.mediaExcluded,missing:value.mediaMissing+(value.missingTranscripts?.length??0)});
}
export function ChatExportDialog({conversation,onClose,onBusy,onExport,onExported}:{conversation:Conversation;onClose:()=>void;onBusy:(busy:boolean)=>void;onExport:(options:{password?:string;excludeMedia?:boolean})=>Promise<ChatArchiveOutcome>;onExported:(value:Extract<ChatArchiveOutcome,{status:"ok"}>)=>void}){
 const {t}=useTranslation(),id=useId();
 const [encrypt,setEncrypt]=useState(false),[password,setPassword]=useState(""),[confirmation,setConfirmation]=useState(""),[show,setShow]=useState(false),[working,setWorking]=useState(false),[error,setError]=useState("");
 const [oversize,setOversize]=useState<Extract<ChatArchiveOutcome,{status:"oversize"}>>(),[excludeMedia,setExcludeMedia]=useState(false);
 useEffect(()=>{onBusy(working);return()=>onBusy(false);},[working,onBusy]);
 const mismatch=encrypt&&!!password&&!!confirmation&&password!==confirmation,short=encrypt&&!!password&&password.length<4;
 const valid=!working&&(!encrypt||password.length>=4&&password===confirmation);
 async function save(){
  if(!valid)return;setWorking(true);setError("");
  try{
   const result=await onExport({...(encrypt?{password}:{}),...(excludeMedia?{excludeMedia:true}:{})});
   if(result.status==="canceled")return;
   if(result.status==="oversize"){
    if(result.mediaBytes>0&&!excludeMedia){setOversize(result);setExcludeMedia(true);}
    else setError(t("chat.exportTooLarge",{limit:Math.round(result.limitBytes/1048576)}));
    return;
   }
   if(result.status!=="ok"||!result.filePath)throw Error(t("chat.exportInvalidResponse"));
   onExported(result);onClose();
  }catch(error){setError(error instanceof Error?error.message:String(error));}finally{setWorking(false);}
 }
 return <Dialog open onOpenChange={open=>{if(!open&&!working)onClose();}}><DialogContent className="chat-export-dialog cindy-chat-ui" showCloseButton={!working} onKeyDown={event=>event.stopPropagation()}>
  <DialogTitle>{t("chat.exportTask")} · {conversation.title}</DialogTitle>
  <DialogDescription>{t("chat.exportDescription")}</DialogDescription>
  {oversize&&<p role="status" className="text-sm text-muted-foreground">{t("chat.exportExcludeHint",{limit:Math.round(oversize.limitBytes/1048576),media:Math.round(oversize.mediaBytes/1048576)})}</p>}
  <label className="flex items-center gap-2"><Checkbox checked={encrypt} disabled={working} onCheckedChange={value=>setEncrypt(value===true)}/><span>{t("chat.exportEncrypt")}</span></label>
  {encrypt&&<div className="space-y-3">
   <div className="space-y-1"><label htmlFor={id+"-password"}>{t("chat.exportPassword")}</label><div className="flex items-center gap-1"><Input id={id+"-password"} type={show?"text":"password"} autoComplete="new-password" value={password} disabled={working} onChange={event=>setPassword(event.target.value)}/><Button variant="ghost" size="icon" icon={show?EyeOff:Eye} aria-label={t(show?"chat.exportHidePassword":"chat.exportShowPassword")} onClick={()=>setShow(!show)}/></div></div>
   <div className="space-y-1"><label htmlFor={id+"-confirmation"}>{t("chat.exportPasswordAgain")}</label><Input id={id+"-confirmation"} type={show?"text":"password"} autoComplete="new-password" value={confirmation} disabled={working} onChange={event=>setConfirmation(event.target.value)}/></div>
   {(short||mismatch)&&<p role="alert" className="text-sm text-destructive">{t(short?"chat.exportShortPassword":"chat.exportPasswordMismatch")}</p>}
  </div>}
  {error&&<p role="alert" className="text-sm text-destructive [overflow-wrap:anywhere]">{error}</p>}
  <DialogFooter><Button disabled={working} onClick={onClose}>{t("chat.cancel")}</Button><Button variant="primary" icon={Download} busy={working} disabled={!valid} onClick={()=>void save()}>{t(excludeMedia?"chat.exportWithoutMedia":"chat.exportConfirm")}</Button></DialogFooter>
 </DialogContent></Dialog>;
}

export function ChatMoveDialog({conversation,projects,busy,error,onClose,onMove}:{conversation:Conversation;projects:ChatProject[];busy:boolean;error:string;onClose:()=>void;onMove:(projectId:string)=>void}){
 const {t}=useTranslation(),[projectId,setProjectId]=useState(conversation.projectId??"");
 const options=[{id:"",name:t("chat.noProject"),path:conversation.directory},...projects];
 return <Dialog open onOpenChange={open=>{if(!open&&!busy)onClose();}}><DialogContent className="chat-move-dialog"><DialogTitle>{t("chat.moveToProject")} · {conversation.title}</DialogTitle><DialogDescription>{t("chat.moveDescription")}</DialogDescription>
  <ScrollArea disableContentFit viewportProps={{style:{maxHeight:320}}}><div className="space-y-1">{options.map(project=><Button key={project.id} variant="ghost" className="w-full justify-start text-left" aria-pressed={projectId===project.id} disabled={busy} onClick={()=>setProjectId(project.id)}><FolderOpen size={16}/><span className="min-w-0 flex-1"><span className="block">{project.name}</span><span className="block text-xs text-muted-foreground [overflow-wrap:anywhere]">{project.path}</span></span>{projectId===project.id&&<Check size={15}/>}</Button>)}</div></ScrollArea>
  {error&&<p role="alert" className="text-sm text-destructive [overflow-wrap:anywhere]">{error}</p>}
  <DialogFooter><Button disabled={busy} onClick={onClose}>{t("chat.cancel")}</Button><Button variant="primary" busy={busy} disabled={projectId===(conversation.projectId??"")} onClick={()=>onMove(projectId)}>{t("chat.moveConfirm")}</Button></DialogFooter>
 </DialogContent></Dialog>;
}
