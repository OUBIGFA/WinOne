import {useEffect,useRef,useState} from "react";
import {captureLaunchPort,captureOpeningPort,getSnapshot,revealCaptureEditor,windowFramePort} from "./bridge";
import {setApplicationLanguage,useTranslation} from "./i18n";
import {startCaptureLaunchReceiver} from "./features/capture-launch";
import {readCaptureDraft,type CaptureDraft} from "./features/capture-draft-api";
import {openCaptureEditor} from "./features/capture-surface-session";
import type {CaptureSurface} from "./features/capture-surface-layout";
import {CaptureDraftWorkspace} from "./features/capture-draft-workspace";
import {Button} from "./components/ui/button";
import {InlineError} from "./ui/notice";

// This entry owns a screenshot, never a settings page. Theme and language are
// read before starting the receiver so the first visible frame is final.
export function CaptureApp(){
 const {t}=useTranslation();
 const [ready,setReady]=useState(false),[error,setError]=useState("");
 const [active,setActive]=useState<{draft:CaptureDraft;surface?:CaptureSurface;warning:string}>();
 const current=useRef<string|undefined>(undefined),opening=useRef(false);
 useEffect(()=>{
  let disposed=false;
  const query=matchMedia("(prefers-color-scheme: dark)");
  let theme="system";
  const apply=()=>{document.documentElement.dataset.theme=theme==="system"?(query.matches?"dark":"light"):theme;};
  void getSnapshot().then(async snapshot=>{
   theme=snapshot.settings.theme;await setApplicationLanguage(snapshot.settings.language||"system",snapshot.locale);
   if(disposed)return;apply();setReady(true);
  }).catch(async failure=>{if(!disposed){setError(String(failure));try{await revealCaptureEditor();}catch(revealError){setError(String(failure)+"\n"+String(revealError));}}});
  query.addEventListener("change",apply);
  return()=>{disposed=true;query.removeEventListener("change",apply);};
 },[]);
 useEffect(()=>{
  if(!ready)return;
  const receiver=startCaptureLaunchReceiver(captureLaunchPort,async request=>{
   if(current.current)return current.current===request.draftId;
   if(opening.current)return false;
   opening.current=true;
   try{
    const draft=await readCaptureDraft(request.draftId);
    const {surface,warning}=await openCaptureEditor(draft,captureOpeningPort,t("capture.failed"));
    current.current=draft.id;setActive({draft,surface,warning});setError("");return true;
   }finally{opening.current=false;}
  },failure=>{setError(String(failure));void revealCaptureEditor().catch(revealError=>setError(String(failure)+"\n"+String(revealError)));});
  return()=>receiver.stop();
 },[ready,t]);
 if(active)return <CaptureDraftWorkspace key={active.draft.id} draft={active.draft} initialSurface={active.surface} surfaceError={active.warning}
  onReplace={(previous,draft)=>{if(current.current!==previous)throw Error(t("capture.draftConflict"));current.current=draft.id;setActive({draft,warning:""});}}
  // Completion closes this dedicated window. Keep the final frame mounted
  // until native destruction instead of briefly rendering another page.
  onDone={()=>{}}/>;
 return error?<main className="mx-auto max-w-lg space-y-4 p-6"><InlineError>{error}</InlineError><Button onClick={()=>void windowFramePort.command("close").catch(failure=>setError(String(failure)))}>{t("window.close")}</Button></main>:null;
}
