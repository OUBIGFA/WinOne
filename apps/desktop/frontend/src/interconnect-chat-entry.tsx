import {useEffect,useRef,useState} from 'react';
import {createRoot} from 'react-dom/client';
import {CSPProvider} from '@base-ui/react/csp-provider';
import {Check,Copy,FilePlus,FolderOpen,Info,LogOut,MessageCircle,Send,Settings2,ShieldCheck,Users,X} from 'lucide-react';
import {MessagesCircle} from './components/icons/messages-circle';
import {i18n,useTranslation} from './i18n';
import './styles/shadcn.css';
import './style.css';
import './styles/settings.css';
import './features/interconnect.css';
import {Button} from './ui/button';
import {Notice} from './ui/notice';
import {Input} from './components/ui/input';
import {Textarea} from './components/ui/textarea';
import {Label} from './components/ui/label';
import {Dialog,DialogContent,DialogDescription,DialogHeader,DialogTitle} from './components/ui/dialog';

import {ChatMarkdown} from './features/chat-markdown';
import {ChatRoom,type ChatRoomOptions} from './vendor/chitchatter/core/chat/ChatRoom';
import {ChatRoomRegistry} from './vendor/chitchatter/core/chat/ChatRoomRegistry';
import {ChatRoomEvent} from './vendor/chitchatter/core/chat/events';
import {TrysteroTransport} from './vendor/chitchatter/core/transport/TrysteroTransport';
import {encryption} from './vendor/chitchatter/core/crypto/Encryption';
import {createPlatformAdapters} from './vendor/chitchatter/core/adapters/noop';
import {PeerVerificationState,isInlineMedia,isMessageReceived} from './vendor/chitchatter/core/models/chat';
import {getPeerDisplayName} from './vendor/chitchatter/core/names/peerNames';
import type {FileHandle} from './vendor/chitchatter/core/chat/fileOffers';
import type {FileTransferAdapter} from './vendor/chitchatter/core/adapters/types';
import {useChatNickname} from './features/interconnect-chat-preferences';
import type {PeerRecord} from './features/interconnect-peer';

const pending=new Map<string,{resolve:(value:any)=>void;reject:(error:Error)=>void;timer:number}>();
document.documentElement.dataset.settingsUi='true';
function native<T>(method:string,params?:unknown):Promise<T>{
 return new Promise((resolve,reject)=>{const id=crypto.randomUUID(),timer=window.setTimeout(()=>{pending.delete(id);reject(Error('WinOne 请求超时'));},30000);pending.set(id,{resolve,reject,timer});parent.postMessage({type:'interconnect-native',id,method,params},location.origin);});
}
window.addEventListener('message',event=>{
 if(event.source!==parent||event.origin!==location.origin)return;
 const data=event.data;
 if(data?.type==='interconnect-result'){const item=pending.get(data.id);if(!item)return;pending.delete(data.id);clearTimeout(item.timer);data.error?item.reject(Error(data.error)):item.resolve(data.result);}
 if(data?.type==='interconnect-theme'){void i18n.changeLanguage(data.language==='en-US'?'en-US':'zh-CN');document.documentElement.dataset.theme=data.theme==='dark'?'dark':'light';document.documentElement.classList.toggle('dark',data.theme==='dark');}
});

async function ready(id:string,cancelled:()=>boolean){
 for(;;){
  if(cancelled())throw Error('操作已取消');
  const record=(await native<PeerRecord[]>('list')).find(r=>r.id===id);
  if(!record)throw Error('共享任务不存在');
  if(record.state.status!=='running')throw Error(record.state.error||'共享已停止');
  if(record.state.relayError)throw new Error(record.state.relayError);
  if(record.state.ready&&(record.spec.mode!=='share_server'||record.state.relayReady))return record;
  await new Promise(resolve=>setTimeout(resolve,500));
 }
}

// Chitchatter deliberately injects file transfer. Opaque offer identifiers are
// official Tailcat addresses; all chat payloads and room behavior stay upstream.
function filesAdapter():FileTransferAdapter{
 const offers=new Map<string,string>(),starting=new Set<string>();let closed=false;
 const stop=async(id:string)=>{await native('stop',id);starting.delete(id);};
 return {
  offer:async(files:readonly FileHandle[])=>{
   if(closed)throw Error('房间已关闭');
   const paths=files.map(file=>(file as FileHandle&{path:string}).path);
   if(paths.some(path=>!path))throw Error('请选择本机文件');
   const {id}=await native<{id:string}>('share',paths);starting.add(id);
   try{const record=await ready(id,()=>closed);const code=record.state.code!;offers.set(code,id);starting.delete(id);return code;}catch(error){await stop(id);throw error;}
  },
  rescind:async(code:string)=>{const id=offers.get(code);if(id){await stop(id);offers.delete(code);}},
  rescindAll:async()=>{closed=true;await Promise.all([...new Set([...offers.values(),...starting])].map(stop));offers.clear();},
  isOffering:code=>offers.has(code),
 };
}
type Session={room:ChatRoom;transport:TrysteroTransport;registry:ChatRoomRegistry;options:Omit<ChatRoomOptions,'adapters'>;roomId:string;password:string;subscriptions:(()=>void)[]};

function PeerChat(){
 const {t,i18n:language}=useTranslation(),tr=(key:string)=>t('interconnect.'+key);
 const [roomId,setRoomId]=useState(''),[password,setPassword]=useState(''),[roomName,setRoomName]=useState('');
 const [tracker,setTracker]=useState(''),[rtc,setRTC]=useState(''),[drafts,setDrafts]=useState<Record<string,string>>({});
 const [session,setSession]=useState<Session>(),[selected,setSelected]=useState(''),[revision,setRevision]=useState(0),[busy,setBusy]=useState(false),[error,setError]=useState(''),[browser,setBrowser]=useState('');
 const {nickname,setNickname,ready:nicknameReady,save:saveNickname,load:loadNickname}=useChatNickname(native,setError);
 const [entryMode,setEntryMode]=useState('home'),[panel,setPanel]=useState<'network'|'room'|null>(null),[showPeers,setShowPeers]=useState(false),[copied,setCopied]=useState(false);
 const live=useRef<Session|undefined>(undefined),root=useRef<HTMLDivElement>(null),leaving=useRef(false),receivers=useRef(new Map<string,string>());
 const transcript=useRef<HTMLDivElement>(null),follow=useRef(true),composer=useRef<HTMLTextAreaElement>(null),copyTimer=useRef<number|undefined>(undefined);
 const room=session?(selected?session.registry.getDirect(selected):session.room):null;
 const draft=drafts[selected]??'';
 const dirty=!!session||Object.values(drafts).some(Boolean);
 useEffect(()=>{parent.postMessage({type:'interconnect-state',dirty,busy,acceptsFiles:!!session},location.origin);},[dirty,busy,session]);
 // Only follow incoming messages while the reader is already at the bottom.
 // Room switches start at the latest message without moving the page itself.
 useEffect(()=>{follow.current=true;const node=transcript.current;if(node)node.scrollTop=node.scrollHeight;},[selected,session]);
 useEffect(()=>{const node=transcript.current;if(node&&follow.current)node.scrollTop=node.scrollHeight;},[revision]);
 useEffect(()=>()=>window.clearTimeout(copyTimer.current),[]);
 useEffect(()=>{const receive=(event:MessageEvent)=>{if(event.source===parent&&event.origin===location.origin&&event.data?.type==='interconnect-files'&&room&&!busy)void action(()=>room.offerFiles(event.data.files));};window.addEventListener('message',receive);return()=>window.removeEventListener('message',receive);},[room,busy]);
 async function action(work:()=>Promise<unknown>){setBusy(true);setError('');try{await work();}catch(e){setError(String(e));}finally{setBusy(false);}}
 function subscribe(room:ChatRoom,subscriptions:(()=>void)[]){
  for(const event of Object.values(ChatRoomEvent)){subscriptions.push(room.on(event,()=>setRevision(r=>r+1)));}
  subscriptions.push(room.on(ChatRoomEvent.ERROR,({error})=>setError(error.message)));
 }
 async function leave(){
  leaving.current=true;const current=live.current;live.current=undefined;setSession(undefined);setSelected('');setBrowser('');setPanel(null);setShowPeers(false);setEntryMode('home');
  if(!current)return;
  current.subscriptions.forEach(f=>f());
  try{await Promise.all([current.room.leave(),current.registry.clearDirect(),...[...receivers.current.values()].map(id=>native('stop',id))]);}finally{current.transport.leaveRoom();current.registry.setGroup(null);receivers.current.clear();}
 }
 useEffect(()=>()=>{void leave().catch(console.error);},[]);
 async function join(create:boolean){
  const id=create?crypto.randomUUID():roomId.trim(),secret=create?crypto.randomUUID():password;
  if(!id||!secret)throw Error(tr('roomPassword'));
  const rtcConfig=rtc.trim()?JSON.parse(rtc):undefined;
  if(rtcConfig&&(typeof rtcConfig!=='object'||Array.isArray(rtcConfig)))throw Error(tr('rtcConfig'));
  const urls=tracker.split(String.fromCharCode(10)).map(s=>s.trim()).filter(Boolean);
  for(const text of urls){const url=new URL(text);if(url.protocol!=='wss:'&&!(url.protocol==='ws:'&&url.hostname==='127.0.0.1'))throw Error(tr('tracker'));}
  // Remember the personal name only; room names/passwords never enter defaults.
  await saveNickname();
  const identity=await encryption.generateKeyPair();leaving.current=false;
  const transport=new TrysteroTransport({appId:'winone-interconnect-chitchatter',password:secret,relayConfig:{urls:urls.length?urls:undefined,redundancy:4},rtcConfig},id);
  const options={roomId:id,userId:crypto.randomUUID(),customUsername:nickname.trim()||'WinOne',publicKey:identity.publicKey,privateKey:identity.privateKey,transport,password:secret,shareHistoryOnJoin:true};
  const registry=new ChatRoomRegistry(),subscriptions:(()=>void)[]=[];
  const group=new ChatRoom({...options,adapters:createPlatformAdapters({uuid:()=>crypto.randomUUID()},{fileTransfer:filesAdapter(),logger:{warn:message=>setError(message),error:message=>setError(message)}})});
  registry.setGroup(group);subscribe(group,subscriptions);
  const value={room:group,transport,registry,options,roomId:id,password:secret,subscriptions};live.current=value;
  // Each peer's direct room listens before its UI is opened, so incoming
  // private messages do not depend on the recipient first clicking “Private”.
  subscriptions.push(group.on(ChatRoomEvent.PEER_JOIN,({peerId})=>{void ensureDirect(value,peerId).catch(e=>setError(String(e)));}));
  try{await group.join();setSession(value);setRoomId(id);setPassword(secret);}catch(error){await leave();throw error;}
 }
 async function updateNickname(){
  await saveNickname();
  if(!session)return;
  const customUsername=nickname.trim()||'WinOne';
  session.options.customUsername=customUsername;
  await Promise.all([session.room,...session.transport.getPeers().map(id=>session.registry.getDirect(id)).filter((value):value is ChatRoom=>!!value)].map(value=>value.setCustomUsername(customUsername)));
  setRevision(value=>value+1);
 }
 async function ensureDirect(current:Session,peerId:string){
  const existing=current.registry.getDirect(peerId);
  if(existing){await existing.setCustomUsername(current.options.customUsername);return;}
  if(peerId){
   const direct=new ChatRoom({...current.options,targetPeerId:peerId,adapters:createPlatformAdapters({uuid:()=>crypto.randomUUID()},{fileTransfer:filesAdapter(),logger:{warn:message=>setError(message),error:message=>setError(message)}})});
   current.registry.setDirect(peerId,direct);subscribe(direct,current.subscriptions);await direct.join();
  }
 }
 async function selectPeer(peerId:string){
  if(!session)return;
  await ensureDirect(session,peerId);
  setSelected(peerId);setShowPeers(false);
 }
 async function receive(code:string){let id=receivers.current.get(code);if(!id){id=(await native<{id:string}>('connect',code)).id;receivers.current.set(code,id);}try{const record=await ready(id,()=>leaving.current);setBrowser('http://'+record.state.address);}catch(error){receivers.current.delete(code);await native('stop',id);throw error;}}
 async function send(){
  if(!room)return;const target=selected,content=draft,previous=new Set(room.getMessageLog().map(m=>m.id));
  await room.sendMessage(content);
  // Upstream reports send errors as events rather than rejected promises.
  // Clear only the exact draft whose new transcript entry was delivered.
  if(!room.getMessageLog().some(m=>!previous.has(m.id)&&m.authorId===session?.options.userId&&isMessageReceived(m)))throw Error(tr('sendNotDelivered'));
  setDrafts(current=>current[target]===content?{...current,[target]:''}:current);
  follow.current=true;const node=transcript.current;if(node)node.scrollTop=node.scrollHeight;composer.current?.focus();
 }
 async function copyRoom(){
  if(!session)return;await navigator.clipboard.writeText(session.roomId+'\n'+session.password);
  window.clearTimeout(copyTimer.current);setCopied(true);copyTimer.current=window.setTimeout(()=>setCopied(false),2000);
 }
 const peers=session?.room.getPeers()??[];
 const activePeer=peers.find(peer=>peer.peerId===selected),messages=room?.getMessageLog()??[];
 const groupName=roomName.trim()||tr('groupRoom');
 const conversation=selected?(activePeer?getPeerDisplayName(activePeer):tr('chatPrivate')):groupName;
 // Group messages are recorded immediately and backfilled to later members.
 const canSend=!busy&&!!draft.trim()&&!!room&&(!selected||!!room.getPeers().length);
 const typing=selected?activePeer?.isTypingDirectMessage:peers.some(peer=>peer.isTypingGroupMessage);
 return <div ref={root} className="settings-surface flash-chat-root">
  {!session?<div className="flash-chat-lobby"><div className="flash-chat-entry">
   <header className="flash-chat-welcome"><div className="flash-chat-brand"><MessagesCircle size={28} strokeWidth={1.5} aria-hidden="true"/><h1>{tr('peerChat')}</h1></div><p>{tr('chatWelcome')}</p></header>
   {error&&<Notice actions={!nicknameReady?<Button onClick={()=>void action(loadNickname)}>{tr('retry')}</Button>:undefined}>{error}</Notice>}
   <div className="flash-chat-entry-form">
    <Input id="chat-room-name" aria-label={tr('roomName')} placeholder={tr('roomName')} value={roomName} onChange={e=>setRoomName(e.target.value)} maxLength={80} autoComplete="off" disabled={busy}/>
    <Input id="chat-nickname" aria-label={tr('nickname')} placeholder={tr('nickname')} value={nickname} onChange={e=>setNickname(e.target.value)} onBlur={()=>{if(nicknameReady)void saveNickname().catch(e=>setError(String(e)));}} maxLength={80} autoComplete="nickname" disabled={busy||!nicknameReady}/>
   </div>
   {entryMode==='home'?<div className="flash-chat-entry-actions">
    <Button variant="primary" busy={busy} disabled={!nicknameReady} onClick={()=>void action(()=>join(true))}>{tr('createRoom')}</Button>
    <Button disabled={busy} onClick={()=>setEntryMode('join')}>{tr('joinRoom')}</Button>
   </div>:<form className="flash-chat-entry-form" onSubmit={e=>{e.preventDefault();if(!busy)void action(()=>join(false));}}>
    <Input id="chat-room-id" aria-label={tr('roomCode')} placeholder={tr('roomCode')} value={roomId} onChange={e=>setRoomId(e.target.value)} autoComplete="off" required autoFocus/>
    <Input id="chat-room-password" aria-label={tr('roomPassword')} placeholder={tr('roomPassword')} value={password} onChange={e=>setPassword(e.target.value)} autoComplete="off" required/>
    <Button type="submit" variant="primary" busy={busy} disabled={!nicknameReady||!roomId.trim()||!password}>{tr('joinRoom')}</Button>
    <Button variant="ghost" disabled={busy} onClick={()=>setEntryMode('home')}>{tr('backToEntry')}</Button>
   </form>}
   <footer className="flash-chat-entry-footer"><Button variant="ghost" icon={Settings2} onClick={()=>setPanel('network')}>{tr('networkOptions')}</Button><p><ShieldCheck size={15} aria-hidden="true"/>{tr('chatEncryption')}</p></footer>
  </div></div>:<div className="flash-chat-workspace" data-members-open={showPeers}>
   <aside className="flash-chat-sidebar" aria-label={tr('chatPeers')}>
    <header className="flash-chat-sidebar-heading"><MessagesCircle size={20} aria-hidden="true"/><span>{tr('peerChat')}</span><Button className="flash-chat-mobile-only" variant="ghost" size="icon" icon={X} aria-label={tr('backToChat')} onClick={()=>setShowPeers(false)}/></header>
    <nav className="flash-chat-conversations" aria-label={tr('chatConversations')}>
     <Button className="flash-chat-conversation" variant="ghost" aria-label={tr('groupRoom')} aria-pressed={!selected} disabled={busy} onClick={()=>void action(()=>selectPeer(''))}><Users size={19} aria-hidden="true"/><span className="flash-chat-peer-label"><span title={groupName}>{groupName}</span><small>{t('interconnect.memberCount',{count:peers.length+1})}</small></span></Button>
     <h2>{tr('chatPeers')}</h2>
     {!peers.length&&<p className="flash-chat-help flash-chat-waiting">{tr('noPeers')}</p>}
     {peers.map(peer=><Button key={peer.peerId} className="flash-chat-conversation" variant="ghost" aria-label={tr('chatPrivate')+' · '+getPeerDisplayName(peer)} aria-pressed={selected===peer.peerId} disabled={busy} onClick={()=>void action(()=>selectPeer(peer.peerId))}><span className="flash-chat-avatar" aria-hidden="true">{Array.from(getPeerDisplayName(peer))[0]}</span><span className="flash-chat-peer-label"><span title={getPeerDisplayName(peer)}>{getPeerDisplayName(peer)}</span><small>{tr(peer.verificationState===PeerVerificationState.VERIFIED?'verified':'unverified')}</small></span></Button>)}
    </nav>
    <footer className="flash-chat-self"><span className="flash-chat-avatar" aria-hidden="true">{Array.from(session.options.customUsername)[0]}</span><span title={session.options.customUsername}>{session.options.customUsername}<small>{tr('chatYou')}</small></span><Button variant="ghost" size="icon" icon={LogOut} disabled={busy} aria-label={tr('leaveRoom')} title={tr('leaveRoom')} onClick={()=>void action(leave)}/></footer>
   </aside>
   <main className="flash-chat-main">
    <header className="flash-chat-toolbar"><Button className="flash-chat-mobile-only" variant="ghost" size="icon" icon={Users} aria-label={tr('chatPeers')} onClick={()=>setShowPeers(true)}/><div className="flash-chat-conversation-title"><h1 title={conversation}>{conversation}</h1><p>{selected?tr('chatPrivate'):t('interconnect.memberCount',{count:peers.length+1})}</p></div><Button variant="ghost" size="icon" icon={Info} aria-label={tr('roomDetails')} title={tr('roomDetails')} onClick={()=>setPanel('room')}/></header>
    {error&&<div className="flash-chat-feedback"><Notice>{error}</Notice></div>}
    <div ref={transcript} className="flash-chat-transcript cindy-chat-ui" role="log" aria-label={tr('peerChat')} onScroll={()=>{const node=transcript.current;if(node)follow.current=node.scrollHeight-node.scrollTop-node.clientHeight<48;}}>
     {!messages.length?<div className="flash-chat-empty"><MessageCircle size={30} strokeWidth={1.25} aria-hidden="true"/><h2>{tr(peers.length?'chatStart':'noPeers')}</h2><p>{tr(peers.length?'chatStartHint':'chatInviteHint')}</p>{!peers.length&&<Button icon={copied?Check:Copy} disabled={busy} onClick={()=>void action(copyRoom)}>{tr(copied?'copied':'roomCopy')}</Button>}</div>:messages.map(message=>{const own=message.authorId===session.options.userId;const author=own?session.options.customUsername:peers.find(peer=>peer.userId===message.authorId);const name=typeof author==='string'?author:author?getPeerDisplayName(author):message.authorId;return <article key={message.id} className="flash-chat-message" data-own={own}><div className="flash-chat-message-meta"><span>{name}</span><time dateTime={new Date(message.timeSent).toISOString()}>{new Date(message.timeSent).toLocaleTimeString(language.language,{hour:'2-digit',minute:'2-digit'})}</time></div><div className="flash-chat-message-body">{isInlineMedia(message)?<Button icon={FolderOpen} disabled={busy} onClick={()=>void action(()=>receive(message.magnetURI))}>{tr('fileOffer')}</Button>:<ChatMarkdown text={message.text}/>}</div></article>;})}
    </div>
    <div className="flash-chat-shares">{Object.entries(room?.getFileOffers()??{}).map(([peerId,offer])=><div key={peerId}><FolderOpen size={16} aria-hidden="true"/><span>{tr('fileOffer')}</span><Button disabled={busy} onClick={()=>void action(()=>receive(offer.magnetURI))}>{tr('browseShare')}</Button></div>)}</div>
    <form className="flash-chat-composer" onSubmit={e=>{e.preventDefault();if(canSend)void action(send);}}>
     <p className="flash-chat-typing" aria-live="polite">{typing?tr('typing'):!selected&&!peers.length?tr('chatSoloHint'):'\u00a0'}</p>
     <div className="flash-chat-input"><Textarea ref={composer} aria-label={tr('chatDraft')} placeholder={tr('chatDraft')} value={draft} onChange={e=>{setDrafts(current=>({...current,[selected]:e.target.value}));room?.notifyTyping();}} onKeyDown={e=>{if(e.key==='Enter'&&!e.shiftKey&&!e.nativeEvent.isComposing&&e.keyCode!==229){e.preventDefault();if(canSend)void action(send);}}}/><div className="flash-chat-compose-actions"><Button variant="ghost" size="icon" icon={FilePlus} aria-label={tr('shareInChat')} title={tr('shareInChat')} disabled={busy} onClick={()=>void action(async()=>{const files=await native<FileHandle[]>('files');if(files.length)await room!.offerFiles(files);})}/>{room?.getSelfFileOfferMagnetUri()&&<Button variant="ghost" disabled={busy} onClick={()=>void action(()=>room.rescindFileOffer())}>{tr('stopChatShare')}</Button>}<span className="flash-chat-shortcut">{tr('chatKeyboardHint')}</span><Button type="submit" icon={Send} variant="primary" disabled={!canSend} aria-label={tr('sendMessage')}>{tr('sendMessage')}</Button></div></div>
    </form>
   </main>
  </div>}
  <Dialog open={panel!==null} onOpenChange={open=>{if(!open)setPanel(null);}}><DialogContent className="flash-chat-dialog"><DialogHeader><DialogTitle>{tr(panel==='network'?'networkOptions':'roomDetails')}</DialogTitle><DialogDescription>{tr(panel==='network'?'chatNetworkHint':'roomDetailsHint')}</DialogDescription></DialogHeader>{panel==='network'?<>
   <div className="flash-chat-field"><Label htmlFor="chat-tracker">{tr('tracker')}</Label><Textarea id="chat-tracker" value={tracker} onChange={e=>setTracker(e.target.value)}/></div><div className="flash-chat-field"><Label htmlFor="chat-rtc">{tr('rtcConfig')}</Label><Textarea id="chat-rtc" value={rtc} onChange={e=>setRTC(e.target.value)}/></div><Button onClick={()=>setPanel(null)}>{t('common.close')}</Button>
  </>:session&&<><div className="flash-chat-entry-form"><Input aria-label={tr('roomName')} placeholder={tr('roomName')} value={roomName} maxLength={80} onChange={e=>setRoomName(e.target.value)}/><Input aria-label={tr('nickname')} placeholder={tr('nickname')} value={nickname} maxLength={80} disabled={busy} onChange={e=>setNickname(e.target.value)} onBlur={()=>{if(nicknameReady)void updateNickname().catch(e=>setError(String(e)));}}/></div><div className="flash-chat-field"><Label htmlFor="chat-active-room">{tr('roomCode')}</Label><Input id="chat-active-room" readOnly value={session.roomId}/></div><div className="flash-chat-field"><Label htmlFor="chat-active-password">{tr('roomPassword')}</Label><Input id="chat-active-password" readOnly value={session.password}/></div><Button icon={copied?Check:Copy} disabled={busy} onClick={()=>void action(copyRoom)}>{tr(copied?'copied':'roomCopy')}</Button><p className="flash-chat-help">{tr('chatHistoryHint')}</p><p className="flash-chat-help">{tr('chatHint')}</p><Button icon={LogOut} disabled={busy} onClick={()=>void action(leave)}>{tr('leaveRoom')}</Button></>}</DialogContent></Dialog>
  <Dialog open={!!browser} onOpenChange={open=>{if(!open)setBrowser('');}}><DialogContent className="flash-chat-file-dialog"><DialogHeader><DialogTitle>{tr('browseShare')}</DialogTitle></DialogHeader>{browser&&<iframe title={tr('browseShare')} src={browser} sandbox="allow-scripts allow-same-origin allow-forms allow-downloads"/>}</DialogContent></Dialog>
 </div>;
}
createRoot(document.getElementById('root')!).render(<CSPProvider disableStyleElements><PeerChat/></CSPProvider>);
