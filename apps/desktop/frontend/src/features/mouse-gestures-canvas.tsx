import {useEffect,useRef,useState,type PointerEvent} from "react";
import {strokeCall} from "../bridge";
import {Dialog,DialogContent,DialogDescription,DialogHeader,DialogTitle} from "../components/ui/dialog";
import {Button} from "../ui/button";
import {Switch} from "../ui/switch";
import {Notice} from "../ui/notice";
import {useTranslation} from "../i18n";
import type {GesturePoint,StrokeGesture,StrokePreferences} from "./mouse-gestures-types";

// Stroke.Configure.DrawGesture integrates the 128 vectors and scales the
// longest dimension into the preview. This plots its data; it does not replace
// the recognition algorithm with browser directions or a second recognizer.
export function gesturePreviewPoints(gesture:StrokeGesture):GesturePoint[]{
 if(gesture.vectors?.length){const points=[{x:0,y:0}];for(let i=1;i<gesture.vectors.length;i++)points.push({x:points[i-1].x+gesture.vectors[i][0]/127,y:points[i-1].y+gesture.vectors[i][1]/127});return points;}
 return gesture.points??[];
}
function paint(canvas:HTMLCanvasElement,points:GesturePoint[],color:string,fit:boolean,widthValue=3,opacity=1,padding=24){
 const ratio=window.devicePixelRatio||1,box=canvas.getBoundingClientRect(),width=Math.max(1,box.width),height=Math.max(1,box.height);canvas.width=Math.round(width*ratio);canvas.height=Math.round(height*ratio);
 const context=canvas.getContext("2d");if(!context)return;context.scale(ratio,ratio);context.clearRect(0,0,width,height);if(points.length<2||widthValue===0||opacity===0)return;
 let values=points;
 if(fit){const xs=points.map(p=>p.x),ys=points.map(p=>p.y),minX=Math.min(...xs),minY=Math.min(...ys),w=Math.max(...xs)-minX,h=Math.max(...ys)-minY,scale=Math.min((width-padding*2)/Math.max(w,1),(height-padding*2)/Math.max(h,1));values=points.map(p=>({x:(p.x-minX)*scale+(width-w*scale)/2,y:(p.y-minY)*scale+(height-h*scale)/2}));}
 context.strokeStyle=color;context.fillStyle=color;context.lineWidth=widthValue;context.globalAlpha=opacity;context.lineCap="round";context.lineJoin="round";context.beginPath();context.moveTo(values[0].x,values[0].y);for(const point of values.slice(1))context.lineTo(point.x,point.y);context.stroke();
 const end=values.at(-1)!,prior=values.at(-2)!,angle=Math.atan2(end.y-prior.y,end.x-prior.x);context.beginPath();context.moveTo(end.x,end.y);context.lineTo(end.x-10*Math.cos(angle-.4),end.y-10*Math.sin(angle-.4));context.lineTo(end.x-10*Math.cos(angle+.4),end.y-10*Math.sin(angle+.4));context.closePath();context.fill();
}
// Scene rows reuse the existing Stroke vector preview at a smaller size. No
// direction-only icon or second gesture recognizer replaces a custom trail.
export function StrokeGestureThumbnail({gesture,color}:{gesture:StrokeGesture;color:string}){
 const {t}=useTranslation(),canvas=useRef<HTMLCanvasElement>(null);
 useEffect(()=>{const element=canvas.current;if(!element)return;const draw=()=>paint(element,gesturePreviewPoints(gesture),color,true,2,1,8);draw();const observer=new ResizeObserver(draw);observer.observe(element);return()=>observer.disconnect();},[gesture,color]);
 return <canvas ref={canvas} role="img" aria-label={t("stroke.preview")+": "+gesture.name} className="h-12 w-16 max-w-full"/>;
}
export function StrokeGesturePreview({gesture,color,onRecord,onReset,disabled}:{gesture:StrokeGesture;color:string;onRecord:()=>void;onReset:()=>void;disabled:boolean}){
 const {t}=useTranslation(),canvas=useRef<HTMLCanvasElement>(null);
 useEffect(()=>{const element=canvas.current;if(!element)return;const draw=()=>paint(element,gesturePreviewPoints(gesture),color,true);draw();const observer=new ResizeObserver(draw);observer.observe(element);return()=>observer.disconnect();},[gesture,color]);
 return <button type="button" className="w-full overflow-hidden rounded-lg border border-border bg-background text-left" aria-label={t("stroke.preview")} disabled={disabled} onClick={onRecord} onContextMenu={event=>{event.preventDefault();if(!disabled)onRecord();}} onPointerDown={event=>{if(event.button===1){event.preventDefault();if(!disabled)onReset();}}}><canvas ref={canvas} className="h-52 w-full"/>{gesturePreviewPoints(gesture).length<2&&<span className="block p-3 text-sm text-muted-foreground">{t("stroke.recordEmpty")}</span>}</button>;
}

type Recorded={id:string;state:string;points:GesturePoint[];vectors:number[][];match?:{id:string;name:string;score:number}};
export function StrokeRecorder({gesture,preferences,onUse,onClose}:{gesture:StrokeGesture;preferences:StrokePreferences;onUse:(result:Pick<StrokeGesture,"points"|"vectors">)=>void;onClose:()=>void}){
 const {t}=useTranslation(),canvas=useRef<HTMLCanvasElement>(null),points=useRef<GesturePoint[]>([]),drawing=useRef(false),live=useRef(true),recordId=useRef<string|undefined>(undefined);
 const [ready,setReady]=useState(false),[busy,setBusy]=useState(false),[error,setError]=useState(""),[result,setResult]=useState<Recorded>(),[blend,setBlend]=useState(!!gesture.vectors?.length);
 useEffect(()=>{live.current=true;void strokeCall<{id:string}>("record.start").then(value=>{recordId.current=value.id;if(live.current)setReady(true);else void strokeCall("record.cancel",{id:value.id}).catch(console.warn);},error=>{if(live.current)setError(String(error));});return()=>{live.current=false;if(recordId.current)void strokeCall("record.cancel",{id:recordId.current}).catch(console.warn);};},[]);
 const trigger={left:0,middle:1,right:2,x1:3,x2:4}[preferences.triggerButton]??2;
 function point(event:PointerEvent<HTMLCanvasElement>){const box=event.currentTarget.getBoundingClientRect();return{x:Math.round(event.clientX-box.left),y:Math.round(event.clientY-box.top)};}
 async function finish(){drawing.current=false;if(!recordId.current||points.current.length<2)return;setBusy(true);setError("");try{const value=await strokeCall<Recorded>("record.read",{id:recordId.current,gestureId:gesture.id,blend,priorVectors:gesture.vectors,points:points.current});if(live.current)setResult(value);}catch(error){if(live.current)setError(String(error));}finally{if(live.current)setBusy(false);}}
 return <Dialog open onOpenChange={open=>{if(!open)onClose();}}><DialogContent className="sm:max-w-3xl"><DialogHeader><DialogTitle>{t("stroke.recordTitle")}</DialogTitle><DialogDescription>{t("stroke.recordHint")} {t("stroke."+preferences.triggerButton)}</DialogDescription></DialogHeader>
  <canvas ref={canvas} aria-label={t("stroke.recordArea")} aria-busy={!ready||busy} className="h-80 w-full touch-none rounded-lg border border-border bg-muted/30" onContextMenu={event=>event.preventDefault()} onPointerDown={event=>{if(event.button!==trigger||!ready||busy)return;event.preventDefault();event.currentTarget.setPointerCapture(event.pointerId);drawing.current=true;points.current=[point(event)];setResult(undefined);paint(event.currentTarget,points.current,preferences.pen.color,false,preferences.pen.thickness,preferences.pen.opacity);}} onPointerMove={event=>{if(!drawing.current||points.current.length>=4096)return;points.current.push(point(event));paint(event.currentTarget,points.current,preferences.pen.color,false,preferences.pen.thickness,preferences.pen.opacity);}} onPointerUp={event=>{if(!drawing.current)return;points.current.push(point(event));if(event.currentTarget.hasPointerCapture(event.pointerId))event.currentTarget.releasePointerCapture(event.pointerId);void finish();}} onPointerCancel={()=>{drawing.current=false;points.current=[];}}/>
  <label className="flex items-center justify-between gap-3 text-sm"><span>{t("stroke.blend")}</span><Switch checked={blend} disabled={!gesture.vectors?.length||busy} onCheckedChange={setBlend}/></label>
  {busy&&<p role="status">{t("stroke.recordBusy")}</p>}{result&&<p role="status" className="text-sm">{t("stroke.recordDone")}{result.match&&<> · {t("stroke.recordMatch",result.match)}</>}</p>}{error&&<Notice>{error}</Notice>}
  <div className="flex justify-end gap-2"><Button variant="ghost" onClick={onClose}>{t("stroke.cancelRecord")}</Button><Button disabled={!result||busy} onClick={()=>{if(result){onUse({points:result.points,vectors:result.vectors});onClose();}}}>{t("stroke.useRecord")}</Button></div>
 </DialogContent></Dialog>;
}
