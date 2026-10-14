import {Archive,ArchiveRestore,ChevronsRightLeft,ChevronDown,ChevronRight,Copy,FolderOpen,FolderPlus,FolderInput,Download,MoreHorizontal,Pin,PinOff,Plus,Search,Settings2,Trash2,PenLine,Timer,Sparkles,LoaderCircle,CircleAlert} from "lucide-react";
import {Button} from "../ui/button";
import {useTranslation} from "../i18n";
import {DropdownMenu,DropdownMenuContent,DropdownMenuItem,DropdownMenuSeparator,DropdownMenuTrigger} from "../components/ui/dropdown-menu";
import type {ChatCatalog,ChatProject,Conversation} from "./chat-state";
import {useState} from "react";
import {ChevronsDownUp,ChevronsUpDown} from 'lucide-react';
import {SectionCollapse} from './cindy/SectionCollapse';
import {ScrollArea} from './lobe/scroll-area/ScrollArea';
import {ChatFeedback} from './chat-feedback';

export type ConversationAction="rename"|"aiRename"|"pin"|"archive"|"delete"|"copy"|"move"|"export";
/** Cindy's session actions are keyed by row id. Keep this menu independent of
 * the active transcript, and use WinOne's Base UI menu for focus and dismissal. */
export function ConversationMenu({conversation,busy,onAction,titlePending=false}:{conversation:Conversation;busy:boolean;titlePending?:boolean;onAction:(conversation:Conversation,action:ConversationAction)=>void}){
 const {t}=useTranslation();
 return <DropdownMenu><DropdownMenuTrigger render={<Button variant="ghost" size="icon" icon={MoreHorizontal} aria-label={t("chat.moreFor",{name:conversation.title})} disabled={busy}/>}/>
  <DropdownMenuContent align="end" className="chat-row-menu">
   <DropdownMenuItem onClick={()=>onAction(conversation,"rename")}><PenLine/>{t("chat.rename")}</DropdownMenuItem>
   <DropdownMenuItem disabled={titlePending} onClick={()=>onAction(conversation,"aiRename")}><Sparkles/>{t("chat.aiRename")}</DropdownMenuItem>
   <DropdownMenuItem onClick={()=>onAction(conversation,"pin")}>{conversation.pinned?<PinOff/>:<Pin/>}{t(conversation.pinned?"chat.unpin":"chat.pin")}</DropdownMenuItem>
   <DropdownMenuItem onClick={()=>onAction(conversation,"archive")}>{conversation.archived?<ArchiveRestore/>:<Archive/>}{t(conversation.archived?"chat.restore":"chat.archive")}</DropdownMenuItem>
   <DropdownMenuItem onClick={()=>onAction(conversation,"copy")}><Copy/>{t("chat.sourcePath")}</DropdownMenuItem>
   <DropdownMenuItem onClick={()=>onAction(conversation,"move")}><FolderInput/>{t("chat.moveToProject")}</DropdownMenuItem>
   <DropdownMenuItem onClick={()=>onAction(conversation,"export")}><Download/>{t("chat.exportTask")}</DropdownMenuItem>
   <DropdownMenuSeparator/>
   <DropdownMenuItem variant="destructive" onClick={()=>onAction(conversation,"delete")}><Trash2/>{t("chat.delete")}</DropdownMenuItem>
  </DropdownMenuContent>
 </DropdownMenu>;
}

export function ChatSidebar({catalog,activeId,busy,archived,onArchived,onChoose,onNew,onProject,onHide,onSettings,onAction,onProjectAction,onAutomations,automationsOpen=false,titleJobs={}}:{titleJobs?:Record<string,{pending:boolean;error?:string}>;catalog:ChatCatalog;activeId:string;busy:boolean;archived:boolean;onArchived:(value:boolean)=>void;onChoose:(id:string)=>void;onNew:(projectId?:string)=>void;onProject:()=>void;onHide:()=>void;onSettings:()=>void;onAction:(conversation:Conversation,action:ConversationAction)=>void;onProjectAction:(project:ChatProject,action:"rename"|"remove")=>void;onAutomations:()=>void;automationsOpen?:boolean}){
 const {t}=useTranslation();
 const [search,setSearch]=useState(""),[collapsed,setCollapsed]=useState(new Set<string>());
 const [sections,setSections]=useState<Record<string,boolean>>(()=>{try{return JSON.parse(localStorage.getItem('winone.chat.sections.v1')||'{}');}catch{return {};}});
 const allProjectsCollapsed=catalog.projects.length>0&&catalog.projects.every(project=>collapsed.has(project.id));
 function toggleSection(section:string){setSections(current=>{const next={...current,[section]:!current[section]};try{localStorage.setItem('winone.chat.sections.v1',JSON.stringify(next));}catch(error){console.warn('Chat section preference could not be saved',error);}return next;});}
 const heading=(section:'projects'|'conversations',label:string)=><div className="chat-group-heading"><h3>{label}</h3><Button variant="ghost" size="icon" icon={sections[section]?ChevronsUpDown:ChevronsDownUp} aria-label={t(sections[section]?'chat.expandSection':'chat.collapseSection',{name:label})} title={t(sections[section]?'chat.expandSection':'chat.collapseSection',{name:label})} aria-expanded={!sections[section]} onClick={()=>toggleSection(section)}/></div>;
 const visible=catalog.conversations.filter(value=>!!value.archived===archived&&(value.title+" "+value.directory).toLowerCase().includes(search.toLowerCase())).sort((a,b)=>b.updatedAt.localeCompare(a.updatedAt));
 const rows=(items:Conversation[])=>items.map(value=><div key={value.id} className={"chat-sidebar-item"+(activeId===value.id?" is-active":"")+(titleJobs[value.id]?.error?" has-title-error":"")}>
  <button type="button" className="chat-sidebar-row" aria-current={activeId===value.id?"page":undefined} disabled={busy} onClick={()=>onChoose(value.id)}><span>{value.title}</span>{value.pinned&&<Pin size={13}/>}</button>
  {titleJobs[value.id]?.pending&&<span className="chat-title-status" role="status" aria-label={t("chat.titlePending",{name:value.title})} title={t("chat.titlePending",{name:value.title})}><LoaderCircle size={14} className="animate-spin motion-reduce:animate-none"/></span>}
  <ChatFeedback label={t("chat.titleRequestFailed",{name:value.title})} message={titleJobs[value.id]?.error} role="alert" className="chat-title-error"/>
  <ConversationMenu conversation={value} busy={busy} titlePending={titleJobs[value.id]?.pending} onAction={onAction}/>
 </div>);
 return <aside className="chat-sidebar" aria-label={t("chat.sidebar")}>
  <div className="chat-sidebar-heading"><span>{t("chat.title")}</span><Button variant="ghost" size="icon" icon={ChevronsRightLeft} aria-label={t("chat.hideSidebar")} onClick={onHide}/></div>
  <Button className="chat-new" variant="ghost" icon={Plus} disabled={busy} onClick={()=>onNew()}>{t("chat.newChat")}</Button>
  <div className="chat-search"><Search size={15}/><input aria-label={t("chat.search")} placeholder={t("chat.search")} value={search} onChange={event=>setSearch(event.target.value)}/></div>
  <ScrollArea disableContentFit className="chat-sidebar-scroll" viewportProps={{className:"chat-sidebar-viewport"}}>
   {visible.some(c=>c.pinned)&&<section><h3>{t("chat.pinned")}</h3>{rows(visible.filter(c=>c.pinned))}</section>}
   <Button variant="ghost" icon={Timer} className="chat-project-create" aria-current={automationsOpen?"page":undefined} disabled={busy} onClick={onAutomations}>{t("chat.automations")}</Button>
   <Button variant="ghost" icon={FolderPlus} className="chat-project-create" title={t("chat.openOrCreateProject")} disabled={busy} onClick={onProject}><span>{t("chat.openOrCreateProject")}</span></Button>
   <section><div className="chat-group-heading"><h3>{t("chat.projects")}</h3><Button variant="ghost" size="icon" icon={allProjectsCollapsed?ChevronsUpDown:ChevronsDownUp} disabled={!catalog.projects.length} aria-label={t(allProjectsCollapsed?'chat.expandProjectConversations':'chat.collapseProjectConversations')} title={t(allProjectsCollapsed?'chat.expandProjectConversations':'chat.collapseProjectConversations')} aria-expanded={!allProjectsCollapsed} onClick={()=>setCollapsed(allProjectsCollapsed?new Set():new Set(catalog.projects.map(project=>project.id)))}/></div>
    {catalog.projects.map(project=><div className="chat-project" key={project.id}><div className="chat-project-heading"><button type="button" onClick={()=>setCollapsed(current=>{const next=new Set(current);if(next.has(project.id))next.delete(project.id);else next.add(project.id);return next;})} aria-expanded={!collapsed.has(project.id)}>{collapsed.has(project.id)?<ChevronRight size={14}/>:<ChevronDown size={14}/>}<FolderOpen size={15}/><span title={project.path}>{project.name}</span></button><Button variant="ghost" size="icon" icon={Plus} aria-label={t("chat.newChat")+" · "+project.name} title={t("chat.newChat")} disabled={busy} onClick={()=>onNew(project.id)}/><DropdownMenu><DropdownMenuTrigger render={<Button variant="ghost" size="icon" icon={MoreHorizontal} aria-label={t("chat.moreFor",{name:project.name})} disabled={busy}/>}/><DropdownMenuContent align="end" className="chat-row-menu"><DropdownMenuItem onClick={()=>onProjectAction(project,"rename")}><PenLine/>{t("chat.renameProject")}</DropdownMenuItem><DropdownMenuItem onClick={()=>onProjectAction(project,"remove")}><Trash2/>{t("chat.removeProject")}</DropdownMenuItem></DropdownMenuContent></DropdownMenu></div>{!collapsed.has(project.id)&&rows(visible.filter(c=>c.projectId===project.id&&!c.pinned))}</div>)}
   </section>
   <section>{heading('conversations',t(archived?"chat.archived":"chat.conversations"))}<SectionCollapse collapsed={!!sections.conversations&&!search}>{rows(visible.filter(c=>!c.projectId&&!c.pinned))}</SectionCollapse></section>
   {search&&!visible.length&&<p className="chat-empty-list">{t("chat.empty")}</p>}
  </ScrollArea>
  <div className="chat-sidebar-footer"><Button variant="ghost" icon={Settings2} onClick={onSettings}>{t("chat.settings")}</Button><Button variant="ghost" size="icon" icon={archived?ArchiveRestore:Archive} aria-label={t(archived?"chat.hideArchived":"chat.showArchived")} title={t(archived?"chat.hideArchived":"chat.showArchived")} aria-pressed={archived} onClick={()=>onArchived(!archived)}/></div>
 </aside>;
}
