import {useEffect,useMemo,useRef,useState} from 'react';
import {MessageSquarePlus} from 'lucide-react';
import {chatCall} from '../../../bridge';
import {useTranslation} from '../../../i18n';
import {Button} from '../panel-ui/button';
import {Spinner} from '../panel-ui/spinner';
import {ConfirmDialogProvider} from '../panel-ui/confirm-dialog-provider';
import '../panel-ui/locales';
import '../panel-ui/theme.css';
import '../panel-ui/foundation.css';
import {ReviewTabBody} from './ReviewTabBody';
import {createReviewHost,ReviewHostProvider,ReviewFeedback,setReviewActivity,type Snapshot} from './host';
import {defaultReviewState,type ReviewState,type TabKindHostContext} from './state';
import {ChatFileNavigation} from '../../chat-file-navigation';

export function ReviewPanel({sessionId,turnId,initialPath,running=false,lastTurnPaths=[],onOpenFile,onComment}:{sessionId:string;turnId?:string;initialPath?:string;running?:boolean;lastTurnPaths?:string[];onOpenFile:(path:string,line?:number)=>void;onComment?:(text:string)=>void}){
 const {t}=useTranslation(),[snapshot,setSnapshot]=useState<Snapshot>(),[error,setError]=useState('');
 const current=useRef({onOpenFile,onComment});current.current={onOpenFile,onComment};
 const [state,setState]=useState<ReviewState>(()=>{
  try{const saved=JSON.parse(localStorage.getItem('winone.review.'+sessionId)||'null');if(saved)return {...defaultReviewState,...saved,jumpTarget:initialPath?{path:initialPath,diffId:null,nonce:Date.now()}:null};}catch{/* A damaged display preference does not affect any file or Git state. */}
  return {...defaultReviewState,jumpTarget:initialPath?{path:initialPath,diffId:null,nonce:Date.now()}:null};
 });
 useEffect(()=>{let alive=true;setSnapshot(undefined);setError('');void chatCall<Snapshot>('review.snapshot',{sessionId,turnId,scope:'working',base:''}).then(value=>{if(!alive)return;setSnapshot(value);if(turnId||value.source==='session'){const descriptor={kind:'turn-set' as const,targetSessionId:sessionId,changeSetIds:[turnId||'recorded-session']};setState(state=>({...state,descriptor,messageSnapshot:descriptor}));}}).catch(error=>{if(alive)setError(String(error));});return()=>{alive=false;};},[sessionId,turnId]);
 useEffect(()=>{if(!turnId)try{localStorage.setItem('winone.review.'+sessionId,JSON.stringify(state));}catch{/* Display preferences remain valid for this mounted tab. */}},[sessionId,turnId,state]);
 useEffect(()=>{setReviewActivity(sessionId,running,lastTurnPaths);},[sessionId,running,JSON.stringify(lastTurnPaths)]);
 useEffect(()=>{if(initialPath)setState(state=>({...state,jumpTarget:{path:initialPath,diffId:null,nonce:Date.now()}}));},[initialPath]);
 const host=useMemo(()=>createReviewHost(sessionId,{turnId,initial:snapshot,onOpenFile:(path,line)=>current.current.onOpenFile(path,line)}),[sessionId,turnId,snapshot]);
 const ctx=useMemo<TabKindHostContext>(()=>({tabId:'review:'+sessionId+(turnId||''),sessionId,workdir:snapshot?.root??'',remoteHostId:null,deviceLinkDeviceId:null,patchState:patch=>setState(state=>({...state,...patch})),onVisibilityChange:()=>{},setCloseInterceptor:()=>()=>{}}),[sessionId,turnId,snapshot?.root]);
 return <ChatFileNavigation id={sessionId} cwd={snapshot?.root??''}><div className="cindy-chat-ui flex h-full min-h-0 min-w-0 flex-col overflow-hidden text-13" data-cindy-review>
  {error?<div role="alert" className="p-4 text-[var(--text-secondary)]">{error}</div>:!snapshot?<div role="status" className="flex flex-1 items-center justify-center gap-2 text-[var(--text-tertiary)]"><Spinner size={16}/>{t('chat.loadingResources')}</div>:<ReviewHostProvider host={host}><ConfirmDialogProvider>
   {snapshot.source==='session'&&<p className="shrink-0 px-3 py-2 text-12 text-[var(--text-tertiary)]">{t(turnId?'chat.turnReview':'chat.reviewSessionRecords')}</p>}
   <div className="min-h-0 min-w-0 flex-1"><ReviewTabBody state={state} ctx={ctx}/></div>
   {onComment&&<div className="flex shrink-0 justify-end border-t border-[var(--border-default)] px-2 py-1"><Button variant="secondary" compact size="xs" onClick={()=>current.current.onComment?.(`Review ${state.jumpTarget?.path??initialPath??''}:\n${window.getSelection()?.toString()||''}`)}><MessageSquarePlus size={13}/>{t('chat.reviewComment')}</Button></div>}
   <ReviewFeedback/>
  </ConfirmDialogProvider></ReviewHostProvider>}
 </div></ChatFileNavigation>;
}
