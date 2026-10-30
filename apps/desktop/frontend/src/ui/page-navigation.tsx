import {InlineError} from "./notice";
import {useCallback,useEffect,useRef,useState} from "react";
import {Dialog,DialogContent,DialogTitle,DialogDescription,DialogFooter} from "../components/ui/dialog";
import {useTranslation} from "../i18n";
import {Button} from "./button";

// discard reverts a section that remains mounted (for example a draft shared
// by several tabs). Leaving the page unmounts it, so page navigation does not
// need to call it.
export type PageActivityChanged=(dirty:boolean,busy:boolean,source?:string,discard?:()=>void)=>void;
export interface SettingsClosePort {
 setActivity(dirty:boolean,busy:boolean):Promise<void>;
 pending():Promise<boolean>;
 subscribe(listener:()=>void):()=>void;
 cancel():Promise<void>;
 approve():Promise<void>;
}
type PendingDestination={page:string;onArrive?:()=>void}|{close:true}|{change:()=>void};
type SectionActivity={dirty:boolean;busy:boolean;discard?:()=>void};

// Report the active page without keeping inactive modules mounted or polling.
// The latest discard closure is read when needed instead of re-reporting the
// section on every render.
export function usePageActivity(onActivity:PageActivityChanged|undefined,dirty:boolean,busy:boolean,source="page",discard?:()=>void) {
 const latest=useRef(discard);latest.current=discard;
 const [revert]=useState(()=>()=>latest.current?.());
 const available=!!discard;
 useEffect(()=>{onActivity?.(dirty,busy,source,available?revert:undefined);},[onActivity,dirty,busy,source,available,revert]);
 useEffect(()=>()=>onActivity?.(false,false,source),[onActivity,source]);
}

function summarize(sections:Map<string,SectionActivity>){
 const values=[...sections.values()];
 return {dirty:values.some(value=>value.dirty),busy:values.some(value=>value.busy)};
}

export function usePageNavigation(page:string,navigate:(page:string)=>void,windowPort?:SettingsClosePort) {
 const {t}=useTranslation();
 const [activity,setActivity]=useState({dirty:false,busy:false});
 const [pending,setPending]=useState<PendingDestination>();
 const [error,setError]=useState("");
 const [committing,setCommitting]=useState(false);
 const updates=useRef(Promise.resolve());
 const sections=useRef(new Map<string,SectionActivity>());
 const closing=useRef(false);
 const closeAttempt=useRef(0);
 const approvalSent=useRef(false);
 const [approving,setApproving]=useState(false);
 const onActivity=useCallback<PageActivityChanged>((dirty,busy,source="page",discard)=>{
  if(dirty||busy)sections.current.set(source,{dirty,busy,discard});else sections.current.delete(source);
  const aggregate=summarize(sections.current);
  setActivity(current=>current.dirty===aggregate.dirty&&current.busy===aggregate.busy?current:aggregate);
  if(windowPort){
   updates.current=updates.current.catch(()=>{}).then(()=>windowPort.setActivity(aggregate.dirty,aggregate.busy));
   void updates.current.catch(error=>setError(String(error)));
  }
 },[windowPort]);
 useEffect(()=>{
  if(!windowPort)return;
  let alive=true;
  const request=()=>{setError("");setPending({close:true});};
  const unsubscribe=windowPort.subscribe(request);
  void windowPort.pending().then(pending=>{if(alive&&pending)request();}).catch(error=>{if(alive)setError(String(error));});
  return()=>{alive=false;unsubscribe();};
 },[windowPort]);
 const finish=useCallback(async(destination:PendingDestination)=>{
  // A tab change keeps the page mounted. Sections report their own clean
  // state after reverting, so their registrations are not cleared here.
  if("change" in destination){setPending(undefined);destination.change();return;}
  if("page" in destination){setPending(undefined);sections.current.clear();setActivity({dirty:false,busy:false});navigate(destination.page);destination.onArrive?.();return;}
  if(!windowPort||closing.current)return;
  closing.current=true;setCommitting(true);
  const attempt=++closeAttempt.current;
  try{await updates.current;if(attempt!==closeAttempt.current)return;approvalSent.current=true;setApproving(true);await windowPort.approve();setPending(undefined);}
  catch(error){setError(String(error));}
  finally{if(attempt===closeAttempt.current){closing.current=false;approvalSent.current=false;setApproving(false);setCommitting(false);}}
 },[navigate,windowPort]);
 const cancel=()=>{
  if(approvalSent.current)return;
  // Cancelling while native activity synchronization is pending invalidates
  // the continuation, so its late response cannot approve a discarded close.
  closeAttempt.current++;closing.current=false;setCommitting(false);
  if(pending&&"close" in pending&&windowPort)void windowPort.cancel().catch(error=>setError(String(error)));
  setPending(undefined);
 };
 const requestPage=(target:string,onArrive?:()=>void)=>{
  if(target===page){onArrive?.();return;}
  // Item intents must wait for the same unsaved-change decision as the page;
  // cancelling navigation must not leave a stale item to open on a later visit.
  if(activity.dirty||activity.busy){setError("");setPending({page:target,onArrive});return;}
  navigate(target);onArrive?.();
 };
 // Tabs use the same decision as page navigation. The callback is stable and
 // reads the current registrations, so a guarded tab list does not re-render
 // every time another section reports activity.
 const requestChange=useCallback((change:()=>void)=>{
  const current=summarize(sections.current);
  if(!current.dirty&&!current.busy){change();return;}
  setError("");setPending({change});
 },[]);
 // A requested navigation can finish after a successful save. Failed saves
 // keep dirty=true, so the user still decides whether to abandon the draft.
 useEffect(()=>{
  if(pending&&!activity.dirty&&!activity.busy)void finish(pending);
 },[pending,activity.dirty,activity.busy,finish]);
 const discard=()=>{
  if(!pending||activity.busy)return;
  // Revert drafts that stay mounted after a tab switch; an unmounted panel
  // loses its draft by itself, exactly as when leaving the page.
  if("change" in pending)for(const section of [...sections.current.values()])if(section.dirty)section.discard?.();
  void finish(pending);
 };
 const closeWindow=!!pending&&"close" in pending;
 const tabChange=!!pending&&"change" in pending;
 const description=activity.busy
  ?(closeWindow?"navigation.waitCloseDescription":tabChange?"navigation.tabWaitDescription":"navigation.waitDescription")
  :(closeWindow?"navigation.closeDescription":tabChange?"navigation.tabDirtyDescription":"navigation.dirtyDescription");
 const dialog=<>{pending!==undefined&&<Dialog open onOpenChange={open=>{if(!open)cancel();}}>
  <DialogContent showCloseButton={false}>
   <DialogTitle>{t(activity.busy?"navigation.waitTitle":"navigation.dirtyTitle")}</DialogTitle>
   <DialogDescription>{t(description)}</DialogDescription>
   {error&&<InlineError>{error}</InlineError>}
   <DialogFooter>
    <Button disabled={approving} onClick={cancel}>{t(activity.busy?"navigation.stay":"navigation.keepEditing")}</Button>
    {!activity.busy&&<Button variant="danger" disabled={committing} onClick={discard}>{t(closeWindow?"navigation.discardClose":"navigation.discard")}</Button>}
   </DialogFooter>
  </DialogContent>
 </Dialog>}{error&&!pending&&<InlineError>{error}</InlineError>}</>;
 return {onActivity,requestPage,requestChange,dialog,activity};
}
