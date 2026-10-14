import {useEffect,useRef,useState} from "react";
import {ChevronRight} from "lucide-react";
import {Dialog,DialogContent,DialogTitle,DialogDescription} from "../components/ui/dialog";
import {Input} from "../components/ui/input";
import {Button} from "../ui/button";
import {useTranslation} from "../i18n";
import {AIProviderIcon} from "./ai-provider-icon";
import type {AIAgent} from "./ai-configuration";

// Choosing is separate from creating: dismissing the picker must not leave an
// empty provider or silently sign in to the first agent in the catalog.
export function AIAgentPicker({open,onOpenChange,agents,onChoose}:{open:boolean;onOpenChange:(open:boolean)=>void;agents:AIAgent[];onChoose:(agent:AIAgent)=>void}){
 const {t}=useTranslation();
 const [query,setQuery]=useState(""),chosen=useRef(false);
 useEffect(()=>{if(open){setQuery("");chosen.current=false;}},[open]);
 const entries=agents.filter(agent=>`${agent.name} ${agent.id}`.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()));
 return <Dialog open={open} onOpenChange={onOpenChange}>
  <DialogContent finalFocus={()=>{
   if(!chosen.current)return true;
   const target=document.getElementById("ai-account-connection");
   // Focus restoration alone prevents scrolling. Bring the next step into
   // the pane when the dialog closes, including short desktop windows.
   target?.scrollIntoView?.({block:"start"});
   return target;
  }}>
   <div className="space-y-2 pr-8"><DialogTitle>{t("ai.chooseAgent")}</DialogTitle><DialogDescription>{t("ai.chooseAgentHint")}</DialogDescription></div>
   <Input aria-label={t("ai.searchAgents")} placeholder={t("ai.searchAgents")} value={query} onChange={event=>setQuery(event.target.value)}/>
   <div className="settings-choice-list">
    {entries.map(agent=><Button key={agent.id} variant="ghost" className="settings-choice w-full min-h-11 justify-start gap-3 px-3 py-2.5 text-left" onClick={()=>{chosen.current=true;onChoose(agent);onOpenChange(false);}}>
     <AIProviderIcon icon={agent.icon}/><span>{agent.name}</span><ChevronRight className="ml-auto text-muted-foreground" aria-hidden="true"/>
    </Button>)}
    {!entries.length&&<p className="settings-empty" role="status">{t("ai.noMatchingAgents")}</p>}
   </div>
  </DialogContent>
 </Dialog>;
}
