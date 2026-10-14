import {useEffect,useState,type DragEvent} from 'react';
import {ArrowUp,ChevronsRightLeft,FileText,FolderOpen,RefreshCw,AtSign,ScrollText,BookOpen,Puzzle,Plug,CheckCircle2,CircleSlash} from 'lucide-react';
import {PageToolbar} from 'chatgpt-desktop-kit/components/layout';
import {Tabs} from 'chatgpt-desktop-kit/components/navigation';
import {InternalScrollArea,ListStack,RecordRow} from 'chatgpt-desktop-kit/components/lists';
import {EmptyState,InlineNotice} from 'chatgpt-desktop-kit/components/feedback';
import {chatCall} from '../bridge';
import {useTranslation} from '../i18n';
import {ChatPanelButton as Button} from './chat-desktop-kit';
import {FILE_REFERENCE_MIME} from './chat-panel-state';
import {formatBytes} from './cindy/fileMeta';
import type {ChatFiles,ChatResources} from './chat-types';

/** Upstream workspace/list/control composition. WinOne supplies only real
 * directory data and commands; no copied tab, row or toolbar paint lives here. */
export function ChatInspector({id,initialDirectory,onClose,onReference,initialTab='files',onOpenFile,embedded=false}:{id:string;initialDirectory?:string;onClose:()=>void;onReference:(path:string)=>void;initialTab?:'files'|'resources';onOpenFile?:(path:string)=>void;embedded?:boolean}){
 const {t}=useTranslation();
 const [tab,setTab]=useState<string>(initialTab),[path,setPath]=useState(''),[history,setHistory]=useState<string[]>([]),[files,setFiles]=useState<ChatFiles>(),[resources,setResources]=useState<ChatResources>(),[error,setError]=useState(''),[loading,setLoading]=useState(false),[revision,setRevision]=useState(0);
 useEffect(()=>setTab(initialTab),[initialTab]);
 useEffect(()=>{setPath(initialDirectory??'');setHistory(initialDirectory?['']:[]);setFiles(undefined);setResources(undefined);},[id,initialDirectory]);
 useEffect(()=>{
  if(!id)return;let alive=true;setLoading(true);setError('');
  const request=tab==='files'?chatCall<ChatFiles>('files',{id,path}).then(value=>{if(alive)setFiles(value);}):chatCall<ChatResources>('resources',{id}).then(value=>{if(alive)setResources(value);});
  void request.catch(error=>{if(alive)setError(error instanceof Error?error.message:String(error));}).finally(()=>{if(alive)setLoading(false);});return()=>{alive=false;};
 },[id,path,tab,revision]);
 const reference=(path:string)=>({draggable:true,onDragStart:(event:DragEvent)=>{event.dataTransfer.setData(FILE_REFERENCE_MIME,path);event.dataTransfer.setData('text/plain',path);event.dataTransfer.effectAllowed='copy';}});
 const empty=<p className="chat-resource-empty">{t('chat.noResources')}</p>;
 return <div className="chat-file-browser">
  {!embedded&&<Tabs value={tab} onValueChange={setTab} ariaLabel={t('chat.details')} items={[{value:'files',label:t('chat.files')},{value:'resources',label:t('chat.resources')}]}/>}
  <PageToolbar>
   {!embedded&&<Button size="icon" icon={ChevronsRightLeft} aria-label={t('chat.hideDetails')} onClick={onClose}/>}
   {tab==='files'&&<Button size="icon" icon={ArrowUp} disabled={!history.length||loading} aria-label={t('chat.parentFolder')} onClick={()=>{setPath(history[history.length-1]);setHistory(value=>value.slice(0,-1));}}/>}
   <span className="chat-file-location-path" title={files?.directory}>{tab==='files'?(files?.directory||t('chat.sourcePath')):t('chat.resources')}</span>
   <Button size="icon" icon={RefreshCw} disabled={!id||loading} aria-label={t('chat.refresh')} onClick={()=>setRevision(value=>value+1)}/>
  </PageToolbar>
  {!id?<EmptyState title={t('chat.selectConversation')}/>:<InternalScrollArea className="desktop-workbench__content chat-file-list">
   {error&&<InlineNotice tone="danger" title={error}/>}{loading&&<InlineNotice title={t('chat.loadingResources')}/>}
   {tab==='files'?<ListStack>
    {files?.entries?.map(entry=><RecordRow key={entry.path} density="compact" className="chat-file-entry">
     <Button className="chat-file-open" icon={entry.directory?FolderOpen:FileText} {...(!entry.directory?reference(entry.path):{})} onClick={()=>{if(entry.directory){setHistory(value=>[...value,path]);setPath(entry.path);}else onOpenFile?.(entry.path);}}><span title={entry.path}>{entry.name}</span>{!entry.directory&&<small>{formatBytes(entry.size)}</small>}</Button>
     {!entry.directory&&<Button size="icon" icon={AtSign} aria-label={t('chat.fileReference')+' · '+entry.name} onClick={()=>onReference(entry.path)}/>}
    </RecordRow>)}
    {!loading&&files?.entries?.length===0&&<EmptyState title={t('chat.noFiles')}/>}
   </ListStack>:<div className="chat-resource-sections">
    <section><h3><ScrollText size={18}/><span>AGENTS.md</span><small>{resources?.agents?.length??0}</small></h3>{resources?.agents?.length?<ListStack>{resources.agents.map(agent=><Button key={agent.path} className="chat-resource-link" icon={FileText} {...reference(agent.path)} onClick={()=>onOpenFile?.(agent.path)}><span><strong>AGENTS.md</strong><small title={agent.path}>{agent.path}</small></span></Button>)}</ListStack>:empty}</section>
    <section><h3><BookOpen size={18}/><span>{t('chat.skills')}</span><small>{resources?.skills?.length??0}</small></h3>{resources?.skills?.length?<ListStack>{resources.skills.map(skill=><Button key={skill.path} className="chat-resource-link" icon={BookOpen} {...reference(skill.path)} onClick={()=>onOpenFile?.(skill.path)}><span><strong>{skill.name}</strong><small title={skill.description}>{skill.description}</small></span></Button>)}</ListStack>:empty}</section>
    <section><h3><Puzzle size={18}/><span>{t('chat.plugins')}</span><small>{resources?.plugins?.length??0}</small></h3>{resources?.plugins?.length?<ListStack>{resources.plugins.map(plugin=><RecordRow key={plugin.path} className="chat-resource-row"><Puzzle size={18}/><span><strong>{plugin.name}</strong><small title={plugin.path}>{plugin.path}</small></span><span className="chat-resource-state">{plugin.enabled?<CheckCircle2 size={14}/>:<CircleSlash size={14}/>}{t(plugin.enabled?'chat.enabled':'chat.disabled')}</span></RecordRow>)}</ListStack>:empty}</section>
    <section><h3><Plug size={18}/><span>MCP</span><small>{resources?.mcpServers?.length??0}</small></h3>{resources?.mcpServers?.length?<ListStack>{resources.mcpServers.map(server=><RecordRow key={server.name} className="chat-resource-row"><Plug size={18}/><span><strong>{server.name}</strong><small className={server.error?'text-destructive':undefined}>{server.error||server.status||t(server.enabled?'chat.enabled':'chat.disabled')}</small></span>{server.enabled?<CheckCircle2 size={14}/>:<CircleSlash size={14}/>}</RecordRow>)}</ListStack>:empty}</section>
    {resources?.diagnostics?.map((diagnostic,index)=><InlineNotice key={index} title={diagnostic}/>)}
   </div>}
  </InternalScrollArea>}
 </div>;
}
