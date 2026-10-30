import {useEffect,useRef,useState} from "react";
import {FolderOpen,ArrowRight,Undo2,Play,RefreshCw} from "lucide-react";
import {directoryLinksCall,pickDirectory} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {Notice} from "../ui/notice";
import {Input} from "../components/ui/input";
import {Label} from "../components/ui/label";
import {Checkbox} from "../components/ui/checkbox";
import {Dialog,DialogContent,DialogDescription,DialogFooter,DialogTitle} from "../components/ui/dialog";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";
import type {DirectoryLinkRecord} from "./directory-links";

export type MigrationSelection={record:DirectoryLinkRecord;action:"restore"|"recover"|"cleanup";index?:number;nonce:number};
type MigrationPlan={id:string;operation:"migrate"|"restore";source:string;destination:string;files:number;bytes:number;warnings:string[];expiresAt:string};
type MigrationJob={id:string;requestId:string;state:"running"|"completed"|"failed"|"cancelled";canCancel:boolean;progress:{phase:string;files:number;totalFiles:number;bytes:number;totalBytes:number};plan?:MigrationPlan;record?:DirectoryLinkRecord;error?:string};
export function migrationSize(value:number){if(!Number.isFinite(value)||value<0)return "—";const units=["B","KiB","MiB","GiB","TiB"];let index=0;while(value>=1024&&index<units.length-1){value/=1024;index++;}return `${value.toFixed(index?1:0)} ${units[index]}`;}

// The Host owns admission and the recovery journal. The UI follows one ticket,
// never retries a write on a lost reply, and keeps navigation/close protection
// while a known job is still running (including a temporarily failed poll).
export function DirectoryLinksMigration({disabled=false,selection,onChanged,onActivity}:{disabled?:boolean;selection?:MigrationSelection;onChanged:(rows:DirectoryLinkRecord[])=>void;onActivity?:PageActivityChanged}){
 const {t}=useTranslation();
 const [source,setSource]=useState(""),[directory,setDirectory]=useState(""),[name,setName]=useState("");
 const [chosen,setChosen]=useState<MigrationSelection>(),[plan,setPlan]=useState<MigrationPlan>(),[job,setJob]=useState<MigrationJob>();
 const [pending,setPending]=useState(false),[confirm,setConfirm]=useState(false),[ack,setAck]=useState(false),[error,setError]=useState("");
 const alive=useRef(true),current=useRef<MigrationJob|undefined>(undefined),requestID=useRef("");
 const busy=pending||job?.state==="running",locked=disabled||busy;
 const mode=chosen?.action??"migrate";
 const reset=()=>{setSource("");setDirectory("");setName("");setChosen(undefined);setPlan(undefined);setJob(undefined);current.current=undefined;setAck(false);setConfirm(false);setError("");};
 usePageActivity(onActivity,!!plan||!!source||!!directory||!!name||!!chosen,busy,"directory-links-migration",reset);
 useEffect(()=>{if(!selection)return;setChosen(selection);setPlan(undefined);setJob(undefined);current.current=undefined;setAck(false);setError("");setSource("");setDirectory("");setName("");setConfirm(selection.action!=="restore");},[selection]);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;const value=current.current;if(value?.state==="running"&&value.canCancel)void directoryLinksCall("migration.cancel",{id:value.id}).catch(error=>console.warn("Directory migration cancellation was not confirmed",error));};},[]);
 function accept(value:MigrationJob){if(!alive.current)return;if(value.requestId!==requestID.current)throw Error(t("links.migrationUncertain"));current.current=value;setJob(value);if(value.state!=="running"){
  if(value.plan&&value.state==="completed")setPlan(value.plan);
  if(value.record){onChanged([value.record]);const record=value.record;setChosen(prior=>prior?.record.id===record.id?{...prior,record}:prior);}
  if(value.error)setError(value.error);
  if(value.record&&value.state==="completed"){setSource("");setDirectory("");setName("");setChosen(undefined);setPlan(undefined);}
 }}
 async function follow(value:MigrationJob){accept(value);while(value.state==="running"&&alive.current){await new Promise(resolve=>setTimeout(resolve,250));if(!alive.current)return;value=await directoryLinksCall<MigrationJob>("migration.status",{id:value.id});accept(value);}}
 async function start(method:string,args:unknown){
  if(busy)return;setPending(true);setError("");setConfirm(false);setJob(undefined);current.current=undefined;requestID.current=crypto.randomUUID();
  // A consumed execution token is never offered as a retry button.
  if(method!=="migration.preview")setPlan(undefined);
  try{await follow(await directoryLinksCall<MigrationJob>(method,{...(args as Record<string,unknown>),requestId:requestID.current}));}catch(e){if(alive.current)setError(`${String(e)} ${t("links.migrationUncertain")}`);}finally{if(alive.current)setPending(false);}
 }
 async function readAgain(){if(pending)return;setPending(true);setError("");try{await follow(await directoryLinksCall<MigrationJob>("migration.status",current.current?.id?{id:current.current.id}:{requestId:requestID.current}));}catch(e){if(alive.current)setError(String(e));}finally{if(alive.current)setPending(false);}}
 async function cancel(){const value=current.current;if(!value?.canCancel)return;const resume=!pending;if(resume)setPending(true);try{const next=await directoryLinksCall<MigrationJob>("migration.cancel",{id:value.id});if(resume)await follow(next);else accept(next);}catch(e){setError(String(e));}finally{if(resume)setPending(false);}}
 async function pick(which:"source"|"directory"){setPending(true);setError("");try{const path=await pickDirectory();if(path){if(which==="source")setSource(path);else setDirectory(path);setPlan(undefined);}}catch(e){setError(String(e));}finally{setPending(false);}}
 function preview(){setAck(false);setPlan(undefined);void start("migration.preview",chosen?{recordId:chosen.record.id,checkedAt:chosen.record.checkedAt}:{source,directory,name});}
 function execute(){
  if(mode==="recover"||mode==="cleanup"){if(chosen)void start(`migration.${mode}`,{id:chosen.record.id,index:chosen.index??0,path:chosen.record.migration?.retained?.[chosen.index??0]?.path??"",confirmed:true});return;}
  if(plan)void start("migration.execute",{token:plan.id,confirmed:true,acknowledged:ack});
 }
 const detailPath=chosen?.action==="cleanup"?chosen.record.migration?.retained?.[chosen.index??0]?.path:chosen?.record.linkPath;
 const progress=job?.progress;
 return <section className="settings-stack" aria-label={t("links.migrationTitle")}>
  <p className="settings-help">{t("links.migrationIntro")}</p>
  <p className="settings-help">{t("links.migrationLimits")}</p>
  {mode==="migrate"?<>
   <div className="directory-links-field"><Label htmlFor="migration-source">{t("links.migrationSource")}</Label><div className="directory-links-input-action"><Input id="migration-source" disabled={locked} value={source} onChange={e=>{setSource(e.target.value);setPlan(undefined);}}/><Button icon={FolderOpen} disabled={locked} onClick={()=>void pick("source")}>{t("links.folder")}</Button></div></div>
   <div className="directory-links-field"><Label htmlFor="migration-directory">{t("links.migrationDirectory")}</Label><div className="directory-links-input-action"><Input id="migration-directory" disabled={locked} value={directory} onChange={e=>{setDirectory(e.target.value);setPlan(undefined);}}/><Button icon={FolderOpen} disabled={locked} onClick={()=>void pick("directory")}>{t("links.folder")}</Button></div></div>
   <div className="directory-links-field"><Label htmlFor="migration-name">{t("links.migrationName")}</Label><Input id="migration-name" disabled={locked} value={name} placeholder={t("links.migrationNameHint")} onChange={e=>{setName(e.target.value);setPlan(undefined);}}/></div>
  </>:<div className="settings-stack-compact"><h3 className="settings-group-title">{t(`links.migrationAction.${mode}`)}</h3><p className="break-all">{detailPath}</p><Button icon={Undo2} disabled={locked} onClick={reset}>{t("links.migrationNew")}</Button></div>}
  {(mode==="migrate"||mode==="restore")&&<div className="directory-links-actions"><Button icon={RefreshCw} disabled={locked||mode==="migrate"&&(!source.trim()||!directory.trim())} onClick={preview}>{t("links.migrationPreview")}</Button><Button icon={Play} disabled={locked||!plan} variant="primary" onClick={()=>setConfirm(true)}>{t(mode==="restore"?"links.migrationRestore":"links.migrationStart")}</Button></div>}
  {(mode==="recover"||mode==="cleanup")&&<Button disabled={locked} onClick={()=>setConfirm(true)}>{t(`links.migrationAction.${mode}`)}</Button>}
  {plan&&<section className="settings-stack-compact"><h3 className="settings-group-title">{t("links.migrationPlan")}</h3><div className="directory-links-record-paths"><p className="break-all">{plan.source}</p><ArrowRight size={18} aria-hidden="true"/><p className="break-all">{plan.destination}</p></div><p>{t("links.migrationSize",{count:plan.files,size:migrationSize(plan.bytes)})}</p>{plan.warnings.map((warning,index)=><Notice key={index}>{warning}</Notice>)}</section>}
  {job&&<section className="settings-stack-compact" aria-live="polite"><p role="status">{job.plan?t("links.migrationPlanReady"):t(`links.migrationState.${job.state}`)}{job.state==="running"&&` · ${t(`links.migrationPhase.${progress?.phase??"checking"}`)}`}</p>{job.state==="running"&&<><progress className="w-full" aria-label={t("links.migrationProgress")} max={Math.max(1,progress?.totalBytes??0)} value={progress?.totalBytes?Math.min(progress.bytes,progress.totalBytes):undefined}/><p className="settings-help">{migrationSize(progress?.bytes??0)} / {migrationSize(progress?.totalBytes??0)} · {progress?.files??0} / {progress?.totalFiles??0}</p><Button disabled={!job.canCancel} onClick={()=>void cancel()}>{t("links.migrationCancel")}</Button></>}{job.record?.migration?.message&&<Notice tone="info">{job.record.migration.message}</Notice>}</section>}
  {error&&<Notice actions={<Button disabled={pending||!requestID.current} onClick={()=>void readAgain()}>{t("links.migrationReadAgain")}</Button>}>{error}</Notice>}
  <Dialog open={confirm} onOpenChange={open=>{if(!busy)setConfirm(open);}}><DialogContent>
   <DialogTitle>{t(mode==="migrate"?"links.migrationConfirm":`links.migrationAction.${mode}`)}</DialogTitle>
   <DialogDescription>{t(mode==="cleanup"?"links.migrationCleanupHint":mode==="recover"?"links.migrationRecoverHint":"links.migrationConfirmHint")}</DialogDescription>
   <div className="break-all"><p>{plan?.source||detailPath}</p>{plan?.destination&&<p>{plan.destination}</p>}</div>
   {!!plan?.warnings.length&&<><Notice>{plan.warnings.join("\n")}</Notice><Label className="directory-links-check"><Checkbox checked={ack} onCheckedChange={setAck}/>{t("links.migrationAcknowledge")}</Label></>}
   <DialogFooter><Button disabled={busy} onClick={()=>setConfirm(false)}>{t("links.cancel")}</Button><Button variant={mode==="cleanup"?"danger":"primary"} disabled={busy||!!plan?.warnings.length&&!ack} onClick={execute}>{t("links.migrationConfirmAction")}</Button></DialogFooter>
  </DialogContent></Dialog>
 </section>;
}
