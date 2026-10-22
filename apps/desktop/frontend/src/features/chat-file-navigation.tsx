// Cindy MarkdownRenderer routing adapted to WinOne's scoped file-preview bridge.
import {FileTargetChip} from './cindy/FileTargetChip';
import {createContext,useContext,useEffect,useState,type MouseEvent,type ReactNode} from 'react';
import {chatCall} from '../bridge';
import {classifyMarkdownHref,looksLikeFilePath,looksLikeDirectoryPath,resolveLocalPath,resolveKnownLocalFileHref,resolveToolFilePath,type KnownLocalFileRef} from './cindy/localPathResolver';
import {inspectChatPath,type ChatFileStat} from './chat-file-access';
import type {ChatFilePreviewData} from './chat-file-preview';
import {useFileChipContextMenu} from './cindy/useFileChipContextMenu';
import {activateInlineLinkFromKeyboard} from './cindy/linkInteraction';
import {useTranslation} from '../i18n';
import {chatErrorSummary} from './chat-error';
import {openMedia,prepareMedia,registerPreviewMedia} from './cindy/media/host';

type Navigation={id:string;cwd:string;files?:KnownLocalFileRef[];onOpen?:(path:string,line?:number)=>void;onDirectory?:(path:string)=>void;onBrowser?:(url:string)=>void};
const NavigationContext=createContext<Navigation>({id:'',cwd:''});
export const useChatFileNavigation=()=>useContext(NavigationContext);
export function ChatFileNavigation({children,...value}:Partial<Navigation>&{children:ReactNode}){
 const parent=useContext(NavigationContext);
 return <NavigationContext.Provider value={{...parent,...Object.fromEntries(Object.entries(value).filter(([,item])=>item!==undefined))}}>{children}</NavigationContext.Provider>;
}

// Coordinates belong to the viewer, not the filesystem path. Handle both
// GitHub-style anchors and the native path:line[:column] form used by agents.
function target(href:string,navigation:Navigation){
 const coordinates=href.match(/(?:#L|:)([1-9]\d*)(?:(?:C|:)([1-9]\d*))?(?:-L?\d+)?$/);
 const raw=coordinates?href.slice(0,coordinates.index):href;
 let decoded=raw;try{decoded=raw.startsWith('file://')?raw:decodeURIComponent(raw);}catch{/* Literal percent signs remain part of a filename. */}
 // Attachment basenames may resolve outside CWD. Explicit absolute paths and
 // web URLs must never be redirected to a same-named attachment.
 const known=!/[\\/]/.test(decoded)&&!/^[a-z][a-z0-9+.-]*:/i.test(decoded)?resolveKnownLocalFileHref(decoded,navigation.files):null;
 return {kind:known?'text-local' as const:classifyMarkdownHref(decoded),path:known??(decoded.startsWith('file://')?resolveLocalPath(decoded,navigation.cwd):resolveToolFilePath(decoded,navigation.cwd)),line:coordinates?Number(coordinates[1]):undefined,column:coordinates?.[2]?Number(coordinates[2]):undefined};
}
export function ChatFileLink({href,children,directory,chip=false,...props}:{href?:string;children?:ReactNode;title?:string;directory?:boolean;chip?:boolean}){
 const navigation=useContext(NavigationContext),{t}=useTranslation(),[error,setError]=useState('');
 const destination=target(href??'',navigation),local=destination.kind!=='external',path=destination.path;
 const [resolved,setResolved]=useState<{path:string;file?:ChatFileStat}>();
 useEffect(()=>{
  if(!chip||!local||!navigation.id)return;
  let alive=true;setResolved(undefined);
  void inspectChatPath(navigation.id,path).then(result=>{if(alive)setResolved({path,file:result.file});}).catch(()=>{if(alive)setResolved({path});});
  return()=>{alive=false;};
 },[chip,local,navigation.id,path]);
 const report=(error:unknown)=>setError(error instanceof Error?error.message:String(error));
 const relative=path.replace(/\\/g,'/'),root=navigation.cwd.replace(/\\/g,'/').replace(/\/$/,'')+'/';
 const location=destination.line?`${relative.toLowerCase().startsWith(root.toLowerCase())?relative.slice(root.length):path}:${destination.line}${destination.column?':'+destination.column:''}`:undefined;
 async function inspect(){
  setError('');
  if(!navigation.id)return {path,directory:!!directory||destination.kind==='directory'};
  const result=await inspectChatPath(navigation.id,path);
  if(result.error)throw Error(result.error);
  if(result.missing||!result.file)throw Error(t('chat.fileLinkMissing'));
  return result.file;
 }
 async function open(source=false){
  const file=await inspect();
  if(file.directory){if(!navigation.onDirectory)throw Error(t('chat.fileLinkUnavailable'));navigation.onDirectory(file.path);}
  else {if(!navigation.onOpen)throw Error(t('chat.fileLinkUnavailable'));navigation.onOpen(file.path,source?destination.line??1:destination.line);}
 }
 async function browser(){
  await inspect();
  const file=await chatCall<ChatFilePreviewData>('file.preview',{id:navigation.id,path});
  if(!navigation.onBrowser)throw Error(t('chat.fileLinkUnavailable'));
  navigation.onBrowser(file.url);
 }
 const menu=useFileChipContextMenu({id:navigation.id,fileName:path,getAbsPath:()=>path,sidebarFileBrowserKind:directory||destination.kind==='directory'?'directory':'file',onOpenInSidebar:navigation.onOpen||navigation.onDirectory?()=>open():undefined,onOpenInBrowser:navigation.onBrowser?browser:undefined,onViewSource:()=>open(true),location,onError:report});
 if(!href)return <span>{children}</span>;
 const activate=(event:MouseEvent<HTMLElement>)=>menu.onActivate(event,()=>open());
 if(chip&&local&&navigation.id){
  if(resolved?.path!==path||!resolved.file)return <span>{children}</span>;
  return <span className="chat-file-link"><FileTargetChip {...props} path={path} directory={resolved.file.directory} onClick={activate} onAuxClick={event=>{if(event.button===1)activate(event);}} onContextMenu={menu.onContextMenu}>{children}</FileTargetChip>{menu.menu}{error&&<span role="alert" className="chat-file-link-error" title={error}>{chatErrorSummary(error,t)}</span>}</span>;
 }
 return <span className="chat-file-link"><a {...props} href={href} onKeyDown={local?activateInlineLinkFromKeyboard:undefined} onContextMenu={local?menu.onContextMenu:undefined} onAuxClick={event=>{if(local&&event.button===1)menu.onActivate(event,()=>open());}} onClick={event=>{
  if(href.startsWith('#'))return;
  setError('');
  if(local){menu.onActivate(event,()=>open());return;}
  event.preventDefault();void chatCall('link.open',{url:href}).catch(report);
 }}>{children}</a>{local&&menu.menu}{error&&<span role="alert" className="chat-file-link-error" title={error}>{chatErrorSummary(error,t)}</span>}</span>;
}
export function ChatFileImage({src,alt}:{src?:string;alt?:string}){
 const navigation=useContext(NavigationContext),[file,setFile]=useState<ChatFilePreviewData>(),[error,setError]=useState('');
 const local=!!src&&!/^(?:https?:|data:)/i.test(src),destination=src?target(src,navigation):undefined;
 useEffect(()=>{if(!src||!navigation.id)return;let alive=true;setFile(undefined);setError('');const request=local?chatCall<ChatFilePreviewData>('file.preview',{id:navigation.id,path:destination!.path}).then(value=>{registerPreviewMedia(navigation.id,value);return value;}):prepareMedia(src,navigation.id).then(value=>({path:value.path??src,name:alt||value.name,kind:'image' as const,size:value.size,modified:0,revision:'',mimeType:value.mimeType,url:value.url}));void request.then(value=>{if(alive)setFile(value);}).catch(error=>{if(alive)setError(String(error));});return()=>{alive=false;};},[navigation.id,local,destination?.path,src]);
 if(!navigation.id)return <img src={src} alt={alt}/>;
 if(error)return <span role="alert">{alt}: {error}</span>;
 return file?<button type="button" className="chat-inline-image" onClick={event=>{document.querySelectorAll('[data-gallery-active]').forEach(node=>node.removeAttribute('data-gallery-active'));event.currentTarget.querySelector('img')?.setAttribute('data-gallery-active','1');openMedia({id:navigation.id,src:file.url,alt:alt||file.name,gallery:!event.currentTarget.closest('.chat-side-panel')});}} aria-label={alt||file.name}><img src={file.url} alt={alt||file.name} data-gallery-src={file.url}/></button>:<span>{alt}</span>;
}

// Cindy turns unambiguous inline paths into links only after existence is
// confirmed. Identifiers and fenced commands remain ordinary code.
export function ChatInlineCode({children,className,inline=!className}:{children?:ReactNode;className?:string;inline?:boolean}){
 const navigation=useContext(NavigationContext),text=typeof children==='string'?children:'',destination=target(text,navigation),[file,setFile]=useState<ChatFileStat>();
 const candidate=inline&&!/[\r\n]/.test(text)&&destination.kind!=='external'&& (looksLikeFilePath(text.replace(/(?:#L|:)[1-9]\d*(?:(?:C|:)[1-9]\d*)?(?:-L?\d+)?$/,''))||looksLikeDirectoryPath(text)||/^file:\/\//i.test(text)) &&!!navigation.id;
 useEffect(()=>{let alive=true;setFile(undefined);if(candidate)void inspectChatPath(navigation.id,destination.path).then(result=>{if(alive)setFile(result.file);}).catch(()=>{/* An unconfirmed inline path stays plain code. */});return()=>{alive=false;};},[candidate,navigation.id,destination.path]);
 return file?<ChatFileLink chip href={text} directory={file.directory}>{children}</ChatFileLink>:<code className={className}>{children}</code>;
}
