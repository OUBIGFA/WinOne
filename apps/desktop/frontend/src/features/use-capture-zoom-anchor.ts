import {useLayoutEffect,useRef} from "react";
import {captureZoomAnchor,captureZoomDelta,type CaptureZoom} from "./capture-viewport";

export function useCaptureZoomAnchor(scroll:HTMLDivElement|null,image:HTMLElement|null,zoom:CaptureZoom,scale:number){
 const pending=useRef<{image:HTMLElement;point:{x:number;y:number}}|undefined>(undefined);
 const view=()=>{
  const rect=scroll!.getBoundingClientRect();
  return {left:rect.left+scroll!.clientLeft,top:rect.top+scroll!.clientTop,width:scroll!.clientWidth,height:scroll!.clientHeight};
 };
 useLayoutEffect(()=>{
  const anchor=pending.current;
  pending.current=undefined;
  if(!scroll||!image||anchor?.image!==image)return;
  const delta=captureZoomDelta(anchor.point,image.getBoundingClientRect(),view());
  scroll.scrollLeft+=delta.x;scroll.scrollTop+=delta.y;
 },[scroll,image,zoom,scale]);
 return ()=>{
  if(!scroll||!image)return;
  const rect=image.getBoundingClientRect();
  if(rect.width>0&&rect.height>0)pending.current={image,point:captureZoomAnchor(rect,view())};
 };
}
