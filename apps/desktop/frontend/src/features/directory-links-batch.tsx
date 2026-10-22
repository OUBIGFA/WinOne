import {useEffect,useLayoutEffect,useRef,useState} from "react";
import {ClipboardPaste,Download,FilePlus2,FolderPlus,GripVertical,Link,Plus,ShieldCheck} from "lucide-react";
import {directoryLinksCall,pickDirectories,pickDirectory,pickFiles} from "../bridge";
import {useTranslation} from "../i18n";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";
import {Button} from "../ui/button";
import {Notice} from "../ui/notice";
import {Label} from "../components/ui/label";
import {Checkbox} from "../components/ui/checkbox";
import {Input} from "../components/ui/input";
import {DropdownMenu,DropdownMenuContent,DropdownMenuItem,DropdownMenuTrigger} from "../components/ui/dropdown-menu";
import {Popover,PopoverContent,PopoverTrigger} from "../components/ui/popover";
import {Select,SelectContent,SelectItem,SelectTrigger,SelectValue} from "../components/ui/select";
import {ResizablePanelGroup,ResizablePanel,ResizableHandle} from "../components/ui/resizable";
import {Item,ItemContent,ItemDescription,ItemGroup,ItemMedia,ItemTitle} from "../components/ui/item";
import {Dialog,DialogContent,DialogDescription,DialogFooter,DialogTitle} from "../components/ui/dialog";
import {DirectoryFolderPane,useDirectoryBrowser,pathKey,pathName,destinationPath,type DirectoryEntry,type DirectoryIdentity,type DirectoryListing} from "./directory-links-browser";
import type {DirectoryLinkRecord} from "./directory-links";

interface BatchPlan{id:string;items:{id:string;source:string;linkPath:string;kind:string;objectType:string;targetText:string}[]}
export interface DirectoryLinkLaunchOptions{kind?:"auto"|"hardlink"|"junction"|"symlink";relative?:boolean;retainScript?:boolean;hideSuccess?:boolean}
const pathsFromText=(text:string)=>[...new Map(text.split(/\r?\n/).map(s=>s.trim().replace(/^"(.*)"$/,"$1")).filter(Boolean).map(path=>[pathKey(path),path])).values()];
// Keep a chosen height across navigation; an absent preference fills the
// available window while leaving the creation controls in view.
function readPaneHeight(){try{const saved=Number(localStorage.getItem("winone.directory-links.pane-height.v1"));return saved>=256&&saved<=4000?saved:undefined;}catch{return undefined;}}

// SymlinkCreator's full multi-source workflow remains available. The new
// primary view uses the existing Item, Checkbox and resizable-panel components
// to show the source selection beside the destination's actual contents. The
 // link type and the create button sit directly under the two panes so choosing
// a type and creating is a single, obvious step.
export function DirectoryLinksBatch({disabled=false,onActivity,onChanged,incoming=[],targetIncoming=[],launch}:{disabled?:boolean;onActivity?:PageActivityChanged;onChanged:(rows:DirectoryLinkRecord[])=>void;incoming?:string[];targetIncoming?:string[];launch?:DirectoryLinkLaunchOptions}){
 const {t}=useTranslation();const [text,setText]=useState("");const [kind,setKind]=useState("auto");const [relative,setRelative]=useState(false);const [busy,setBusy]=useState(false);const [error,setError]=useState("");const [notice,setNotice]=useState("");const [plan,setPlan]=useState<BatchPlan>();
 const [retainScript,setRetainScript]=useState(false);const [hideSuccess,setHideSuccess]=useState(false);
 const [customName,setCustomName]=useState<string>();const [moreOpen,setMoreOpen]=useState(false);
 const [renaming,setRenaming]=useState<{entry:DirectoryEntry;parentIdentity:DirectoryIdentity}>(),[renameName,setRenameName]=useState(""),[renameError,setRenameError]=useState("");const renameInput=useRef<HTMLInputElement>(null);
 const workspace=useRef<HTMLElement>(null),createBar=useRef<HTMLDivElement>(null);
 const heightHandle=useRef<HTMLDivElement>(null),preferredHeight=useRef<number|undefined>(readPaneHeight()),fitHeight=useRef<(()=>void)|undefined>(undefined),heightDrag=useRef<{y:number;height:number}|null>(null);
 const [paneSize,setPaneSize]=useState({height:256,maximum:4000});
 function resizePanes(height:number){preferredHeight.current=Math.max(256,Math.min(paneSize.maximum,height));try{localStorage.setItem("winone.directory-links.pane-height.v1",String(preferredHeight.current));}catch(error){console.warn("Pane height could not be saved",error);}fitHeight.current?.();}
 const source=useDirectoryBrowser(),target=useDirectoryBrowser();const directory=target.address;
 usePageActivity(onActivity,!!text||!!renaming&&renameName!==renaming.entry.name,busy||source.loading||target.loading||!!renaming,"directory-links-batch");
 const locked=disabled||busy||!!renaming;const sources=pathsFromText(text);const selected=new Set(sources.map(pathKey));
 const singleSource=sources.length===1?sources[0]:"";
 useEffect(()=>{setCustomName(undefined);},[singleSource]);
 useLayoutEffect(()=>{
  const root=workspace.current,bar=createBar.current,pane=root?.querySelector<HTMLElement>(".directory-links-panes");
  if(!root||!bar||!pane)return;
  const scroll=root.closest<HTMLElement>('[data-slot="page-scroll"]');
  // Use measured surrounding controls, including wrapped translations. Scroll
  // offset keeps resizing independent of where the user scrolled the page.
  // A user-chosen height can exceed the viewport; the sticky action bar and
  // page scroll keep creation reachable while the two lists grow together.
  const fit=()=>{const bounds=pane.getBoundingClientRect();if(!bounds.width)return;const gap=parseFloat(getComputedStyle(root).rowGap)||0;const bottom=scroll?.getBoundingClientRect().bottom??window.innerHeight;const available=Math.max(256,bottom-bounds.top-(scroll?.scrollTop??0)-bar.offsetHeight-(heightHandle.current?.offsetHeight??0)-gap*2);const maximum=4000;const height=Math.min(preferredHeight.current??available,maximum);pane.style.setProperty("--directory-links-pane-height",`${height}px`);setPaneSize(current=>current.height===height&&current.maximum===maximum?current:{height,maximum});};
  fitHeight.current=fit;
  // Updating pane height also resizes the observed page. Defer that write
  // until the next frame so ResizeObserver can finish delivering this frame.
  let frame=0;const schedule=()=>{if(!frame)frame=requestAnimationFrame(()=>{frame=0;fit();});};
  const observer=new ResizeObserver(schedule);observer.observe(bar);observer.observe(root.closest('[data-slot="page-content"]')??root);if(scroll)observer.observe(scroll);window.addEventListener("resize",schedule);fit();
  return()=>{observer.disconnect();window.removeEventListener("resize",schedule);if(frame)cancelAnimationFrame(frame);fitHeight.current=undefined;};
 },[]);
 const listedSources=new Set((source.listing?.entries??[]).map(entry=>pathKey(entry.path)));
 const outsideSources=sources.filter(path=>!listedSources.has(pathKey(path)));
 const existing=new Set((target.listing?.entries??[]).map(entry=>pathKey(entry.name)));
 const nameCounts=new Map<string,number>();for(const path of sources){const name=pathKey(pathName(path));nameCounts.set(name,(nameCounts.get(name)??0)+1);}
 function resetPlan(){setPlan(undefined);setNotice("");setError("");}
 function add(paths:string[]){setText(old=>pathsFromText([...pathsFromText(old),...paths].join("\n")).join("\n"));resetPlan();}
 function pastePaths(value:string){const paths=pathsFromText(value);if(!paths.length)throw Error(t("links.clipboardEmpty"));add(paths);}
 function select(paths:string[],checked:boolean){const keys=new Set(paths.map(pathKey));if(checked)add(paths);else{setText(old=>pathsFromText(old).filter(path=>!keys.has(pathKey(path))).join("\n"));resetPlan();}}
 useEffect(()=>{if(incoming.length)add(incoming);},[incoming]);
 useEffect(()=>{if(targetIncoming.length)void dropDestination(targetIncoming);},[targetIncoming]);
 useEffect(()=>{if(!launch)return;if(launch.kind)setKind(launch.kind);if(launch.relative!==undefined)setRelative(launch.relative);if(launch.retainScript!==undefined)setRetainScript(launch.retainScript);if(launch.hideSuccess!==undefined)setHideSuccess(launch.hideSuccess);setPlan(undefined);},[launch]);
 async function work(run:()=>Promise<void>){if(busy)return;setBusy(true);setError("");setNotice("");try{await run();}catch(e){setError(String(e));}finally{setBusy(false);}}
 async function dropDestination(paths:string[]){await work(async()=>{
  if(paths.length!==1)throw new Error(t("links.dropOneFolder"));
  // Native paths are verified before replacing the current destination. An
  // accidental file drop must not destroy the user's valid folder selection.
  let listing:DirectoryListing;
  try{listing=await directoryLinksCall<DirectoryListing>("browse",{directory:paths[0],offset:0});}
  catch(reason){throw new Error(t("links.dropFolderOnly")+" "+String(reason));}
  target.accept(listing);resetPlan();
 });}
 function beginRename(entry:DirectoryEntry){if(locked||!target.listing?.identity)return;setRenaming({entry,parentIdentity:target.listing.identity});setRenameName(entry.name);setRenameError("");}
 async function renameTarget(){
  if(!renaming||busy||disabled||!renameName.trim())return;
  const {entry,parentIdentity}=renaming;setBusy(true);setRenameError("");
  try{
   const result=await directoryLinksCall<{path:string;records:DirectoryLinkRecord[];warning?:string}>("rename",{path:entry.path,name:renameName,entryIdentity:entry.identity,parentIdentity});
   // Rename is immediate, separate from link creation. A new safety preview is
   // required afterwards; history and the visible destination reflect the real result.
   resetPlan();onChanged(result.records);setRenaming(undefined);setNotice(t("links.renamed",{name:pathName(result.path)}));if(result.warning)setError(result.warning);
   setText(current=>pathsFromText(current).map(path=>pathKey(path)===pathKey(entry.path)?result.path:path).join("\n"));
   await target.load(directory);if(source.listing)await source.load(source.address);
  }catch(reason){setRenameError(String(reason));}finally{setBusy(false);}
 }
 async function previewBatch():Promise<BatchPlan|undefined>{if(singleSource&&customName!==undefined&&!customName.trim())throw Error(t("links.nameRequired"));const listing=await target.load(directory);if(!listing)return;return await directoryLinksCall<BatchPlan>("previewBatch",{sources,directory:listing.directory,kind,relative,...(singleSource&&customName?.trim()?{linkName:customName.trim()}:{})});}
 async function preview(){await work(async()=>{setPlan(undefined);const nextPlan=await previewBatch();if(nextPlan)setPlan(nextPlan);});}
 // Choosing a link type and creating is one step: the create button previews
 // and creates together, so a separate "preview then create" confirmation is
 // no longer required. An explicit preview remains available for review/export.
 async function create(elevated:boolean){if(!sources.length||!directory)return;await work(async()=>{
  const nextPlan=await previewBatch();if(!nextPlan)return;
  let retained="";
  if(retainScript){const destination=await pickDirectory();if(!destination)return;retained=(await directoryLinksCall<{path:string}>("exportPlan",{token:nextPlan.id,directory:destination})).path;}
  const result=await directoryLinksCall<{records:DirectoryLinkRecord[];completed:number;total:number}>(elevated?"createBatch.elevated":"createBatch",{token:nextPlan.id,confirmed:elevated});onChanged(result.records);
  if(!hideSuccess||result.completed!==result.total)setNotice(t("links.batchResult",{count:result.completed,total:result.total})+(retained?"\n"+t("links.exported",{path:retained}):""));
  if(result.completed===result.total)setText("");
  setPlan(undefined);
  // Keep both addresses after creation; actual new entries replace the preview.
  await target.load(directory);
 });}
 const previewRows=sources.map(path=>{const item=plan?.items.find(item=>pathKey(item.source)===pathKey(path));const name=singleSource&&customName?.trim()?customName.trim():pathName(path);const collision=existing.has(pathKey(name))||(nameCounts.get(pathKey(name))??0)>1;return {path,name,item,collision};});
 // Matching source/target entries share a type tint. Unchecked automatic
 // choices remain neutral until the host confirms their actual link type.
 const sourceKinds=new Map((target.listing?.entries??[]).filter(entry=>entry.kind&&entry.targetPath).map(entry=>[pathKey(entry.targetPath!),entry.kind!]));
 for(const {path,item} of previewRows)sourceKinds.set(pathKey(path),item?.kind??(kind!=="auto"?kind:"pending"));
 return <section ref={workspace} className="settings-stack directory-links-workspace" aria-label={t("links.createLinks")} onDragOver={event=>{if(!locked)event.preventDefault();}} onDrop={event=>{event.preventDefault();if(locked)return;const paths=pathsFromText(event.dataTransfer.getData("text/plain"));if(!paths.length)return;const side=(event.target as Element).closest('[data-directory-links-drop]')?.getAttribute('data-directory-links-drop');if(side==="target")void dropDestination(paths);else add(paths);}}>
  {error&&<Notice>{error}</Notice>}{notice&&<Notice tone="info">{notice}</Notice>}
  <p className="muted directory-links-workspace-hint">{t("links.browserHint")}</p>
  <ResizablePanelGroup orientation="horizontal" className="directory-links-panes" style={{height:"var(--directory-links-pane-height)"}}>
   <ResizablePanel id="link-source" defaultSize="50%" minSize="30%">
    <DirectoryFolderPane side="source" browser={source} disabled={locked} selected={selected} linkKinds={sourceKinds} onSelect={select} onPastePaths={pastePaths} actions={<DropdownMenu><DropdownMenuTrigger render={<Button icon={Plus} disabled={locked}>{t("links.addSources")}</Button>}/><DropdownMenuContent align="end"><DropdownMenuItem onClick={()=>void work(async()=>add(await pickFiles()))}><FilePlus2/>{t("links.pickSourceFiles")}</DropdownMenuItem><DropdownMenuItem onClick={()=>void work(async()=>add(await pickDirectories()))}><FolderPlus/>{t("links.pickSourceFolders")}</DropdownMenuItem><DropdownMenuItem onClick={()=>void work(async()=>{if(!navigator.clipboard)throw Error(t("links.clipboardUnavailable"));pastePaths(await navigator.clipboard.readText());})}><ClipboardPaste/>{t("links.pasteSources")}</DropdownMenuItem></DropdownMenuContent></DropdownMenu>}>
     {outsideSources.length>0&&<ItemGroup className="directory-links-entries directory-links-external-sources" aria-label={t("links.selectedOutside")}><p className="directory-links-selected-label">{t("links.selectedOutside")}</p>{outsideSources.map(path=><Item size="sm" role="listitem" key={pathKey(path)} className="directory-links-entry" data-selected="true" data-link-kind={sourceKinds.get(pathKey(path))}><Checkbox checked disabled={locked} aria-label={t("links.selectSource",{name:pathName(path)})} onCheckedChange={value=>select([path],value)}/><ItemMedia variant="icon"><Link aria-hidden="true"/></ItemMedia><ItemContent className="directory-links-entry-content"><ItemTitle className="directory-links-entry-title">{pathName(path)}</ItemTitle><ItemDescription className="directory-links-entry-description">{path}</ItemDescription></ItemContent></Item>)}</ItemGroup>}
    </DirectoryFolderPane>
   </ResizablePanel>
   <ResizableHandle withHandle aria-label={t("links.resizePanes")}/>
   <ResizablePanel id="link-target" minSize="30%">
    <DirectoryFolderPane side="target" browser={target} disabled={locked} onAddressChange={resetPlan} onRename={beginRename}>
     {sources.length>0&&directory&&<ItemGroup className="directory-links-entries directory-links-planned" aria-label={t("links.plannedContents")}>
      {previewRows.map(({path,name,item,collision})=><Item size="sm" role="listitem" key={pathKey(path)} className="directory-links-entry" data-planned="" data-link-kind={sourceKinds.get(pathKey(path))} data-conflict={collision||undefined}>
       <ItemMedia variant="icon"><Link aria-hidden="true"/></ItemMedia><ItemContent className="directory-links-entry-content"><ItemTitle className="directory-links-entry-title" title={item?.linkPath??destinationPath(directory,name)}>{name}</ItemTitle><ItemDescription className="directory-links-entry-description"><span className="muted" aria-hidden="true">→ </span><span title={path}>{path}</span></ItemDescription></ItemContent>
       {item&&<span className={`directory-links-kind directory-links-kind--${item.kind}`}>{t(`links.${item.kind}`)}</span>}
       <span className={`directory-links-entry-state${collision?" directory-links-error":""}`}>{t(collision?"links.nameCollision":item?"links.willCreate":"links.pendingCheck")}</span>
      </Item>)}
     </ItemGroup>}
    </DirectoryFolderPane>
   </ResizablePanel>
  </ResizablePanelGroup>
  <div ref={heightHandle} role="separator" tabIndex={0} aria-orientation="horizontal" aria-label={t("links.resizePaneHeight")} aria-valuemin={256} aria-valuemax={Math.round(paneSize.maximum)} aria-valuenow={Math.round(paneSize.height)} className="directory-links-height-handle" title={t("links.resizePaneHeight")}
   onPointerDown={event=>{if(event.button!==0)return;heightDrag.current={y:event.clientY,height:paneSize.height};event.currentTarget.setPointerCapture(event.pointerId);event.preventDefault();}}
   onPointerMove={event=>{if(heightDrag.current)resizePanes(heightDrag.current.height+event.clientY-heightDrag.current.y);}}
   onPointerUp={event=>{heightDrag.current=null;if(event.currentTarget.hasPointerCapture(event.pointerId))event.currentTarget.releasePointerCapture(event.pointerId);}}
   onLostPointerCapture={()=>{heightDrag.current=null;}}
   onKeyDown={event=>{if(event.key==="ArrowUp"||event.key==="ArrowDown"){event.preventDefault();resizePanes(paneSize.height+(event.key==="ArrowUp"?-24:24));}else if(event.key==="End"){event.preventDefault();resizePanes(paneSize.maximum);}else if(event.key==="Home"){event.preventDefault();preferredHeight.current=undefined;try{localStorage.removeItem("winone.directory-links.pane-height.v1");}catch(error){console.warn("Pane height could not be reset",error);}fitHeight.current?.();}}}><GripVertical aria-hidden="true" size={16}/></div>
  <div ref={createBar} className="directory-links-create-bar">
   <div className="directory-links-selection-toolbar"><span role="status">{t("links.selectedCount",{count:sources.length})}</span><Button variant="ghost" disabled={locked||!text} onClick={()=>{setText("");resetPlan();}}>{t("links.clearSources")}</Button></div>
   {singleSource&&<div className="directory-links-create-name"><Label htmlFor="links-custom-name">{t("links.linkName")}</Label><Input id="links-custom-name" value={customName??pathName(singleSource)} disabled={locked} onChange={event=>{setCustomName(event.target.value);resetPlan();}}/></div>}
   <div className="directory-links-create-kind"><Label htmlFor="links-batch-kind">{t("links.kind")}</Label><Select value={kind} disabled={locked} onValueChange={value=>{if(value){setKind(value);if(value!=="symlink")setRelative(false);resetPlan();}}}><SelectTrigger id="links-batch-kind"><SelectValue>{t(`links.${kind}`)}</SelectValue></SelectTrigger><SelectContent>{["auto","hardlink","junction","symlink"].map(value=><SelectItem value={value} key={value}>{t(`links.${value}`)}</SelectItem>)}</SelectContent></Select></div>
   {kind==="symlink"&&<Label className="directory-links-check"><Checkbox checked={relative} disabled={locked} onCheckedChange={value=>{setRelative(value);resetPlan();}}/>{t("links.relative")}</Label>}
   <div className="directory-links-create-actions"><Button variant="primary" icon={Link} busy={busy} disabled={locked||!sources.length||!directory} onClick={()=>void create(false)}>{t("links.createLinks")}</Button><Popover open={moreOpen} onOpenChange={setMoreOpen}><PopoverTrigger render={<Button>{t("links.advancedOptions")}</Button>}/><PopoverContent align="end" side="top" className="directory-links-advanced">
    <Button icon={ShieldCheck} disabled={locked||!sources.length||!directory} onClick={()=>{setMoreOpen(false);void create(true);}}>{t("links.elevate")}</Button>
    <Label className="directory-links-check"><Checkbox checked={retainScript} disabled={locked} onCheckedChange={setRetainScript}/>{t("links.retainScript")}</Label><Label className="directory-links-check"><Checkbox checked={hideSuccess} disabled={locked} onCheckedChange={setHideSuccess}/>{t("links.hideSuccess")}</Label>
    <Button variant="ghost" disabled={locked||!sources.length||!directory} onClick={()=>{setMoreOpen(false);void preview();}}>{t("links.preview")}</Button>
   </PopoverContent></Popover></div>
  </div>
  {plan&&<div className="directory-links-plan"><p>{t("links.batchPlan",{count:plan.items.length})}</p><ol className="directory-links-batch-plan">{plan.items.map(item=><li key={item.id}><span className={`directory-links-kind directory-links-kind--${item.kind}`}>{t(`links.${item.kind}`)}</span><dl><dt>{t("links.newEntry")}</dt><dd>{item.linkPath}</dd><dt>{t("links.target")}</dt><dd>{item.source}</dd></dl></li>)}</ol><p>{t("links.direction")}</p><div className="directory-links-actions"><Button icon={Download} disabled={locked} onClick={()=>void work(async()=>{const destination=await pickDirectory();if(destination){const result=await directoryLinksCall<{path:string}>("exportPlan",{token:plan.id,directory:destination});setNotice(t("links.exported",{path:result.path}));}})}>{t("links.exportPlan")}</Button></div><p className="muted">{t("links.elevationHint")}</p></div>}
  <Dialog open={!!renaming} onOpenChange={open=>{if(!open&&!busy)setRenaming(undefined);}}><DialogContent initialFocus={renameInput} finalFocus={()=>document.querySelector<HTMLElement>('[data-directory-links-drop="target"]')} showCloseButton={!busy}>
   <DialogTitle>{t(renaming?.entry.objectType==="directory"?"links.renameFolder":"links.renameFile")}</DialogTitle><DialogDescription>{t("links.renameHint")}</DialogDescription>
   <form className="directory-links-rename-form" onSubmit={event=>{event.preventDefault();void renameTarget();}}><p className="directory-links-confirm-path">{renaming?.entry.path}</p><Label htmlFor="links-rename-name">{t("links.newName")}</Label><Input ref={renameInput} id="links-rename-name" value={renameName} disabled={busy} onChange={event=>{setRenameName(event.target.value);setRenameError("");}} onFocus={event=>{const dot=renameName.lastIndexOf(".");event.currentTarget.setSelectionRange(0,renaming?.entry.objectType==="file"&&dot>0?dot:renameName.length);}}/>{renameError&&<Notice>{renameError}</Notice>}<DialogFooter><Button type="button" disabled={busy} onClick={()=>setRenaming(undefined)}>{t("links.cancel")}</Button><Button type="submit" variant="primary" busy={busy} disabled={disabled||busy||!renameName.trim()||renameName===renaming?.entry.name}>{t("links.rename")}</Button></DialogFooter></form>
  </DialogContent></Dialog>
 </section>;
}
