import {useEffect,useRef,useState} from "react";
import {Download,RefreshCw,Trash2,X} from "lucide-react";
import {captureCall} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {Notice} from "../ui/notice";
import {SettingsToggle} from "../components/settings-layout";
import {Dialog,DialogContent,DialogHeader,DialogTitle,DialogDescription,DialogFooter} from "../components/ui/dialog";

export type PrivacyModelStatus={on:boolean;supported:boolean;installed:boolean;size:number;downloading?:boolean;done?:number;total?:number;error?:string;running?:boolean;device?:string;gpu?:boolean;loadMs?:number;gpuError?:string;failed?:string};
type ModelAction="on"|"off"|"cancel"|"remove";

// Reuse Magpie's independent local model lifecycle, composed with WinOne's
// settings/confirmation primitives. None of these operations saves a provider
// draft or downloads anything merely by opening this page.
export function AIPrivacyModel({active,disabled}:{active:boolean;disabled:boolean}){
 const {t}=useTranslation();
 const [state,setState]=useState<PrivacyModelStatus>(),[error,setError]=useState(""),[busy,setBusy]=useState(false),[confirm,setConfirm]=useState(false),[retry,setRetry]=useState(0);
 const act=useRef<((action:ModelAction)=>void)|undefined>(undefined);
 useEffect(()=>{

  if(!active){setState(undefined);setError("");setBusy(false);return;}
  const viewer=crypto.randomUUID(),pending=new Set<Promise<unknown>>();
  let live=true,timer:ReturnType<typeof setTimeout>|undefined,sequence=0,starts=0;
  setState(undefined);setError("");setBusy(true);
  const request=(method:string,action?:ModelAction)=>{
   const task=captureCall<PrivacyModelStatus>("ai.magpie.privacy."+method,{viewer,...(action?{action}:{})});pending.add(task);
   void task.then(()=>pending.delete(task),()=>pending.delete(task));return task;
  };
  async function load(method="state",action?:ModelAction){
   const ticket=++sequence;clearTimeout(timer);
   if(action){setBusy(true);starts=0;}
   try{
    const next=await request(method,action);
    if(!live||ticket!==sequence)return;
    if(typeof next?.on!=="boolean"||typeof next.supported!=="boolean"||!Number.isFinite(next.size))throw Error(t("ai.privacyInvalidStatus"));
    setState(next);setError("");
    if(next.downloading||(next.on&&next.installed&&!next.running&&!next.failed&&starts++<30))timer=setTimeout(()=>void load(),1000);
   }catch(e){if(live&&ticket===sequence)setError(String(e));}
   finally{if(live&&ticket===sequence)setBusy(false);}
  }
  act.current=action=>void load("action",action);
  void load("open");
  return()=>{
   live=false;act.current=undefined;clearTimeout(timer);
   // A late status reply must not acquire a lease after the page closed.
   void Promise.allSettled([...pending]).then(()=>captureCall("ai.magpie.privacy.close",{viewer})).catch(error=>console.warn("Privacy page lease release failed",error));
  };
 },[active,retry,t]);
 const size=((state?.size??1637572192)/1e9).toFixed(1)+" GB";
 let description=t("ai.privacyModelHint",{size});
 if(state){
  if(!state.supported&&!state.installed)description=t("ai.privacyUnsupported");
  else if(state.downloading)description=t("ai.privacyDownloading",{done:((state.done??0)/1e6).toFixed(0),total:((state.total||state.size)/1e6).toFixed(0)});
  else if(state.on&&state.running)description=t(state.gpu?"ai.privacyGPU":"ai.privacyCPU",{device:state.device||"GPU"});
  else if(state.on)description=t("ai.privacyReady");
  else if(state.installed)description=t("ai.privacyInstalled",{size});
 }
 return <div className="settings-stack-compact" aria-label={t("ai.privacyModel")}>
  <SettingsToggle label={t("ai.privacyModel")} description={description} checked={!!(state?.on||state?.downloading)} disabled={disabled||busy||!state||!state.supported&&!state.installed} onChange={on=>act.current?.(on?"on":"off")}/>
  <p className="settings-help">{t("ai.privacyModelScope")}</p>
  {busy&&!state&&<p role="status">{t("capture.loading")}</p>}
  {state?.downloading&&<progress className="w-full" aria-label={t("ai.privacyDownloadProgress")} max={state.total||state.size} value={Math.min(state.done??0,state.total||state.size)}/>}
  {!!state?.loadMs&&state.running&&<p className="settings-help">{t("ai.privacyLoadTime",{ms:state.loadMs})}</p>}
  {state?.gpuError&&<Notice tone="info">{t("ai.privacyGPUFallback",{error:state.gpuError})}</Notice>}
  {(error||state?.error||state?.on&&state?.failed)&&<Notice actions={<Button icon={RefreshCw} disabled={disabled||busy} onClick={()=>state?.error?act.current?.("on"):setRetry(value=>value+1)}>{t("common.retry")}</Button>}>{error||t(state?.failed?"ai.privacyFailed":"ai.privacyDownloadFailed",{error:state?.failed||state?.error})}</Notice>}
  <div className="flex flex-wrap gap-2">
   {state?.downloading&&<Button icon={X} disabled={disabled||busy} onClick={()=>act.current?.("cancel")}>{t("ai.privacyCancel")}</Button>}
   {state?.installed&&!state.on&&!state.downloading&&<Button icon={Trash2} disabled={disabled||busy} onClick={()=>setConfirm(true)}>{t("ai.privacyDelete")}</Button>}
   {state&&!state.installed&&!state.downloading&&state.supported&&<Button icon={Download} disabled={disabled||busy} onClick={()=>act.current?.("on")}>{t("ai.privacyDownloadEnable")}</Button>}
  </div>
  <Dialog open={confirm} onOpenChange={setConfirm}><DialogContent><DialogHeader><DialogTitle>{t("ai.privacyDeleteQuestion")}</DialogTitle><DialogDescription>{t("ai.privacyDeleteHint",{size})}</DialogDescription></DialogHeader><DialogFooter><Button onClick={()=>setConfirm(false)}>{t("common.cancel")}</Button><Button icon={Trash2} disabled={disabled||busy} onClick={()=>{setConfirm(false);act.current?.("remove");}}>{t("ai.privacyDelete")}</Button></DialogFooter></DialogContent></Dialog>
 </div>;
}
