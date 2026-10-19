// WinOne's resource inventory uses Cindy's existing sidebar controls and row
// composition. Its entries come from Pi's real resource discovery, not a demo.
import {useEffect,useState,type DragEvent,type ReactNode} from 'react';
import {BookOpen,FileText,Plug,Puzzle,RefreshCw,ScrollText} from 'lucide-react';
import {chatCall} from '../bridge';
import {useTranslation} from '../i18n';
import {ChromeIconButton} from './cindy/panel-ui/ChromeIconButton';
import {Spinner} from './cindy/panel-ui/spinner';
import {FILE_REFERENCE_MIME} from './chat-panel-state';
import type {ChatResources} from './chat-types';

export function ChatResourcesPanel({id,onOpenFile,active=true}:{id:string;onOpenFile:(path:string)=>void;active?:boolean}){
 const {t}=useTranslation();
 const [resources,setResources]=useState<ChatResources>(),[error,setError]=useState(''),[loading,setLoading]=useState(false),[revision,setRevision]=useState(0);
 useEffect(()=>{
  if(!id||!active)return;let alive=true;setResources(undefined);setLoading(true);setError('');
  void chatCall<ChatResources>('resources',{id}).then(value=>{if(alive)setResources(value);}).catch(error=>{if(alive)setError(String(error));}).finally(()=>{if(alive)setLoading(false);});
  return()=>{alive=false;};
 },[id,revision,active]);
 const reference=(path:string)=>({draggable:true,onDragStart:(event:DragEvent)=>{event.dataTransfer.setData(FILE_REFERENCE_MIME,path);event.dataTransfer.setData('text/plain',path);event.dataTransfer.effectAllowed='copy';}});
 const row='flex min-w-0 w-full items-center gap-2 rounded-md px-2 py-2 text-left text-13';
 const text=(name:string,detail:string)=><span className="flex min-w-0 flex-1 flex-col gap-1"><span className="break-words font-medium">{name}</span><span className="break-words text-12 text-[var(--text-tertiary)]" title={detail}>{detail}</span></span>;
 function group(name:string,icon:ReactNode,count:number,children:ReactNode){return <section><h3 className="mb-2 flex items-center gap-2 text-13 font-medium">{icon}<span className="flex-1">{name}</span><span className="text-12 text-[var(--text-tertiary)]">{count}</span></h3>{count?children:<p className="px-6 text-13 text-[var(--text-tertiary)]">{t('chat.noResources')}</p>}</section>;}
 return <div className="flex h-full min-h-0 flex-col">
  <div className="flex shrink-0 items-center justify-between px-3 py-1"><span className="text-13">{t('chat.resources')}</span><ChromeIconButton aria-label={t('chat.refresh')} disabled={!id||loading} onClick={()=>setRevision(value=>value+1)}><RefreshCw size={14}/></ChromeIconButton></div>
  <div className="flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto overflow-x-hidden px-3 pb-4 pt-3">
   {!id?<p className="text-13">{t('chat.selectConversation')}</p>:<>
    {error&&<p role="alert" className="text-13 text-[var(--text-danger)]">{error}</p>}
    {loading&&<div role="status" className="flex items-center gap-2 text-13"><Spinner size={14}/>{t('chat.loadingResources')}</div>}
    {resources&&<>{group('AGENTS.md',<ScrollText size={16}/>,resources?.agents?.length??0,resources?.agents?.map(agent=><button key={agent.path} className={row+' hover:bg-sidebar-item-hover'} {...reference(agent.path)} onClick={()=>onOpenFile(agent.path)}><FileText size={16}/>{text('AGENTS.md',agent.path)}</button>))}
    {group(t('chat.skills'),<BookOpen size={16}/>,resources?.skills?.length??0,resources?.skills?.map(skill=><button key={skill.path} className={row+' hover:bg-sidebar-item-hover'} {...reference(skill.path)} onClick={()=>onOpenFile(skill.path)}><BookOpen size={16}/>{text(skill.name,skill.description)}</button>))}
    {group(t('chat.plugins'),<Puzzle size={16}/>,resources?.plugins?.length??0,resources?.plugins?.map(plugin=><div key={plugin.path} className={row}><Puzzle size={16}/>{text(plugin.name,plugin.path)}<span className="shrink-0 text-12">{t(plugin.enabled?'chat.enabled':'chat.disabled')}</span></div>))}
    {group('MCP',<Plug size={16}/>,resources?.mcpServers?.length??0,resources?.mcpServers?.map(server=><div key={server.name} className={row}><Plug size={16}/>{text(server.name,server.error||server.status||t(server.enabled?'chat.enabled':'chat.disabled'))}</div>))}
    {resources.diagnostics?.map((value,index)=><p key={index} role="status" className="text-13 text-[var(--text-secondary)]">{value}</p>)}</>}
   </>}
  </div>
 </div>;
}
