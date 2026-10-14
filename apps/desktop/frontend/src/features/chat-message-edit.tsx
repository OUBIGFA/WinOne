// UserMessageEditBox layout and keyboard behavior from Cindy 69661422,
// Apache-2.0. WinOne retains Pi attachments and branches its saved input.
import {useEffect,useLayoutEffect,useRef,useState} from 'react';
import {Button} from '../ui/button';
import {Checkbox} from '../components/ui/checkbox';
import {ListComposerTextarea} from './cindy/ListComposerTextarea';
import {chatCall} from '../bridge';
import {useTranslation} from '../i18n';
import type {TurnReview} from './chat-turn-changes';

export function ChatMessageEdit({id,entry,text:initialText,hasAttachments,onStop,onCancel,onCommit}:{id:string;entry:string;text:string;hasAttachments:boolean;onStop?:()=>Promise<void>;onCancel:()=>void;onCommit:(text:string,restoreRevision?:string)=>Promise<void>}){
 const {t}=useTranslation(),[text,setText]=useState(initialText),[busy,setBusy]=useState(false),[loading,setLoading]=useState(true),[error,setError]=useState(''),[review,setReview]=useState<TurnReview>(),[restore,setRestore]=useState(false),input=useRef<HTMLTextAreaElement>(null),stop=useRef(onStop);
 useEffect(()=>{let alive=true;void (async()=>{try{await stop.current?.();const result=await chatCall<TurnReview>('review.turn',{sessionId:id,turnId:entry});if(alive){setReview(result);setRestore(!!result.snapshot.isReversible&&result.snapshot.workspaceState==='applied');}}catch(error){if(alive)setError(String(error));}finally{if(alive)setLoading(false);}})();input.current?.focus();return()=>{alive=false;};},[id,entry]);
 useLayoutEffect(()=>{const el=input.current;if(el){el.style.height='auto';el.style.height=Math.min(window.innerHeight*.4,Math.max(44,el.scrollHeight))+'px';}},[text]);
 const canSend=!busy&&!loading&&(!!text.trim()||hasAttachments);
 async function send(){if(!canSend)return;setBusy(true);setError('');try{await onCommit(text,restore?review?.snapshot.revision:undefined);onCancel();}catch(error){setError(String(error));}finally{setBusy(false);}}
 return <div className="cindy-chat-ui w-full rounded-xl border border-[var(--border)] bg-[var(--muted)] px-4 pt-3 pb-2.5">
  <ListComposerTextarea ref={input} value={text} onChange={event=>setText(event.target.value)} aria-label={t('chat.editMessage')} disabled={busy} maxLength={100000} rows={1} className="block w-full resize-none border-none bg-transparent p-0 text-sm leading-relaxed outline-none" onKeyDown={event=>{if(event.nativeEvent.isComposing||event.keyCode===229)return;if(event.key==='Escape'&&!busy){event.preventDefault();onCancel();}if(event.key==='Enter'&&!event.shiftKey){event.preventDefault();void send();}}}/>
  {review?.snapshot.isReversible&&review.snapshot.workspaceState==='applied'?<label className="mt-2 flex items-center gap-2 text-sm"><Checkbox checked={restore} onCheckedChange={value=>setRestore(!!value)}/>{t('chat.editRestoreFiles')}</label>:<p className="mt-2 text-sm text-muted-foreground">{t('chat.editKeepFiles')}</p>}
  {error&&<p role="alert" className="mt-2 text-sm">{error}</p>}
  <div className="mt-2 flex items-center justify-end gap-2"><Button size="sm" disabled={busy} onClick={onCancel}>{t('chat.cancel')}</Button><Button size="sm" variant="primary" busy={busy||loading} disabled={!canSend} onClick={()=>void send()}>{t('chat.editResend')}</Button></div>
 </div>;
}
