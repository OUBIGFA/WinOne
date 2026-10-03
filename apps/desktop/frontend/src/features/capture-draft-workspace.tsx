import type {CaptureSelectionPort,SelectionCandidates} from "./capture-selection";
import type {CropRect} from "./capture-editor-model";
import type {CaptureSurface} from "./capture-surface-layout";
import {captureToolbarPreferences} from "./capture-toolbar-preferences";
import {lazy,Suspense,useEffect,useRef,useState} from "react";
import {captureCall,cancelCaptureClose,captureClosePending,setCaptureEditing,subscribeCaptureClose,captureSurfacePort,revealCaptureEditor} from "../bridge";
import {useTranslation} from "../i18n";
import {DraftSession} from "./capture-draft-session";
import {readCaptureDraft,runCaptureJob,retakeScrollingDraft,type CaptureDraft} from "./capture-draft-api";
import type {AnnotationDocument,EditPreview} from "./capture-editor-model";

const CaptureEditor=lazy(()=>import("./capture-editor"));

export function CaptureDraftWorkspace({draft,onDone,onReplace,initialSurface,surfaceError=""}:{draft:CaptureDraft;onDone:()=>void;onReplace?:(previousId:string,next:CaptureDraft)=>void|Promise<void>;initialSurface?:CaptureSurface;surfaceError?:string}) {
 const {t}=useTranslation();
 const alive=useRef(true),completed=useRef(false);
 const [error,setError]=useState("");
 const [viewError,setViewError]=useState(surfaceError);
 const [surface,setSurface]=useState(initialSurface);
 const [finalized,setFinalized]=useState(false);
 const [replacement,setReplacement]=useState<CaptureDraft>();
 const [retakeJob,setRetakeJob]=useState<string>();
 const [retakeStopping,setRetakeStopping]=useState(false);
 const dismissed=useRef(false),recoveringSurface=useRef(false);
 const surfaceActive=useRef(!!initialSurface);
 const [closeSignal,setCloseSignal]=useState(0);
 const nativeJobs=useRef(Promise.resolve());
 function runJob<T>(method:string,params:unknown){
  // Autosave and rendering share the worker's single task slot. Preserve each
  // caller's error while keeping later recovery requests usable.
  const result=nativeJobs.current.then(()=>runCaptureJob<T>(method,params));
  nativeJobs.current=result.then(()=>{},()=>{});
  return result;
 }
 function makeSession(value:CaptureDraft){return new DraftSession(value.id,value.revision,value.document,{
  update:async(revision,document)=>{
   const job=await runJob<{draftId:string;revision:number}>("draft.update",{expectedId:draft.id,revision,document});
   if(!job.result)throw Error(t("capture.unknownResult"));return job.result;
  },read:async()=>readCaptureDraft(draft.id),
 });}
 const [session,setSession]=useState(()=>makeSession(draft));
 const [selecting,setSelecting]=useState(!!draft.selectionPending);
 const pendingSelection=useRef(!!draft.selectionPending);
 useEffect(()=>{
  alive.current=true;
  const unsubscribe=subscribeCaptureClose(()=>setCloseSignal(value=>value+1));
  void captureClosePending().then(pending=>{if(pending&&alive.current)setCloseSignal(value=>value+1);}).catch(error=>{if(alive.current)setError(String(error));});
  return ()=>{alive.current=false;unsubscribe();};
 },[]);
 function changed(document:AnnotationDocument) {
  if(completed.current||dismissed.current||pendingSelection.current)return;
  void session.save(document).then(()=>{if(alive.current)setError("");}).catch(error=>{if(alive.current)setError(String(error));});
 }
 async function flush(document:AnnotationDocument) {
  if(session.failed)await session.recover();
  await session.save(document);await session.settle();setError("");
 }
 async function retake(document:AnnotationDocument){
  if(!onReplace)return;
  let restored=false;
  try{
   let next=replacement;
   if(!next){await flush(document);next=await retakeScrollingDraft(draft.id,session.revision,setRetakeJob);}
   if(!next)return;
   completed.current=true;setReplacement(next);setFinalized(true);
   await revealCaptureEditor();restored=true;
   await onReplace(draft.id,next);
  }finally{
   if(alive.current){setRetakeJob(undefined);setRetakeStopping(false);}
   if(!restored)await revealCaptureEditor();
  }
 }
 async function close() {
  if(!completed.current&&!dismissed.current) {
   if(session.failed)await session.recover();
   await session.settle();
   const job=await runJob("draft.cancel",{expectedId:draft.id,revision:session.revision});
   if(job.state!=="completed")throw Error(t("capture.cancelled"));
   dismissed.current=true;setFinalized(true);
  }
  if(initialSurface||draft.placement&&draft.desktopPreviewId)await restoreWindow();
  await setCaptureEditing(false);onDone();
 }
 async function complete(document:AnnotationDocument,saveCopy:boolean,saveAs=false) {
   await flush(document);
   const job=await runJob<{copied?:boolean}>("draft.complete",{expectedId:draft.id,revision:session.revision,...(saveCopy?{saveCopy:true}:{}),...(saveAs?{saveAs:true}:{})});
   if(job.state==="cancelled")return false;
   if(saveCopy&&!job.result?.copied)throw Error(t("capture.unknownResult"));
   completed.current=true;setFinalized(true);return true;
 }
 async function confirm(document:AnnotationDocument) {
  await flush(document);
  const job=await runJob<{draftId:string;revision:number;copied:boolean}>("draft.confirm",{expectedId:draft.id,revision:session.revision});
  if(!job.result?.copied||job.result.draftId!==draft.id||job.result.revision!==session.revision)throw Error(t("capture.unknownResult"));
  completed.current=true;setFinalized(true);return true;
 }
 async function select(crop:CropRect){
  await session.settle();
  let next:CaptureDraft;
  try {
   await runJob("selection.commit",{expectedId:draft.id,revision:session.revision,crop});
   next=await readCaptureDraft(draft.id);
  }catch(error){
   const recovered=await readCaptureDraft(draft.id);
   if(recovered.selectionPending)throw error;
   next=recovered;
  }
  const actual=next.document.crop;
  if(next.id!==draft.id||next.selectionPending||next.revision!==session.revision+1||!actual||actual.x!==crop.x||actual.y!==crop.y||actual.width!==crop.width||actual.height!==crop.height)throw Error(t("capture.draftConflict"));
  pendingSelection.current=false;setSession(makeSession(next));setSelecting(false);
  if(surfaceActive.current)try{const monitor=await captureSurfacePort.monitor(draft.id);setSurface(current=>current?{...current,monitor}:current);}catch(error){setError(String(error));}
 }
 const selection:CaptureSelectionPort|undefined=selecting?{
  mode:draft.sourceKind==="region"?"region":"window",commit:select,saveImmediately:draft.settings?.openEditor===false,
  candidates:async point=>{
   const job=await runJob<SelectionCandidates>("selection.candidates",{expectedId:draft.id,...point});
   if(!job.result||!Array.isArray(job.result.windows))throw Error(t("capture.draftUnavailable"));
   return job.result;
  },
 }:undefined;
 async function surfaceFailed(error:unknown){
  if(draft.sourceKind==="window"||draft.sourceKind==="region"||draft.sourceKind==="fullscreen"){setViewError(String(error));return;}
  if(completed.current||dismissed.current||recoveringSurface.current||!surfaceActive.current)return;
  recoveringSurface.current=true;
  try {
   await restoreWindow();
   if(completed.current||dismissed.current)return;
   surfaceActive.current=false;setSurface(undefined);setViewError(t("capture.surfaceFallback")+" "+String(error));
   await revealCaptureEditor();
  } finally {recoveringSurface.current=false;}
 }
 async function restoreWindow(){
  try {await captureSurfacePort.release?.(draft.id);}catch(error){setViewError(String(error));}
  try {await captureSurfacePort.end(draft.id);}
  catch(error){
   surfaceActive.current=false;setSurface(undefined);
   try{await captureSurfacePort.recovery(draft.id);}catch(showError){throw Error(String(error)+"\n"+String(showError));}
   throw error;
  }
 }
 async function showSurface(){
  await captureSurfacePort.show(draft.id);
  await new Promise<void>(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>resolve())));
  try{await captureSurfacePort.release?.(draft.id);}catch(error){setViewError(String(error));}
 }
 return <Suspense fallback={<p role="status">{t("capture.working")}</p>}><CaptureEditor toolbarPort={captureToolbarPreferences}
  onRetake={onReplace&&draft.sourceKind==="scroll"&&(!finalized||replacement)?retake:undefined} retakeLabel={t(replacement?"capture.openNewCapture":"capture.retake")}
  retakeStopping={retakeStopping} onStopRetake={retakeJob?async()=>{setRetakeStopping(true);try{const result=await captureCall<{stopping:boolean}>("scroll.stop",{id:retakeJob});if(!result.stopping)setRetakeStopping(false);}catch(error){setRetakeStopping(false);throw error;}}:undefined}
  finalized={finalized} surface={surface} onSurfaceReady={showSurface} onSurfaceError={error=>{void surfaceFailed(error).catch(error=>setViewError(String(error)));}}
  image={draft} sourceImage selection={selection} onConfirm={confirm} initialZoom={draft.sourceKind==="scroll"?"width":"fit"} selectionRequired={draft.selectionRequired} captureNotice={draft.notice?t("capture."+draft.notice):undefined} initialDocument={draft.document} onDocumentChange={changed} saveError={[viewError,error].filter(Boolean).join("\n")}
  closeSignal={closeSignal} onKeepEditing={()=>{void cancelCaptureClose().catch(error=>setError(String(error)));}}
  onClose={close}
  onCopy={async document=>{
   await flush(document);
   const job=await runJob<{draftId:string;revision:number;copied:boolean}>("draft.copy",{expectedId:draft.id,revision:session.revision});
   if(!job.result?.copied||job.result.draftId!==draft.id||job.result.revision!==session.revision)throw Error(t("capture.unknownResult"));
  }}
  onPin={async document=>{
   await flush(document);
   const job=await runJob<{draftId:string;revision:number;pinned:boolean}>("draft.pin",{expectedId:draft.id,revision:session.revision});
   if(!job.result?.pinned||job.result.draftId!==draft.id||job.result.revision!==session.revision)throw Error(t("capture.unknownResult"));
  }}
  onPreview={async document=>{
   // A canvas preview may contain only a paint-order prefix. It must never
   // replace the user's complete draft, crop, or autosaved annotations.
   const job=await runJob<EditPreview>("edit.preview",{expectedId:draft.id,document});
   const result=job.result;
   if(!result||!/^[a-f0-9]{32}$/.test(result.id)||result.sourceId!==draft.id||result.width!==(document.crop?.width??document.width)||result.height!==(document.crop?.height??document.height))throw Error(t("capture.previewFailed"));
   return result;
  }}
  onSave={document=>complete(document,false)} onSaveAs={document=>complete(document,false,true)}/></Suspense>;
}
