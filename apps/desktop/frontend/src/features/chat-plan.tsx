import {useLayoutEffect,useState,type RefObject} from 'react';
import {useTranslation} from '../i18n';
import {InlinePlanCard,TodoListCard} from './cindy/TodoListCard';
import type {ChatWorkflow} from './chat-state';

// Both presentations are Cindy's real components. The host only maps durable
// items and decides visibility from the actual turn, not from all steps checked.
export function ChatPlan({workflow,animated=false,anchor}:{workflow?:ChatWorkflow;animated?:boolean;anchor?:string}){
 const {t}=useTranslation();
 if(!workflow?.items.length)return null;
 return <section className="chat-plan cindy-chat-ui" data-chat-plan-key={anchor} aria-label={t('chat.planTitle')}>
  <InlinePlanCard todos={workflow.items.map(item=>({content:item.step,status:item.status}))} animated={animated} explanation={workflow.explanation}/>
 </section>;
}

export function ChatPlanReminder({workflow,anchor,active,animated,viewport}:{workflow?:ChatWorkflow;anchor?:string;active:boolean;animated:boolean;viewport:RefObject<HTMLDivElement|null>}){
 const [inlineVisible,setInlineVisible]=useState(true),[dismissed,setDismissed]=useState<string>();
 const signature=JSON.stringify(workflow);
 useLayoutEffect(()=>{
  const root=viewport.current;if(!root||!anchor||!active){setInlineVisible(true);return;}
  const card=Array.from(root.querySelectorAll<HTMLElement>('[data-chat-plan-key]')).find(node=>node.dataset.chatPlanKey===anchor);
  if(!card){setInlineVisible(true);return;}
  const measure=()=>{const a=card.getBoundingClientRect(),b=root.getBoundingClientRect();setInlineVisible(a.bottom>b.top&&a.top<b.bottom);};
  measure();const observer=typeof IntersectionObserver==='undefined'?null:new IntersectionObserver(entries=>setInlineVisible(entries.some(entry=>entry.isIntersecting)),{root});observer?.observe(card);
  return()=>observer?.disconnect();
 },[anchor,active,viewport,signature]);
 if(!active||inlineVisible||!workflow?.items.length||dismissed===anchor)return null;
 return <div className="chat-plan-reminder cindy-chat-ui"><TodoListCard key={anchor} maxWidth="100%" todos={workflow.items.map(item=>({content:item.step,status:item.status}))} animated={animated} onDismiss={()=>setDismissed(anchor)}/></div>;
}
