// Cindy MarkdownRenderer routing adapted to WinOne's scoped file-preview bridge.
import {createContext,useContext,useEffect,useState,type ReactNode} from 'react';
import {chatCall} from '../bridge';
import {classifyMarkdownHref,looksLikeFilePath,resolveLocalPath,resolveKnownLocalFileHref,resolveToolFilePath,type KnownLocalFileRef} from './cindy/localPathResolver';
import type {ChatFilePreviewData} from './chat-file-preview';

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
export function ChatFileLink({href,children,...props}:{href?:string;children?:ReactNode;title?:string}){
 const navigation=useContext(NavigationContext),[error,setError]=useState('');
 if(!href)return <span>{children}</span>;
 const destination=target(href,navigation);
 return <span><a {...props} href={href} onClick={event=>{
  if(href.startsWith('#'))return;
  event.preventDefault();setError('');
  if(destination.kind==='directory'&&navigation.onDirectory){navigation.onDirectory(destination.path);return;}
  if(destination.kind!=='external'&&navigation.onOpen){navigation.onOpen(destination.path,destination.line);return;}
  void chatCall('link.open',{url:href}).catch(error=>setError(String(error)));
 }}>{children}</a>{error&&<span role="alert">{error}</span>}</span>;
}
export function ChatFileImage({src,alt}:{src?:string;alt?:string}){
 const navigation=useContext(NavigationContext),[file,setFile]=useState<ChatFilePreviewData>(),[error,setError]=useState('');
 const local=!!src&&!/^(?:https?:|data:)/i.test(src),destination=src?target(src,navigation):undefined;
 useEffect(()=>{if(!local||!destination||!navigation.id)return;let alive=true;setFile(undefined);setError('');void chatCall<ChatFilePreviewData>('file.preview',{id:navigation.id,path:destination.path}).then(value=>{if(alive)setFile(value);}).catch(error=>{if(alive)setError(String(error));});return()=>{alive=false;};},[navigation.id,local,destination?.path]);
 if(!local)return <img src={src} alt={alt}/>;
 if(error)return <span role="alert">{alt}: {error}</span>;
 return file?<button type="button" className="chat-inline-image" onClick={()=>navigation.onOpen?.(file.path)} aria-label={alt||file.name}><img src={file.url} alt={alt||file.name}/></button>:<span>{alt}</span>;
}

// Cindy turns unambiguous inline paths into links only after existence is
// confirmed. Identifiers and fenced commands remain ordinary code.
export function ChatInlineCode({children,className}:{children?:ReactNode;className?:string}){
 const navigation=useContext(NavigationContext),text=typeof children==='string'?children:'',destination=target(text,navigation),[path,setPath]=useState('');
 const candidate=!className&&!/[\r\n]/.test(text)&&looksLikeFilePath(destination.path)&&!!navigation.id;
 useEffect(()=>{let alive=true;setPath('');if(candidate)void chatCall<ChatFilePreviewData>('file.preview',{id:navigation.id,path:destination.path}).then(file=>{if(alive)setPath(file.path);}).catch(()=>{/* An unconfirmed inline path stays plain code. */});return()=>{alive=false;};},[candidate,navigation.id,destination.path]);
 return <code className={className}>{path?<ChatFileLink href={text}>{children}</ChatFileLink>:children}</code>;
}
