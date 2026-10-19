// GeneratedFilesCard chip/menu adaptation from makecindy/cindy 69661422
// (Copyright XD Inc., Apache-2.0). Host preview replaces Electron stat/open.
import {useEffect,useRef,useState} from 'react';
import {ChevronDown,ChevronUp,FileText,RefreshCw} from 'lucide-react';
import {chatCall} from '../bridge';
import {useTranslation} from '../i18n';
import {copyText} from './clipboard';
import {DropdownMenu,DropdownMenuContent,DropdownMenuItem,DropdownMenuTrigger} from './cindy/panel-ui/dropdown-menu';
import {inspectChatPaths,invalidateChatFileStats,type ChatFileStat} from './chat-file-access';

export function ChatGeneratedFiles({id,paths,onOpen}:{id:string;paths:string[];onOpen:(path:string)=>void}){
 const {t}=useTranslation(),[files,setFiles]=useState<ChatFileStat[]>([]),[expanded,setExpanded]=useState(false),[error,setError]=useState(''),[failed,setFailed]=useState(0),[revision,setRevision]=useState(0);
 const owner=useRef(id);
 const fingerprint=JSON.stringify([...new Set(paths)]);
 useEffect(()=>{
  let alive=true;setError('');setFailed(0);
  if(owner.current!==id){owner.current=id;setFiles([]);}
  // Stat, not full preview: checking chips never reads every file's contents
  // or mints unused HTTP grants. Deleted historical outputs are omitted;
  // permission/transport failures stay visible and explicitly retryable.
  void inspectChatPaths(id,JSON.parse(fingerprint)).then(results=>{
   if(!alive)return;
   setFiles(results.flatMap(result=>result.file&&!result.file.directory?[result.file]:[]));
   setFailed(results.filter(result=>result.error).length);
  }).catch(error=>{if(alive)setError(String(error));});return()=>{alive=false;};
 },[id,fingerprint,revision]);
 const action=async(method:string,path:string)=>{try{if(method==='copy')await copyText(path);else await chatCall(method,{id,path});setError('');}catch(error){setError(String(error));}};
 const visibleFiles=owner.current===id?files:[];
 return <div className="cindy-chat-ui chat-generated-files">
  <div className="flex flex-wrap items-center gap-2">{(expanded?visibleFiles:visibleFiles.slice(0,6)).map(file=><GeneratedFileChip key={file.path} file={file} onOpen={onOpen} action={action}/>)}
  {visibleFiles.length>6&&<button type="button" className="inline-flex items-center gap-1 text-sm" aria-expanded={expanded} onClick={()=>setExpanded(!expanded)}>{expanded?<ChevronUp size={14}/>:<ChevronDown size={14}/>} {expanded?t('chat.generatedFilesLess'):t('chat.generatedFilesMore',{count:visibleFiles.length-6})}</button>}</div>
  {(error||failed>0)&&<div role="status" className="flex items-center gap-2 text-sm text-muted-foreground"><span>{error||t('chat.generatedFilesUnavailable',{count:failed})}</span><button type="button" aria-label={t('chat.retry')} onClick={()=>{invalidateChatFileStats(id);setRevision(value=>value+1);}}><RefreshCw size={14}/></button></div>}
 </div>;
}
function GeneratedFileChip({file,onOpen,action}:{file:ChatFileStat;onOpen:(path:string)=>void;action:(method:string,path:string)=>Promise<void>}){
 const {t}=useTranslation(),[menu,setMenu]=useState<{x:number;y:number}|null>(null),button=useRef<HTMLButtonElement>(null);
 // Cindy keeps activation separate from the context-menu anchor: a left
 // click always opens the document, and the menu cannot intercept it.
 return <><button ref={button} type="button" title={file.path} className="inline-flex h-7 max-w-[280px] items-center gap-1.5 rounded-full bg-[var(--msg-md-inline-code-bg)] px-2.5 py-1.5 text-sm font-medium text-[var(--msg-assistant-text)] hover:bg-[var(--cmd-palette-item-hover)]" onClick={()=>onOpen(file.path)} onContextMenu={event=>{event.preventDefault();setMenu({x:event.clientX,y:event.clientY});}} onKeyDown={event=>{if(event.key==='ContextMenu'||event.shiftKey&&event.key==='F10'){event.preventDefault();const rect=event.currentTarget.getBoundingClientRect();setMenu({x:rect.left,y:rect.bottom});}}}><FileText size={14}/><span className="truncate">{file.name}</span></button>
  <DropdownMenu open={menu!==null} onOpenChange={open=>{if(!open)setMenu(null);}}><DropdownMenuTrigger asChild><span aria-hidden tabIndex={-1} data-fixed-menu-anchor style={{position:'fixed',left:menu?.x??0,top:menu?.y??0,width:0,height:0,pointerEvents:'none'}}/></DropdownMenuTrigger><DropdownMenuContent align="start" sideOffset={2} onCloseAutoFocus={event=>{event.preventDefault();button.current?.focus();}}><DropdownMenuItem onSelect={()=>onOpen(file.path)}>{t('chat.reviewOpenFile')}</DropdownMenuItem><DropdownMenuItem onSelect={()=>void action('copy',file.path)}>{t('chat.copyPath')}</DropdownMenuItem><DropdownMenuItem onSelect={()=>void action('file.reveal',file.path)}>{t('chat.revealFile')}</DropdownMenuItem><DropdownMenuItem onSelect={()=>void action('file.open',file.path)}>{t('chat.openSystem')}</DropdownMenuItem></DropdownMenuContent></DropdownMenu>
 </>;
}
