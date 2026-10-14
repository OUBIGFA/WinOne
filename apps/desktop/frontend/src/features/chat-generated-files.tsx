// GeneratedFilesCard chip/menu adaptation from makecindy/cindy 69661422
// (Copyright XD Inc., Apache-2.0). Host preview replaces Electron stat/open.
import {useEffect,useState} from 'react';
import {ChevronDown,ChevronUp,FileText} from 'lucide-react';
import {chatCall} from '../bridge';
import {useTranslation} from '../i18n';
import {copyText} from './clipboard';
import {DropdownMenu,DropdownMenuContent,DropdownMenuItem,DropdownMenuTrigger} from '../components/ui/dropdown-menu';
import type {ChatFilePreviewData} from './chat-file-preview';

export function ChatGeneratedFiles({id,paths,onOpen}:{id:string;paths:string[];onOpen:(path:string)=>void}){
 const {t}=useTranslation(),[files,setFiles]=useState<ChatFilePreviewData[]>([]),[expanded,setExpanded]=useState(false),[error,setError]=useState('');
 const fingerprint=JSON.stringify([...new Set(paths)]);
 useEffect(()=>{
  let alive=true;setFiles([]);setError('');
  // A successful historical tool is a candidate, not proof the file still
  // exists. Validate with the same Host boundary used by its actual preview.
  void Promise.allSettled((JSON.parse(fingerprint) as string[]).map(path=>chatCall<ChatFilePreviewData>('file.preview',{id,path}))).then(results=>{
   if(!alive)return;
   setFiles(results.flatMap(result=>result.status==='fulfilled'?[result.value]:[]));
   const failed=results.filter(result=>result.status==='rejected');
   if(failed.length)setError(t('chat.generatedFilesUnavailable',{count:failed.length}));
  });return()=>{alive=false;};
 },[id,fingerprint,t]);
 const action=async(method:string,path:string)=>{try{if(method==='copy')await copyText(path);else await chatCall(method,{id,path});setError('');}catch(error){setError(String(error));}};
 return <div className="cindy-chat-ui chat-generated-files">
  <div className="flex flex-wrap items-center gap-2">{(expanded?files:files.slice(0,6)).map(file=><GeneratedFileChip key={file.path} file={file} onOpen={onOpen} action={action}/>)}
  {files.length>6&&<button type="button" className="inline-flex items-center gap-1 text-sm" aria-expanded={expanded} onClick={()=>setExpanded(!expanded)}>{expanded?<ChevronUp size={14}/>:<ChevronDown size={14}/>} {expanded?t('chat.generatedFilesLess'):t('chat.generatedFilesMore',{count:files.length-6})}</button>}</div>
  {error&&<p role="status" className="text-sm text-muted-foreground">{error}</p>}
 </div>;
}
function GeneratedFileChip({file,onOpen,action}:{file:ChatFilePreviewData;onOpen:(path:string)=>void;action:(method:string,path:string)=>Promise<void>}){
 const {t}=useTranslation(),[menu,setMenu]=useState(false);
 return <DropdownMenu open={menu} onOpenChange={setMenu}><DropdownMenuTrigger render={<button type="button" title={file.path} className="inline-flex h-7 max-w-[280px] items-center gap-1.5 rounded-full bg-[var(--msg-md-inline-code-bg)] px-2.5 py-1.5 text-sm font-medium text-[var(--msg-assistant-text)] hover:bg-[var(--cmd-palette-item-hover)]" onClick={event=>{event.preventDefault();onOpen(file.path);}} onContextMenu={event=>{event.preventDefault();setMenu(true);}}/>}><FileText size={14}/><span className="truncate">{file.name}</span></DropdownMenuTrigger><DropdownMenuContent className="chat-row-menu"><DropdownMenuItem onClick={()=>onOpen(file.path)}>{t('chat.reviewOpenFile')}</DropdownMenuItem><DropdownMenuItem onClick={()=>void action('copy',file.path)}>{t('chat.copyPath')}</DropdownMenuItem><DropdownMenuItem onClick={()=>void action('file.reveal',file.path)}>{t('chat.revealFile')}</DropdownMenuItem><DropdownMenuItem onClick={()=>void action('file.open',file.path)}>{t('chat.openSystem')}</DropdownMenuItem></DropdownMenuContent></DropdownMenu>;
}
