import {useEffect,useId,useRef,useState} from "react";
import {useTranslation} from "../i18n";
import {AIModelSelectionActions} from "./ai-model-selection-actions";
import {Input} from "../components/ui/input";
import {Checkbox} from "../components/ui/checkbox";
import {AIProviderIcon} from "./ai-provider-icon";
import {AIModelIcon} from "./ai-model-icon";
import {modelLabel,isFreeModel} from "./ai-model-label";
import {AIModelBadge} from "./ai-model-badge";
import type {AIProvider,AISelection} from "./ai-configuration";

const route=(value:AISelection)=>value.provider+"/"+value.model;

// Gateway choices are separate from the feature model list. Route IDs remain
// internal; names, brand icons and checkboxes are the user-facing hierarchy.
export function AIGatewayModels({providers,value,disabled,onChange}:{providers:AIProvider[];value:AISelection[]|null;disabled:boolean;onChange:(value:AISelection[])=>void}){
 const {t}=useTranslation();const [query,setQuery]=useState("");const id=useId();
 const list=useRef<HTMLDivElement>(null);
 const storageKey="winone.gateway-model-height.v1";
 const [height]=useState(()=>{try{const saved=Number(localStorage.getItem(storageKey));return saved>=160&&saved<=1200?saved:400;}catch(error){console.warn("Gateway model size unavailable",error);return 400;}});
 // Observe the native resize grip and remember its explicit height, not a viewport clamp.
 useEffect(()=>{const element=list.current;if(!element||typeof ResizeObserver==="undefined")return;const observer=new ResizeObserver(()=>{const value=Number.parseFloat(element.style.height);if(value>=160&&value<=1200)try{localStorage.setItem(storageKey,String(value));}catch(error){console.warn("Gateway model size could not be saved",error);}});observer.observe(element);return()=>observer.disconnect();},[providers.length]);
 const all=providers.flatMap(p=>p.models.map(model=>({provider:p.id,model})));
 const enabled=new Set((value??all).map(route));
 const count=all.filter(item=>enabled.has(route(item))).length;
 const terms=query.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean);
 const groups=providers.map(provider=>({provider,models:provider.models.filter(model=>terms.every(term=>(provider.name+" "+modelLabel(provider,model)+(isFreeModel(provider,model)?" "+t("ai.free"):"")).toLocaleLowerCase().includes(term)))})).filter(group=>group.models.length);
 function toggle(items:AISelection[],checked:boolean){
  const next=new Set(enabled);for(const item of items){if(checked)next.add(route(item));else next.delete(route(item));}
  onChange(all.filter(item=>next.has(route(item))));
 }
 return <section className="settings-group gap-3" aria-label={t("gateway.models")}>
  <div className="settings-group-heading"><div className="flex flex-wrap items-center justify-between gap-2"><h3 className="settings-group-title">{t("gateway.models")}</h3><span className="text-sm text-muted-foreground">{t("gateway.enabledModels",{count,total:all.length})}</span></div>
  <p className="settings-group-description">{t("gateway.modelsHint")}</p></div>
  {all.length>0?<>
   <div className="flex flex-wrap gap-2"><Input className="min-w-0 flex-1 basis-48" aria-label={t("gateway.searchModels")} placeholder={t("gateway.searchModels")} value={query} onChange={e=>setQuery(e.target.value)}/><AIModelSelectionActions total={all.length} selected={count} disabled={disabled} onAll={()=>onChange(all)} onNone={()=>onChange([])}/></div>
   <div ref={list} style={{height}} title={t("ai.resizeModelList")} className="gateway-model-list min-h-40 max-h-[1200px] resize-y overflow-y-auto overflow-x-hidden rounded-lg border border-border p-2 space-y-3">
    {groups.map(({provider,models})=>{
     const chosen=provider.models.filter(model=>enabled.has(route({provider:provider.id,model}))).length;
     const refs=provider.models.map(model=>({provider:provider.id,model}));
     const providerLabel=id+"-"+provider.id;
     return <div key={provider.id} className="gateway-model-group space-y-1" data-provider={provider.id}>
      <label className="flex cursor-pointer items-center gap-3 rounded-md bg-muted/70 px-2 py-2"><Checkbox aria-labelledby={providerLabel} checked={chosen===provider.models.length} indeterminate={chosen>0&&chosen<provider.models.length} disabled={disabled} onCheckedChange={checked=>toggle(refs,checked)}/><AIProviderIcon icon={provider.avatar||provider.icon}/><span id={providerLabel} className="min-w-0 flex-1 font-medium [overflow-wrap:anywhere]">{provider.name}</span><span className="shrink-0 text-sm text-muted-foreground">{chosen} / {provider.models.length}</span></label>
      {models.map((model,index)=>{const ref={provider:provider.id,model},name=modelLabel(provider,model),label=providerLabel+"-model-"+index;return <label key={model} className="flex cursor-pointer items-center gap-1.5 rounded-md py-2 pl-7 pr-2 hover:bg-muted focus-within:bg-muted"><Checkbox aria-labelledby={providerLabel+" "+label} checked={enabled.has(route(ref))} disabled={disabled} onCheckedChange={checked=>toggle([ref],checked)}/><AIModelIcon model={model} name={provider.modelNames?.[model]}/><span id={label} className="min-w-0 [overflow-wrap:anywhere]">{name}</span><AIModelBadge provider={provider} id={model}/></label>})}
     </div>;
    })}
    {!groups.length&&<p className="settings-empty">{t("gateway.noMatchingModels")}</p>}
   </div>
  </>:<p className="muted">{t("gateway.empty")}</p>}
 </section>;
}
