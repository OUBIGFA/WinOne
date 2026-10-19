// Adapted from Cindy ChatInput.tsx, Copyright 2026 XD Inc., Apache-2.0.
// Keep the standalone composer surface; WinOne owns viewport-aware resizing.
import {useLayoutEffect,useRef,useState,type ReactNode,type RefObject,type PointerEvent} from "react";
import {GripVertical} from "lucide-react";
import {useTranslation} from "../../i18n";
import {cn} from "./adapter";
const minimum=132,initialHeight=220;
export function ComposerFrame({children,newConversation,editor,content,attachmentCount=0}:{children:ReactNode;newConversation:boolean;editor:RefObject<HTMLTextAreaElement|null>;content?:string;attachmentCount?:number}){
 const {t}=useTranslation(),root=useRef<HTMLDivElement>(null),welcomeLayout=useRef(''),welcomeBottom=useRef(24),welcomeBase=useRef(initialHeight);
 const [limit,setLimit]=useState(()=>Math.max(minimum,window.innerHeight-180));
 // Every new-chat action remounts this frame. A previous conversation's tall
 // editor must not turn the next welcome screen into a docked editor.
 const [height,setHeight]=useState(initialHeight);
 const [contentHeight,setContentHeight]=useState(0);
 const [minimumHeight,setMinimumHeight]=useState(minimum);
 const heightRef=useRef(height);heightRef.current=height;
 const displayedHeight=Math.min(Math.max(height,contentHeight,minimumHeight),Math.max(limit,minimumHeight));
 useLayoutEffect(()=>{
  const textarea=editor.current;
  if(!textarea)return;
  const measure=()=>{
   const composer=root.current?.querySelector<HTMLElement>('.chat-composer'),toolbar=root.current?.querySelector<HTMLElement>('.chat-composer-toolbar');
   const attachments=root.current?.querySelector<HTMLElement>('.chat-attachments');
   if(!composer||!toolbar||!composer.getBoundingClientRect().width)return;
   // A flexed textarea's scrollHeight includes its assigned empty space. Measure
   // its natural content before paint, then restore the exact inline styles.
   // Measure an unscrolled mirror. Shrinking the focused, scrolled editor can
   // retain its old scroll extent in Chromium and prevent it shrinking again.
   const mirror=textarea.cloneNode(false) as HTMLTextAreaElement;
   mirror.removeAttribute('id');mirror.removeAttribute('aria-label');mirror.tabIndex=-1;mirror.value=content??'';
   Object.assign(mirror.style,{position:'absolute',visibility:'hidden',pointerEvents:'none',flex:'none',height:'0',minHeight:'0',maxHeight:'none',width:`${textarea.getBoundingClientRect().width}px`});
   composer.appendChild(mirror);const textHeight=mirror.scrollHeight;mirror.remove();
   const styles=getComputedStyle(composer),toolbarStyles=getComputedStyle(toolbar);
   const chrome=parseFloat(styles.paddingTop)+parseFloat(styles.paddingBottom)+parseFloat(styles.borderTopWidth)+parseFloat(styles.borderBottomWidth)+parseFloat(toolbarStyles.marginTop);
   const attachmentsHeight=attachments?Math.min(72,attachments.scrollHeight)+parseFloat(getComputedStyle(attachments).marginBottom):0;
   const controls=toolbar.getBoundingClientRect().height+attachmentsHeight+chrome;
   // Reserve the toolbar and a usable text line before allocating attachments.
   // Existing conversations also need this floor after a previously tiny resize.
   setMinimumHeight(Math.max(minimum,44+controls));
   setContentHeight(newConversation?textHeight+controls:0);
  };
  const observer=new ResizeObserver(measure);observer.observe(textarea);const toolbar=root.current?.querySelector('.chat-composer-toolbar');if(toolbar)observer.observe(toolbar);measure();
  return()=>observer.disconnect();
 },[content,attachmentCount,newConversation,editor]);
 useLayoutEffect(()=>{
  const main=root.current?.closest('.chat-main'),composer=root.current?.querySelector<HTMLElement>('.chat-composer');
  const welcome=root.current?.closest<HTMLElement>('.chat-welcome'),area=root.current?.closest<HTMLElement>('.chat-welcome-composer');
  if(!main||!composer)return;
  const measure=()=>{
   // A minimized/hidden WebView reports zero bounds. Preserve the last useful
   // geometry instead of treating it as a normal transcript at the bottom.
   if(!main.getBoundingClientRect().height)return;
   if(welcome&&area){
    if(!welcome.clientHeight)return;
    const extra=area.getBoundingClientRect().height-composer.getBoundingClientRect().height;
    welcome.style.setProperty('--chat-welcome-composer-size',`${area.getBoundingClientRect().height}px`);
    const layoutKey=`${welcome.clientHeight}:${welcome.clientWidth}:${extra}`;
    if(welcomeLayout.current!==layoutKey){
     welcomeLayout.current=layoutKey;
     const gap=Math.min(192,Math.max(128,welcome.clientHeight*.18));
     const logo=Math.min(176,Math.max(0,welcome.clientHeight-extra-initialHeight-gap-48));
     const base=Math.min(initialHeight,Math.max(minimum,welcome.clientHeight-extra-logo-gap-48));
     const total=base+extra+logo+gap;
     welcomeBase.current=base;
     welcome.style.setProperty('--chat-welcome-logo-size',`${logo}px`);
     welcome.style.setProperty('--chat-welcome-gap',`${gap}px`);
     welcomeBottom.current=Math.max(24,(welcome.clientHeight-total)/2);
    }
    // Grow about the initial composer centre from the first pixel. Once either
    // edge reaches the viewport, the remaining space goes to the other edge.
    const bottom=Math.max(12,Math.min(welcomeBottom.current-(displayedHeight-welcomeBase.current)/2,welcome.clientHeight-extra-displayedHeight-12));
    welcome.style.setProperty('--chat-welcome-bottom',`${bottom}px`);
    setLimit(Math.max(minimum,welcome.clientHeight-extra-24));
    return;
   }
   const top=main.querySelector('.chat-header')?.getBoundingClientRect().bottom??main.getBoundingClientRect().top;
   setLimit(Math.max(minimum,composer.getBoundingClientRect().bottom-top-54));
  };
  const observer=new ResizeObserver(measure);observer.observe(main);if(area)observer.observe(area);window.addEventListener('resize',measure);measure();
  return()=>{observer.disconnect();window.removeEventListener('resize',measure);};
 },[newConversation,displayedHeight,height,contentHeight]);
 // Sizing belongs to this mounted page. Entering a conversation starts at its
 // default docked height instead of reusing a welcome page's expanded editor.
 function persist(next:number){heightRef.current=next;setHeight(next);}
 function resizeFrom(start:DOMRect,delta:number){
  const composer=root.current?.querySelector<HTMLElement>('.chat-composer'),main=root.current?.closest('.chat-main');
  if(!composer||!main)return;
  const welcome=root.current?.closest<HTMLElement>('.chat-welcome'),area=root.current?.closest<HTMLElement>('.chat-welcome-composer');
  if(welcome&&area){
   const bounds=welcome.getBoundingClientRect(),box=composer.getBoundingClientRect(),outer=area.getBoundingClientRect();
   // The top handle follows the pointer while both edges have room to grow.
   const max=Math.max(minimum,bounds.height-(outer.height-box.height)-24);
   setLimit(max);persist(Math.max(minimumHeight,Math.min(max,start.height+delta*2)));return;
  }
  const top=main.querySelector('.chat-header')?.getBoundingClientRect().bottom??main.getBoundingClientRect().top;
  const max=Math.max(minimumHeight,start.bottom-top-54);setLimit(max);persist(Math.max(minimumHeight,Math.min(max,start.height+delta)));
 }
 function startResize(event:PointerEvent<HTMLDivElement>){
  if(event.button!==0)return;
  event.preventDefault();const handle=event.currentTarget,startY=event.clientY,start=handle.parentElement!.getBoundingClientRect();
  handle.setPointerCapture(event.pointerId);
  const move=(next:globalThis.PointerEvent)=>resizeFrom(start,startY-next.clientY);
  const done=()=>{handle.removeEventListener('pointermove',move);handle.removeEventListener('lostpointercapture',done);};
  handle.addEventListener('pointermove',move);handle.addEventListener('lostpointercapture',done,{once:true});
 }
 return <div ref={root} className="cindy-chat-ui relative w-full" data-chat-input-root data-new-conversation={newConversation||undefined}>
  <div style={{height:displayedHeight}} className={cn('chat-composer relative flex w-full flex-col')} onMouseDown={event=>{
   if(event.button===0&&(event.target===event.currentTarget||(event.target as HTMLElement).classList.contains('chat-composer-spacer'))){event.preventDefault();editor.current?.focus();}
  }}><div role="separator" tabIndex={0} aria-orientation="horizontal" aria-label={t('chat.resizeComposer')} aria-valuemin={Math.round(minimumHeight)} aria-valuemax={Math.max(limit,minimumHeight)} aria-valuenow={Math.round(displayedHeight)} className="chat-composer-resize" onPointerDown={startResize} onKeyDown={event=>{if(event.key==='ArrowUp'||event.key==='ArrowDown'){event.preventDefault();persist(Math.max(minimumHeight,Math.min(limit,displayedHeight+(event.key==='ArrowUp'?24:-24))));}}}><GripVertical aria-hidden="true" size={16}/></div>{children}</div>
 </div>;
}
