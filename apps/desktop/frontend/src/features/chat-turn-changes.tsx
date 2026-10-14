// TurnChangesCard / TurnChangeFileRow adapted from Cindy 69661422 (Apache-2.0).
// The original card, file rows, totals and expand controls use WinOne's Host
// snapshot instead of maker IPC. Each card is bound to its original Pi turn.
import {useEffect,useMemo,useState} from 'react';
import {ChevronDown,ChevronUp,FileDiff,FileText,Undo2,Redo2} from 'lucide-react';
import {Button} from '../ui/button';
import {chatCall} from '../bridge';
import {useTranslation} from '../i18n';
import {parseGitDiff} from './cindy/review/diffParser';
import {DropdownMenu,DropdownMenuContent,DropdownMenuItem,DropdownMenuTrigger} from '../components/ui/dropdown-menu';
import {copyText} from './clipboard';

type TurnSnapshot={isReversible?:boolean;workspaceState?:string;root:string;revision:string;files:{path:string;status:string}[]};
export type TurnReview={snapshot:TurnSnapshot;patches:Record<string,string>};
export function ChatTurnChanges({id,turnId,version,disabled=false,onReview,onOpen}:{id:string;turnId:string;version:string;disabled?:boolean;onReview:(path?:string)=>void;onOpen?:(path:string)=>void}){
 const {t}=useTranslation(),[data,setData]=useState<TurnReview>(),[expanded,setExpanded]=useState(false),[error,setError]=useState(''),[applying,setApplying]=useState(false);
 useEffect(()=>{let alive=true;setError('');void chatCall<TurnReview>('review.turn',{sessionId:id,turnId}).then(value=>{if(alive)setData(value);}).catch(error=>{if(alive)setError(String(error));});return()=>{alive=false;};},[id,turnId,version]);
 const files=useMemo(()=>(data?.snapshot?.files??[]).map(file=>({...file,diff:parseGitDiff(data?.patches?.[file.path]??'',{source:'unstaged',pathHint:file.path,isUntracked:file.status==='A'})})),[data]);
 async function apply(){if(!data||applying)return;setApplying(true);setError('');try{const snapshot=await chatCall<TurnSnapshot>('review.turn.apply',{sessionId:id,turnId,mutation:{revision:data.snapshot.revision,action:data.snapshot.workspaceState==='undone'?'reapply':'undo'}});setData(current=>current?{...current,snapshot}:current);window.dispatchEvent(new CustomEvent('winone:chat-files-changed',{detail:{id,turnId}}));}catch(error){setError(String(error));}finally{setApplying(false);}}
 const additions=files.reduce((sum,file)=>sum+file.diff.additions,0),deletions=files.reduce((sum,file)=>sum+file.diff.deletions,0);
 const absolute=(path:string)=>/^[A-Za-z]:/.test(path)||path.startsWith('/')||path.startsWith('\\')?path:(data?.snapshot.root??'').replace(/[\/]$/,'')+'/'+path;
 const action=async(method:string,path:string)=>{try{if(method==='copy')await copyText(absolute(path));else await chatCall(method,{id,path:absolute(path)});}catch(error){setError(String(error));}};
 if(!data&&!error)return null;
 if(data&&!files.length&&!error)return null;
 return <section className="cindy-chat-ui chat-turn-changes my-1 overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--background)]">
  <div className="flex min-h-16 items-center gap-3 px-4 py-3">
   <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[var(--muted)] text-muted-foreground"><FileDiff size={18}/></div>
   <div className="min-w-0 flex-1"><div className="text-sm font-medium">{data?t('chat.turnFilesChanged',{count:files.length}):t('chat.turnReview')}</div>{files.length>0&&<div className="mt-0.5 font-mono text-xs tabular-nums"><span className="text-[var(--diff-add-fg)]">+{additions}</span>{' '}<span className="text-[var(--diff-del-fg)]">−{deletions}</span></div>}</div>
   {data?.snapshot.isReversible&&<Button size="sm" icon={data.snapshot.workspaceState==='undone'?Redo2:Undo2} busy={applying} disabled={disabled||applying||data.snapshot.workspaceState==='conflict'} onClick={()=>void apply()}>{t(data.snapshot.workspaceState==='undone'?'chat.turnReapply':'chat.turnUndo')}</Button>}
   <Button size="sm" onClick={()=>onReview()}>{t('chat.reviewViewChanges')}</Button>
  </div>
  {files.length>0&&<div className="border-t border-[var(--border)] px-3 py-2">{(expanded?files:files.slice(0,3)).map(file=><TurnFileRow key={file.path} path={file.path} additions={file.diff.additions} deletions={file.diff.deletions} onReview={()=>onReview(file.path)} onOpen={()=>onOpen?.(absolute(file.path))} action={method=>void action(method,file.path)}/>)}
   {files.length>3&&<button type="button" aria-expanded={expanded} onClick={()=>setExpanded(!expanded)} className="flex h-8 items-center gap-1 rounded-lg px-2 text-sm text-muted-foreground hover:bg-[var(--muted)]">{expanded?t('chat.generatedFilesLess'):t('chat.generatedFilesMore',{count:files.length-3})}{expanded?<ChevronUp size={14}/>:<ChevronDown size={14}/>}</button>}
  </div>}
  {data?.snapshot.workspaceState==='conflict'&&<p role="status" className="px-4 py-2 text-sm">{t('chat.turnConflict')}</p>}
  {data&&!data.snapshot.isReversible&&<p role="status" className="px-4 py-2 text-sm text-muted-foreground">{t('chat.turnCapturePartial')}</p>}
  {error&&<p role="alert" className="px-4 py-2 text-sm">{error}</p>}
 </section>;
}
function TurnFileRow({path,additions,deletions,onReview,onOpen,action}:{path:string;additions:number;deletions:number;onReview:()=>void;onOpen:()=>void;action:(method:string)=>void}){
 const {t}=useTranslation(),[menu,setMenu]=useState(false);
 return <DropdownMenu open={menu} onOpenChange={setMenu}><DropdownMenuTrigger render={<button type="button" title={path} className="flex h-9 w-full items-center gap-2 rounded-lg px-2 text-left transition-colors hover:bg-[var(--muted)]" onClick={event=>{event.preventDefault();onReview();}} onContextMenu={event=>{event.preventDefault();setMenu(true);}}/>}><FileText size={15} className="shrink-0 text-muted-foreground"/><span className="min-w-0 flex-1 truncate text-sm">{path}</span><span className="shrink-0 whitespace-nowrap font-mono text-xs tabular-nums"><span className="text-[var(--diff-add-fg)]">+{additions}</span>{' '}<span className="text-[var(--diff-del-fg)]">−{deletions}</span></span></DropdownMenuTrigger><DropdownMenuContent className="chat-row-menu"><DropdownMenuItem onClick={onReview}>{t('chat.reviewViewChanges')}</DropdownMenuItem><DropdownMenuItem onClick={onOpen}>{t('chat.reviewOpenFile')}</DropdownMenuItem><DropdownMenuItem onClick={()=>action('copy')}>{t('chat.copyPath')}</DropdownMenuItem><DropdownMenuItem onClick={()=>action('file.reveal')}>{t('chat.revealFile')}</DropdownMenuItem><DropdownMenuItem onClick={()=>action('file.open')}>{t('chat.openSystem')}</DropdownMenuItem></DropdownMenuContent></DropdownMenu>;
}
