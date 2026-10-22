// Cindy FileBrowserBody with its actual tree, toolbar, file filter and search.
// WinOne supplies scoped operations; the user-requested tree sits on the left.
import {lazy,Suspense,useEffect,useRef,useState,type PointerEvent as ReactPointerEvent} from 'react';
import '../panel-ui/locales';
import {useTranslation} from 'react-i18next';
import {useConfirmDialog,ConfirmDialogProvider} from '../panel-ui/confirm-dialog-provider';
import {Spinner} from '../panel-ui/spinner';
import {FileTreeView} from './FileTreeView';
import {TreeHeader} from './TreeHeader';
import {FileFilterInput} from './FileFilterInput';
import {FilterResultList} from './FilterResultList';
import {filterFiles,FILTER_RESULT_LIMIT} from './lib/filterFiles';
import {SearchPanel} from './search/SearchPanel';
import {fileQuery,joinWorkspace,useWorkspaceTree,type FileResult,type SearchStatus} from './host';
import {chatCall} from '../../../bridge';
import {copyText} from '../../clipboard';
import {openMedia,toast} from '../media/host';
import type {ChatFileHandle} from '../../chat-file-preview';
const ChatFilePreview=lazy(()=>import('../../chat-file-preview').then(module=>({default:module.ChatFilePreview})));
type Props={id:string;active?:boolean;initialDirectory?:string;onOpenFile?:(path:string)=>void;onReference:(path:string)=>void;onDirty?:(dirty:boolean)=>void;onHandle?:(handle:ChatFileHandle|undefined)=>void;onOpenBrowser?:(url:string)=>void};
export function FileBrowser(props:Props){return <ConfirmDialogProvider><WorkspaceBrowser {...props}/></ConfirmDialogProvider>;}
function WorkspaceBrowser({id,active=true,initialDirectory,onOpenFile,onReference,onDirty,onHandle,onOpenBrowser}:Props){
 const {t}=useTranslation(),{confirmThree}=useConfirmDialog(),{root,tree,error,refresh,collapseAll}=useWorkspaceTree(id,initialDirectory,active);
 const [selected,setSelected]=useState<string|null>(null),[line,setLine]=useState<number>(),[mode,setMode]=useState<'tree'|'search'>('tree'),[filter,setFilter]=useState(''),[query,setQuery]=useState(''),[sensitive,setSensitive]=useState(false),[index,setIndex]=useState<{files:string[];truncated:boolean}>({files:[],truncated:false}),[indexLoading,setIndexLoading]=useState(false),[indexError,setIndexError]=useState<string|null>(null),[search,setSearch]=useState<{results:FileResult[];totalMatches:number;totalFiles:number;status:SearchStatus;errorMessage:string|null}>({results:[],totalMatches:0,totalFiles:0,status:'idle',errorMessage:null});
 const handle=useRef<ChatFileHandle|null>(null),container=useRef<HTMLDivElement>(null),[width,setWidth]=useState(()=>{try{return Number(localStorage.getItem('winone.file-tree.'+id))||200;}catch{return 200;}}),[containerWidth,setContainerWidth]=useState(420),drag=useRef<{x:number;width:number}|undefined>(undefined);
 const [indexRevision,setIndexRevision]=useState(0),switching=useRef(false);
 useEffect(()=>{const observer=new ResizeObserver(()=>setContainerWidth(container.current?.clientWidth??420));if(container.current)observer.observe(container.current);return()=>observer.disconnect();},[]);
 useEffect(()=>{onHandle?.({dirty:()=>handle.current?.dirty()??false,save:()=>handle.current?.save()??Promise.resolve(true)});return()=>{onHandle?.(undefined);onDirty?.(false);};},[id]);
 useEffect(()=>{if(!filter.trim()||!root)return;const controller=new AbortController();setIndexLoading(true);setIndexError(null);void fileQuery<{files:string[];truncated:boolean}>(id,'index',{path:root},controller.signal).then(value=>{if(!controller.signal.aborted)setIndex(value);}).catch(error=>{if(!controller.signal.aborted)setIndexError(String(error));}).finally(()=>{if(!controller.signal.aborted)setIndexLoading(false);});return()=>controller.abort();},[id,root,!!filter.trim(),indexRevision]);
 useEffect(()=>{if(mode!=='search'||!query.trim()||!root){setSearch({results:[],totalMatches:0,totalFiles:0,status:'idle',errorMessage:null});return;}
  const controller=new AbortController(),searchId=crypto.randomUUID();const timer=setTimeout(()=>{setSearch(value=>({...value,status:'searching',errorMessage:null}));void fileQuery<{results:FileResult[];totalMatches:number;totalFiles:number;truncated:boolean}>(id,'search',{path:root,query,caseSensitive:sensitive,maxMatches:500,searchId},controller.signal).then(value=>{if(!controller.signal.aborted)setSearch({...value,status:value.truncated?'truncated':'done',errorMessage:null});}).catch(error=>{if(!controller.signal.aborted)setSearch({results:[],totalMatches:0,totalFiles:0,status:'error',errorMessage:String(error)});});},250);
  return()=>{clearTimeout(timer);controller.abort();void chatCall('files.cancel',{id,searchId}).catch(console.error);};
 },[id,root,query,sensitive,mode]);
 async function choose(path:string,jump?:number){
  if(switching.current)return;switching.current=true;
  try{const absolute=joinWorkspace(root,path);if(absolute!==selected&&handle.current?.dirty()){const choice=await confirmThree({title:t('ccAgent.workdirBrowse.confirmSwitchAway.title'),description:t('ccAgent.workdirBrowse.confirmSwitchAway.descriptionSwitchFile',{path:selected}),confirmText:t('ccAgent.workdirBrowse.confirmSwitchAway.saveAndSwitch'),tertiaryText:t('ccAgent.workdirBrowse.confirmSwitchAway.tertiary'),cancelText:t('ccAgent.workdirBrowse.confirmSwitchAway.cancel')});if(choice==='cancel'||choice==='confirm'&&!await handle.current.save())return;}setSelected(absolute);setLine(jump);}catch(error){toast.error(String(error));}finally{switching.current=false;}
 }
 // The native drop includes client coordinates. Only the visible file pane
 // consumes its drop; the composer retains drops everywhere else.
 const chooseRef=useRef(choose);chooseRef.current=choose;
 useEffect(()=>{if(!active||!id)return;const drop=(event:Event)=>{const {paths,position}=(event as CustomEvent<{paths:string[];position?:{x:number;y:number}}>).detail??{};if(!paths?.length||!position||!container.current?.contains(document.elementFromPoint(position.x,position.y)))return;event.stopImmediatePropagation();void chooseRef.current(paths[0]);};window.addEventListener('winone:chat-files-drop',drop,true);return()=>window.removeEventListener('winone:chat-files-drop',drop,true);},[active,id]);
 useEffect(()=>{try{localStorage.setItem('winone.file-tree.'+id,String(width));}catch(error){toast.error(String(error));}},[id,width]);
 const treeWidth=Math.min(Math.max(120,width),500,Math.max(80,containerWidth-100)),filtered=filterFiles(filter,index.files);
 const treeSelected=selected?.replace(/\\/g,'/').startsWith(root.replace(/\\/g,'/').replace(/\/$/,'')+'/')?selected.replace(/\\/g,'/').slice(root.replace(/[\\/]$/,'').length+1):null;
 function begin(event:ReactPointerEvent<HTMLDivElement>){if(event.button!==0)return;drag.current={x:event.clientX,width:treeWidth};event.currentTarget.setPointerCapture(event.pointerId);}
 if(!id)return <p className="p-4 text-13">{t('chat.selectConversation')}</p>;
 return <div ref={container} className="rsb-fbody cindy-chat-ui flex h-full min-h-0 min-w-0 overflow-hidden">
  <div className="rsb-fbody-tree-pane flex h-full shrink-0 flex-col overflow-hidden" style={{width:treeWidth}}>
   <TreeHeader workdir={root} mode={mode} onToggleSearch={()=>setMode(value=>value==='tree'?'search':'tree')} onCollapseAll={collapseAll} onRefresh={()=>{refresh();setIndexRevision(value=>value+1);}}/>
   {mode==='tree'?<div className="flex min-h-0 flex-1 flex-col"><FileFilterInput value={filter} onChange={setFilter}/>{error&&<p role="alert" className="px-3 py-2 text-12">{error}</p>}{filter?<FilterResultList files={filtered} truncated={index.truncated||filtered.length>=FILTER_RESULT_LIMIT} isLoading={indexLoading} indexError={indexError} selectedPath={treeSelected} onSelectFile={path=>void choose(path)}/>:<div className="rsb-fbody-tree-scroll min-h-0 flex-1"><FileTreeView tree={tree} selectedPath={treeSelected} onSelectFile={path=>void choose(path)} onPreviewImage={entry=>openMedia({id,src:entry.absolutePath??joinWorkspace(root,entry.relPath)})} onCopyFilePath={entry=>void copyText(entry.absolutePath??joinWorkspace(root,entry.relPath))} onRevealInFolder={entry=>void chatCall('file.reveal',{id,path:entry.absolutePath??joinWorkspace(root,entry.relPath)}).catch(error=>toast.error(String(error)))} onOpenInFileBrowser={entry=>onOpenFile?.(entry.absolutePath??joinWorkspace(root,entry.relPath))} onOpenInSidebarBrowser={entry=>void chatCall<{url:string}>('file.preview',{id,path:entry.absolutePath??joinWorkspace(root,entry.relPath)}).then(value=>onOpenBrowser?.(value.url)).catch(error=>toast.error(String(error)))} onOpenInBrowser={entry=>void chatCall('file.open',{id,path:entry.absolutePath??joinWorkspace(root,entry.relPath)}).catch(error=>toast.error(String(error)))}/></div>}</div>:<SearchPanel query={query} onQueryChange={setQuery} caseSensitive={sensitive} onCaseSensitiveChange={setSensitive} {...search} maxMatches={500} onOpenMatch={(path,line)=>void choose(path,line)}/>}
   {selected&&<button className="shrink-0 border-t border-[var(--border-default)] px-3 py-2 text-left text-12 text-[var(--text-secondary)]" onClick={()=>onReference(joinWorkspace(root,selected))}>{t('chat.fileReference')} · {selected.split('/').pop()}</button>}
  </div>
  {/* The tree is on the left, so dragging/right-arrow increases its width. */}
  <div role="separator" aria-label={t('chat.resizeFileTree')} aria-orientation="vertical" aria-valuenow={Math.round(treeWidth)} tabIndex={0} className="w-1 shrink-0 cursor-col-resize bg-[var(--border-default)] hover:bg-[var(--text-tertiary)]" onPointerDown={begin} onPointerMove={event=>{if(drag.current)setWidth(Math.max(120,Math.min(500,containerWidth-100,drag.current.width+event.clientX-drag.current.x)));}} onPointerUp={event=>{drag.current=undefined;event.currentTarget.releasePointerCapture(event.pointerId);}} onPointerCancel={()=>{drag.current=undefined;}} onDoubleClick={()=>setWidth(200)} onKeyDown={event=>{if(event.key==='ArrowLeft'||event.key==='ArrowRight'){event.preventDefault();setWidth(Math.max(120,Math.min(500,containerWidth-100,treeWidth+(event.key==='ArrowRight'?16:-16))));}}}/>
  <div className="rsb-fbody-compact flex h-full min-h-0 min-w-0 flex-1 flex-col overflow-hidden">{selected?<Suspense fallback={<Spinner/>}><ChatFilePreview key={selected} id={id} path={joinWorkspace(root,selected)} line={line} onDirty={value=>onDirty?.(value)} onHandle={value=>{handle.current=value;}}/></Suspense>:<div className="flex flex-1 items-center justify-center p-5 text-center text-13 text-[var(--text-tertiary)]">{t('ccAgent.workdirBrowse.fileBody.selectFile')}</div>}</div>
 </div>;
}
