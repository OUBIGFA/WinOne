import {useEffect,useState,type ReactNode} from 'react';
import {X} from 'lucide-react';
import {useTranslation} from '../i18n';
import {Button} from '../ui/button';

/** A polling transport can repeat the same failure indefinitely. Dismiss the
 * current message locally; a different failure or an explicit retry shows again. */
export function ChatFeedback({message,children,label,role='status',className='chat-notice'}:{message?:string;label?:string;children?:ReactNode;role?:'status'|'alert';className?:string}){
 const {t}=useTranslation(),[dismissed,setDismissed]=useState(false);
 useEffect(()=>setDismissed(false),[message]);
 if(!message||dismissed)return null;
 return <div role={role} aria-label={label} className={'chat-feedback '+className}><span>{message}</span><div className="chat-feedback-actions">{children}<Button variant="ghost" size="icon" icon={X} aria-label={t('chat.close')} title={t('chat.close')} onClick={()=>setDismissed(true)}/></div></div>;
}
