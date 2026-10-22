import {useLayoutEffect,useRef,useState,type RefObject} from 'react';
import {createPortal,flushSync} from 'react-dom';
import {ArrowUpToLine,ArrowDownToLine,ChevronUp,ChevronDown} from 'lucide-react';
import {Button} from '../components/ui/button';
import {useTranslation} from '../i18n';
import {chatScrollMetrics,scrollChatToBottom} from './chat-scroll-position';

// Each user message starts a round. Tool steps and intermediate AI replies
// stay within that round and never become extra navigation stops.
export function ChatScrollControls({viewportRef,conversationId,onNavigate}:{viewportRef:RefObject<HTMLDivElement|null>;conversationId:string;onNavigate?:(latest:boolean)=>void}){
 const {t}=useTranslation(),targets=useRef<number[]>([]);
 const [state,setState]=useState({top:true,bottom:true,previous:-1,next:-1});
 const [placement,setPlacement]=useState<{top:number;header:HTMLElement|null}>({top:8,header:null});
 useLayoutEffect(()=>{
  const viewport=viewportRef.current;if(!viewport)return;
  let frame=0,previousPlacement=placement;
  const update=(beforePaint=false)=>{
   const {main,origin,usableHeight,bottomTop}=chatScrollMetrics(viewport);
   // The ordinary buttons float in the reading gutter. A very tall composer
   // leaves too little vertical room: use the existing header's horizontal
   // toolbar instead of covering the editor, approval card or text.
   const header=usableHeight<134?main?.querySelector<HTMLElement>('.chat-header')??null:null;
   const top=Math.max(8,usableHeight-118);
   if(previousPlacement.top!==top||previousPlacement.header!==header){
    previousPlacement={top,header};
    if(beforePaint)flushSync(()=>setPlacement(previousPlacement));else setPlacement(previousPlacement);
   }
   targets.current=Array.from(viewport.querySelectorAll<HTMLElement>('[data-chat-turn]')).map(node=>Math.max(0,node.getBoundingClientRect().top-origin+viewport.scrollTop-12));
   const atTop=viewport.scrollTop<=1,atBottom=Math.abs(bottomTop-viewport.scrollTop)<=1;
   let current=targets.current.length?0:-1;targets.current.forEach((top,index)=>{if(top<=viewport.scrollTop+16)current=index;});
   // The first round includes the top padding. A short final round may not
   // reach the top even at maximum scroll, so the bottom belongs to that round.
   if(atBottom&&!atTop)current=targets.current.length-1;
   const next={top:atTop,bottom:atBottom,previous:current>0?current-1:-1,next:!atBottom&&current+1<targets.current.length?current+1:-1};
   setState(value=>value.top===next.top&&value.bottom===next.bottom&&value.previous===next.previous&&value.next===next.next?value:next);
  };
  const schedule=()=>{cancelAnimationFrame(frame);frame=requestAnimationFrame(()=>update());};
  // Only a changed button placement needs a synchronous pre-paint commit.
  // Flushing unchanged placement also flushed unrelated content measurements
  // inside ResizeObserver and produced resize-loop notifications.
  const resize=new ResizeObserver(()=>update(true));resize.observe(viewport);if(viewport.firstElementChild)resize.observe(viewport.firstElementChild);
  const main=viewport.closest('.chat-main'),footer=main?.querySelector('.chat-input-area');if(footer)resize.observe(footer);
  if(main)resize.observe(main);
  const changes=new MutationObserver(schedule);changes.observe(viewport,{childList:true,subtree:true,characterData:true});
  viewport.addEventListener('scroll',schedule,{passive:true});update();
  return()=>{resize.disconnect();changes.disconnect();cancelAnimationFrame(frame);viewport.removeEventListener('scroll',schedule);};
 },[viewportRef,conversationId]);
 function jump(top:number,latest=false){
  const viewport=viewportRef.current;if(!viewport)return;onNavigate?.(latest);
  const behavior=matchMedia('(prefers-reduced-motion: reduce)').matches?'auto':'smooth';
  if(latest)scrollChatToBottom(viewport,behavior);else viewport.scrollTo({top,behavior});
 }
 const controls=<nav className={'chat-scroll-controls'+(placement.header?' chat-scroll-controls--header':'')} style={placement.header?undefined:{top:placement.top}} aria-label={t('chat.scrollNavigation')}>
  <Button variant="ghost" size="icon" title={t('navigation.scrollToTop')} aria-label={t('navigation.scrollToTop')} disabled={state.top} onClick={()=>jump(0)}><ArrowUpToLine/></Button>
  <Button variant="ghost" size="icon" title={t('chat.previousTurn')} aria-label={t('chat.previousTurn')} disabled={state.previous<0} onClick={()=>jump(targets.current[state.previous])}><ChevronUp/></Button>
  <Button variant="ghost" size="icon" title={t('chat.nextTurn')} aria-label={t('chat.nextTurn')} disabled={state.next<0} onClick={()=>jump(targets.current[state.next])}><ChevronDown/></Button>
  <Button variant="ghost" size="icon" title={t('navigation.scrollToBottom')} aria-label={t('navigation.scrollToBottom')} disabled={state.bottom} onClick={()=>jump(0,true)}><ArrowDownToLine/></Button>
 </nav>;
 return placement.header?createPortal(controls,placement.header):controls;
}
