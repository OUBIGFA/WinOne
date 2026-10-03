import {useEffect,useRef,useState} from "react";
import {draggedSelection,selectionAt,type CaptureSelectionPort} from "./capture-selection";
import type {CropRect,Point} from "./capture-editor-model";

// Selection and annotation share the existing canvas. Only this small state
// machine changes; neither the image nor its native window is replaced.
export function useCaptureSelection(port:CaptureSelectionPort|undefined,width:number,height:number,onSelected:(crop:CropRect,saveImmediately:boolean)=>void){
 const [crop,setCrop]=useState<CropRect>(),[busy,setBusy]=useState(false),[error,setError]=useState("");
 const current=useRef(port),selected=useRef(onSelected);current.current=port;selected.current=onSelected;
 const pointer=useRef<Point|undefined>(undefined),windows=useRef<CropRect[]>([]),hover=useRef<CropRect|undefined>(undefined),drag=useRef<{start:Point;candidate?:CropRect;moved:boolean}|undefined>(undefined);
 const alive=useRef(true),committing=useRef(false);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
 useEffect(()=>{
  if(!port||port.mode!=="window")return;
  let stopped=false,inFlight=false;
  async function query(){
   if(stopped||inFlight||drag.current||committing.current)return;
   inFlight=true;const point=pointer.current;
   try{
    const result=await current.current!.candidates(point);
    if(stopped||drag.current||committing.current||pointer.current!==point)return;
    windows.current=result.windows;
    pointer.current??={x:result.x,y:result.y};
    hover.current=result.region??undefined;setCrop(hover.current);setError("");
   }catch(error){if(!stopped)setError(String(error));}
   finally{inFlight=false;}
  }
  void query();const timer=setInterval(()=>void query(),80);
  return()=>{stopped=true;clearInterval(timer);};
 },[!!port,port?.mode]);
 function move(point:Point){
  pointer.current=point;
  const active=drag.current;
  if(active){active.moved ||= Math.abs(point.x-active.start.x)>=4||Math.abs(point.y-active.start.y)>=4;setCrop(active.moved?draggedSelection(active.start,point,width,height):active.candidate);return;}
  const top=port?.mode==="window"?selectionAt(windows.current,point):undefined;
  // Retain a refined region while moving inside it; do not alternate it with
  // the top-level bounds on every pointer event between asynchronous replies.
  const inside=hover.current&&selectionAt([hover.current],point);
  const nearEdge=top&&(point.x-top.x<12||point.y-top.y<12||top.x+top.width-point.x<=12||top.y+top.height-point.y<=12);
  if(!inside||!top||nearEdge)hover.current=top;
  setCrop(hover.current);
 }
 function down(point:Point){if(committing.current)return;drag.current={start:point,candidate:hover.current,moved:false};}
 async function up(point:Point){
  const active=drag.current;if(!active||committing.current)return;
  move(point);drag.current=undefined;
  const next=active.moved?draggedSelection(active.start,point,width,height):active.candidate;
  if(!next)return;
  committing.current=true;setBusy(true);setError("");
  const immediate=!!current.current?.saveImmediately;
  try{await current.current!.commit(next);if(alive.current)selected.current(next,immediate);}
  catch(error){if(alive.current)setError(String(error));}
  finally{committing.current=false;if(alive.current)setBusy(false);}
 }
 function cancel(){drag.current=undefined;}
 return {crop,busy,error,move,down,up,cancel};
}
