import { useEffect, useRef, useState } from "react";
import { Copy, Save, Pin, Pencil, Layers, X } from "lucide-react";
import { useTranslation } from "../i18n";
import { Button } from "./button";
import "./capture-toolbar.css";

export type CaptureAction="copy"|"save"|"edit"|"pin"|"closePins";

// The same actions can be hosted in the settings page or a selection surface.
// Busy and completion state remain owned by the capture workflow, not the view.
export function CaptureToolbar({onAction,busy,canEdit=true,hasPins=false,onCancel}:{onAction:(action:CaptureAction)=>void;busy:boolean;canEdit?:boolean;hasPins?:boolean;onCancel?:()=>void}) {
  const {t}=useTranslation();
  const [focused,setFocused]=useState("copy");
  const nodes=useRef<Record<string,HTMLButtonElement|null>>({});
  const items=[
    {id:"copy",label:"capture.copy",icon:Copy,disabled:busy},
    {id:"save",label:"capture.saveAs",icon:Save,disabled:busy},
    {id:"edit",label:"capture.edit",icon:Pencil,disabled:busy||!canEdit},
    {id:"pin",label:"capture.pin",icon:Pin,disabled:busy},
    {id:"closePins",label:"capture.closePins",icon:Layers,disabled:busy||!hasPins},
    ...(onCancel?[{id:"cancel",label:"capture.cancelAction",icon:X,disabled:busy}]:[]),
  ];
  const enabled=items.filter(item=>!item.disabled).map(item=>item.id);
  const focusKey=enabled.join(",");
  useEffect(()=>{if(!enabled.includes(focused)&&enabled[0])setFocused(enabled[0]);},[focusKey,focused]);
  return <div role="toolbar" aria-label={t("capture.actions")} className="capture-toolbar" onKeyDown={event=>{
    if(!["ArrowLeft","ArrowRight","Home","End"].includes(event.key)||!enabled.length)return;
    event.preventDefault();
    const index=enabled.findIndex(id=>nodes.current[id]===document.activeElement);
    const next=event.key==="Home"?0:event.key==="End"?enabled.length-1:(Math.max(0,index)+(event.key==="ArrowRight"?1:-1)+enabled.length)%enabled.length;
    nodes.current[enabled[next]]?.focus();
  }}>
    {items.map(item=><Button key={item.id} ref={node=>{nodes.current[item.id]=node;}} tabIndex={focused===item.id?0:-1} disabled={item.disabled} variant={item.id==="copy"?"primary":"secondary"} onFocus={()=>setFocused(item.id)} onClick={()=>item.id==="cancel"?onCancel?.():onAction(item.id as CaptureAction)}>
      <item.icon size={18} strokeWidth={1.75} aria-hidden="true"/>{t(item.label)}
    </Button>)}
  </div>;
}
