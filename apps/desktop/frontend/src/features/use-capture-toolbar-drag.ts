import {useRef,useState,type PointerEvent,type KeyboardEvent} from "react";

type Position={left:number;top:number};
type Bounds={x:number;y:number;width:number;height:number};
export function useCaptureToolbarDrag(layout?:{toolbar:Position&{width:number;maxHeight:number};boundary:Bounds},height=0){
 const [position,setPosition]=useState<Position>();
 const drag=useRef<{id:number;x:number;y:number;origin:Position;previous?:Position}|undefined>(undefined);
 const constrain=(next:Position)=>{
  if(!layout)return next;
  const b=layout.boundary;
  return {left:Math.max(b.x+8,Math.min(next.left,b.x+b.width-layout.toolbar.width-8)),top:Math.max(b.y+8,Math.min(next.top,b.y+b.height-Math.min(height,layout.toolbar.maxHeight)-8))};
 };
 const cancel=()=>{if(drag.current){setPosition(drag.current.previous);drag.current=undefined;}};
 const move=(event:PointerEvent<HTMLDivElement>)=>{
  const start=drag.current;if(!start||start.id!==event.pointerId)return;
  setPosition(constrain({left:start.origin.left+event.clientX-start.x,top:start.origin.top+event.clientY-start.y}));
 };
 return {
  position:position&&constrain(position),
  onPointerDown:(event:PointerEvent<HTMLDivElement>)=>{
   // Buttons, menus and form controls retain their normal pointer behavior.
   const handle=(event.target as HTMLElement).closest<HTMLElement>("[data-toolbar-drag-handle]");
   if(!layout||event.button!==0||!event.currentTarget.contains(event.target as Node)||(!handle&&(event.target as HTMLElement).closest("button,input,select,textarea,[role=menu],[role=menuitem]")))return;
   event.preventDefault();event.stopPropagation();(handle??event.currentTarget).focus({preventScroll:true});
   drag.current={id:event.pointerId,x:event.clientX,y:event.clientY,origin:position?constrain(position):layout.toolbar,previous:position};
   event.currentTarget.setPointerCapture(event.pointerId);
  },
  onPointerMove:move,
  onPointerUp:(event:PointerEvent<HTMLDivElement>)=>{if(drag.current?.id!==event.pointerId)return;move(event);drag.current=undefined;event.currentTarget.releasePointerCapture(event.pointerId);},
  onPointerCancel:cancel,
  onLostPointerCapture:cancel,
  onKeyDown:(event:KeyboardEvent<HTMLDivElement>)=>{
   if(!layout||(event.target!==event.currentTarget&&!(event.target as HTMLElement).closest("[data-toolbar-drag-handle]")))return;
   if(event.key==="Escape"&&drag.current){event.preventDefault();event.stopPropagation();cancel();return;}
   const step=event.shiftKey?10:1,dx=event.key==="ArrowLeft"?-step:event.key==="ArrowRight"?step:0,dy=event.key==="ArrowUp"?-step:event.key==="ArrowDown"?step:0;
   if(!dx&&!dy)return;event.preventDefault();event.stopPropagation();
   const start=position??layout.toolbar;setPosition(constrain({left:start.left+dx,top:start.top+dy}));
  },
 };
}
