import {useEffect,useRef,useState} from "react";
import {Check,ChevronDown,FolderOpen,FolderPlus,GitBranch,LoaderCircle,RefreshCw} from "lucide-react";
import {chatCall} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {DropdownMenu,DropdownMenuContent,DropdownMenuItem,DropdownMenuSeparator,DropdownMenuTrigger} from "../components/ui/dropdown-menu";
import type {ChatProject} from "./chat-state";
import "./chat-project-context.css";

interface GitState {projectId:string;repository:boolean;branch?:string;commit?:string;detached?:boolean;unborn?:boolean}
type GitView={key:string;loading:boolean;value?:GitState;error?:string};

// Project context sits above the new-chat composer, using the existing WinOne
// menu controls. Branch data is only a read of the selected project's real HEAD.
export function ChatProjectContext({projects,projectId,onProject,onOpenProject,disabled}:{projects:ChatProject[];projectId:string;onProject:(id:string)=>void;onOpenProject:()=>void;disabled:boolean}){
 const {t}=useTranslation(),project=projects.find(project=>project.id===projectId),key=project?project.id+"\0"+project.path:"";
 const [git,setGit]=useState<GitView>(),generation=useRef(0),reload=useRef<(()=>void)|undefined>(undefined);
 useEffect(()=>{
  let disposed=false;if(!project){reload.current=undefined;setGit(undefined);return;}
  const refresh=()=>{const request=++generation.current;setGit(previous=>({key,loading:true,value:previous?.key===key?previous.value:undefined}));void chatCall<GitState>("project.git",{projectId:project.id}).then(value=>{
   if(disposed||request!==generation.current)return;
   if(value.projectId!==project.id||typeof value.repository!=="boolean"||value.repository&&!(value.detached?value.commit:value.branch))throw Error(t("chat.gitInvalidResponse"));
   setGit({key,loading:false,value});
  }).catch(error=>{if(!disposed&&request===generation.current)setGit({key,loading:false,error:error instanceof Error?error.message:String(error)});});};
  // Window activation is not a Git change. Avoid repeated reads and flicker
  // when menus, file dialogs or another application return focus to Chat.
  reload.current=refresh;refresh();
  return()=>{disposed=true;generation.current++;if(reload.current===refresh)reload.current=undefined;};
 },[key,t]);
 // The render-time key check removes the old branch in the same render that
 // changes the project, before the next effect or a slower request can settle.
 const current=git?.key===key?git:undefined,loading=!!project&&(!current||current.loading),value=current?.value;
 const branch=loading&&!value?t("chat.readGitBranch"):current?.error?t("chat.gitBranchFailed"):!value?.repository?t("chat.noGitBranch"):value.detached?t("chat.gitDetached",{commit:value.commit}):value.branch;
 const title=loading?t("chat.readGitBranch"):current?.error??(value?.repository?value.unborn?t("chat.gitUnborn"):branch:t("chat.notGitRepository"));
 const name=project?.name??t(projectId?"chat.projectUnavailable":"chat.noProject");
 return <div className="client-composer-context chat-project-context" aria-label={t("chat.projectContext")}>
  <DropdownMenu><DropdownMenuTrigger render={<button type="button" className="client-context-select chat-project-choice" disabled={disabled} title={project?.path} aria-label={t("chat.projectContextLabel",{name})}><FolderOpen size={16}/><span>{name}</span><ChevronDown size={14}/></button>}/><DropdownMenuContent align="start" className="chat-project-choice-menu">
   <DropdownMenuItem onClick={()=>onProject("")}><FolderOpen/><span>{t("chat.noProject")}</span>{!projectId&&<Check className="ml-auto"/>}</DropdownMenuItem>
   {projects.map(project=><DropdownMenuItem key={project.id} onClick={()=>onProject(project.id)}><FolderOpen/><span className="min-w-0 flex-1"><span className="block truncate">{project.name}</span><span className="block truncate text-[13px] text-muted-foreground">{project.path}</span></span>{project.id===projectId&&<Check/>}</DropdownMenuItem>)}
   <DropdownMenuSeparator/><DropdownMenuItem onClick={onOpenProject}><FolderPlus/><span>{t("chat.openOrCreateProject")}</span></DropdownMenuItem>
  </DropdownMenuContent></DropdownMenu>
  {project&&(loading||current?.error||value?.repository)&&<div className="chat-project-git"><GitBranch size={15}/><span role="status" aria-label={t("chat.gitBranch")} title={title}>{branch}</span>{loading?<LoaderCircle className="animate-spin" size={13}/>:<Button variant="ghost" size="icon" className="chat-project-refresh" icon={RefreshCw} aria-label={t("chat.refreshGitBranch")} title={t("chat.refreshGitBranch")} disabled={disabled} onClick={()=>reload.current?.()}/>}</div>}
 </div>;
}
