import {useEffect,useRef,useState} from 'react';
import {Bot,MessageSquare,RefreshCw,X} from 'lucide-react';
import {chatCall} from '../bridge';
import {useTranslation} from '../i18n';
import {Button} from '../ui/button';
import {Popover,PopoverContent,PopoverTrigger} from '../components/ui/popover';
import {Dialog,DialogContent,DialogTitle,DialogFooter} from '../components/ui/dialog';
import {ChatMessageView} from './chat-messages';
import {copyText} from './clipboard';
import {useChatTheme} from './chat-desktop-kit';
import type {ChatMessage,ChatSubagent} from './chat-state';

type History={entries:{id:number;model?:ChatMessage[]}[];cursor?:Record<string,unknown>};
// Reuse the existing message renderer and controls. A child's transcript and
// draft are scoped to its persistent name; they never replace the parent draft.
export function ChatAgents({conversationId,agents,busy,paused,onStop}:{conversationId:string;agents:ChatSubagent[];busy:boolean;paused:boolean;onStop?:(name:string)=>void}){
 const {t}=useTranslation(),theme=useChatTheme();
 const [menu,setMenu]=useState(false),[selected,setSelected]=useState(''),[history,setHistory]=useState<History>(),[error,setError]=useState(''),[loading,setLoading]=useState(false),[sending,setSending]=useState(false),[drafts,setDrafts]=useState<Record<string,string>>({});
 const readToken=useRef(0),agent=agents.find(agent=>agent.name===selected);
 async function read(cursor?:History['cursor']){
  const token=++readToken.current;setLoading(true);setError('');
  try{const page=await chatCall<History>('agents',{id:conversationId,action:'read',name:selected,...(cursor?{agentCursor:cursor}:{})});
   if(token===readToken.current)setHistory(current=>cursor?{...page,entries:[...page.entries,...(current?.entries??[])]}:page);
  }catch(error){if(token===readToken.current)setError(error instanceof Error?error.message:String(error));}
  finally{if(token===readToken.current)setLoading(false);}
 }
 useEffect(()=>{setHistory(undefined);setError('');setLoading(false);},[selected]);
 // Refresh at committed status edges rather than polling storage while the
 // dialog is closed. Invalidate late reads before switching child or pausing.
 // A fast follow-up may complete between Host snapshots, leaving status
 // unchanged. Its committed finish timestamp still invalidates the history.
 useEffect(()=>{if(selected&&!paused)void read();return()=>{readToken.current++;};},[selected,paused,agent?.status,agent?.finishedAt]);
 async function send(){
  const name=selected,text=drafts[name]??'';if(!text.trim())return;
  setSending(true);setError('');
  try{await chatCall('agents',{id:conversationId,action:'follow_up',name,text});setDrafts(current=>({...current,[name]:current[name]===text?'':current[name]}));await read();}
  catch(error){setError(error instanceof Error?error.message:String(error));}
  finally{setSending(false);}
 }
 if(!agents.length)return null;
 return <>
  <Popover open={menu} onOpenChange={setMenu}><PopoverTrigger render={<Button variant="ghost" size="icon" icon={Bot} aria-label={t('chat.agents')+' · '+agents.length} disabled={busy}/>}/><PopoverContent side="top" align="start" className="chat-row-menu max-h-80 overflow-y-auto"><div role="list" aria-label={t('chat.agents')}>{agents.map(agent=><div key={agent.name} role="listitem" className="chat-agent-row"><div className="chat-agent-copy"><strong>{agent.name}</strong><span className="chat-agent-status" data-status={agent.status}>{t(paused?'chat.taskWaitingForResume':'chat.agentStatus_'+agent.status)}</span>{agent.task&&<p>{agent.task}</p>}</div><Button variant="ghost" size="icon" icon={MessageSquare} aria-label={t('chat.agentMessages')+' · '+agent.name} disabled={busy||paused} onClick={()=>{setMenu(false);setSelected(agent.name);}}/>{agent.status==='running'&&onStop&&<Button variant="ghost" size="icon" icon={X} aria-label={t('chat.stopAgent')+' · '+agent.name} disabled={busy||paused} onClick={()=>onStop(agent.name)}/>}</div>)}</div></PopoverContent></Popover>
  <Dialog open={!!selected} onOpenChange={open=>{if(!open&&!sending)setSelected('');}}><DialogContent className="desktop-ui-portal chat-desktop-surface chat-agent-dialog sm:max-w-3xl" data-theme={theme} showCloseButton={!sending}>
   <DialogTitle>{t('chat.agentMessages')} · {selected}</DialogTitle>
   {agent?.model&&<p className="text-sm text-muted-foreground break-all">{agent.model} · {t(paused?'chat.taskWaitingForResume':'chat.agentStatus_'+agent.status)}</p>}
   {error&&<p role="alert">{error}</p>}
   <Button className="self-start" variant="ghost" icon={RefreshCw} disabled={busy||paused||loading||sending} onClick={()=>void read()}>{t('chat.agentRefresh')}</Button>
   <div className="client-thread client-thread--chat min-h-32 max-h-[45vh] overflow-y-auto">
    {history?.cursor&&<Button disabled={loading||paused} onClick={()=>void read(history.cursor)}>{t('chat.loadOlder')}</Button>}
    {loading&&<p role="status">{t('chat.settingsLoading')}</p>}
    {history?.entries.flatMap(entry=>(entry.model??[]).map((message,index)=><ChatMessageView key={entry.id+':'+index} message={{...message,id:String(entry.id)}} conversationId={conversationId} onCopy={text=>void copyText(text).catch(error=>setError(String(error)))} onFork={()=>{}} canFork={false}/>))}
   </div>
   <label className="chat-dialog-field">{t('chat.agentMessage')}<textarea rows={3} maxLength={100000} value={drafts[selected]??''} disabled={busy||paused||sending} onChange={event=>setDrafts(current=>({...current,[selected]:event.target.value}))}/></label>
   <DialogFooter><Button disabled={sending} onClick={()=>setSelected('')}>{t('chat.close')}</Button><Button variant="primary" busy={sending} disabled={busy||paused||!drafts[selected]?.trim()} onClick={()=>void send()}>{t('chat.agentFollowUp')}</Button></DialogFooter>
  </DialogContent></Dialog>
 </>;
}
