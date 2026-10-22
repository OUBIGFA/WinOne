import {useEffect,useState,type ReactNode} from 'react';
import {ChevronRight,X} from 'lucide-react';
import {useTranslation} from '../i18n';
import {Button} from '../ui/button';
import {chatErrorSummary} from './chat-error';

/** A polling transport can repeat the same failure indefinitely. Dismiss the
 * current message locally; a different failure or an explicit retry shows again. */
export function ChatFeedback({message,children,label,role='status',className='chat-notice',error=role==='alert'||className.includes('error')}:{message?:string;label?:string;children?:ReactNode;role?:'status'|'alert';className?:string;error?:boolean}){
 const {t}=useTranslation(),[dismissed,setDismissed]=useState(false);
 useEffect(()=>setDismissed(false),[message]);
 if(!message||dismissed)return null;
 const summary=error?chatErrorSummary(message,t):message;
 return <div role={role} aria-label={label} className={'chat-feedback '+className}><div className="chat-feedback-content"><span>{summary}</span>{summary!==message&&<details key={message} className="chat-feedback-details"><summary><ChevronRight size={13} aria-hidden/>{t('chat.errorDetails')}</summary><pre>{message}</pre></details>}</div><div className="chat-feedback-actions">{children}<Button variant="ghost" size="icon" icon={X} aria-label={t('chat.close')} title={t('chat.close')} onClick={()=>setDismissed(true)}/></div></div>;
}
