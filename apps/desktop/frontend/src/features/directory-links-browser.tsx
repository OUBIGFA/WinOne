import {usePathDrop} from "../ui/path-drop";
import {useEffect,useRef,useState,type ReactNode} from "react";
import {ArrowUp,ChevronRight,File,Folder,FolderOpen,PencilLine,RefreshCw} from "lucide-react";
import {directoryLinksCall,pickDirectory} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {Notice} from "../ui/notice";
import {Input} from "../components/ui/input";
import {Label} from "../components/ui/label";
import {Checkbox} from "../components/ui/checkbox";
import {Item,ItemContent,ItemDescription,ItemGroup,ItemMedia,ItemTitle} from "../components/ui/item";

export interface DirectoryIdentity {volume:number;volumeId?:string;scope?:string;file:string;created:number}
export interface DirectoryEntry {name:string;path:string;objectType:"file"|"directory";kind?:string;targetPath?:string;error?:string;identity?:DirectoryIdentity}
export interface DirectoryListing {directory:string;parent:string;identity?:DirectoryIdentity;entries:DirectoryEntry[];total:number;offset:number;hasMore:boolean}
/**
 * Compare Windows paths by stable identity while preserving user-entered
 * display text. Host preview responses can remove dot path segments.
 */
export const pathKey=(path:string)=>{
 const value=path.trim().replace(/\//g,"\\");
 const prefix=value.startsWith("\\\\")?"\\\\":value.match(/^[A-Za-z]:\\?/)?.[0]??(value.startsWith("\\")?"\\":"");
 const stack:string[]=[];
 for(const part of value.slice(prefix.length).split("\\")){
  if(!part||part===".")continue;
  if(part===".."&&stack.length&&stack.at(-1)!=="..")stack.pop();
  else stack.push(part);
 }
 return (prefix+stack.join("\\")).replace(/\\+$/g,"").toLowerCase();
};
export const pathName=(path:string)=>path.split(/[\\/]/).filter(Boolean).at(-1)??path;
export const destinationPath=(directory:string,name:string)=>directory.replace(/[\\/]+$/,"")+"\\"+name;

// Wry positions arrive in CSS pixels. Startup/prefill events have no pointer
// location and continue to add sources, including when this tab is not visible.
export function directoryDropSide(position?:{x:number;y:number}):"source"|"target"{
 if(!position||!Number.isFinite(position.x)||!Number.isFinite(position.y))return "source";
 const bounds=document.querySelector('[data-directory-links-drop="target"]')?.getBoundingClientRect();
 return bounds&&bounds.width>0&&bounds.height>0&&position.x>=bounds.left&&position.x<bounds.right&&position.y>=bounds.top&&position.y<bounds.bottom?"target":"source";
}

// A response belongs to the address that requested it. Typing or navigating
// again invalidates older reads, so a slow folder cannot overwrite the new one.
export function useDirectoryBrowser(){
 const [address,setAddress]=useState("");const [listing,setListing]=useState<DirectoryListing>();
 const [loading,setLoading]=useState(false);const [error,setError]=useState("");const sequence=useRef(0);
 useEffect(()=>()=>{sequence.current++;},[]);
 function edit(value:string){sequence.current++;setAddress(value);setListing(undefined);setLoading(false);setError("");}
 async function load(directory=address,append=false){
  const requested=directory.trim();if(!requested)return;
  const ticket=++sequence.current;const previous=append?listing:undefined;
  setAddress(requested);setLoading(true);setError("");if(!append)setListing(undefined);
  try{
   const value=await directoryLinksCall<DirectoryListing>("browse",{directory:requested,offset:previous?.entries.length??0});
   if(ticket!==sequence.current)return;
   const next=previous?{...value,entries:[...previous.entries,...value.entries]}:value;
   setListing(next);setAddress(value.directory);return next;
  }catch(reason){if(ticket===sequence.current)setError(String(reason));}
  finally{if(ticket===sequence.current)setLoading(false);}
 }
 function accept(value:DirectoryListing){sequence.current++;setAddress(value.directory);setListing(value);setLoading(false);setError("");}
 return {address,listing,loading,error,edit,load,accept,reportError:(reason:unknown)=>setError(String(reason))};
}

type BrowserState=ReturnType<typeof useDirectoryBrowser>;
interface FolderPaneProps {side:"source"|"target";browser:BrowserState;disabled:boolean;onAddressChange?:()=>void;selected?:Set<string>;linkKinds?:Map<string,string>;onSelect?:(paths:string[],selected:boolean)=>void;children?:ReactNode;actions?:ReactNode;onPastePaths?:(text:string)=>void;onRename?:(entry:DirectoryEntry)=>void}

// File rows and focus behavior are assembled from the repository's pinned
// shadcn Item/Base UI controls; there is no second explorer widget framework.
export function DirectoryFolderPane({side,browser,disabled,onAddressChange,selected,onSelect,children,actions,onPastePaths,linkKinds,onRename}:FolderPaneProps){
 const {t}=useTranslation();const isSource=side==="source";const id=`links-${side}-directory`;
 const entries=browser.listing?.entries??[];
 const selectable=entries.filter(entry=>!entry.error);
 const chosen=selectable.filter(entry=>selected?.has(pathKey(entry.path))).length;
 const addressDrop=usePathDrop({kind:"directory",disabled,onPaths:paths=>navigate(paths[0]),onError:browser.reportError});
 function navigate(path:string){onAddressChange?.();void browser.load(path);}
 return <section className="directory-links-pane" data-directory-links-drop={side} aria-label={t(isSource?"links.sourceFolder":"links.targetFolder")} tabIndex={0} onPaste={event=>{
  // Pasting into the address edits that address. Pasting elsewhere in the
  // source pane imports a path list without keeping a second editable form.
  if(disabled||!isSource||(event.target as Element).closest("input,textarea,[contenteditable=true]"))return;
  const text=event.clipboardData.getData("text/plain");if(text){event.preventDefault();onPastePaths?.(text);}
 }}>
  <header className="directory-links-pane-header">
   <div className="directory-links-toolbar"><h3>{t(isSource?"links.sourceFolder":"links.targetFolder")}</h3></div>
   <div className="directory-links-folder-address" {...addressDrop}><Label htmlFor={id} className="sr-only">{t(isSource?"links.sourceFolder":"links.location")}</Label><Input id={id} value={browser.address} disabled={disabled} placeholder={t("links.enterFolder")} onChange={event=>{browser.edit(event.target.value);onAddressChange?.();}} onKeyDown={event=>{if(event.key==="Enter"){event.preventDefault();navigate(browser.address);}}}/><Button icon={ChevronRight} size="icon" title={t(isSource?"links.openSourceFolder":"links.openTargetFolder")} aria-label={t(isSource?"links.openSourceFolder":"links.openTargetFolder")} disabled={disabled||!browser.address.trim()} onClick={()=>navigate(browser.address)}/></div>
   <div className="directory-links-actions"><Button icon={FolderOpen} aria-label={t(isSource?"links.browseSource":"links.pickLocation")} disabled={disabled} onClick={()=>{void pickDirectory().then(path=>{if(path)navigate(path);}).catch(browser.reportError);}}>{t("links.browseFolder")}</Button><Button icon={ArrowUp} size="icon" title={t("links.parentFolder")} aria-label={t(isSource?"links.sourceParent":"links.targetParent")} disabled={disabled||!browser.listing?.parent} onClick={()=>navigate(browser.listing!.parent)}/><Button icon={RefreshCw} size="icon" title={t("links.refreshFolder")} aria-label={t(isSource?"links.refreshSource":"links.refreshTarget")} disabled={disabled||!browser.address||browser.loading} onClick={()=>navigate(browser.address)}/>{actions}</div>
  </header>
  {browser.error&&<Notice>{browser.error}</Notice>}
  {isSource&&selectable.length>0&&<Label className="directory-links-check directory-links-select-visible"><Checkbox aria-label={t("links.selectCurrentFolder")} checked={chosen===selectable.length} indeterminate={chosen>0&&chosen<selectable.length} disabled={disabled||browser.loading} onCheckedChange={value=>onSelect?.(selectable.map(entry=>entry.path),value)}/>{t("links.selectCurrentFolder")}</Label>}
  <div className="directory-links-file-list" aria-busy={browser.loading}>
   {children}
   {browser.loading&&!browser.listing&&<p className="directory-links-pane-empty" role="status">{t("links.loadingFolder")}</p>}
   {!browser.loading&&!browser.listing&&!browser.error&&!children&&<p className="directory-links-pane-empty">{t(isSource?"links.sourceEmpty":"links.targetEmpty")}</p>}
   {browser.listing&&entries.length===0&&<p className="directory-links-pane-empty">{t("links.folderEmpty")}</p>}
   <ItemGroup className="directory-links-entries" aria-label={t(isSource?"links.sourceContents":"links.existingContents")}>
    {entries.map(entry=><DirectoryEntryRow key={entry.path} entry={entry} linkKind={linkKinds?.get(pathKey(entry.path))??entry.kind} disabled={disabled||browser.loading} selected={selected?.has(pathKey(entry.path))} selectable={isSource} onSelect={value=>onSelect?.([entry.path],value)} onOpen={()=>navigate(entry.path)} onRename={!isSource&&onRename?()=>onRename(entry):undefined}/>)}
   </ItemGroup>
   {browser.listing?.hasMore&&<Button className="directory-links-more" busy={browser.loading} disabled={disabled} onClick={()=>void browser.load(browser.address,true)}>{t("links.loadMore",{shown:entries.length,total:browser.listing.total})}</Button>}
  </div>
 </section>;
}

function DirectoryEntryRow({entry,disabled,selected,selectable,onSelect,onOpen,onRename,linkKind}:{entry:DirectoryEntry;linkKind?:string;disabled:boolean;selected?:boolean;selectable:boolean;onSelect:(selected:boolean)=>void;onOpen:()=>void;onRename?:()=>void}){
 const {t}=useTranslation();const folder=entry.objectType==="directory";const Icon=folder?Folder:File;
 const canRename=!!onRename&&!disabled&&!!entry.identity?.file;
 return <Item size="sm" role="listitem" tabIndex={onRename?0:undefined} className="directory-links-entry" data-selected={selected||undefined} data-link-kind={linkKind} onKeyDown={event=>{if(event.key==="F2"&&canRename){event.preventDefault();onRename?.();}}}>
  {selectable&&<Checkbox checked={selected??false} disabled={disabled||!!entry.error} aria-label={t("links.selectSource",{name:entry.name})} onCheckedChange={onSelect}/>}
  <ItemMedia variant="icon"><Icon aria-hidden="true"/></ItemMedia>
  <ItemContent className="directory-links-entry-content"><ItemTitle className="directory-links-entry-title">{folder?<Button variant="ghost" className="directory-links-entry-name" disabled={disabled} aria-label={t("links.openFolder",{name:entry.name})} title={entry.path} onClick={onOpen}>{entry.name}</Button>:<span title={entry.path}>{entry.name}</span>}</ItemTitle>
   {entry.targetPath&&<ItemDescription className="directory-links-entry-description" title={entry.targetPath}>→ {entry.targetPath}</ItemDescription>}
   {entry.error&&<ItemDescription className="directory-links-entry-description directory-links-error">{entry.error}</ItemDescription>}
  </ItemContent>
  <div className="directory-links-entry-badges"><span className={entry.kind?`directory-links-kind directory-links-kind--${entry.kind}`:"directory-links-entry-type"}>{t(entry.kind?`links.${entry.kind}`:folder?"links.directoryType":"links.fileType")}</span>{entry.kind&&!entry.error&&<span className="directory-links-linked">{t("links.linked")}</span>}{onRename&&<Button icon={PencilLine} size="icon" variant="ghost" aria-label={t("links.renameEntry",{name:entry.name})} title={t("links.renameEntry",{name:entry.name})+" (F2)"} disabled={!canRename} onClick={onRename}/>}</div>
 </Item>;
}
