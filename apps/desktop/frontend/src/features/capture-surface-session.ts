import type {CaptureDraft} from "./capture-draft-api";
import type {CaptureSurface} from "./capture-surface-layout";

interface SurfacePort {
 begin(id:string):Promise<boolean>;
 monitor(id:string):Promise<CaptureSurface["monitor"]>;
 end(id:string):Promise<void>;
 recovery(id:string):Promise<void>;
 release?(id:string):Promise<void>;
}

interface OpeningPort extends SurfacePort {
 setEditing(active:boolean):Promise<void>;
 abortOpening():Promise<void>;
 revealEditor():Promise<void>;
}

export async function openCaptureEditor(draft:CaptureDraft,port:OpeningPort,fallbackMessage:string){
 try {
  await port.setEditing(true);
  const result=await prepareCaptureSurface(draft,port,fallbackMessage);
  // A fresh capture is one screen throughout. Do not substitute a second,
  // ordinary editor when the screenshot surface could not be prepared.
  if(!result.surface&&["window","region","fullscreen"].includes(draft.sourceKind??""))throw Error(result.warning||fallbackMessage);
  if(!result.surface&&!result.recoveryShown)await port.revealEditor();
  return result;
 } catch(error) {
  try {await port.release?.(draft.id);}catch(releaseError){error=Error(String(error)+" / "+String(releaseError));}
  try {await port.abortOpening();}
  catch(cleanupError){throw Error(String(error)+" / "+String(cleanupError));}
  throw error;
 }
}

// Keep entry and rollback in one operation so a failed entry cannot strand a
// hidden window or lose the failure that prompted recovery.
export async function prepareCaptureSurface(draft:CaptureDraft,port:SurfacePort,fallbackMessage:string):Promise<{surface?:CaptureSurface;warning:string;recoveryShown:boolean}> {
 let surface:CaptureSurface|undefined,warning="",recoveryShown=false;
 if(!draft.placement||!draft.desktopPreviewId)return {warning,recoveryShown};
 try {
  if(await port.begin(draft.id))surface={placement:draft.placement,desktopPreviewId:draft.desktopPreviewId,monitor:await port.monitor(draft.id)};
 } catch(error) {
  warning=fallbackMessage+" "+String(error);
  try {await port.end(draft.id);}
  catch(restoreError){
   warning+=" / "+String(restoreError);
   try {await port.recovery(draft.id);recoveryShown=true;}
   catch(showError){throw Error(warning+" / "+String(showError));}
  }
  try {await port.release?.(draft.id);}catch(releaseError){warning+=" / "+String(releaseError);}
 }
 return {surface,warning,recoveryShown};
}
