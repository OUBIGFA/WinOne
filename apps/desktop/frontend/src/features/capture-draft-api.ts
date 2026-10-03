import {captureCall} from "../bridge";
import {i18n} from "../i18n";
import {restoreHistory,type AnnotationDocument} from "./capture-editor-model";

export interface CapturePlacement {x:number;y:number;desktopX:number;desktopY:number;desktopWidth:number;desktopHeight:number}
export interface CaptureDraft {id:string;revision:number;width:number;height:number;previewReady:boolean;document:AnnotationDocument;placement?:CapturePlacement|null;desktopPreviewId?:string|null;selectionRequired?:boolean;sourceKind?:"scroll"|"window"|"region"|"fullscreen"|null;selectionPending?:boolean;settings?:{openEditor?:boolean};replaces?:{id:string;revision:number}|null;notice?:"scrollStopped"|"scrollPartial"|"scrollTargetChanged"|null}
export function validCapturePlacement(value:CapturePlacement,width:number,height:number):boolean {
 const fields=[value.x,value.y,value.desktopX,value.desktopY,value.desktopWidth,value.desktopHeight,width,height];
 if(fields.some(number=>!Number.isInteger(number)||number< -2147483648||number>2147483647))return false;
 const right=value.desktopX+value.desktopWidth,bottom=value.desktopY+value.desktopHeight;
 return value.desktopWidth>0&&value.desktopHeight>0&&right<=2147483647&&bottom<=2147483647&&width>0&&height>0&&value.x>=value.desktopX&&value.y>=value.desktopY&&value.x+width<=right&&value.y+height<=bottom;
}
export async function readCaptureDraft(id:string):Promise<CaptureDraft> {
 const {draft}=await captureCall<{draft:CaptureDraft|null}>("draft.get",{expectedId:id});
 if(!draft||draft.id!==id||!Number.isSafeInteger(draft.revision)||draft.revision<0||draft.width!==draft.document.width||draft.height!==draft.document.height)throw Error(i18n.t("capture.draftUnavailable"));
 if(draft.placement!=null&&!validCapturePlacement(draft.placement,draft.width,draft.height))throw Error(i18n.t("capture.draftUnavailable"));
 if(draft.desktopPreviewId!=null&&(!draft.placement||draft.desktopPreviewId===draft.id||!/^[a-f0-9]{32}$/.test(draft.desktopPreviewId)))throw Error(i18n.t("capture.draftUnavailable"));
 if(draft.selectionRequired!=null&&typeof draft.selectionRequired!=="boolean"||draft.selectionRequired&&!draft.document.crop)throw Error(i18n.t("capture.draftUnavailable"));
 if(draft.notice!=null&&!["scrollStopped","scrollPartial","scrollTargetChanged"].includes(draft.notice))throw Error(i18n.t("capture.draftUnavailable"));
 if(draft.sourceKind!=null&&!["scroll","window","region","fullscreen"].includes(draft.sourceKind))throw Error(i18n.t("capture.draftUnavailable"));
 if(draft.replaces!=null&&(!/^[a-f0-9]{32}$/.test(draft.replaces.id)||draft.replaces.id===draft.id||!Number.isSafeInteger(draft.replaces.revision)||draft.replaces.revision<0))throw Error(i18n.t("capture.draftUnavailable"));
 if(draft.selectionPending!=null&&typeof draft.selectionPending!=="boolean"||draft.selectionPending&&(!draft.selectionRequired||!draft.document.crop||!["window","region"].includes(draft.sourceKind??"")))throw Error(i18n.t("capture.draftUnavailable"));
 return {...draft,document:restoreHistory(draft.document).document};
}

// Recover a committed replacement after a lost response. A different draft is
// accepted only when its persisted lineage matches this exact requested version.
export async function retakeScrollingDraft(id:string,revision:number,onStarted?:(id:string)=>void):Promise<CaptureDraft|undefined>{
 const matches=(draft:CaptureDraft)=>draft.replaces?.id===id&&draft.replaces.revision===revision;
 try{
  const job=await runCaptureJob<{draftId:string}>("scroll.retake",{expectedId:id,revision},onStarted);
  if(job.state==="cancelled")return;
  if(!job.result?.draftId)throw Error(i18n.t("capture.unknownResult"));
  const draft=await readCaptureDraft(job.result.draftId);
  if(!matches(draft))throw Error(i18n.t("capture.draftConflict"));
  return draft;
 }catch(error){
  try{
   const response=await captureCall<{draft:CaptureDraft|null}>("draft.get");
   if(response.draft&&matches(response.draft))return await readCaptureDraft(response.draft.id);
  }catch(recoveryError){throw Error(String(error)+"\n"+String(recoveryError));}
  throw error;
 }
}
export async function runCaptureJob<T>(method:string,params:unknown,onStarted?:(id:string)=>void):Promise<{state:"completed"|"cancelled";result?:T}> {
 let job=await captureCall<{id:string;state:string;error?:string;result?:T}>(method,params);
 // A file picker may precede the first response. Only a read-only preview can
 // stop waiting without reconciling a possibly committed mutation.
 const started=performance.now();
 const bounded=method==="edit.preview";
 const id=job.id;
 if(job.state==="running"&&typeof id==="string"&&id)onStarted?.(id);
 while(job.state==="running") {
  if(typeof id!=="string"||!id)throw Error(i18n.t("capture.unknownResult"));
  // Stop waiting, not the worker: a slow save or capture may still complete.
  if(bounded&&performance.now()-started>=300_000)throw Error(i18n.t("capture.pending"));
  await new Promise(resolve=>setTimeout(resolve,300));
  job=await captureCall("jobs.get",{id});
  if(job.id!==id)throw Error(i18n.t("capture.unknownResult"));
 }
 if(job.state==="failed")throw Error(job.error||i18n.t("capture.failed"));
 if(job.state!=="completed"&&job.state!=="cancelled")throw Error(i18n.t("capture.unknownResult"));
 return {state:job.state,result:job.result};
}
