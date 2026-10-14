import {useCallback,useEffect,useRef,useState} from "react";
import {RefreshCw,FolderOpen,Trash2,Unlink,ArrowRight,History} from "lucide-react";
import {directoryLinksCall} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {Notice} from "../ui/notice";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";
import {Label} from "../components/ui/label";
import {Checkbox} from "../components/ui/checkbox";
import {Dialog,DialogContent,DialogDescription,DialogFooter,DialogTitle} from "../components/ui/dialog";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "../components/ui/tabs";
import {DirectoryLinksBatch,type DirectoryLinkLaunchOptions} from "./directory-links-batch";
import {directoryDropSide} from "./directory-links-browser";
import {DirectoryLinksScan} from "./directory-links-scan";
import {DirectoryLinksMenu} from "./directory-links-menu";
import "./directory-links.css";

type Kind="auto"|"hardlink"|"junction"|"symlink";
export interface DirectoryLinkRecord {id:string;name:string;linkPath:string;targetPath:string;kind:Exclude<Kind,"auto">|"shortcut";objectType:"file"|"directory";targetText:string;relative:boolean;createdAt:string;registeredAt?:string;checkedAt:string;status:string;reason:string;detail?:string;accessible:boolean;actualType?:string;actualTarget?:string;resolvedTarget?:string;linkCount?:number;entryIdentity?:{file:string}}

// Uses the existing settings primitives. Every displayed status comes from the
// Host's type/identity/access inspection, never from a successful old creation.
export function DirectoryLinksPage({disabled=false,onActivity}:{disabled?:boolean;onActivity?:PageActivityChanged}){
 const {t,i18n}=useTranslation();
 const [tab,setTab]=useState("batch");const [incoming,setIncoming]=useState<string[]>([]);
 const [targetIncoming,setTargetIncoming]=useState<string[]>([]);
 const [launchOptions,setLaunchOptions]=useState<DirectoryLinkLaunchOptions>();
 const [launchQueue,setLaunchQueue]=useState<{paths:string[];options?:DirectoryLinkLaunchOptions;side?:"source"|"target"}[]>([]);
 const [records,setRecords]=useState<DirectoryLinkRecord[]>([]);
 const [selected,setSelected]=useState<Set<string>>(new Set());
 const [busy,setBusy]=useState(false);
 const [childBusy,setChildBusy]=useState(false);const activities=useRef(new Map<string,boolean>());
 const reportActivity=useCallback<PageActivityChanged>((dirty,busy,source="links-child")=>{activities.current.set(source,busy);setChildBusy([...activities.current.values()].some(Boolean));onActivity?.(dirty,busy,source);},[onActivity]);
 const [loading,setLoading]=useState(true);
 const [error,setError]=useState("");const [notice,setNotice]=useState("");
 const [confirmation,setConfirmation]=useState<{record:DirectoryLinkRecord;forget:boolean}>();
 const alive=useRef(true);const action=useRef(0);
 usePageActivity(onActivity,false,busy,"directory-links");
 useEffect(()=>{const drop=(event:Event)=>{const value=(event as CustomEvent<{requested?:boolean;paths:string[];options?:DirectoryLinkLaunchOptions;position?:{x:number;y:number}}>).detail;if(value?.requested||value?.paths?.length||value?.options)setLaunchQueue(queue=>[...queue,{paths:value.paths??[],options:value.options,side:directoryDropSide(value.position)}]);};window.addEventListener("winone:directory-links-drop",drop);return()=>window.removeEventListener("winone:directory-links-drop",drop);},[]);
 useEffect(()=>{if(disabled||busy||childBusy||!launchQueue.length)return;const sources=launchQueue.filter(value=>value.side!=="target").flatMap(value=>value.paths);if(sources.length)setIncoming(sources);const destination=[...launchQueue].reverse().find(value=>value.side==="target");if(destination)setTargetIncoming(destination.paths);const options=[...launchQueue].reverse().find(value=>value.options)?.options;if(options)setLaunchOptions(options);setTab("batch");setLaunchQueue([]);},[launchQueue,disabled,busy,childBusy]);
 useEffect(()=>{let live=true;void directoryLinksCall<{requested:boolean;paths:string[];options?:DirectoryLinkLaunchOptions}>("pending").then(value=>{if(live&&value.requested)setLaunchQueue(queue=>[...queue,{paths:value.paths??[],options:value.options}]);}).catch(e=>{if(live)setError(String(e));});return()=>{live=false;};},[]);
 useEffect(()=>{alive.current=true;void directoryLinksCall<DirectoryLinkRecord[]>("list").then(async rows=>{
  if(!alive.current)return;setRecords(rows);
  const ids=rows.filter(r=>r.status!=="removed").map(r=>r.id);
  if(ids.length){const checked=await directoryLinksCall<DirectoryLinkRecord[]>("inspectBatch",{ids});if(alive.current)setRecords(current=>merge(current,checked));}
 }).catch(e=>{if(alive.current)setError(String(e));}).finally(()=>{if(alive.current)setLoading(false);});return()=>{alive.current=false;action.current++;};},[]);
 const date=(value:string)=>value?new Date(value).toLocaleString(i18n.language):t("links.neverChecked");
 const locked=disabled||busy||childBusy||loading;
 async function work(run:()=>Promise<void>){if(busy)return;const id=++action.current;setBusy(true);setError("");setNotice("");try{await run();}catch(e){if(alive.current&&action.current===id)setError(String(e));}finally{if(alive.current&&action.current===id)setBusy(false);}}
 async function refresh(ids:string[]){await work(async()=>{const checked=await directoryLinksCall<DirectoryLinkRecord[]>("inspectBatch",{ids});if(alive.current)setRecords(current=>merge(current,checked));});}
 async function confirm(){if(!confirmation)return;const current=confirmation;await work(async()=>{
  if(current.forget){await directoryLinksCall("forget",{id:current.record.id,confirmed:true});setRecords(rows=>rows.filter(r=>r.id!==current.record.id));setNotice(t("links.forgotten"));}
  else{const row=await directoryLinksCall<DirectoryLinkRecord>("remove",{id:current.record.id,confirmed:true,checkedAt:current.record.checkedAt});setRecords(rows=>merge(rows,[row]));setNotice(t("links.removed"));}
  setSelected(ids=>{const next=new Set(ids);next.delete(current.record.id);return next;});setConfirmation(undefined);
 });}
 return <div className="directory-links settings-stack" data-directory-links-root="">
  <p className="muted">{t("links.intro")}</p>
  {error&&<Notice>{error}</Notice>}{notice&&<Notice tone="info">{notice}</Notice>}
  <Tabs value={tab} onValueChange={value=>{if(!busy&&!childBusy)setTab(String(value));}}><TabsList variant="line"><TabsTrigger value="batch">{t("links.createWorkspace")}</TabsTrigger><TabsTrigger value="scan">{t("links.findExisting")}</TabsTrigger></TabsList><TabsContent value="batch" keepMounted><DirectoryLinksBatch disabled={disabled||loading||busy} onActivity={reportActivity} incoming={incoming} targetIncoming={targetIncoming} launch={launchOptions} onChanged={rows=>setRecords(current=>merge(current,rows))}/></TabsContent><TabsContent value="scan" keepMounted><DirectoryLinksScan disabled={disabled||loading||busy} onActivity={reportActivity} onChanged={rows=>setRecords(current=>merge(current,rows))}/>
  {/* History belongs to finding/managing existing entries; creation keeps its own result feedback. */}
  {tab==="scan"&&<section className="directory-links-history settings-stack" aria-label={t("links.history")}>
   <div className="directory-links-toolbar"><h3><History size={18} aria-hidden="true"/>{t("links.history")}<span className="directory-links-history-count">{records.length}</span></h3><div className="directory-links-actions"><Button icon={RefreshCw} disabled={locked||selected.size===0} onClick={()=>void refresh([...selected])}>{t("links.checkSelected")}</Button><Button icon={RefreshCw} disabled={locked||records.length===0} onClick={()=>void refresh(records.map(r=>r.id))}>{t("links.checkAll")}</Button></div></div>
   {(busy||loading)&&<p role="status" className="muted">{t(loading?"links.checking":"links.working")}</p>}
   {!loading&&records.length===0&&<p className="muted">{t("links.empty")}</p>}
   {records.length>0&&<Label className="directory-links-check"><Checkbox aria-label={t("links.selectAll")} checked={selected.size===records.length} indeterminate={selected.size>0&&selected.size<records.length} disabled={locked} onCheckedChange={value=>setSelected(value?new Set(records.map(r=>r.id)):new Set())}/>{t("links.selectAll")}</Label>}
   {[...records].reverse().map(record=><article className="directory-links-record" data-link-kind={record.kind} key={record.id}>
    <div className="directory-links-record-head"><Checkbox aria-label={t("links.select",{name:record.name})} checked={selected.has(record.id)} disabled={locked} onCheckedChange={value=>setSelected(ids=>{const next=new Set(ids);if(value)next.add(record.id);else next.delete(record.id);return next;})}/><span className="directory-links-record-name">{record.name}</span><span className={`directory-links-kind directory-links-kind--${record.kind}`}>{t(`links.${record.kind}`)}</span></div>
    <div className="directory-links-status"><span className={`directory-links-status-label ${record.status==="active"?"is-active":""}`}>{t(`links.status.${record.status}`)}</span><span>{t(`links.reason.${record.reason}`,{defaultValue:record.reason})}</span></div>
    <p className="directory-links-time muted">{t("links.checked")}：{date(record.checkedAt)}</p>
    <div className="directory-links-record-paths"><dl><dt>{t(record.kind==="hardlink"?"links.recordedSource":"links.expectedTarget")}</dt><dd>{record.targetPath}</dd></dl><ArrowRight size={18} aria-hidden="true"/><dl><dt>{t("links.newEntry")}</dt><dd>{record.linkPath}</dd></dl></div>
    <details><summary>{t("links.details")}</summary><dl className="directory-links-paths"><dt>{t(record.createdAt?"links.createdAt":"links.registeredAt")}</dt><dd>{date(record.createdAt||record.registeredAt||"")}</dd>{record.actualType&&<><dt>{t("links.actualType")}</dt><dd>{record.actualType}</dd></>}{record.actualTarget&&<><dt>{t("links.rawTarget")}</dt><dd>{record.actualTarget}</dd></>}{record.resolvedTarget&&<><dt>{t("links.resolvedTarget")}</dt><dd>{record.resolvedTarget}</dd></>}</dl>{record.kind==="hardlink"&&!!record.linkCount&&<p>{t("links.linksCount",{count:record.linkCount})}</p>}{record.accessible&&<p>{t("links.dataAvailable")}</p>}{record.detail&&<p className="directory-links-error">{record.detail}</p>}</details>
    <div className="directory-links-actions"><Button icon={RefreshCw} disabled={locked||record.status==="removed"} onClick={()=>void refresh([record.id])}>{t("links.check")}</Button><Button icon={FolderOpen} disabled={locked} onClick={()=>void work(async()=>{await directoryLinksCall("reveal",{id:record.id,side:"link"});})}>{t("links.openLocation")}</Button><Button icon={Trash2} disabled={locked||record.status==="removed"||record.status==="replaced"||!record.entryIdentity?.file} onClick={()=>setConfirmation({record,forget:false})}>{t("links.remove")}</Button><Button variant="ghost" icon={Unlink} disabled={locked} onClick={()=>setConfirmation({record,forget:true})}>{t("links.forget")}</Button></div>
   </article>)}
  </section>}</TabsContent></Tabs>
  <section className="directory-links-integration" aria-label={t("links.explorerIntegration")}><DirectoryLinksMenu disabled={disabled||busy} onActivity={reportActivity}/></section>
  <Dialog open={!!confirmation} onOpenChange={open=>{if(!open&&!busy)setConfirmation(undefined);}}><DialogContent><DialogTitle>{t(confirmation?.forget?"links.forgetTitle":"links.removeTitle")}</DialogTitle><DialogDescription>{t(confirmation?.forget?"links.forgetHint":"links.removeHint")}</DialogDescription><p className="directory-links-confirm-path">{confirmation?.record.linkPath}</p>{confirmation&&!confirmation.forget&&<p className="directory-links-confirm-path">{t("links.resolvedTarget")}：{confirmation.record.resolvedTarget||confirmation.record.actualTarget||confirmation.record.targetPath}</p>}{error&&<Notice>{error}</Notice>}<DialogFooter><Button disabled={busy} onClick={()=>setConfirmation(undefined)}>{t("links.cancel")}</Button><Button variant="danger" busy={busy} onClick={()=>void confirm()}>{t(confirmation?.forget?"links.confirmForget":"links.confirmRemove")}</Button></DialogFooter></DialogContent></Dialog>
 </div>;
}

function merge(current:DirectoryLinkRecord[],changed:DirectoryLinkRecord[]){const next=new Map(current.map(r=>[r.id,r]));for(const row of changed)next.set(row.id,row);return [...next.values()];}
