// Cindy MarkdownRenderer routing adapted to WinOne's scoped file-preview bridge.
import {createContext,useContext,useEffect,useState,type ReactNode} from 'react';
import {chatCall} from '../bridge';
import {classifyMarkdownHref,looksLikeFilePath,looksLikeDirectoryPath,resolveLocalPath,resolveKnownLocalFileHref,resolveToolFilePath,type KnownLocalFileRef} from './cindy/localPathResolver';
import {inspectChatPath,type ChatFileStat} from './chat-file-access';
import type {ChatFilePreviewData} from './chat-file-preview';
import {openMedia,prepareMedia,registerPreviewMedia} from './cindy/media/host';

type Navigation={id:string;cwd:string;files?:KnownLocalFileRef[];onOpen?:(path:string,line?:number)=>void;onDirectory?:(path:string)=>void};
const NavigationContext=createContext<Navigation>({id:'',cwd:''});
export function ChatFileNavigation({children,...value}:Partial<Navigation>&{children:ReactNode}){
 const parent=useContext(NavigationContext);
 return <NavigationContext.Provider value={{...parent,...Object.fromEntries(Object.entries(value).filter(([,item])=>item!==undefined))}}>{children}</NavigationContext.Provider>;
}

// Coordinates belong to the viewer, not the filesystem path. Handle both
// GitHub-style anchors and the native path:line[:column] form used by agents.
function target(href:string,navigation:Navigation){
 const coordinates=href.match(/(?:#L|:)([1-9]\d*)(?:(?:C|:)[1-9]\d*)?(?:-L?\d+)?$/);
 const raw=coordinates?href.slice(0,coordinates.index):href;
 let decoded=raw;try{decoded=raw.startsWith('file://')?raw:decodeURIComponent(raw);}catch{/* Literal percent signs remain part of a filename. */}
 const known=resolveKnownLocalFileHref(decoded,navigation.files);
 return {kind:classifyMarkdownHref(decoded),path:known??(decoded.startsWith('file://')?resolveLocalPath(decoded,navigation.cwd):resolveToolFilePath(decoded,navigation.cwd)),line:coordinates?Number(coordinates[1]):undefined};
}
export function ChatFileLink({href,children,directory,...props}:{href?:string;children?:ReactNode;title?:string;directory?:boolean}){
 const navigation=useContext(NavigationContext),[error,setError]=useState('');
 if(!href)return <span>{children}</span>;
 const destination=target(href,navigation);
 return <span><a {...props} href={href} onClick={event=>{
  if(href.startsWith('#'))return;
  event.preventDefault();setError('');
  if((directory||destination.kind==='directory')&&navigation.onDirectory){navigation.onDirectory(destination.path);return;}
  if(destination.kind!=='external'&&navigation.onOpen){
   if(navigation.id&&navigation.onDirectory){
    void inspectChatPath(navigation.id,destination.path).then(result=>{
     if(result.error)throw Error(result.error);
     if(result.missing)throw Error('文件位置已不存在');
     if(result.file?.directory)navigation.onDirectory?.(result.file.path);
     else navigation.onOpen?.(result.file?.path??destination.path,destination.line);
    }).catch(error=>setError(String(error)));
   }else navigation.onOpen(destination.path,destination.line);
   return;
  }
  void chatCall('link.open',{url:href}).catch(error=>setError(String(error)));
 }}>{children}</a>{error&&<span role="alert">{error}</span>}</span>;
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
 const candidate=inline&&!/[\r\n]/.test(text)&&destination.kind!=='external'&& (looksLikeFilePath(text)||looksLikeDirectoryPath(text)) &&!!navigation.id;
 useEffect(()=>{let alive=true;setFile(undefined);if(candidate)void inspectChatPath(navigation.id,destination.path).then(result=>{if(alive)setFile(result.file);}).catch(()=>{/* An unconfirmed inline path stays plain code. */});return()=>{alive=false;};},[candidate,navigation.id,destination.path]);
 return <code className={className}>{file?<ChatFileLink href={text} directory={file.directory}>{children}</ChatFileLink>:children}</code>;
}
