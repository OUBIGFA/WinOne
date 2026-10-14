import {useEffect,useRef,useState} from "react";
import {FolderOpen,Plus,Trash2} from "lucide-react";
import {chatCall,pickDirectory} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {PageScrollControls} from "../components/page-scroll-controls";
import {Dialog,DialogContent,DialogTitle,DialogFooter} from "../components/ui/dialog";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "../components/ui/tabs";


import type {AIProvider,AISelection} from "./ai-configuration";
import {AIModelSelect} from "./ai-model-select";
import type {ChatSettingsData,ChatMCPServer} from "./chat-types";

const errorText=(error:unknown)=>error instanceof Error?error.message:String(error);
/** Settings are edited as one draft. A failed save leaves every field intact;
 * closing and reopening also retains this draft until explicitly reset. */
export function ChatSettings({open,onOpenChange,providers,defaultModel,onSaved,onConversation}:{open:boolean;onOpenChange:(open:boolean)=>void;providers:AIProvider[];defaultModel:AISelection;onSaved:()=>void;onConversation:(id:string)=>void}){
 const {t}=useTranslation(),scrollBody=useRef<HTMLDivElement>(null);const [value,setValue]=useState<ChatSettingsData>(),[saved,setSaved]=useState<ChatSettingsData>(),[busy,setBusy]=useState(false),[error,setError]=useState(""),[notice,setNotice]=useState("");
 const dirty=!!value&&JSON.stringify(value)!==JSON.stringify(saved);
 const perform=async(action:()=>Promise<void>)=>{setBusy(true);setError("");setNotice("");try{await action();}catch(error){setError(errorText(error));}finally{setBusy(false);}};
 useEffect(()=>{if(!open||value)return;let alive=true;setBusy(true);void chatCall<ChatSettingsData>("settings.get").then(settings=>{if(alive){setValue(settings);setSaved(settings);}}).catch(error=>{if(alive)setError(errorText(error));}).finally(()=>{if(alive)setBusy(false);});return()=>{alive=false;};},[open]);
 function change(patch:Partial<ChatSettingsData>){setValue(current=>current?{...current,...patch}:current);setNotice("");}
 const updateServer=(index:number,patch:Partial<ChatMCPServer>)=>change({mcpServers:value!.mcpServers.map((server,i)=>i===index?{...server,...patch}:server)});
 function directoryList(key:"skillDirectories"|"pluginDirectories"){
  return <div className="chat-settings-section"><h3>{t("chat."+key)}</h3>{value?.[key].map((path,index)=><div className="chat-settings-directory" key={path}><span>{path}</span><Button variant="ghost" size="icon" icon={Trash2} aria-label={t("chat.removeDirectory")+" · "+path} disabled={busy} onClick={()=>change({[key]:value[key].filter((_,i)=>i!==index)})}/></div>)}<Button icon={FolderOpen} disabled={busy} onClick={()=>void perform(async()=>{const path=await pickDirectory();if(path&&!value![key].includes(path))change({[key]:[...value![key],path]});})}>{t("chat.addDirectory")}</Button></div>;
 }
 return <Dialog open={open} onOpenChange={next=>{if(!busy)onOpenChange(next);}}><DialogContent className="chat-settings-dialog"><DialogTitle>{t("chat.settings")}</DialogTitle>
  {error&&<p role="alert" className="chat-settings-feedback">{error}</p>}
  {!value?<div role="status">{busy?t("chat.settingsLoading"):t("chat.failed")}{!busy&&<Button onClick={()=>void perform(async()=>{const settings=await chatCall<ChatSettingsData>("settings.get");setValue(settings);setSaved(settings);})}>{t("chat.retry")}</Button>}</div>:<Tabs defaultValue="general" className="chat-settings-tabs relative"><TabsList variant="line"><TabsTrigger value="general">{t("chat.general")}</TabsTrigger><TabsTrigger value="resources">{t("chat.resources")}</TabsTrigger><TabsTrigger value="mcp">MCP</TabsTrigger></TabsList>
   <div className="chat-settings-body" ref={scrollBody}>
    <TabsContent value="general"><label className="chat-dialog-field">{t("chat.workspaceDirectory")}<div><input aria-label={t("chat.workspaceDirectory")} value={value.workspaceDirectory} disabled={busy} onChange={event=>change({workspaceDirectory:event.target.value})}/><Button icon={FolderOpen} disabled={busy} onClick={()=>void perform(async()=>{const path=await pickDirectory();if(path)change({workspaceDirectory:path});})}>{t("chat.chooseLocation")}</Button></div><p>{t("chat.workspaceHint")}</p></label><section className="chat-settings-section"><h3>{t("chat.titleModel")}</h3><AIModelSelect providers={providers} value={value.titleModel?.provider?value.titleModel:undefined} onChange={titleModel=>change({titleModel})} disabled={busy} label={t("chat.titleModel")} inheritLabel={t("chat.titleModelInherit")}/><p className="chat-settings-hint">{t("chat.titleModelHint")}</p></section></TabsContent>
    <TabsContent value="resources"><p className="chat-settings-hint">{t("chat.resourceHint")}</p>{directoryList("skillDirectories")}{directoryList("pluginDirectories")}</TabsContent>
    <TabsContent value="mcp">{value.mcpServers.map((server,index)=><section className="chat-settings-section" key={index}><div className="chat-settings-row"><label className="chat-dialog-field">{t("chat.serverName")}<input value={server.name} disabled={busy} onChange={event=>updateServer(index,{name:event.target.value})}/></label><label className="chat-settings-check"><input type="checkbox" checked={server.enabled} disabled={busy} onChange={event=>updateServer(index,{enabled:event.target.checked})}/>{t("chat.enabled")}</label><Button variant="ghost" size="icon" icon={Trash2} aria-label={t("chat.remove")+" · "+server.name} disabled={busy} onClick={()=>change({mcpServers:value.mcpServers.filter((_,i)=>i!==index)})}/></div>
     <label className="chat-dialog-field">{t("chat.transport")}<select value={server.url!==undefined?"url":"command"} disabled={busy} onChange={event=>updateServer(index,event.target.value==="url"?{url:"",command:undefined,args:undefined}:{command:"",url:undefined})}><option value="command">{t("chat.commandTransport")}</option><option value="url">{t("chat.urlTransport")}</option></select></label>
     {server.url!==undefined?<label className="chat-dialog-field">{t("chat.serverUrl")}<input value={server.url} disabled={busy} onChange={event=>updateServer(index,{url:event.target.value})}/></label>:<><label className="chat-dialog-field">{t("chat.command")}<input value={server.command??""} disabled={busy} onChange={event=>updateServer(index,{command:event.target.value})}/></label><label className="chat-dialog-field">{t("chat.arguments")}<textarea rows={3} value={(server.args??[]).join("\n")} disabled={busy} onChange={event=>updateServer(index,{args:event.target.value?event.target.value.split("\n"):[]})}/></label></>}
     <details className="chat-settings-advanced"><summary>{t("chat.advanced")}</summary><label className="chat-dialog-field">{t("chat.sourcePath")}<input value={server.cwd??""} disabled={busy} onChange={event=>updateServer(index,{cwd:event.target.value})}/></label><label className="chat-dialog-field">{t("chat.timeout")}<input type="number" min={1} value={server.timeout??30} disabled={busy} onChange={event=>updateServer(index,{timeout:Number(event.target.value)})}/></label></details>
    </section>)}<Button icon={Plus} disabled={busy} onClick={()=>change({mcpServers:[...value.mcpServers,{name:"",enabled:true,command:"",args:[]}]})}>{t("chat.addServer")}</Button></TabsContent>

   </div>
   <PageScrollControls viewportRef={scrollBody}/>
  </Tabs>}
  <DialogFooter><span role="status" className="chat-settings-status">{notice||dirty&&t("chat.unsavedSettings")}</span><Button disabled={busy||!dirty} onClick={()=>{setValue(saved);setError("");setNotice("");}}>{t("chat.reset")}</Button><Button disabled={busy} onClick={()=>onOpenChange(false)}>{t("chat.close")}</Button><Button variant="primary" disabled={busy||!value||!dirty} onClick={()=>void perform(async()=>{const settings=await chatCall<ChatSettingsData>("settings.save",{settings:value});setValue(settings);setSaved(settings);setNotice("");onSaved();onOpenChange(false);})}>{t("chat.saveSettings")}</Button></DialogFooter>
 </DialogContent></Dialog>;
}
