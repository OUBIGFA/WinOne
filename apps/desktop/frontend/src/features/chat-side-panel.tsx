// Cindy right-sidebar tab/plugin pattern, adapted to WinOne's shared desktop.
// Contents are kept mounted across tab changes; native surfaces receive active
// explicitly so hidden browser/terminal tabs retain state without covering UI.
import {lazy,Suspense,useCallback,useEffect,useRef,useState} from 'react';
import type {DesktopMenuItem} from 'chatgpt-desktop-kit';
import {Input} from './cindy/panel-ui/input';
import {ConfirmDialog} from './cindy/panel-ui/confirm-dialog';
import {PanelTabs,PanelMenuItems} from './cindy/panel-ui/PanelTabs';
import {SegmentedControl} from './cindy/panel-ui/segmented-control';
import {ChromeIconButton} from './cindy/panel-ui/ChromeIconButton';
import {DropdownMenu,DropdownMenuTrigger,DropdownMenuContent} from './cindy/panel-ui/dropdown-menu';
import {FolderOpen,Maximize2,Minimize2,MoreHorizontal,PanelsTopLeft} from 'lucide-react';
import {SidebarSimple} from '@phosphor-icons/react';
import {PanelHome} from './cindy/panel-ui/PanelHome';
import {MermaidLightboxHost} from './cindy/editor/MermaidLightboxHost';
import {MermaidSourceEditorHost} from './cindy/editor/MermaidSourceEditor';
import {DocumentFeedback} from './cindy/editor/host';
import {chatCall} from '../bridge';


import {useTranslation} from '../i18n';
import {ChatResourcesPanel} from './chat-resources';
import type {ChatFileHandle} from './chat-file-preview';
const ChatFilePreview=lazy(()=>import('./chat-file-preview').then(module=>({default:module.ChatFilePreview})));
const FileBrowser=lazy(()=>import('./cindy/file-browser/FileBrowser').then(module=>({default:module.FileBrowser})));
const ChatBrowserView=lazy(()=>import('./chat-browser').then(module=>({default:module.ChatBrowserView})));
const ChatTerminalView=lazy(()=>import('./chat-terminal').then(module=>({default:module.ChatTerminalView})));
const ChatReviewView=lazy(()=>import('./chat-review').then(module=>({default:module.ChatReviewView})));
import {restorePanelTabs,type ChatPanelTab,type ChatLibraryTab} from './chat-panel-state';
import type {ChatBrowserAnnotation} from './chat-types';
import './chat-side-panel.css';

export function ChatSidePanel({id,initialTab,initialTabRequest=0,initialDirectory,initialURL,onURLHandled,initialFile,initialLine,initialReview,onClose,onReference,onComment,onReviewComment,commentMarkerNumbers,onDirtyChange,onDock,dock,visible=true,running=false,lastTurnPaths=[]}:{running?:boolean;lastTurnPaths?:string[];id:string;initialTab:'home'|'files'|'resources'|'review';initialTabRequest?:number;initialURL?:string;onURLHandled?:()=>void;initialFile?:string;initialLine?:number;initialDirectory?:string;initialReview?:{turnId:string;path?:string};onClose:()=>void;onReference:(path:string)=>void;onComment:(item:ChatBrowserAnnotation)=>Promise<void>;commentMarkerNumbers:number[];onReviewComment:(text:string)=>void;onDirtyChange:(value:boolean)=>void;onDock:()=>void;dock:'right'|'bottom';visible?:boolean}){
 const {t}=useTranslation();const [state,setState]=useState(()=>restorePanelTabs(id)),[fullscreen,setFullscreen]=useState(false),[pending,setPending]=useState<string>(),[error,setError]=useState('');
 const [libraryMounted,setLibraryMounted]=useState(()=>state.section==='library');
 const handles=useRef(new Map<string,ChatFileHandle>()),dirty=useRef(new Set<string>()),closed=useRef<ChatPanelTab[]>([]);
 const [dirtyRevision,setDirtyRevision]=useState(0),[terminalClose,setTerminalClose]=useState<string>(),[newPage,setNewPage]=useState(false),[pageName,setPageName]=useState('Untitled.md'),[creating,setCreating]=useState(false),[paneMenuOpen,setPaneMenuOpen]=useState(false);
 const terminals=useRef(new Set<string>()),closeQueue=useRef<string[]>([]);
 const savingFiles=useRef(false),creatingPage=useRef(false),[saving,setSaving]=useState(false);
 // Collapsing retains editor drafts and running terminals. Native surfaces
 // still receive visibility so a hidden pane cannot cover the conversation.
 useEffect(()=>{if(!visible)setFullscreen(false);},[visible]);
 const [toolActions,setToolActions]=useState<Record<string,DesktopMenuItem[]>>({});
 const registerActions=useCallback((key:string,items:DesktopMenuItem[])=>setToolActions(current=>({...current,[key]:items})),[]);
 useEffect(()=>{
  if(initialURL){open({id:'browser:'+crypto.randomUUID(),kind:'browser',url:initialURL});onURLHandled?.();return;}
  if(initialFile){openFile(initialFile,initialLine);return;}
  if(initialTab==='files'||initialTab==='resources'){openLibrary(initialTab,initialTab==='files'?initialDirectory:undefined);return;}
  if(initialTab==='review'&&initialReview){open({id:'review:'+initialReview.turnId,kind:'review',turnId:initialReview.turnId,path:initialReview.path,title:t('chat.turnReview')});return;}
  // A normal reopen restores both selections; only an explicit request may
  // replace the saved document selection with a home page.
  if(initialTab==='review'||initialTabRequest>0)open({id:initialTab,kind:initialTab});
 },[initialTab,initialTabRequest,initialReview,initialDirectory,initialFile,initialLine]);
 useEffect(()=>{try{localStorage.setItem(`winone.chat.panel.v1:${id}`,JSON.stringify(state));}catch(error){setError(String(error));}},[id,state]);
 useEffect(()=>{onDirtyChange(dirty.current.size>0||terminals.current.size>0);},[dirtyRevision,onDirtyChange]);
 const changeDirty=useCallback((key:string,value:boolean)=>{if(dirty.current.has(key)===value)return;if(value)dirty.current.add(key);else dirty.current.delete(key);setDirtyRevision(value=>value+1);},[]);
 function openLibrary(libraryTab:ChatLibraryTab,directory?:string){setLibraryMounted(true);setPaneMenuOpen(false);setState(current=>({...current,section:'library',libraryTab,directory:directory??current.directory}));}
 function open(tab:ChatPanelTab){if(tab.kind==='files'||tab.kind==='resources'){openLibrary(tab.kind,tab.path);return;}setPaneMenuOpen(false);setState(current=>({...current,section:'tabs',tabs:current.tabs.some(value=>value.id===tab.id)?current.tabs.map(value=>value.id===tab.id?{...value,...tab}:value):[...current.tabs,tab],selected:tab.id}));}
 function openFile(path:string,line?:number){open({id:'file:'+path,kind:'file',path,line,title:path.split(/[\\/]/).pop()});}
 function remove(key:string){if(key==='panel'){dirty.current.clear();onDirtyChange(false);onClose();return;}setState(current=>{const old=current.tabs.find(tab=>tab.id===key);if(old)closed.current.push(old);const index=current.tabs.findIndex(tab=>tab.id===key),tabs=current.tabs.filter(tab=>tab.id!==key);return {...current,tabs,selected:current.selected===key?(tabs[Math.min(index,tabs.length-1)]?.id??''):current.selected};});dirty.current.delete(key);setDirtyRevision(value=>value+1);const next=closeQueue.current.shift();if(next)queueMicrotask(()=>close(next));}
 // Bulk tab closing uses the same per-file/per-terminal protection as a
 // single close. A pending confirmation pauses the sequence instead of racing.
 function closeMany(keys:string[]){closeQueue.current=keys;const first=closeQueue.current.shift();if(first)close(first);}
 function cancelClose(){closeQueue.current=[];setPending(undefined);setTerminalClose(undefined);}
 function close(key:string){if(key==='panel'?terminals.current.size>0:terminals.current.has(key)){setTerminalClose(key);return;}closeFiles(key);}
 function closeFiles(key:string){if(key==='panel'?dirty.current.size>0:dirty.current.has(key)){setPending(key);return;}remove(key);}
 async function saveAndClose(){if(savingFiles.current)return;savingFiles.current=true;setSaving(true);try{const keys=pending==='panel'?[...dirty.current]:[pending!];for(const key of keys){if(!await handles.current.get(key)?.save())return;}const key=pending!;setPending(undefined);remove(key);}finally{savingFiles.current=false;setSaving(false);}}
 function createPage(){if(creatingPage.current||!id||!pageName.trim())return;creatingPage.current=true;setCreating(true);setError('');void chatCall<{path:string}>('file.create',{id,path:pageName.trim()}).then(file=>{setNewPage(false);openFile(file.path);}).catch(error=>setError(String(error))).finally(()=>{creatingPage.current=false;setCreating(false);});}
 function add(kind:'home'|'browser'|'terminal'|'review'|'files'){open({id:kind==='files'||kind==='review'?kind:kind+':'+crypto.randomUUID(),kind,url:kind==='browser'?'about:blank':undefined});}
 const label=(tab:ChatPanelTab)=>tab.title||t('chat.'+({home:'paneNewTab',files:'files',resources:'resources',browser:'browser',terminal:'terminal',review:'review',file:'files'}[tab.kind]));
 const library=state.section==='library',tabsVisible=visible&&!library;
 const selected=library?undefined:state.tabs.find(tab=>tab.id===state.selected);
 const paneItems:DesktopMenuItem[]=[...(!library?toolActions[state.selected]??[]:[]),
  {id:'pane.files',label:t('chat.files'),separator:true,onSelect:()=>add('files')},
  {id:'pane.resources',label:t('chat.resources'),onSelect:()=>open({id:'resources',kind:'resources'})},
  {id:'pane.terminal',label:t('chat.terminal'),disabled:!id,onSelect:()=>add('terminal')},
  {id:'pane.review',label:t('chat.review'),disabled:!id,onSelect:()=>add('review')},
  {id:'pane.reopen',label:t('chat.reopenTab'),separator:true,disabled:!closed.current.length,onSelect:()=>{const tab=closed.current.pop();if(tab)open(tab);}},
  {id:'pane.pin',label:t('chat.pinTab'),disabled:!selected,checked:!!selected?.pinned,onSelect:()=>setState(current=>({...current,tabs:current.tabs.map(tab=>tab.id===current.selected?{...tab,pinned:!tab.pinned}:tab).sort((a,b)=>Number(!!b.pinned)-Number(!!a.pinned))}))},
  {id:'pane.dock',label:t(dock==='right'?'chat.dockBottom':'chat.dockRight'),onSelect:onDock},
 ];
 function tool(kind:string){if(kind==='resources')open({id:'resources',kind:'resources'});else if(kind==='new-page')setNewPage(true);else if(kind==='review'||kind==='terminal'||kind==='files'||kind==='browser'||kind==='home')add(kind);}
 return <aside className={'cindy-chat-ui chat-side-panel'+(fullscreen?' chat-side-panel--fullscreen':'')} data-panel-drag-root="right-tabs" data-pane-collapsed={visible?undefined:''} aria-label={t('chat.details')} onKeyDown={event=>{
  const modifier=event.ctrlKey||event.metaKey;if(!modifier)return;
  if(event.key.toLowerCase()==='w'&&!library){event.preventDefault();close(state.selected);}
  if(event.shiftKey&&event.key.toLowerCase()==='t'){event.preventDefault();const tab=closed.current.pop();if(tab)open(tab);}
  if(event.key==='\u0060'){event.preventDefault();if(id)add('terminal');}
  if(event.shiftKey&&event.key.toLowerCase()==='g'){event.preventDefault();if(id)add('review');}
  if(event.key.toLowerCase()==='p'){event.preventDefault();add('files');}
 }}>
  <div className="chat-panel-section-switch"><SegmentedControl value={state.section} role="tablist" aria-label={t('chat.panelSections')} className="chat-panel-major-tabs" fullWidth height={28} optionHeight={27} options={[
   {value:'library',label:<><FolderOpen size={14} aria-hidden/><span>{t('chat.panelLibrary')}</span></>,title:t('chat.panelLibrary'),'aria-label':t('chat.panelLibrary'),id:'chat-panel-library-tab','aria-controls':'chat-panel-library'},
   {value:'tabs',label:<><PanelsTopLeft size={14} aria-hidden/><span>{t('chat.panelDocuments')}</span></>,title:t('chat.panelDocuments'),'aria-label':t('chat.panelDocuments'),id:'chat-panel-documents-tab','aria-controls':'chat-panel-documents'},
  ]} onValueChange={section=>{if(section==='library')setLibraryMounted(true);setPaneMenuOpen(false);setState(current=>({...current,section}));}}/></div>
  <div id="chat-panel-documents" role="tabpanel" aria-labelledby="chat-panel-documents-tab" hidden={library} className="chat-panel-section">
  <PanelTabs tabs={state.tabs} selected={state.selected} dirty={dirty.current} label={label} full={fullscreen} visible={tabsVisible} onSelect={selected=>setState(current=>({...current,selected}))} onClose={close} onAdd={tool} onFull={()=>setFullscreen(value=>!value)} onHide={onClose} onReorder={ids=>setState(current=>({...current,tabs:ids.map(id=>current.tabs.find(tab=>tab.id===id)!).filter(Boolean)}))} onCloseOthers={keep=>closeMany(state.tabs.filter(tab=>tab.id!==keep).map(tab=>tab.id))} onCloseAll={()=>closeMany(state.tabs.map(tab=>tab.id))} items={paneItems} onPopupOpen={setPaneMenuOpen}/>
  <div className="chat-panel-contents">{!state.tabs.length&&<PanelHome onTool={tool}/>} {state.tabs.map(tab=><div key={tab.id} id={'chat-tabpanel-'+tab.id} aria-labelledby={'chat-tab-'+tab.id} role="tabpanel" hidden={state.selected!==tab.id} className="chat-panel-content"><Suspense fallback={<p role="status">{t('chat.loadingResources')}</p>}>
   {tab.kind==='home'&&<PanelHome onTool={tool}/>}
   {tab.kind==='file'&&tab.path&&<ChatFilePreview id={id} path={tab.path} onActions={items=>registerActions(tab.id,items)} line={tab.line} onDirty={value=>changeDirty(tab.id,value)} onHandle={handle=>{if(handle)handles.current.set(tab.id,handle);else handles.current.delete(tab.id);}}/>}
   {tab.kind==='browser'&&<ChatBrowserView sessionId={id||'new-chat'} tabId={tab.id} url={tab.url||'about:blank'} active={tabsVisible&&state.selected===tab.id&&!pending&&!terminalClose&&!newPage&&!paneMenuOpen} paneItems={paneItems} onStateChange={value=>setState(current=>({...current,tabs:current.tabs.map(item=>item.id===tab.id?{...item,title:value.title||value.url,url:value.url}:item)}))} onOpenTab={url=>open({id:'browser:'+crypto.randomUUID(),kind:'browser',url})} onClose={()=>close(tab.id)} onComment={onComment} commentMarkerNumbers={commentMarkerNumbers}/>}
   {tab.kind==='terminal'&&<ChatTerminalView sessionId={id} tabId={tab.id} onActions={items=>registerActions(tab.id,items)} active={tabsVisible&&state.selected===tab.id} onRunningChange={running=>{if(terminals.current.has(tab.id)===running)return;if(running)terminals.current.add(tab.id);else terminals.current.delete(tab.id);setDirtyRevision(value=>value+1);}}/>}
   {tab.kind==='review'&&<ChatReviewView running={running} lastTurnPaths={lastTurnPaths} sessionId={id} turnId={tab.turnId} initialPath={tab.path} onOpenFile={openFile} onComment={onReviewComment}/>}
  </Suspense></div>)}</div></div>
  <div id="chat-panel-library" role="tabpanel" aria-labelledby="chat-panel-library-tab" hidden={!library} className="chat-panel-section">
   <div className="chat-library-toolbar"><SegmentedControl value={state.libraryTab} role="tablist" aria-label={t('chat.panelLibraryViews')} className="chat-library-tabs" fullWidth height={30} optionHeight={26} options={[
    {value:'files',label:t('chat.files'),id:'chat-library-files-tab','aria-controls':'chat-library-files'},
    {value:'resources',label:t('chat.libraryResources'),id:'chat-library-resources-tab','aria-controls':'chat-library-resources'},
   ]} onValueChange={openLibrary}/><div className="chat-library-actions">
    <DropdownMenu open={library&&paneMenuOpen} onOpenChange={setPaneMenuOpen}><DropdownMenuTrigger asChild><ChromeIconButton aria-label={t('chat.panelActions')}><MoreHorizontal size={15}/></ChromeIconButton></DropdownMenuTrigger><DropdownMenuContent align="end"><PanelMenuItems items={paneItems}/></DropdownMenuContent></DropdownMenu>
    <ChromeIconButton aria-label={t(fullscreen?'chat.restorePanel':'chat.expandPanel')} onClick={()=>setFullscreen(value=>!value)}>{fullscreen?<Minimize2 size={15}/>:<Maximize2 size={15}/>}</ChromeIconButton>
    <ChromeIconButton aria-label={t('chat.hideDetails')} onClick={onClose}><SidebarSimple size={15}/></ChromeIconButton>
   </div></div>
   <div className="chat-panel-contents">{libraryMounted&&<>
    <div id="chat-library-files" role="tabpanel" aria-labelledby="chat-library-files-tab" hidden={state.libraryTab!=='files'} className="chat-panel-content"><Suspense fallback={<p role="status">{t('chat.loadingResources')}</p>}><FileBrowser active={visible&&library&&state.libraryTab==='files'} id={id} initialDirectory={state.directory} onReference={onReference} onOpenFile={openFile} onOpenBrowser={url=>open({id:'browser:'+crypto.randomUUID(),kind:'browser',url})} onDirty={value=>changeDirty('files',value)} onHandle={handle=>{if(handle)handles.current.set('files',handle);else handles.current.delete('files');}}/></Suspense></div>
    <div id="chat-library-resources" role="tabpanel" aria-labelledby="chat-library-resources-tab" hidden={state.libraryTab!=='resources'} className="chat-panel-content"><ChatResourcesPanel active={visible&&library&&state.libraryTab==='resources'} id={id} onOpenFile={openFile}/></div>
   </>}</div>
  </div>
  {error&&<p role="alert" className="chat-preview-feedback">{error}</p>}
  <DocumentFeedback/><MermaidLightboxHost/><MermaidSourceEditorHost/>
  <ConfirmDialog keepOpenOnConfirm open={!!terminalClose} onOpenChange={open=>{if(!open)cancelClose();}} title={t('chat.terminalClose')} description={t('chat.terminalCloseHint')} confirmText={t('chat.terminalClose')} cancelText={t('chat.cancel')} onCancel={cancelClose} onConfirm={()=>{const key=terminalClose!;setTerminalClose(undefined);closeFiles(key);}}/>
  <ConfirmDialog keepOpenOnConfirm open={!!pending} loading={saving} onOpenChange={open=>{if(!open&&!savingFiles.current)cancelClose();}} title={t('chat.unsavedFile')} description={t('chat.unsavedFileClose')} confirmText={t('chat.saveFile')} tertiaryText={t('chat.discardFile')} cancelText={t('chat.cancel')} onCancel={cancelClose} onConfirm={()=>void saveAndClose()} onTertiary={()=>{const key=pending!;setPending(undefined);remove(key);}}/>
  <ConfirmDialog keepOpenOnConfirm open={newPage} loading={creating} onOpenChange={open=>{if(!creatingPage.current)setNewPage(open);}} title={t('chat.paneNewPage')} description={t('chat.paneNewPageHint')} confirmText={t('chat.create')} cancelText={t('chat.cancel')} confirmDisabled={!id||!pageName.trim()} onConfirm={createPage} content={<><Input aria-label={t('chat.panePageName')} value={pageName} onChange={setPageName}/>{error&&<p role="alert" className="chat-preview-feedback">{error}</p>}</>}/>
 </aside>;
}
