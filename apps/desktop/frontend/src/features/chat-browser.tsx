// Cindy browser chrome, selection overlay and comment editor use WinOne's
// isolated native WebView transport. A page never receives the settings bridge.
import {useEffect,useRef,useState} from 'react';
import {chatBrowserCall} from '../bridge';
import {useTranslation} from '../i18n';
import {UpstreamBrowserBar,type UpstreamBrowserBarHandle} from './chat-upstream-pane';
import type {DesktopMenuItem} from 'chatgpt-desktop-kit';
import {BrowserCommentPopover} from './cindy/browser/BrowserCommentPopover';
import {createEmptyBrowserCommentEditorDraft} from './cindy/browser/browserCommentEditorDraft';
import type {BrowserCommentTargetInfo,BrowserCommentStyleChange} from './cindy/browser/browserComment';
export type {BrowserCommentTargetInfo,BrowserCommentStyleChange};
export type ChatBrowserState={sessionId:string;tabId:string;url:string;title:string;loading:boolean;canGoBack:boolean;canGoForward:boolean;error:string;visible:boolean;favicon:string;isAudible:boolean;muted:boolean;crashed:boolean};
export type ChatBrowserComment={id:string;markerNumber:number;pageUrl:string;target:BrowserCommentTargetInfo;comment:string;screenshotPath:string;styleChanges?:BrowserCommentStyleChange[]};
export type ChatBrowserViewProps={sessionId:string;tabId:string;url:string;active?:boolean;onStateChange?:(state:ChatBrowserState)=>void;onOpenTab?:(url:string)=>void;onClose?:()=>void;onComment?:(item:ChatBrowserComment)=>Promise<void>;commentMarkerNumbers?:number[];reloadToken?:number;paneItems?:DesktopMenuItem[]};
export function ChatBrowserView(props:ChatBrowserViewProps){
 const {sessionId,tabId,url,active=true}=props,{t}=useTranslation();
 const [state,setState]=useState<ChatBrowserState>({sessionId,tabId,url,title:'',loading:false,canGoBack:false,canGoForward:false,error:'',visible:false,favicon:'',isAudible:false,muted:false,crashed:false});
 const [ready,setReady]=useState(false),[error,setError]=useState(''),[notice,setNotice]=useState(''),[mode,setMode]=useState(false),[pending,setPending]=useState<BrowserCommentTargetInfo>(),[submitting,setSubmitting]=useState(false),[draft,setDraft]=useState(createEmptyBrowserCommentEditorDraft),[overlay,setOverlay]=useState(false);
 const slot=useRef<HTMLDivElement>(null),editor=useRef<HTMLDivElement>(null),chrome=useRef<UpstreamBrowserBarHandle>(null),latest=useRef({props,state,mode,pending});latest.current={props,state,mode,pending};
 const waiters=useRef(new Map<string,{resolve:(value:any)=>void;reject:(error:Error)=>void;timer:ReturnType<typeof setTimeout>}>());
 const native=<T,>(method:string,value:Record<string,unknown>={})=>chatBrowserCall<T>(method,{sessionId,tabId,...value});
 const report=(value:unknown)=>setError(value instanceof Error?value.message:String(value));
 const run=(action:()=>Promise<unknown>)=>{setError('');void action().catch(report);};
 function accept(next:ChatBrowserState){setState(next);latest.current.props.onStateChange?.(next);}
 function requestResult(method:'capture'|'comment',value:Record<string,unknown>):Promise<any>{
  const token=crypto.randomUUID();return new Promise((resolve,reject)=>{
   const timer=setTimeout(()=>{waiters.current.delete(token);reject(Error(t('chat.browserTimeout')));},10000);
   waiters.current.set(token,{resolve,reject,timer});
   const payload=method==='capture'?{...value,operationId:token}:{...value,payload:{requestId:token,payload:value.payload}};
   void native(method,payload).catch(error=>{clearTimeout(timer);waiters.current.delete(token);reject(error);});
  });
 }
 const command=(name:string,payload?:unknown)=>requestResult('comment',{channel:'browser-comment:'+name,payload});
 function nextMarker(){return Math.max(0,...(latest.current.props.commentMarkerNumbers??[]))+1;}
 async function submit(comment:string,styleChanges?:BrowserCommentStyleChange[],target=latest.current.pending){
  if(!target||!latest.current.props.onComment)return;setSubmitting(true);setError('');
  try{
   await command('prepare-screenshot',{validMarkerNumbers:latest.current.props.commentMarkerNumbers??[]});
   const shot=await requestResult('capture',{save:true});
   await latest.current.props.onComment({id:crypto.randomUUID(),markerNumber:target.markerNumber,pageUrl:latest.current.state.url,target,comment,screenshotPath:shot.screenshotPath,styleChanges});
   await command('commit-pending',{nextMarkerNumber:Math.max(nextMarker(),target.markerNumber+1)});
   setPending(undefined);setDraft(createEmptyBrowserCommentEditorDraft());setNotice(t('chat.browserCommentAdded'));
  }catch(error){report(error);}finally{setSubmitting(false);}
 }
 useEffect(()=>{
  let alive=true;setReady(false);
  const receive=(event:Event)=>{const detail=(event as CustomEvent).detail;if(detail?.sessionId!==sessionId&&detail?.state?.sessionId!==sessionId)return;if(detail?.tabId!==tabId&&detail?.state?.tabId!==tabId)return;
   if(detail.kind==='state'){accept(detail.state);return;}
   if(detail.kind==='newTab'){latest.current.props.onOpenTab?.(detail.value);return;}
   if(detail.kind==='command'){const key=detail.value;if(key==='focusAddress')chrome.current?.focusUrlBar();else if(key==='close')latest.current.props.onClose?.();else run(async()=>accept(await native(key)));return;}
   const message=detail.value;
   if(detail.kind==='operation'||(detail.kind==='comment'&&message.channel==='browser-comment:command-result')){
    const data=detail.kind==='operation'?message:message.payload,key=data.operationId??data.requestId,waiter=waiters.current.get(key);if(!waiter)return;
    clearTimeout(waiter.timer);waiters.current.delete(key);if(data.error||data.ok===false)waiter.reject(Error(data.error||t('chat.browserCommentFailed')));else waiter.resolve(data.result??data);return;
   }
   if(detail.kind==='comment'&&message.channel==='browser-comment:mode-exited'){setMode(false);setPending(undefined);}
   if(detail.kind==='comment'&&message.channel==='browser-comment:element-selected'&&latest.current.mode){
    const target=message.payload as BrowserCommentTargetInfo;
    // Treat page metadata as data. Reject malformed geometry before it reaches
    // the host editor; formatting later labels all page evidence as untrusted.
    if(!target||!['element','region','text'].includes(target.kind)||!Number.isFinite(target.point?.x)||!Number.isFinite(target.point?.y)||!Number.isSafeInteger(target.markerNumber))return;
    latest.current.pending=target;setPending(target);if(target.immediate)void submit('',undefined,target);
   }
  };
  window.addEventListener('winone:chat-browser',receive);
  void Promise.resolve().then(async()=>{if(!alive)return;const value=await native<ChatBrowserState>('create',{url});if(alive){accept(value);setReady(true);}else await native('close');}).catch(error=>{if(alive)report(error);});
  return()=>{alive=false;window.removeEventListener('winone:chat-browser',receive);for(const waiter of waiters.current.values()){clearTimeout(waiter.timer);waiter.reject(Error('Browser tab closed'));}waiters.current.clear();void native('close').catch(console.warn);};
 },[sessionId,tabId]);
 useEffect(()=>{
  if(!ready)return;let alive=true,frame=0,last='';
  function sync(){cancelAnimationFrame(frame);frame=requestAnimationFrame(()=>{if(!alive||!slot.current)return;const rect=slot.current.getBoundingClientRect(),occlusion=editor.current?.getBoundingClientRect();const modal=!!document.querySelector('[role="dialog"],[role="menu"]');const visible=active&&!overlay&&!modal&&rect.width>0&&rect.height>0;const value={visible,bounds:{x:rect.x,y:rect.y,width:rect.width,height:rect.height},scale:window.devicePixelRatio||1,occlusion:occlusion?{x:occlusion.x-rect.x,y:occlusion.y-rect.y,width:occlusion.width,height:occlusion.height}:null};const encoded=JSON.stringify(value);if(last===encoded)return;last=encoded;void native('sync',value).catch(error=>{if(alive)report(error);});});}
  const resize=new ResizeObserver(sync);if(slot.current)resize.observe(slot.current);if(editor.current)resize.observe(editor.current);
  const mutations=new MutationObserver(sync);mutations.observe(document.body,{childList:true,subtree:true});window.addEventListener('resize',sync);window.addEventListener('scroll',sync,true);sync();
  return()=>{alive=false;cancelAnimationFrame(frame);resize.disconnect();mutations.disconnect();window.removeEventListener('resize',sync);window.removeEventListener('scroll',sync,true);void native('sync',{visible:false}).catch(console.warn);};
 },[ready,active,overlay,!!pending]);
 useEffect(()=>{if(props.reloadToken&&ready)run(async()=>accept(await native('reload')));},[props.reloadToken]);
 async function toggleComment(){
  const entering=!mode;
  // Entering with a pre-existing text selection emits the target before the
  // command acknowledgement. Admit that target during the same event turn.
  latest.current.mode=entering;setMode(entering);
  try{await command(entering?'enter-mode':'exit-mode',entering?{markerNumber:nextMarker()}:undefined);if(!entering)setPending(undefined);}
  catch(error){latest.current.mode=!entering;setMode(!entering);throw error;}
 }
 return <section className="chat-browser" onKeyDown={event=>{if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='l'){event.preventDefault();chrome.current?.focusUrlBar();}}}>
  <UpstreamBrowserBar ref={chrome} url={state.url} loading={state.loading} canBack={state.canGoBack} canForward={state.canGoForward} onNavigate={url=>run(async()=>accept(await native('navigate',{url})))} onReload={()=>run(async()=>accept(await native('reload')))} onStop={()=>run(async()=>accept(await native('stop')))} onBack={()=>run(async()=>accept(await native('back')))} onForward={()=>run(async()=>accept(await native('forward')))} onComment={()=>run(toggleComment)} commentActive={mode} onMenuOpen={setOverlay} items={[
   {id:'browser.capture',label:t('chat.browserScreenshot'),onSelect:()=>run(async()=>{await requestResult('capture',{save:false});setNotice(t('chat.browserScreenshotCopied'));})},
   {id:'browser.copy',label:t('chat.copy'),onSelect:()=>run(async()=>{await navigator.clipboard.writeText(state.url);setNotice(t('chat.copied'));})},
   {id:'browser.external',label:t('chat.openSystem'),disabled:!/^https?:/.test(state.url),onSelect:()=>run(()=>native('external',{url:state.url}))},
   {id:'browser.mute',label:t(state.muted?'chat.browserUnmute':'chat.browserMute'),checked:state.muted,onSelect:()=>run(async()=>accept(await native('mute',{muted:!state.muted})))},
   {id:'browser.release',label:t(state.crashed?'chat.browserRestore':'chat.browserRelease'),onSelect:()=>run(async()=>accept(await native(state.crashed?'reload':'terminate')))},
   ...(props.paneItems??[]),
  ]}/>
  {(error||state.error)&&<p role="alert" className="chat-preview-feedback">{error||state.error}</p>}
  {notice&&<p role="status" className="chat-browser-notice">{notice}</p>}
  <div className="chat-browser-slot" ref={slot}>{pending&&<div className="chat-browser-editor" ref={editor}><BrowserCommentPopover anchor={{x:0,y:0}} submitting={submitting} designBaseline={pending.designBaseline??null} editorDraft={draft} onEditorDraftChange={setDraft} onSubmit={(text,styles)=>void submit(text,styles)} onCancel={()=>run(async()=>{await command('cancel-pending');setPending(undefined);})} onPreviewDesign={payload=>run(()=>native('comment',{channel:'browser-comment:design-preview',payload}))} onResetDesign={()=>run(()=>native('comment',{channel:'browser-comment:design-reset'}))}/></div>}</div>
 </section>;
}
