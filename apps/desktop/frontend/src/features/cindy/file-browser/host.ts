import {useCallback,useEffect,useRef,useState} from 'react';
import {chatCall} from '../../../bridge';
import {FILE_REFERENCE_MIME} from '../../chat-panel-state';
import type {ChatFiles} from '../../chat-types';
export const COMPOSER_MENTION_MIME=FILE_REFERENCE_MIME;
export const encodeComposerMentionPayload=(item:{type:string;relPath:string;name:string})=>item.relPath;
export interface DirEntry{name:string;relPath:string;absolutePath?:string;type:'file'|'directory';size:number;mtimeMs:number}
export interface UseFileTreeReturn{entries:Map<string,DirEntry[]>;expanded:Set<string>;loadingPaths:Set<string>;initialLoading:boolean;toggleFolder:(path:string)=>void}
export interface MatchLine{lineNumber:number;lineText:string;submatches:{start:number;end:number}[]}
export interface FileResult{relPath:string;matches:MatchLine[]}
export type SearchStatus='idle'|'searching'|'done'|'truncated'|'error';
export function joinWorkspace(root:string,path:string){return /^(?:[a-z]:[\\/]|[\\/])/i.test(path)?path:root.replace(/[\\/]$/,'')+(path?'/'+path:'');}
export async function fileQuery<T>(id:string,method:string,params:Record<string,unknown>,signal?:AbortSignal):Promise<T>{const data=await chatCall<{url:string}>('files.'+method,{id,...params});const response=await fetch(data.url,{signal});if(!response.ok)throw Error((await response.text()).trim()||'读取文件失败');return response.json();}

// Lazy directory loading keeps the same visible-tree data shape as Cindy.
// Window focus and successful file tools invalidate open directories; explicit
// refresh is always available. No background scan is started for a closed tree.
export function useWorkspaceTree(id:string,initialDirectory?:string,active=true){
 const [root,setRoot]=useState(initialDirectory??''),[entries,setEntries]=useState(new Map<string,DirEntry[]>()),[expanded,setExpanded]=useState(new Set<string>()),[loadingPaths,setLoadingPaths]=useState(new Set<string>()),[initialLoading,setInitialLoading]=useState(true),[error,setError]=useState('');
 const epoch=useRef(0),current=useRef({root,entries,expanded});current.current={root,entries,expanded};
 const load=useCallback(async(path:string,token=epoch.current)=>{
  setLoadingPaths(value=>new Set(value).add(path));
  try{const base=current.current.root,page=await fileQuery<ChatFiles>(id,'list',{path:path?joinWorkspace(base,path):initialDirectory??''});if(token!==epoch.current)return;
   if(!path)setRoot(page.directory);
   const values=page.entries.map(entry=>({name:entry.name,relPath:(path?path+'/':'')+entry.name,absolutePath:entry.path,type:entry.directory?'directory' as const:'file' as const,size:entry.size,mtimeMs:0}));
   setEntries(value=>new Map(value).set(path,values));setError('');
  }catch(error){if(token===epoch.current)setError(String(error));}finally{if(token===epoch.current){setLoadingPaths(value=>{const next=new Set(value);next.delete(path);return next;});if(!path)setInitialLoading(false);}}
 },[id,initialDirectory]);
 useEffect(()=>{epoch.current++;setRoot(initialDirectory??'');setEntries(new Map());setExpanded(new Set());setLoadingPaths(new Set());setError('');setInitialLoading(!!id);if(id)void load('',epoch.current);return()=>{epoch.current++;};},[id,initialDirectory,load]);
 // A refresh invalidates in-flight reads, not the user's expanded tree.
 const refresh=useCallback(()=>{if(!id)return;const token=++epoch.current;setLoadingPaths(new Set());void load('',token);for(const path of current.current.expanded)void load(path,token);},[id,load]);
 useEffect(()=>{if(!active)return;const changed=(event:Event)=>{if((event as CustomEvent).detail?.id===id)refresh();};window.addEventListener('focus',refresh);window.addEventListener('winone:chat-files-changed',changed);return()=>{window.removeEventListener('focus',refresh);window.removeEventListener('winone:chat-files-changed',changed);};},[id,refresh,active]);
 function toggleFolder(path:string){setExpanded(value=>{const next=new Set(value);if(next.has(path))next.delete(path);else{next.add(path);if(!current.current.entries.has(path))void load(path);}return next;});}
 return {root,error,tree:{entries,expanded,loadingPaths,initialLoading,toggleFolder} satisfies UseFileTreeReturn,refresh,collapseAll:()=>setExpanded(new Set<string>())};
}
