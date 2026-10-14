// Adapted from Cindy UserMessage.tsx, Copyright 2026 XD Inc., Apache-2.0.
// Measure the same Markdown layout used on screen; keep the original source
// intact for copying and branching, even while the message is collapsed.
import {useState} from 'react';
import {ChevronDown,ChevronUp} from 'lucide-react';
import {useTranslation} from '../../i18n';
import {Button} from 'chatgpt-desktop-kit';
import {ChatMarkdown} from '../chat-markdown';
import {mayExceedVisualLineThreshold,useUserMessageAutoCollapse} from './userMessageCollapse';

export function UserMessageBody({text}:{text:string}){
 const {t}=useTranslation(),[expanded,setExpanded]=useState(false);
 // Markdown blocks can exceed the line limit even with few source characters.
 const measure=mayExceedVisualLineThreshold(text)||/^(?:#{1,6} |```|[-*>] |\d+\. )/m.test(text);
 const {mirrorRef,shouldCollapse}=useUserMessageAutoCollapse(text,measure);
 return <div className="chat-user-body">
  {measure&&<div ref={mirrorRef} aria-hidden="true" inert className="chat-user-measure"><ChatMarkdown text={text}/></div>}
  <div className={'chat-user-text'+(shouldCollapse&&!expanded?' chat-user-text--collapsed':'')}><ChatMarkdown text={text}/></div>
  {shouldCollapse&&<Button className="chat-user-disclosure" variant="ghost" size="sm" aria-expanded={expanded} onClick={()=>setExpanded(value=>!value)}>{t(expanded?'chat.collapseUserMessage':'chat.expandUserMessage')}{expanded?<ChevronUp size={13}/>:<ChevronDown size={13}/>}</Button>}
 </div>;
}
