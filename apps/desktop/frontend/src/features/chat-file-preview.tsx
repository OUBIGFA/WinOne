// FileBodyView/TextLightbox adapter for WinOne. Based on Cindy a152497a,
// Copyright 2026 XD Inc., Apache-2.0. Host file I/O replaces Electron IPC.
import {lazy,Suspense,useEffect,useRef,useState} from 'react';
import {FileDocument} from './cindy/editor/FileDocument';
import {ImagePreview} from './cindy/file-view/ImagePreview';
import {registerPreviewMedia} from './cindy/media/host';
import type {FileEditorHandle} from './cindy/editor/FileEditor';
import {createMarkdownAutosave,normalizeBaseline} from './cindy/editor/markdownAutosave';
import {RefreshCw} from 'lucide-react';
import {ChromeIconButton} from './cindy/panel-ui/ChromeIconButton';
import {Spinner} from './cindy/panel-ui/spinner';
import {OpenInSystemActions} from './cindy/file-view/OpenInSystemActions';
import {ChatFileNavigation} from './chat-file-navigation';
import type {DesktopMenuItem} from 'chatgpt-desktop-kit';
import {inspectChatPath} from './chat-file-access';
import {chatCall} from '../bridge';
import {copyText} from './clipboard';
import {useTranslation} from '../i18n';
import {formatBytes,formatMtime} from './cindy/fileMeta';
const ChatPDFPreview=lazy(()=>import('./chat-pdf-preview').then(module=>({default:module.ChatPDFPreview})));

export interface ChatFilePreviewData{path:string;name:string;size:number;modified:number;kind:'text'|'image'|'video'|'audio'|'pdf'|'binary'|'oversize';mimeType:string;revision:string;url:string;readOnly?:boolean}
export interface ChatFileHandle{save:()=>Promise<boolean>;dirty:()=>boolean}

export function ChatFilePreview({id,path,line,active=true,onDirty,onHandle,onActions}:{id:string;path:string;line?:number;active?:boolean;onDirty:(dirty:boolean)=>void;onHandle:(handle:ChatFileHandle|null)=>void;onActions?:(items:DesktopMenuItem[])=>void}){
 const {t}=useTranslation();const [file,setFile]=useState<ChatFilePreviewData>(),[text,setText]=useState(''),[saved,setSaved]=useState(''),[error,setError]=useState(''),[loading,setLoading]=useState(true),[busy,setBusy]=useState(false),[rendered,setRendered]=useState(line===undefined&&/\.(md|markdown|html?)$/i.test(path)),[zoom,setZoom]=useState(1),[rotation,setRotation]=useState(0);
 const current=useRef({text,saved,file});current.current={text,saved,file};
 const [refreshError,setRefreshError]=useState('');
 const loadSequence=useRef(0);
 const editor=useRef<FileEditorHandle>(null),saving=useRef(false),saveRequest=useRef<Promise<boolean>|null>(null),[scrollLine,setScrollLine]=useState(line);
 const isMarkdown=/\.(md|markdown)$/i.test(path);
 const autosave=useRef(createMarkdownAutosave({delayMs:900,isSaving:()=>saving.current,save:()=>void save()}));
 useEffect(()=>()=>autosave.current.cancel(),[]);
 function changeText(value:string){setText(value);if(isMarkdown&&!file?.readOnly){if(value!==normalizeBaseline(current.current.saved))autosave.current.schedule();else autosave.current.cancel();}}
 function toggleRendered(){setScrollLine(editor.current?.getViewportTopLine()??line);setRendered(value=>!value);}
 const [pdfActions,setPdfActions]=useState<DesktopMenuItem[]>([]);
 async function load(signal?:AbortSignal,silent=false){
  const sequence=++loadSequence.current,baseline=current.current;
  const unchanged=()=>!signal?.aborted&&sequence===loadSequence.current&&(!silent||!saving.current&&current.current.file===baseline.file&&current.current.saved===baseline.saved&&current.current.text===current.current.saved);
  if(!silent)setLoading(true);
  try{
   const next=await chatCall<ChatFilePreviewData>('file.preview',{id,path});if(!unchanged())return;
   let content='';
   if(next.kind==='text'){
    const response=await fetch(next.url,{signal,cache:'no-store'});
    if(!response.ok)throw Error(t('chat.fileReadFailed'));
    content=await response.text();
   }
   if(!unchanged())return;
   if(next.kind==='image')registerPreviewMedia(id,next);
   // Publish the baseline first: the retained editor's onChange callback from
   // this external update must not schedule a write back to disk.
   current.current={file:next,text:content,saved:content};
   setFile(next);setText(content);setSaved(content);setError('');
  }catch(error){if(unchanged())setError(String(error));}
  finally{if(!signal?.aborted&&sequence===loadSequence.current&&!silent)setLoading(false);}
 }
 // Explicit save, close and Markdown autosave share one write sequence.
 async function save():Promise<boolean>{
  autosave.current.cancel();
  if(saveRequest.current){if(!await saveRequest.current)return false;return save();}
  const request=performSave();saveRequest.current=request;
  try{return await request;}finally{if(saveRequest.current===request)saveRequest.current=null;}
 }
 async function performSave(){const state=current.current;if(!state.file||state.file.readOnly||state.text===state.saved)return true;saving.current=true;setBusy(true);setError('');try{const next=await chatCall<ChatFilePreviewData>('file.save',{id,path,revision:state.file.revision,text:state.text});const updated={...next,url:state.file.url};current.current={...current.current,saved:state.text,file:updated};setSaved(state.text);setFile(updated);return true;}catch(error){setError(String(error));return false;}finally{saving.current=false;setBusy(false);}}
 useEffect(()=>{const abort=new AbortController();current.current={text:'',saved:'',file:undefined};setFile(undefined);setText('');setSaved('');void load(abort.signal);return()=>{abort.abort();loadSequence.current++;};},[id,path]);
 // An explicit line opens source; reopening the same Markdown chip without a
 // line returns to its document view instead of retaining an earlier line jump.
 useEffect(()=>{setRendered(line===undefined&&/\.(md|markdown|html?)$/i.test(path));},[id,path,line]);
 const refreshRef=useRef(load);refreshRef.current=load;
 useEffect(()=>{
  if(!active)return;
  const abort=new AbortController();let reading=false;
  const refresh=async(force=false)=>{
   if(reading||document.visibilityState==='hidden'||saving.current||!current.current.file)return;
   reading=true;
   try{
    const state=current.current;
    // Stat only the visible file, not its contents or the entire workspace.
    // A dirty buffer stays intact; revision-checked saving reports conflicts.
    if(state.text!==state.saved)return;
    const stat=await inspectChatPath(id,path,true);
    if(abort.signal.aborted)return;
    if(stat.error||stat.missing)throw Error(stat.error||t('chat.fileLinkMissing'));
    setRefreshError('');
    if(force||stat.file&&(stat.file.modified!==state.file!.modified||stat.file.size!==state.file!.size))await refreshRef.current(abort.signal,true);
   }catch(error){if(!abort.signal.aborted)setRefreshError(String(error));}
   finally{reading=false;}
  };
  const changed=(event:Event)=>{if((event as CustomEvent<{id:string}>).detail?.id===id)void refresh(true);};
  const focus=()=>void refresh();
  const interval=window.setInterval(focus,1000);
  window.addEventListener('focus',focus);document.addEventListener('visibilitychange',focus);window.addEventListener('winone:chat-files-changed',changed);
  void refresh();
  return()=>{abort.abort();clearInterval(interval);window.removeEventListener('focus',focus);document.removeEventListener('visibilitychange',focus);window.removeEventListener('winone:chat-files-changed',changed);};
 },[id,path,active,t]);
 useEffect(()=>{onDirty(text!==saved);},[text,saved,onDirty]);
 useEffect(()=>{onHandle({save,dirty:()=>current.current.text!==current.current.saved});return()=>onHandle(null);},[id,path]);
 const copy=async(value:string)=>{try{await copyText(value);}catch(error){setError(String(error));}};
 const action=async(method:string)=>{try{await chatCall(method,{id,path});}catch(error){setError(String(error));}};
 const canRender=/\.(md|markdown|html?)$/i.test(path),isHTML=/\.html?$/i.test(path);
 const actionsCallback=useRef(onActions);actionsCallback.current=onActions;
 useEffect(()=>{
  const items:DesktopMenuItem[]=[];
  if(file)items.push({id:'file.metadata',label:`${file.name} · ${formatBytes(file.size)} · ${formatMtime(file.modified)}`,heading:true});
  if(canRender)items.push({id:'file.render',label:t(rendered?'chat.fileSource':'chat.fileRendered'),onSelect:toggleRendered});
  if(file?.kind==='text'){if(!file.readOnly)items.push({id:'file.save',label:t('chat.saveFile'),shortcut:'Ctrl+S',disabled:busy||text===saved,onSelect:()=>void save()});items.push({id:'file.copy',label:t('chat.copy'),onSelect:()=>void copy(current.current.text)});}
  items.push({id:'file.refresh',label:t('chat.refresh'),disabled:loading||text!==saved,onSelect:()=>void load()},{id:'file.path',label:t('chat.copyPath'),onSelect:()=>void copy(path)},{id:'file.reveal',label:t('chat.revealFile'),onSelect:()=>void action('file.reveal')},{id:'file.open',label:t('chat.openSystem'),onSelect:()=>void action('file.open')});
  if(file?.kind==='image')items.push({id:'image.scale',label:`${Math.round(zoom*100)}%`,heading:true,separator:true},{id:'image.in',label:t('chat.zoomIn'),onSelect:()=>setZoom(value=>Math.min(10,value*1.25))},{id:'image.out',label:t('chat.zoomOut'),onSelect:()=>setZoom(value=>Math.max(.1,value/1.25))},{id:'image.fit',label:t('chat.fitImage'),onSelect:()=>{setZoom(1);setRotation(0);}},{id:'image.rotate',label:t('chat.rotateImage'),onSelect:()=>setRotation(value=>value+90)});
  actionsCallback.current?.([...items,...pdfActions]);return()=>actionsCallback.current?.([]);
 },[file,loading,busy,text===saved,rendered,zoom,pdfActions,t]);
 return <ChatFileNavigation id={id} cwd={path.replace(/[\\/][^\\/]+$/,'')}><section className="chat-file-preview" aria-label={file?.name??path} onKeyDown={event=>{if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='s'){event.preventDefault();void save();}}}>

  {(error||refreshError)&&<p role="alert" className="chat-preview-feedback text-[var(--text-danger)]">{error||refreshError}</p>}
  {file&&file.kind!=='text'&&<div className="chat-file-view-toolbar flex shrink-0 items-center gap-2 px-3 py-1"><span className="chat-file-view-name min-w-0 flex-1 truncate text-13" title={file.path}>{file.name}</span><ChromeIconButton aria-label={t('chat.refresh')} disabled={loading||text!==saved} onClick={()=>void load()}><RefreshCw size={14}/></ChromeIconButton></div>}
  {loading?<div role="status" className="flex items-center gap-2 p-3 text-13"><Spinner size={14}/>{t('chat.loadingResources')}</div>:file&&<>
   {file.kind==='text'&&(rendered&&isHTML?<iframe className="chat-preview-document" sandbox="allow-scripts" src={file.url} title={file.name}/>:<FileDocument id={id} path={path} value={text} markdown={isMarkdown} rendered={rendered} readOnly={file.readOnly} busy={busy} dirty={text!==saved} line={scrollLine} editorRef={editor} onChange={changeText} onSave={save} onToggleMode={toggleRendered}/>)}
   {file.kind==='image'&&<ImagePreview workdir={file.path.replace(/[\\/][^\\/]+$/,'')} relPath={file.name} url={file.url} size={file.size} mtimeMs={file.modified} sessionId={id} zoom={zoom} rotation={rotation}/>}
   {file.kind==='video'&&<video className="chat-media-preview" src={file.url} controls onError={()=>setError(t('chat.fileReadFailed'))}/>}
   {file.kind==='audio'&&<audio className="chat-media-preview" src={file.url} controls onError={()=>setError(t('chat.fileReadFailed'))}/>}
   {file.kind==='pdf'&&<Suspense fallback={<Spinner/>}><ChatPDFPreview url={file.url} onActions={setPdfActions}/></Suspense>}
   {['binary','oversize'].includes(file.kind)&&<div className="flex flex-1 flex-col items-center justify-center gap-4 p-5 text-center text-13"><p>{t(file.kind==='oversize'?'chat.previewOversize':'chat.previewUnsupported')}</p><OpenInSystemActions absPath={path} folderPath={path.replace(/[\\/][^\\/]+$/,'')}/></div>}
  </>}
 </section></ChatFileNavigation>;
}
