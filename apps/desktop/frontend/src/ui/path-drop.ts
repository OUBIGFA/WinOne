import {useCallback,useEffect,useRef,useState,type DragEvent} from "react";
import {inspectDroppedPaths,setFileDropTarget} from "../bridge";
import {i18n} from "../i18n";

export interface PathDropOptions {
 disabled?:boolean;
 multiple?:boolean;
 kind?:"file"|"directory"|"any";
 onPaths:(paths:string[])=>void|Promise<void>;
 onError:(error:string)=>void;
}

export function droppedTextPaths(transfer:DataTransfer):string[]{
 const uri=transfer.getData("text/uri-list");
 const values=uri?uri.split(/\r?\n/).filter(line=>line&&!line.startsWith("#")).map(line=>{
  const url=new URL(line);if(url.protocol!=="file:")throw Error(i18n.t("fileDrop.localOnly"));
  const pathname=decodeURIComponent(url.pathname);return url.hostname?`\\\\${url.hostname}${pathname.replaceAll("/","\\")}`:pathname.replace(/^\/([A-Za-z]:)/,"$1");
 }):transfer.getData("text/plain").split(/\r?\n/).map(line=>line.trim().replace(/^"(.*)"$/,"$1")).filter(Boolean);
 const seen=new Set<string>();return values.filter(value=>{const key=value.replaceAll("/","\\").toLowerCase();if(seen.has(key))return false;seen.add(key);return true;});
}

/** Native paths are hit-tested at the actual drop point. Each zone consumes its
 * own event (including disabled zones), so it cannot leak into Chat attachments
 * or the directory-link source list. The receiving page keeps its save flow. */
export function usePathDrop(options:PathDropOptions){
 const current=useRef(options);current.current=options;
 const [element,ref]=useState<HTMLElement|null>(null);
 const active=useRef(false),mounted=useRef(true);
 const receive=useCallback(async(paths:string[])=>{
  const value=current.current;if(value.disabled||active.current)return;
  active.current=true;
  try{
   if(!paths.length)throw Error(i18n.t("fileDrop.localOnly"));
   if(!value.multiple&&paths.length!==1)throw Error(i18n.t("fileDrop.single"));
   const entries=await inspectDroppedPaths(paths);
   if(!mounted.current||current.current.disabled)return;
   if(value.kind&&value.kind!=="any"&&entries.some(entry=>entry.kind!==value.kind))throw Error(i18n.t(value.kind==="directory"?"fileDrop.folderOnly":"fileDrop.fileOnly"));
   await value.onPaths(entries.map(entry=>entry.path));
  }catch(error){if(mounted.current)current.current.onError(String(error));}
  finally{active.current=false;}
 },[]);
 useEffect(()=>{
  mounted.current=true;const node=element;if(!node)return;
  const drop=(event:Event)=>{if(event.defaultPrevented)return;event.preventDefault();event.stopPropagation();void receive((event as CustomEvent<{paths:string[]}>).detail.paths);};
  node.addEventListener("winone:paths-drop",drop);
  // Browser previews have no native registration to change.
  if(window.go)void setFileDropTarget(true).catch(error=>current.current.onError(String(error)));
  return()=>{mounted.current=false;node.removeEventListener("winone:paths-drop",drop);if(window.go)void setFileDropTarget(false).catch(console.error);};
 },[receive,element]);
 return {ref,"data-path-drop":"",onDragOver:(event:DragEvent<HTMLElement>)=>{event.preventDefault();event.stopPropagation();event.dataTransfer.dropEffect=current.current.disabled?"none":"copy";},onDrop:(event:DragEvent<HTMLElement>)=>{if(event.defaultPrevented)return;event.preventDefault();event.stopPropagation();if(current.current.disabled)return;try{void receive(droppedTextPaths(event.dataTransfer));}catch(error){current.current.onError(String(error));}}};
}
