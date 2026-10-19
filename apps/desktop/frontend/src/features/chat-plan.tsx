import {useEffect,useId,useState} from 'react';
import {ChevronRight,Check,Minus,Ellipsis} from 'lucide-react';
import {Button} from '../ui/button';
import {useTranslation} from '../i18n';
import {Collapse} from './cindy/collapse';
import type {ChatWorkflow} from './chat-state';

// Compose the existing Cindy disclosure and shared controls. Plans retain their
// committed order; a finished plan stays available without remaining expanded.
export function ChatPlan({workflow}:{workflow?:ChatWorkflow}){
 const {t}=useTranslation(),id=useId(),[open,setOpen]=useState(false);
 const items=workflow?.items??[],complete=items.length>0&&items.every(item=>item.status==='completed');
 useEffect(()=>{if(complete)setOpen(false);},[complete]);
 if(!items.length)return null;
 const active=items.find(item=>item.status==='in_progress');
 return <section className="chat-work-process cindy-chat-ui py-2" aria-label={t('chat.planTitle')}>
  <div className="flex min-w-0 items-center gap-2"><Button variant="ghost" aria-expanded={open} aria-controls={id} onClick={()=>setOpen(value=>!value)}><ChevronRight size={14} className={open?'rotate-90':''}/>{t('chat.planTitle')} · {t('chat.planProgress',{completed:items.filter(item=>item.status==='completed').length,total:items.length})}</Button><span className="min-w-0 truncate text-sm text-muted-foreground">{active?.step??t(complete?'chat.planCompleted':'chat.planPending')}</span></div>
  <Collapse open={open} id={id} innerClassName="px-3">
   {workflow?.explanation&&<p className="text-sm text-muted-foreground">{workflow.explanation}</p>}
   <ol className="m-0 list-none space-y-2 p-0">{items.map((item,index)=>{const Icon=item.status==='completed'?Check:item.status==='in_progress'?Ellipsis:Minus;return <li key={index} className="flex items-start gap-2 text-sm"><Icon size={15} className="mt-0.5 shrink-0" aria-label={t('chat.plan_'+item.status)}/><span className="min-w-0 break-words">{item.step}</span></li>;})}</ol>
  </Collapse>
 </section>;
}
