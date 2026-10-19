import {useLayoutEffect,useRef,useState,type RefObject} from "react";
import {ArrowUpToLine,ArrowDownToLine} from "lucide-react";
import {useTranslation} from "../i18n";
import {Button} from "./ui/button";

// Reuse the shared shadcn/Base UI buttons and Lucide icons. Explicit targets
// cover split settings panes without treating text editors as page navigation.
export function PageScrollControls({viewportRef}:{viewportRef:RefObject<HTMLDivElement|null>}){
 const {t}=useTranslation();
 const target=useRef<HTMLElement|null>(null);
 const [position,setPosition]=useState({visible:false,atTop:true,atBottom:false});
 useLayoutEffect(()=>{
  const viewport=viewportRef.current;if(!viewport)return;
  let frame=0;
  let candidates:HTMLElement[]=[];
  const scrollable=(node:HTMLElement)=>node.clientHeight>0&&node.clientWidth>0&&node.scrollHeight-node.clientHeight>1&&/^(auto|scroll)$/.test(getComputedStyle(node).overflowY);
  const update=()=>{
   const available=candidates.filter(scrollable);
   target.current=target.current&&available.includes(target.current)?target.current:available[0]??null;
   const node=target.current;
   const next={visible:!!node,atTop:!node||node.scrollTop<=1,atBottom:!!node&&node.scrollHeight-node.clientHeight-node.scrollTop<=1};
   setPosition(current=>current.visible===next.visible&&current.atTop===next.atTop&&current.atBottom===next.atBottom?current:next);
  };
  const schedule=()=>{cancelAnimationFrame(frame);frame=requestAnimationFrame(update);};
  const observer=new ResizeObserver(schedule);
  const refresh=()=>{
   candidates=[viewport,...Array.from(viewport.querySelectorAll<HTMLElement>('[data-page-scroll-target]'))];
   observer.disconnect();
   for(const node of candidates){observer.observe(node);for(const child of Array.from(node.children))observer.observe(child);}
   schedule();
  };
  const activate=(event:Event)=>{
   const element=event.target;if(!(element instanceof Element))return;
   const next=candidates.find(node=>node!==viewport&&node.contains(element)&&scrollable(node));
   if(next)target.current=next;
   else if(event.type==="scroll"&&element===viewport&&scrollable(viewport))target.current=viewport;
   update();
  };
  // Tabs, async content and responsive viewport changes can add or remove
  // overflow without generating a scroll event.
  const mutations=new MutationObserver(refresh);
  mutations.observe(viewport,{subtree:true,childList:true,characterData:true,attributes:true,attributeFilter:["class","style","hidden","data-state"]});
  for(const event of ["scroll","pointerdown","focusin"])viewport.addEventListener(event,activate,true);
  refresh();
  return ()=>{observer.disconnect();mutations.disconnect();cancelAnimationFrame(frame);for(const event of ["scroll","pointerdown","focusin"])viewport.removeEventListener(event,activate,true);};
 },[viewportRef]);
 function jump(bottom:boolean){
  const node=target.current;if(!node)return;
  node.scrollTo({top:bottom?Math.max(0,node.scrollHeight-node.clientHeight):0,behavior:matchMedia("(prefers-reduced-motion: reduce)").matches?"auto":"smooth"});
 }
 if(!position.visible)return null;
 // Share the bottom action row with Save; the pair never wraps vertically.
 return <div data-slot="page-scroll-controls" className="ml-auto flex shrink-0 flex-row items-center gap-2">
  <Button variant="outline" size="icon" className="bg-background" title={t("navigation.scrollToTop")} aria-label={t("navigation.scrollToTop")} disabled={position.atTop} onClick={()=>jump(false)}><ArrowUpToLine aria-hidden="true"/></Button>
  <Button variant="outline" size="icon" className="bg-background" title={t("navigation.scrollToBottom")} aria-label={t("navigation.scrollToBottom")} disabled={position.atBottom} onClick={()=>jump(true)}><ArrowDownToLine aria-hidden="true"/></Button>
 </div>;
}
