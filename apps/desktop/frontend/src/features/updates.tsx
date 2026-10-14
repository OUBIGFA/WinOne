import {RefreshCw,ExternalLink,X} from "lucide-react";
import {useEffect,useRef,useState} from "react";
import {updateInfo,checkUpdates,cancelUpdateCheck,openUpdateRelease,type UpdateBuildInfo,type UpdateCheckResult} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {Notice} from "../ui/notice";

export function Updates(){
 const {t}=useTranslation();
 const [info,setInfo]=useState<UpdateBuildInfo>(),[result,setResult]=useState<UpdateCheckResult>();
 const [busy,setBusy]=useState(false),[opening,setOpening]=useState(false),[error,setError]=useState("");
 const alive=useRef(true),request=useRef<string|undefined>(undefined);
 async function load(){setError("");try{const value=await updateInfo();if(alive.current)setInfo(value);}catch{if(alive.current)setError("unavailable");}}
 useEffect(()=>{alive.current=true;void load();return()=>{
  alive.current=false;
  // The window lifecycle also cancels requests; cleanup never claims success.
  if(request.current)void cancelUpdateCheck(request.current).catch(()=>undefined);
 };},[]);
 async function check(){
  if(request.current||!info?.configured)return;
  const id=crypto.randomUUID();request.current=id;setBusy(true);setError("");setResult(undefined);
  try{
   const next=await checkUpdates(id);
   if(!alive.current)return;
   if(next.state==="failed")setError(next.errorCode||"unavailable");
   else if(["available","up_to_date","not_configured"].includes(next.state)&&!(next.state==="available"&&!next.latestVersion))setResult(next);
   else setError("invalid_response");
  }catch{if(alive.current)setError("unavailable");}
  finally{if(request.current===id)request.current=undefined;if(alive.current)setBusy(false);}
 }
 async function open(){if(opening)return;setOpening(true);setError("");try{await openUpdateRelease();}catch{if(alive.current)setError("open_failed");}finally{if(alive.current)setOpening(false);}}
 const knownErrors=["network","timeout","rate_limited","not_found","cancelled","unavailable","invalid_response","invalid_version","invalid_source","busy","open_failed"];
 const errorKey=knownErrors.includes(error)?error:"unavailable";
 return <section className="space-y-4 py-5" aria-labelledby="updates-heading">
  <h2 id="updates-heading">{t("updates.title")}</h2>
  <div className="flex flex-wrap items-center justify-between gap-3">
   <p>{info?t("updates.version",{version:info.version}):t("updates.loading")}</p>
   <div className="flex flex-wrap gap-2"><Button icon={RefreshCw} busy={busy} disabled={!info?.configured} onClick={()=>void check()}>{t("updates.check")}</Button>
   {busy&&<Button icon={X} onClick={()=>{const id=request.current;if(id)void cancelUpdateCheck(id).catch(()=>{if(alive.current)setError("unavailable");});}}>{t("updates.cancel")}</Button>}</div>
  </div>
  {(info&&!info.configured||result?.state==="not_configured")&&<p className="text-sm text-muted-foreground">{t("updates.notConfigured")}</p>}
  {result?.state==="available"&&<div className="space-y-3"><p role="status">{t("updates.available",{version:result.latestVersion})}</p><Button icon={ExternalLink} busy={opening} onClick={()=>void open()}>{t("updates.release")}</Button></div>}
  {result?.state==="up_to_date"&&<p role="status">{t("updates.current")}</p>}
  {error==="cancelled"?<p role="status">{t("updates.cancelled")}</p>:error&&<Notice actions={!info?<Button onClick={()=>void load()}>{t("common.retry")}</Button>:undefined}>{t(`updates.${errorKey}`)}</Notice>}
 </section>;
}
