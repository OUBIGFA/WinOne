import {InlineError} from "./notice";
import {useCallback,useEffect,useRef,useState} from "react";
import {Dialog,DialogContent,DialogTitle,DialogDescription,DialogFooter} from "../components/ui/dialog";
import {useTranslation} from "../i18n";
import {Button} from "./button";

export type PageActivityChanged=(dirty:boolean,busy:boolean,source?:string)=>void;
export interface SettingsClosePort {
 setActivity(dirty:boolean,busy:boolean):Promise<void>;
 pending():Promise<boolean>;
 subscribe(listener:()=>void):()=>void;
 cancel():Promise<void>;
 approve():Promise<void>;
}
type PendingDestination={page:string}|{close:true};

// Report the active page without keeping inactive modules mounted or polling.
export function usePageActivity(onActivity:PageActivityChanged|undefined,dirty:boolean,busy:boolean,source="page") {
 useEffect(()=>{onActivity?.(dirty,busy,source);},[onActivity,dirty,busy,source]);
 useEffect(()=>()=>onActivity?.(false,false,source),[onActivity,source]);
}

export function usePageNavigation(page:string,navigate:(page:string)=>void,windowPort?:SettingsClosePort) {
 const {t}=useTranslation();
 const [activity,setActivity]=useState({dirty:false,busy:false});
 const [pending,setPending]=useState<PendingDestination>();
 const [error,setError]=useState("");
 const [committing,setCommitting]=useState(false);
 const updates=useRef(Promise.resolve());
 const sections=useRef(new Map<string,{dirty:boolean;busy:boolean}>());
 const closing=useRef(false);
 const onActivity=useCallback<PageActivityChanged>((dirty,busy,source="page")=>{
  if(dirty||busy)sections.current.set(source,{dirty,busy});else sections.current.delete(source);
  const aggregate={dirty:[...sections.current.values()].some(value=>value.dirty),busy:[...sections.current.values()].some(value=>value.busy)};
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
  if("page" in destination){setPending(undefined);sections.current.clear();setActivity({dirty:false,busy:false});navigate(destination.page);return;}
  if(!windowPort||closing.current)return;
  closing.current=true;setCommitting(true);
  try{await updates.current;await windowPort.approve();setPending(undefined);}
  catch(error){setError(String(error));}
  finally{closing.current=false;setCommitting(false);}
 },[navigate,windowPort]);
 const cancel=()=>{
  if(closing.current)return;
  if(pending&&"close" in pending&&windowPort)void windowPort.cancel().catch(error=>setError(String(error)));
  setPending(undefined);
 };
 const requestPage=(target:string)=>{
  if(target===page)return;
  if(activity.dirty||activity.busy){setError("");setPending({page:target});return;}
  navigate(target);
 };
 // A requested navigation can finish after a successful save. Failed saves
 // keep dirty=true, so the user still decides whether to abandon the draft.
 useEffect(()=>{
  if(pending&&!activity.dirty&&!activity.busy)void finish(pending);
 },[pending,activity.dirty,activity.busy,finish]);
 const discard=()=>{
  if(!pending||activity.busy)return;
  void finish(pending);
 };
 const closeWindow=!!pending&&"close" in pending;
 const dialog=<><Dialog open={pending!==undefined} disablePointerDismissal onOpenChange={open=>{if(!open)cancel();}}>
  <DialogContent showCloseButton={false}>
   <DialogTitle>{t(activity.busy?"navigation.waitTitle":"navigation.dirtyTitle")}</DialogTitle>
   <DialogDescription>{t(activity.busy?(closeWindow?"navigation.waitCloseDescription":"navigation.waitDescription"):(closeWindow?"navigation.closeDescription":"navigation.dirtyDescription"))}</DialogDescription>
   {error&&<InlineError>{error}</InlineError>}
   <DialogFooter>
    <Button disabled={committing} onClick={cancel}>{t(activity.busy?"navigation.stay":"navigation.keepEditing")}</Button>
    {!activity.busy&&<Button variant="danger" disabled={committing} onClick={discard}>{t(closeWindow?"navigation.discardClose":"navigation.discard")}</Button>}
   </DialogFooter>
  </DialogContent>
 </Dialog>{error&&!pending&&<InlineError>{error}</InlineError>}</>;
 return {onActivity,requestPage,dialog,activity};
}
