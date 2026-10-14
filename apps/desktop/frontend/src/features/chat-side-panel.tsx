// Cindy right-sidebar tab/plugin pattern, adapted to WinOne's shared desktop.
// Contents are kept mounted across tab changes; native surfaces receive active
// explicitly so hidden browser/terminal tabs retain state without covering UI.
import {lazy,Suspense,useCallback,useEffect,useRef,useState} from 'react';
import {Input,type DesktopMenuItem} from 'chatgpt-desktop-kit';
import {UpstreamPaneTabs,UpstreamBrowserBar,UpstreamNewTab} from './chat-upstream-pane';
import {chatCall} from '../bridge';
import {ChatPanelButton as Button} from './chat-desktop-kit';
import {Dialog} from 'chatgpt-desktop-kit/components/overlays';
import {InlineNotice} from 'chatgpt-desktop-kit/components/feedback';


import {useTranslation} from '../i18n';
import {ChatInspector} from './chat-inspector';
import type {ChatFileHandle} from './chat-file-preview';
const ChatFilePreview=lazy(()=>import('./chat-file-preview').then(module=>({default:module.ChatFilePreview})));
const ChatBrowserView=lazy(()=>import('./chat-browser').then(module=>({default:module.ChatBrowserView})));
const ChatTerminalView=lazy(()=>import('./chat-terminal').then(module=>({default:module.ChatTerminalView})));
const ChatReviewView=lazy(()=>import('./chat-review').then(module=>({default:module.ChatReviewView})));
import {restorePanelTabs,type ChatPanelTab} from './chat-panel-state';
import type {ChatBrowserAnnotation} from './chat-types';
import './chat-side-panel.css';

export function ChatSidePanel({id,initialTab,initialTabRequest=0,initialDirectory,initialFile,initialLine,initialReview,onClose,onReference,onComment,onReviewComment,commentMarkerNumbers,onDirtyChange,onDock,dock}:{id:string;initialTab:'home'|'files'|'resources'|'review';initialTabRequest?:number;initialFile?:string;initialLine?:number;initialDirectory?:string;initialReview?:{turnId:string;path?:string};onClose:()=>void;onReference:(path:string)=>void;onComment:(item:ChatBrowserAnnotation)=>Promise<void>;commentMarkerNumbers:number[];onReviewComment:(text:string)=>void;onDirtyChange:(value:boolean)=>void;onDock:()=>void;dock:'right'|'bottom'}){
 const {t}=useTranslation();const [state,setState]=useState(()=>restorePanelTabs(id)),[fullscreen,setFullscreen]=useState(false),[pending,setPending]=useState<string>(),[error,setError]=useState('');
 const handles=useRef(new Map<string,ChatFileHandle>()),dirty=useRef(new Set<string>()),closed=useRef<ChatPanelTab[]>([]);
 const [dirtyRevision,setDirtyRevision]=useState(0),[terminalClose,setTerminalClose]=useState<string>(),[newPage,setNewPage]=useState(false),[pageName,setPageName]=useState('Untitled.md'),[creating,setCreating]=useState(false),[paneMenuOpen,setPaneMenuOpen]=useState(false);
 const terminals=useRef(new Set<string>());
 const [toolActions,setToolActions]=useState<Record<string,DesktopMenuItem[]>>({});
 const registerActions=useCallback((key:string,items:DesktopMenuItem[])=>setToolActions(current=>({...current,[key]:items})),[]);
 useEffect(()=>{if(initialTab==='files'&&initialDirectory){open({id:'files',kind:'files',path:initialDirectory});return;}if(initialTab==='review'&&initialReview){open({id:'review:'+initialReview.turnId,kind:'review',turnId:initialReview.turnId,path:initialReview.path,title:t('chat.turnReview')});return;}setState(current=>current.tabs.some(tab=>tab.id===initialTab)?{...current,selected:initialTab}:{tabs:[...current.tabs,{id:initialTab,kind:initialTab}],selected:initialTab});},[initialTab,initialTabRequest,initialReview,initialDirectory]);
 useEffect(()=>{try{localStorage.setItem(`winone.chat.panel.v1:${id}`,JSON.stringify(state));}catch(error){setError(String(error));}},[id,state]);
 useEffect(()=>{onDirtyChange(dirty.current.size>0||terminals.current.size>0);},[dirtyRevision,onDirtyChange]);
 const changeDirty=useCallback((key:string,value:boolean)=>{if(dirty.current.has(key)===value)return;if(value)dirty.current.add(key);else dirty.current.delete(key);setDirtyRevision(value=>value+1);},[]);
 function open(tab:ChatPanelTab){setState(current=>({tabs:current.tabs.some(value=>value.id===tab.id)?current.tabs.map(value=>value.id===tab.id?{...value,...tab}:value):[...current.tabs,tab],selected:tab.id}));}
 function openFile(path:string,line?:number){open({id:'file:'+path,kind:'file',path,line,title:path.split(/[\\/]/).pop()});}
 useEffect(()=>{if(initialFile)openFile(initialFile,initialLine);},[initialFile,initialLine,initialTabRequest]);
 function remove(key:string){if(key==='panel'){dirty.current.clear();onDirtyChange(false);onClose();return;}setState(current=>{const old=current.tabs.find(tab=>tab.id===key);if(old)closed.current.push(old);const index=current.tabs.findIndex(tab=>tab.id===key),tabs=current.tabs.filter(tab=>tab.id!==key);return {tabs,selected:current.selected===key?(tabs[Math.min(index,tabs.length-1)]?.id??''):current.selected};});dirty.current.delete(key);setDirtyRevision(value=>value+1);}
 function close(key:string){if(key==='panel'?terminals.current.size>0:terminals.current.has(key)){setTerminalClose(key);return;}closeFiles(key);}
 function closeFiles(key:string){if(key==='panel'?dirty.current.size>0:dirty.current.has(key)){setPending(key);return;}remove(key);}
 async function saveAndClose(){const keys=pending==='panel'?[...dirty.current]:[pending!];for(const key of keys){if(!await handles.current.get(key)?.save())return;}const key=pending!;setPending(undefined);remove(key);}
 function add(kind:'home'|'browser'|'terminal'|'review'|'files'){open({id:kind==='files'?'files':kind+':'+crypto.randomUUID(),kind,url:kind==='browser'?'about:blank':undefined});}
 function reorder(source:string,target:string){if(!source||source===target)return;setState(current=>{const tabs=[...current.tabs],index=tabs.findIndex(tab=>tab.id===source),to=tabs.findIndex(tab=>tab.id===target);if(index<0||to<0)return current;const [item]=tabs.splice(index,1);tabs.splice(to,0,item);return {...current,tabs};});}
 const label=(tab:ChatPanelTab)=>tab.title||t('chat.'+({home:'paneNewTab',files:'files',resources:'resources',browser:'browser',terminal:'terminal',review:'review',file:'files'}[tab.kind]));
 const selected=state.tabs.find(tab=>tab.id===state.selected);
 const paneItems:DesktopMenuItem[]=[...(toolActions[state.selected]??[]),
  {id:'pane.files',label:t('chat.files'),separator:true,onSelect:()=>add('files')},
  {id:'pane.resources',label:t('chat.resources'),onSelect:()=>open({id:'resources',kind:'resources'})},
  {id:'pane.terminal',label:t('chat.terminal'),disabled:!id,onSelect:()=>add('terminal')},
  {id:'pane.review',label:t('chat.review'),disabled:!id,onSelect:()=>add('review')},
  {id:'pane.reopen',label:t('chat.reopenTab'),separator:true,disabled:!closed.current.length,onSelect:()=>{const tab=closed.current.pop();if(tab)open(tab);}},
  {id:'pane.pin',label:t('chat.pinTab'),disabled:!selected,checked:!!selected?.pinned,onSelect:()=>setState(current=>({...current,tabs:current.tabs.map(tab=>tab.id===current.selected?{...tab,pinned:!tab.pinned}:tab).sort((a,b)=>Number(!!b.pinned)-Number(!!a.pinned))}))},
  {id:'pane.dock',label:t(dock==='right'?'chat.dockBottom':'chat.dockRight'),onSelect:onDock},
 ];
 function tool(kind:'review'|'terminal'|'files'|'new-page'|'resources'){if(kind==='resources')open({id:'resources',kind:'resources'});else if(kind==='new-page')setNewPage(true);else add(kind);}
 const navigate=(url:string)=>open({id:selected?.kind==='home'?selected.id:'browser:'+crypto.randomUUID(),kind:'browser',url});
 return <aside className={'client-right-panel desktop-client chat-side-panel'+(fullscreen?' chat-side-panel--fullscreen':'')} aria-label={t('chat.details')} onKeyDown={event=>{
  const modifier=event.ctrlKey||event.metaKey;if(!modifier)return;
  if(event.key.toLowerCase()==='w'){event.preventDefault();close(state.selected);}
  if(event.shiftKey&&event.key.toLowerCase()==='t'){event.preventDefault();const tab=closed.current.pop();if(tab)open(tab);}
  if(event.key==='\u0060'){event.preventDefault();if(id)add('terminal');}
  if(event.shiftKey&&event.key.toLowerCase()==='g'){event.preventDefault();if(id)add('review');}
  if(event.key.toLowerCase()==='p'){event.preventDefault();add('files');}
 }}>
  <UpstreamPaneTabs tabs={state.tabs} selected={state.selected} dirty={dirty.current} label={label} full={fullscreen} onSelect={selected=>setState(current=>({...current,selected}))} onClose={close} onAdd={()=>add('home')} onFull={()=>setFullscreen(value=>!value)} onHide={()=>close('panel')} onReorder={reorder}/>
  {selected?.kind!=='browser'&&<UpstreamBrowserBar url={selected?.kind==='home'?'':selected?.path||selected&&label(selected)||''} readOnly={selected?.kind!=='home'} onNavigate={navigate} items={paneItems} onMenuOpen={setPaneMenuOpen}/>}
  {error&&<InlineNotice tone="danger" title={error}/>}
  <div className="chat-panel-contents">{!state.tabs.length&&<UpstreamNewTab onTool={tool} suggested={[]}/>} {state.tabs.map(tab=><div key={tab.id} id={'chat-tabpanel-'+tab.id} aria-labelledby={'chat-tab-'+tab.id} role="tabpanel" hidden={state.selected!==tab.id} className="chat-panel-content"><Suspense fallback={<p role="status">{t('chat.loadingResources')}</p>}>
   {tab.kind==='home'&&<UpstreamNewTab onTool={tool} suggested={state.tabs.filter(item=>item.kind==='file'||item.kind==='browser').slice(-4).map(item=>({id:item.id,label:label(item),open:()=>setState(current=>({...current,selected:item.id}))}))}/>}
   {(tab.kind==='files'||tab.kind==='resources')&&<ChatInspector id={id} initialDirectory={tab.path} initialTab={tab.kind} onClose={()=>close('panel')} onReference={onReference} onOpenFile={openFile} embedded/>}
   {tab.kind==='file'&&tab.path&&<ChatFilePreview id={id} path={tab.path} onActions={items=>registerActions(tab.id,items)} line={tab.line} onDirty={value=>changeDirty(tab.id,value)} onHandle={handle=>{if(handle)handles.current.set(tab.id,handle);else handles.current.delete(tab.id);}}/>}
   {tab.kind==='browser'&&<ChatBrowserView sessionId={id||'new-chat'} tabId={tab.id} url={tab.url||'about:blank'} active={state.selected===tab.id&&!pending&&!paneMenuOpen} paneItems={paneItems} onStateChange={value=>setState(current=>({...current,tabs:current.tabs.map(item=>item.id===tab.id?{...item,title:value.title||value.url,url:value.url}:item)}))} onOpenTab={url=>open({id:'browser:'+crypto.randomUUID(),kind:'browser',url})} onClose={()=>close(tab.id)} onComment={onComment} commentMarkerNumbers={commentMarkerNumbers}/>}
   {tab.kind==='terminal'&&<ChatTerminalView sessionId={id} tabId={tab.id} onActions={items=>registerActions(tab.id,items)} active={state.selected===tab.id} onRunningChange={running=>{if(terminals.current.has(tab.id)===running)return;if(running)terminals.current.add(tab.id);else terminals.current.delete(tab.id);setDirtyRevision(value=>value+1);}}/>}
   {tab.kind==='review'&&<ChatReviewView sessionId={id} turnId={tab.turnId} initialPath={tab.path} onOpenFile={openFile} onComment={onReviewComment}/>}
  </Suspense></div>)}</div>
  <Dialog open={!!terminalClose} onClose={()=>setTerminalClose(undefined)} title={t('chat.terminalClose')} description={t('chat.terminalCloseHint')} footer={<><Button onClick={()=>setTerminalClose(undefined)}>{t('chat.cancel')}</Button><Button variant="primary" onClick={()=>{const key=terminalClose!;setTerminalClose(undefined);closeFiles(key);}}>{t('chat.terminalClose')}</Button></>}/>
  <Dialog open={!!pending} onClose={()=>setPending(undefined)} title={t('chat.unsavedFile')} description={t('chat.unsavedFileClose')} footer={<><Button onClick={()=>setPending(undefined)}>{t('chat.cancel')}</Button><Button onClick={()=>{const key=pending!;setPending(undefined);remove(key);}}>{t('chat.discardFile')}</Button><Button variant="primary" onClick={()=>void saveAndClose()}>{t('chat.saveFile')}</Button></>}/>
  <Dialog open={newPage} onClose={()=>{if(!creating)setNewPage(false);}} title={t('chat.paneNewPage')} description={t('chat.paneNewPageHint')} footer={<><Button disabled={creating} onClick={()=>setNewPage(false)}>{t('chat.cancel')}</Button><Button variant="primary" disabled={!id||creating||!pageName.trim()} onClick={()=>{setCreating(true);setError('');void chatCall<{path:string}>('file.create',{id,path:pageName.trim()}).then(file=>{setNewPage(false);openFile(file.path);}).catch(error=>setError(String(error))).finally(()=>setCreating(false));}}>{t('chat.create')}</Button></>}><Input aria-label={t('chat.panePageName')} value={pageName} onChange={event=>setPageName(event.target.value)}/>{error&&<InlineNotice tone="danger" title={error}/>}</Dialog>
 </aside>;
}
