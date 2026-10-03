import {useRef,type PointerEvent} from "react";
import {Button} from "../components/ui/button";
import {useTranslation} from "../i18n";
import type {CropRect} from "./capture-editor-model";
import {resizeCrop,type CropEdge} from "./capture-crop-resize";

const edges:readonly [CropEdge,number,number][]=[
 ["top-left",0,0],["top",.5,0],["top-right",1,0],["right",1,.5],
 ["bottom-right",1,1],["bottom",.5,1],["bottom-left",0,1],["left",0,.5],
];

export function CaptureCropHandles({crop,scale,width,height,disabled,onPreview,onCommit}:{crop:CropRect;scale:number;width:number;height:number;disabled:boolean;onPreview:(crop?:CropRect)=>void;onCommit:(crop:CropRect)=>void}) {
 const {t}=useTranslation();
 const drag=useRef<{pointer:number;x:number;y:number;crop:CropRect;edge:CropEdge}|undefined>(undefined);
 const cancel=()=>{drag.current=undefined;onPreview(undefined);};
 // Keep the whole hit target reachable when a crop touches the screen edge.
 const inset=(value:number,extent:number)=>{const margin=Math.min(12,extent/2);return Math.max(margin,Math.min(value,extent-margin));};
 const position=(event:PointerEvent<HTMLButtonElement>)=>{
  const start=drag.current;
  return start&&start.pointer===event.pointerId?resizeCrop(start.crop,start.edge,(event.clientX-start.x)/scale,(event.clientY-start.y)/scale,width,height):undefined;
 };
 // The shared Button animates all properties. Crop coordinates must update in
 // the same frame as the mask, so handles explicitly opt out of transitions.
 return <>{edges.map(([edge,x,y])=><Button key={edge} type="button" variant="outline" size="icon-xs"
  className="absolute z-10 touch-none rounded-sm p-0 active:translate-y-0"
  aria-label={t(`capture.resize.${edge}`)} title={t("capture.resizeHint")}
  style={{left:inset((crop.x+crop.width*x)*scale,width*scale),top:inset((crop.y+crop.height*y)*scale,height*scale),transform:"translate(-50%, -50%)",translate:"none",transitionProperty:"none",cursor:edge==="top"||edge==="bottom"?"ns-resize":edge==="left"||edge==="right"?"ew-resize":edge==="top-left"||edge==="bottom-right"?"nwse-resize":"nesw-resize"}}
  disabled={disabled}
  onPointerDown={event=>{
   if(event.button!==0||disabled||drag.current)return;
   event.preventDefault();event.stopPropagation();event.currentTarget.focus({preventScroll:true});
   drag.current={pointer:event.pointerId,x:event.clientX,y:event.clientY,crop,edge};
   event.currentTarget.setPointerCapture(event.pointerId);onPreview(crop);
  }}
  onPointerMove={event=>{if(!drag.current)return;event.stopPropagation();const next=position(event);if(next)onPreview(next);}}
  onPointerUp={event=>{
   if(!drag.current)return;event.stopPropagation();const next=position(event);
   if(!next)return;cancel();event.currentTarget.releasePointerCapture(event.pointerId);onCommit(next);
  }}
  onPointerCancel={cancel} onLostPointerCapture={()=>{if(drag.current)cancel();}}
  onDoubleClick={event=>event.stopPropagation()}
  onKeyDown={event=>{
   if(event.key==="Escape"&&drag.current){event.preventDefault();event.stopPropagation();cancel();return;}
   const step=event.shiftKey?10:1;
   const dx=event.key==="ArrowLeft"?-step:event.key==="ArrowRight"?step:0;
   const dy=event.key==="ArrowUp"?-step:event.key==="ArrowDown"?step:0;
   if(!dx&&!dy)return;event.preventDefault();event.stopPropagation();
   if(!drag.current)onCommit(resizeCrop(crop,edge,dx,dy,width,height));
  }}><span aria-hidden="true" className="size-1 bg-current"/></Button>)}</>;
}
