// FileBodyView/TextLightbox adapter for WinOne. Based on Cindy a152497a,
// Copyright 2026 XD Inc., Apache-2.0. Host file I/O replaces Electron IPC.
import {lazy,Suspense,useEffect,useMemo,useRef,useState} from 'react';
import {EditorView} from '@uiw/react-codemirror';
import {ChatCodeEditor} from './chat-code-editor';
import {javascript} from '@codemirror/lang-javascript';
import {json} from '@codemirror/lang-json';
import {markdown} from '@codemirror/lang-markdown';
import {python} from '@codemirror/lang-python';
import {rust} from '@codemirror/lang-rust';
import {html} from '@codemirror/lang-html';
import {css} from '@codemirror/lang-css';
import {ExternalLink,CodeXml,Eye,Save,RefreshCw} from 'lucide-react';
import {PageToolbar} from 'chatgpt-desktop-kit/components/layout';
import {ChatFileNavigation} from './chat-file-navigation';
import {ChatPanelButton as Button,useChatTheme} from './chat-desktop-kit';
import {EmptyState,InlineNotice} from 'chatgpt-desktop-kit/components/feedback';
import type {DesktopMenuItem} from 'chatgpt-desktop-kit';
import {chatCall} from '../bridge';
import {copyText} from './clipboard';
import {useTranslation} from '../i18n';
import {ChatMarkdown} from './chat-messages';
import {formatBytes,formatMtime} from './cindy/fileMeta';
const ChatPDFPreview=lazy(()=>import('./chat-pdf-preview').then(module=>({default:module.ChatPDFPreview})));

export interface ChatFilePreviewData{path:string;name:string;size:number;modified:number;kind:'text'|'image'|'video'|'audio'|'pdf'|'binary'|'oversize';mimeType:string;revision:string;url:string;readOnly?:boolean}
export interface ChatFileHandle{save:()=>Promise<boolean>;dirty:()=>boolean}

export function ChatFilePreview({id,path,line,onDirty,onHandle,onActions}:{id:string;path:string;line?:number;onDirty:(dirty:boolean)=>void;onHandle:(handle:ChatFileHandle|null)=>void;onActions?:(items:DesktopMenuItem[])=>void}){
 const {t}=useTranslation();const [file,setFile]=useState<ChatFilePreviewData>(),[text,setText]=useState(''),[saved,setSaved]=useState(''),[error,setError]=useState(''),[loading,setLoading]=useState(true),[busy,setBusy]=useState(false),[rendered,setRendered]=useState(line===undefined&&/\.(md|markdown|html?)$/i.test(path)),[zoom,setZoom]=useState(1),[rotation,setRotation]=useState(0);
 const current=useRef({text,saved,file});current.current={text,saved,file};
 const theme=useChatTheme();
 const [pdfActions,setPdfActions]=useState<DesktopMenuItem[]>([]);
 const extensions=useMemo(()=>{const ext=path.split('.').pop()?.toLowerCase();return [EditorView.lineWrapping,...(ext==='json'?[json()]:ext==='py'?[python()]:ext==='rs'?[rust()]:ext==='css'?[css()]:ext==='html'||ext==='htm'?[html()]:ext==='md'||ext==='markdown'?[markdown()]:['ts','tsx','js','jsx','mjs','cjs'].includes(ext??'')?[javascript({typescript:ext?.startsWith('ts'),jsx:ext?.endsWith('x')})]:[])];},[path]);
 async function load(signal?:AbortSignal){setLoading(true);setError('');try{const next=await chatCall<ChatFilePreviewData>('file.preview',{id,path});if(signal?.aborted)return;let content='';if(next.kind==='text'){const response=await fetch(next.url,{signal});if(!response.ok)throw Error(t('chat.fileReadFailed'));content=await response.text();}if(signal?.aborted)return;setFile(next);setText(content);setSaved(content);}catch(error){if(!signal?.aborted)setError(String(error));}finally{if(!signal?.aborted)setLoading(false);}}
 async function save(){const state=current.current;if(!state.file||state.file.readOnly||state.text===state.saved)return true;setBusy(true);setError('');try{const next=await chatCall<ChatFilePreviewData>('file.save',{id,path,revision:state.file.revision,text:state.text});setSaved(state.text);setFile(previous=>({...next,url:previous?.url??''}));return true;}catch(error){setError(String(error));return false;}finally{setBusy(false);}}
 useEffect(()=>{const abort=new AbortController();void load(abort.signal);return()=>abort.abort();},[id,path]);
 useEffect(()=>{if(line!==undefined)setRendered(false);},[line]);
 useEffect(()=>{const changed=(event:Event)=>{if((event as CustomEvent<{id:string}>).detail?.id===id&&current.current.text===current.current.saved)void load();};window.addEventListener('winone:chat-files-changed',changed);return()=>window.removeEventListener('winone:chat-files-changed',changed);},[id,path]);
 useEffect(()=>{onDirty(text!==saved);},[text,saved,onDirty]);
 useEffect(()=>{onHandle({save,dirty:()=>current.current.text!==current.current.saved});return()=>onHandle(null);},[id,path]);
 const copy=async(value:string)=>{try{await copyText(value);}catch(error){setError(String(error));}};
 const action=async(method:string)=>{try{await chatCall(method,{id,path});}catch(error){setError(String(error));}};
 const canRender=/\.(md|markdown|html?)$/i.test(path),isHTML=/\.html?$/i.test(path);
 const actionsCallback=useRef(onActions);actionsCallback.current=onActions;
 useEffect(()=>{
  const items:DesktopMenuItem[]=[];
  if(file)items.push({id:'file.metadata',label:`${file.name} · ${formatBytes(file.size)} · ${formatMtime(file.modified)}`,heading:true});
  if(canRender)items.push({id:'file.render',label:t(rendered?'chat.fileSource':'chat.fileRendered'),onSelect:()=>setRendered(value=>!value)});
  if(file?.kind==='text'){if(!file.readOnly)items.push({id:'file.save',label:t('chat.saveFile'),shortcut:'Ctrl+S',disabled:busy||text===saved,onSelect:()=>void save()});items.push({id:'file.copy',label:t('chat.copy'),onSelect:()=>void copy(current.current.text)});}
  items.push({id:'file.refresh',label:t('chat.refresh'),disabled:loading||text!==saved,onSelect:()=>void load()},{id:'file.path',label:t('chat.copyPath'),onSelect:()=>void copy(path)},{id:'file.reveal',label:t('chat.revealFile'),onSelect:()=>void action('file.reveal')},{id:'file.open',label:t('chat.openSystem'),onSelect:()=>void action('file.open')});
  if(file?.kind==='image')items.push({id:'image.scale',label:`${Math.round(zoom*100)}%`,heading:true,separator:true},{id:'image.in',label:t('chat.zoomIn'),onSelect:()=>setZoom(value=>Math.min(10,value*1.25))},{id:'image.out',label:t('chat.zoomOut'),onSelect:()=>setZoom(value=>Math.max(.1,value/1.25))},{id:'image.fit',label:t('chat.fitImage'),onSelect:()=>{setZoom(1);setRotation(0);}},{id:'image.rotate',label:t('chat.rotateImage'),onSelect:()=>setRotation(value=>value+90)});
  actionsCallback.current?.([...items,...pdfActions]);return()=>actionsCallback.current?.([]);
 },[file,loading,busy,text===saved,rendered,zoom,pdfActions,t]);
 return <ChatFileNavigation id={id} cwd={path.replace(/[\\/][^\\/]+$/,'')}><section className="chat-file-preview" aria-label={file?.name??path} onKeyDown={event=>{if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='s'){event.preventDefault();void save();}}}>

  {error&&<InlineNotice tone="danger" title={error}/>}
  {file&&<PageToolbar className="chat-file-view-toolbar"><span className="chat-file-view-name" title={file.path}>{file.name}</span>{canRender&&<Button icon={rendered?CodeXml:Eye} onClick={()=>setRendered(value=>!value)}>{t(rendered?'chat.fileSource':'chat.fileRendered')}</Button>}{file.kind==='text'&&!file.readOnly&&<Button size="icon" icon={Save} aria-label={t('chat.saveFile')} disabled={busy||text===saved} onClick={()=>void save()}/>}<Button size="icon" icon={RefreshCw} aria-label={t('chat.refresh')} disabled={loading||text!==saved} onClick={()=>void load()}/></PageToolbar>}
  {loading?<InlineNotice title={t('chat.loadingResources')}/>:file&&<>
   {file.kind==='text'&&(rendered&&canRender?(isHTML?<iframe className="chat-preview-document" sandbox="allow-scripts" src={file.url} title={file.name}/>:<div className="chat-preview-markdown"><ChatMarkdown text={text} onCopy={value=>void copy(value)}/></div>):<ChatCodeEditor value={text} readOnly={file.readOnly} editable={!file.readOnly} theme={theme} line={line} extensions={extensions} basicSetup={{lineNumbers:true,foldGutter:true,highlightActiveLine:true,searchKeymap:true}} onChange={setText}/>) }
   {file.kind==='image'&&<div className="chat-image-preview"><img src={file.url} alt={file.name} onError={()=>setError(t('chat.fileReadFailed'))} style={{transform:`scale(${zoom}) rotate(${rotation}deg)`}}/></div>}
   {file.kind==='video'&&<video className="chat-media-preview" src={file.url} controls onError={()=>setError(t('chat.fileReadFailed'))}/>}
   {file.kind==='audio'&&<audio className="chat-media-preview" src={file.url} controls onError={()=>setError(t('chat.fileReadFailed'))}/>}
   {file.kind==='pdf'&&<Suspense fallback={<InlineNotice title={t('chat.loadingResources')}/>}><ChatPDFPreview url={file.url} onActions={setPdfActions}/></Suspense>}
   {['binary','oversize'].includes(file.kind)&&<EmptyState title={t(file.kind==='oversize'?'chat.previewOversize':'chat.previewUnsupported')} action={<Button icon={ExternalLink} onClick={()=>void action('file.open')}>{t('chat.openSystem')}</Button>}/>}
  </>}
 </section></ChatFileNavigation>;
}
