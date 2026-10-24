// TurnChangesCard / TurnChangeFileRow adapted from Cindy 69661422 (Apache-2.0).
// The original card, file rows, totals and expand controls use WinOne's Host
// snapshot instead of maker IPC. Cards stay bound to their turns at the transcript end.
import {useEffect,useMemo,useRef,useState} from 'react';
import {ChevronDown,ChevronUp,FileDiff,FileText,Undo2,Redo2} from 'lucide-react';
import {Button} from '../ui/button';
import {chatCall} from '../bridge';
import {useTranslation} from '../i18n';
import {parseGitDiff} from './cindy/review/diffParser';
import {DropdownMenu,DropdownMenuContent,DropdownMenuItem,DropdownMenuTrigger,DropdownMenuRadioGroup,DropdownMenuRadioItem} from './cindy/panel-ui/dropdown-menu';
import {activateInlineLinkFromKeyboard,linkMenuPosition} from './cindy/linkInteraction';
import {copyText} from './clipboard';

type TurnSnapshot={isReversible?:boolean;workspaceState?:string;root:string;revision:string;files:{path:string;status:string}[]};
export type TurnReview={snapshot:TurnSnapshot;patches:Record<string,string>};

/** One transcript surface, with every recorded turn still reachable. Selecting
 * history never rewrites or merges snapshots, so undo stays scoped to its turn. */
export function ChatTranscriptChanges({id,turns,disabled,onReview,onOpen}:{id:string;turns:{turnId:string;title:string;version:string}[];disabled:boolean;onReview:(turnId:string,path?:string)=>void;onOpen:(path:string)=>void}){
 const {t}=useTranslation(),[selected,setSelected]=useState<string>();
 const turn=turns.find(turn=>turn.turnId===selected)??turns.at(-1);
 if(!turn)return null;
 return <div className="chat-transcript-changes">
  {turns.length>1&&<div className="chat-change-turn-select"><span>{t('chat.changeRecords')}</span><DropdownMenu><DropdownMenuTrigger asChild><button type="button" aria-label={t('chat.changeRecords')} title={turn.title}><span>{t('chat.changeRecord',{number:turns.indexOf(turn)+1})}{turn.title?` · ${turn.title}`:''}</span><ChevronDown size={14}/></button></DropdownMenuTrigger><DropdownMenuContent className="chat-change-history-menu" align="end" collisionPadding={12}><DropdownMenuRadioGroup value={turn.turnId} onValueChange={setSelected}>{turns.map((value,index)=><DropdownMenuRadioItem key={value.turnId} value={value.turnId} textValue={value.title} title={value.title}><span>{t('chat.changeRecord',{number:index+1})}{value.title?` · ${value.title}`:''}</span></DropdownMenuRadioItem>)}</DropdownMenuRadioGroup></DropdownMenuContent></DropdownMenu></div>}
  <ChatTurnChanges key={id+':'+turn.turnId} id={id} turnId={turn.turnId} version={turn.version} disabled={disabled} onReview={path=>onReview(turn.turnId,path)} onOpen={onOpen}/>
 </div>;
}

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
 return <section className="cindy-chat-ui chat-turn-changes overflow-hidden rounded-lg border border-[var(--border)] bg-[var(--background)]">
  <div className="chat-turn-changes-header">
   <div className="shrink-0 text-muted-foreground"><FileDiff size={18}/></div>
   <div className="chat-turn-changes-totals"><div className="text-sm font-normal">{data?t('chat.turnFilesChanged',{count:files.length}):t('chat.turnReview')}</div>{files.length>0&&<div className="font-mono text-xs tabular-nums whitespace-nowrap"><span className="text-[var(--diff-add-fg)]">+{additions}</span>{' '}<span className="text-[var(--diff-del-fg)]">−{deletions}</span></div>}</div>
   {data?.snapshot.isReversible&&<Button size="sm" icon={data.snapshot.workspaceState==='undone'?Redo2:Undo2} busy={applying} disabled={disabled||applying||data.snapshot.workspaceState==='conflict'} onClick={()=>void apply()}>{t(data.snapshot.workspaceState==='undone'?'chat.turnReapply':'chat.turnUndo')}</Button>}
   <Button size="sm" onClick={()=>onReview()}>{t('chat.reviewViewChanges')}</Button>
  </div>
  {files.length>0&&<div className="border-t border-[var(--border)] px-1 py-1">{(expanded?files:files.slice(0,3)).map(file=><TurnFileRow key={file.path} path={file.path} additions={file.diff.additions} deletions={file.diff.deletions} onReview={()=>onReview(file.path)} onOpen={()=>onOpen?.(absolute(file.path))} action={method=>void action(method,file.path)}/>)}
   {files.length>3&&<button type="button" aria-expanded={expanded} onClick={()=>setExpanded(!expanded)} className="flex h-7 items-center gap-1 rounded-md px-2 text-xs text-muted-foreground hover:bg-[var(--muted)]">{expanded?t('chat.generatedFilesLess'):t('chat.generatedFilesMore',{count:files.length-3})}{expanded?<ChevronUp size={14}/>:<ChevronDown size={14}/>}</button>}
  </div>}
  {data?.snapshot.workspaceState==='conflict'&&<p role="status" className="px-4 py-2 text-sm">{t('chat.turnConflict')}</p>}
  {data&&!data.snapshot.isReversible&&<p role="status" className="px-4 py-2 text-sm text-muted-foreground">{t('chat.turnCapturePartial')}</p>}
  {error&&<p role="alert" className="px-4 py-2 text-sm">{error}</p>}
 </section>;
}
function TurnFileRow({path,additions,deletions,onReview,onOpen,action}:{path:string;additions:number;deletions:number;onReview:()=>void;onOpen:()=>void;action:(method:string)=>void}){
 const {t}=useTranslation(),[menu,setMenu]=useState<{x:number;y:number}|null>(null),row=useRef<HTMLButtonElement>(null);
 // A file row is not a dropdown trigger. Cindy's fixed menu anchor keeps
 // primary activation (review) independent from the context-menu gesture.
 return <><button ref={row} type="button" title={path} className="flex h-7 w-full items-center gap-2 rounded-md px-2 text-left transition-colors hover:bg-[var(--muted)]" onClick={onReview} onKeyDown={activateInlineLinkFromKeyboard} onContextMenu={event=>{event.preventDefault();event.stopPropagation();setMenu(linkMenuPosition(event));}}><FileText size={15} className="shrink-0 text-muted-foreground"/><span className="min-w-0 flex-1 truncate text-sm">{path}</span><span className="shrink-0 whitespace-nowrap font-mono text-xs tabular-nums"><span className="text-[var(--diff-add-fg)]">+{additions}</span>{' '}<span className="text-[var(--diff-del-fg)]">−{deletions}</span></span></button>
 <DropdownMenu open={menu!==null} onOpenChange={open=>{if(!open)setMenu(null);}}><DropdownMenuTrigger asChild><span aria-hidden tabIndex={-1} data-fixed-menu-anchor style={{position:'fixed',left:menu?.x??0,top:menu?.y??0,width:0,height:0,pointerEvents:'none'}}/></DropdownMenuTrigger><DropdownMenuContent className="chat-row-menu" onCloseAutoFocus={event=>{event.preventDefault();row.current?.focus({preventScroll:true});}}><DropdownMenuItem onSelect={onReview}>{t('chat.reviewViewChanges')}</DropdownMenuItem><DropdownMenuItem onSelect={onOpen}>{t('chat.reviewOpenFile')}</DropdownMenuItem><DropdownMenuItem onSelect={()=>action('copy')}>{t('chat.copyPath')}</DropdownMenuItem><DropdownMenuItem onSelect={()=>action('file.reveal')}>{t('chat.revealFile')}</DropdownMenuItem><DropdownMenuItem onSelect={()=>action('file.open')}>{t('chat.openSystem')}</DropdownMenuItem></DropdownMenuContent></DropdownMenu></>;
}
