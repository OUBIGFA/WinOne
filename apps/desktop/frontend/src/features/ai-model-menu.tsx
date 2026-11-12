import "./ai-model-menu.css";
import {useEffect,useRef,useState,type ReactNode} from "react";
import {Bot,Check,ChevronDown,ChevronRight} from "lucide-react";
import {useTranslation} from "../i18n";
import {AIModelIcon} from "./ai-model-icon";
import {AIProviderIcon} from "./ai-provider-icon";
import {AIModelBadge} from "./ai-model-badge";
import {modelLabel} from "./ai-model-label";
import {ScrollArea} from "./lobe/scroll-area/ScrollArea";
import type {AIProvider,AISelection} from "./ai-configuration";

/** An entry above the models that is not one: following the default, off. */
export interface AIModelChoice{value:string;label:string;icon?:ReactNode}

/** The one model list every picker opens, the Chat composer's: a search
 * field, models grouped under their provider with a count, groups that
 * fold, each model's mark, label and badges, and a check on the chosen one.
 * The native window draws the same list (widgets/modelmenu.go). */
export function AIModelMenuContent({providers,selected,choice,choices=[],open,label,onModel,onChoice}:{providers:AIProvider[];selected?:AISelection;choice?:string;choices?:AIModelChoice[];open:boolean;label:string;onModel:(value:AISelection)=>void;onChoice?:(value:string)=>void}){
 const {t}=useTranslation();
 const [search,setSearch]=useState(""),[collapsed,setCollapsed]=useState(new Set<string>());
 const chosen=useRef<HTMLButtonElement>(null);
 useEffect(()=>{
  if(!open)return;
  setSearch('');
  setCollapsed(current=>{const next=new Set(current);if(selected?.provider)next.delete(selected.provider);return next;});
  // A long list opens at the chosen model, as the native menu does.
  const frame=requestAnimationFrame(()=>chosen.current?.scrollIntoView({block:'center',inline:'nearest',behavior:'instant'}));
  return()=>cancelAnimationFrame(frame);
 // Opening only; the selection is read as it was then.
 // eslint-disable-next-line react-hooks/exhaustive-deps
 },[open]);
 const query=search.toLowerCase();
 const shownChoices=choices.filter(item=>!query||item.label.toLowerCase().includes(query));
 const any=shownChoices.length>0||providers.some(p=>p.models.some(id=>(p.name+" "+id+" "+(p.modelNames?.[id]??"")+" "+modelLabel(p,id)).toLowerCase().includes(query)));
 return <>
  <input autoFocus aria-label={t("aiShared.searchModels")} placeholder={t("aiShared.searchModels")} value={search} onChange={event=>setSearch(event.target.value)}/>
  <ScrollArea disableContentFit viewportProps={{className:"chat-model-viewport"}}><div role="listbox" aria-label={label}>
   {shownChoices.map(item=><button key={"choice:"+item.value} ref={choice===item.value?chosen:undefined} type="button" role="option" aria-selected={choice===item.value} className="ai-model-choice" onClick={()=>onChoice?.(item.value)}>{item.icon??<Bot size={15}/>}<span className="chat-model-label">{item.label}</span><span className="chat-model-check" aria-hidden="true">{choice===item.value&&<Check size={14}/>}</span></button>)}
   {providers.map(p=>{
    const matches=p.models.filter(id=>(p.name+" "+id+" "+(p.modelNames?.[id]??"")+" "+modelLabel(p,id)).toLowerCase().includes(query));
    const expanded=!!search||!collapsed.has(p.id);
    return matches.length>0&&<section key={p.id}><h4><button type="button" className="chat-model-group" aria-label={p.name} aria-expanded={expanded} onClick={()=>setCollapsed(current=>{const next=new Set(current);if(next.has(p.id))next.delete(p.id);else next.add(p.id);return next;})}><AIProviderIcon icon={p.avatar||p.icon}/><span className="chat-model-label">{p.name}</span><small>{matches.length}</small>{expanded?<ChevronDown size={13}/>:<ChevronRight size={13}/>}</button></h4>
     {expanded&&matches.map(id=>{const on=!choice&&selected?.provider===p.id&&selected?.model===id;return <button key={id} ref={on?chosen:undefined} type="button" role="option" aria-selected={on} onClick={()=>onModel({provider:p.id,model:id})}><AIModelIcon model={id} name={p.modelNames?.[id]}/><span className="chat-model-label">{modelLabel(p,id)}</span><AIModelBadge provider={p} id={id}/><span className="chat-model-check" aria-hidden="true">{on&&<Check size={14}/>}</span></button>;})}
    </section>;
   })}
  </div></ScrollArea>
  {!any&&<p>{t("aiShared.modelUnavailable")}</p>}
 </>;
}
