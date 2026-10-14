import {useEffect,useState,type ReactNode} from "react";
import {ContextMenu} from "@base-ui/react/context-menu";
import {Crosshair,Monitor,RefreshCw} from "lucide-react";
import {strokeCall} from "../bridge";
import {Button} from "../ui/button";
import {Dialog,DialogContent,DialogHeader,DialogTitle} from "../components/ui/dialog";
import {Notice} from "../ui/notice";
import {useTranslation} from "../i18n";
import type {StrokeWindow} from "./mouse-gestures-types";

export function StrokeContextMenu({children,items,inline=false}:{children:ReactNode;items:{label:string;run:()=>void;disabled?:boolean}[];inline?:boolean}){
 return <ContextMenu.Root><ContextMenu.Trigger className={inline?"inline-flex min-w-0 align-middle":"block"}>{children}</ContextMenu.Trigger><ContextMenu.Portal><ContextMenu.Positioner className="z-50" sideOffset={4}><ContextMenu.Popup className="min-w-40 rounded-lg bg-popover p-1 text-sm text-popover-foreground shadow-sm ring-1 ring-border">{items.map(item=><ContextMenu.Item key={item.label} disabled={item.disabled} className="cursor-default rounded px-2 py-2 outline-none data-highlighted:bg-accent data-disabled:opacity-50" onClick={item.run}>{item.label}</ContextMenu.Item>)}</ContextMenu.Popup></ContextMenu.Positioner></ContextMenu.Portal></ContextMenu.Root>;
}
export const escapedProcessPattern=(path:string)=>path.replace(/[.*+?^${}()|[\]\\]/g,"\\$&");

// The original crosshair workflow uses pointer capture on this control. Host
// reads the target under the cursor on release; no temporary global hook is used.
export function StrokePathPicker({disabled,onPick}:{disabled:boolean;onPick:(target:StrokeWindow)=>void}){
 const {t}=useTranslation(),[error,setError]=useState(""),[windows,setWindows]=useState<StrokeWindow[]>(),[busy,setBusy]=useState(false);
 useEffect(()=>()=>{document.documentElement.style.cursor="";},[]);
 async function choose(){setBusy(true);setError("");try{setWindows(await strokeCall<StrokeWindow[]>("windows.list"));}catch(error){setError(String(error));}finally{setBusy(false);}}
 async function capture(){setBusy(true);setError("");try{onPick(await strokeCall<StrokeWindow>("inspect.target"));}catch(error){setError(String(error));}finally{setBusy(false);}}
 return <><div className="flex flex-wrap gap-2"><Button icon={Crosshair} disabled={disabled||busy} title={t("stroke.dragPath")} onPointerDown={event=>{if(event.button!==0)return;event.currentTarget.setPointerCapture(event.pointerId);document.documentElement.style.cursor="crosshair";}} onPointerUp={event=>{document.documentElement.style.cursor="";if(event.currentTarget.hasPointerCapture(event.pointerId)){event.currentTarget.releasePointerCapture(event.pointerId);void capture();}}} onPointerCancel={()=>{document.documentElement.style.cursor="";}}>{t("stroke.addPath")}</Button><Button icon={Monitor} disabled={disabled||busy} onClick={()=>void choose()}>{t("stroke.chooseWindow")}</Button></div>{error&&<Notice>{error}</Notice>}
  <Dialog open={windows!==undefined} onOpenChange={open=>{if(!open)setWindows(undefined);}}><DialogContent className="sm:max-w-xl"><DialogHeader><DialogTitle>{t("stroke.windows")}</DialogTitle></DialogHeader><Button icon={RefreshCw} disabled={busy} onClick={()=>void choose()}>{t("stroke.refreshWindows")}</Button><div className="max-h-96 space-y-1 overflow-auto">{windows?.map(target=><button key={target.window} type="button" className="block w-full rounded-lg p-2 text-left hover:bg-muted focus-visible:outline" onClick={()=>{onPick(target);setWindows(undefined);}}><span className="block truncate">{target.title}</span><span className="block break-all text-sm text-muted-foreground">{target.processPath}</span></button>)}{windows?.length===0&&<p>{t("stroke.noWindows")}</p>}</div></DialogContent></Dialog>
 </>;
}
