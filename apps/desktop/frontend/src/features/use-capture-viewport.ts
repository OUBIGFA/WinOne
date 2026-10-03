import {useLayoutEffect,useState} from "react";
import {visibleCaptureViewport} from "./capture-viewport";

export function useCaptureViewport(canvas:HTMLDivElement|null,scroll:HTMLDivElement|null,width:number,height:number){
 const [slice,setSlice]=useState({x:0,y:0,width:0,height:0});
 useLayoutEffect(()=>{
  if(!canvas||!scroll)return;
  let frame=0;
  const measure=()=>{
   frame=0;
   const bounds=scroll.getBoundingClientRect();
   const next=visibleCaptureViewport(canvas.getBoundingClientRect(),{left:bounds.left+scroll.clientLeft,top:bounds.top+scroll.clientTop,width:scroll.clientWidth,height:scroll.clientHeight});
   setSlice(current=>current.x===next.x&&current.y===next.y&&current.width===next.width&&current.height===next.height?current:next);
  };
  const schedule=()=>{if(!frame)frame=requestAnimationFrame(measure);};
  measure();
  scroll.addEventListener("scroll",schedule,{passive:true});
  const observer=new ResizeObserver(schedule);observer.observe(scroll);observer.observe(canvas);
  return()=>{scroll.removeEventListener("scroll",schedule);observer.disconnect();if(frame)cancelAnimationFrame(frame);};
 },[canvas,scroll,width,height]);
 return slice;
}
