import {forwardRef,useImperativeHandle,useLayoutEffect,useRef,useState} from "react";
import {Textarea} from "../components/ui/textarea";
import type {AnnotationMark} from "./capture-editor-model";

export interface InlineTextHandle {commit():void}
export const CaptureInlineText=forwardRef<InlineTextHandle,{mark:AnnotationMark;scale:number;image:{width:number;height:number};label:string;hint:string;background?:string;preparing?:boolean;onCommit:(mark?:AnnotationMark)=>void;onCancel:()=>void}>(function CaptureInlineText({mark,scale,image,label,hint,background,preparing,onCommit,onCancel},ref){
 const input=useRef<HTMLTextAreaElement>(null),finished=useRef(false),composing=useRef(false),commitRequested=useRef(false);
 const [value,setValue]=useState(mark.text??""),[height,setHeight]=useState(mark.height!);
 const commit=()=>{
  if(finished.current)return;
  // A document save can be requested without blurring the IME text field.
  // Finish that request only after the candidate text has been confirmed.
  if(composing.current){commitRequested.current=true;return;}
  commitRequested.current=false;
  finished.current=true;
  const text=input.current?.value??value;
  onCommit(text.trim()?{...mark,text,height}:undefined);
 };
 useImperativeHandle(ref,()=>({commit}));
 useLayoutEffect(()=>{input.current?.focus({preventScroll:true});input.current?.select();},[]);
 useLayoutEffect(()=>{
  const node=input.current;if(!node)return;
  node.style.height="0px";
  const next=Math.min(image.height-mark.y!,Math.max(mark.fontSize!*1.4,Math.ceil((node.scrollHeight+2)/scale)));
  node.style.height=`${next*scale}px`;setHeight(next);
 },[value,scale,mark.fontSize,image.height,mark.y]);
 // Segoe UI's line-spacing/em ratio matches the native DrawString renderer;
 // CSS normal can expand mixed-script lines when a fallback font is present.
 return <div className="absolute z-20" style={{left:mark.x!*scale,top:mark.y!*scale,width:mark.width!*scale,height:height*scale,opacity:preparing?0:1,backgroundImage:background?`url(${background})`:undefined,backgroundSize:`${image.width*scale}px ${image.height*scale}px`,backgroundPosition:`${-mark.x!*scale}px ${-mark.y!*scale}px`}}>
  <Textarea ref={input} aria-label={label} title={hint} placeholder={label} rows={1} maxLength={4096} value={value} onChange={event=>setValue(event.target.value)}
   className="min-h-0 w-full resize-none rounded-none border-0 bg-transparent p-0 shadow-none focus-visible:ring-1" style={{height:height*scale,color:mark.color,fontSize:mark.fontSize!*scale,fontFamily:"Segoe UI",lineHeight:2724/2048}}
   onPointerDown={event=>event.stopPropagation()} onDoubleClick={event=>event.stopPropagation()}
   onCompositionStart={()=>{composing.current=true;}} onCompositionEnd={()=>{composing.current=false;if(commitRequested.current||document.activeElement!==input.current)queueMicrotask(commit);}}
   onBlur={commit} onKeyDown={event=>{
    event.stopPropagation();
    if(event.nativeEvent.isComposing||composing.current)return;
    if(event.key==="Escape"){event.preventDefault();finished.current=true;onCancel();}
    else if(event.key==="Enter"&&(event.ctrlKey||event.metaKey)){event.preventDefault();commit();}
   }}/>
 </div>;
});
