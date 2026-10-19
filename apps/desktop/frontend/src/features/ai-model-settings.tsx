import {useId} from "react";
import {SlidersHorizontal} from "lucide-react";
import {Dialog,DialogContent,DialogHeader,DialogTitle,DialogDescription} from "../components/ui/dialog";
import {Input} from "../components/ui/input";
import {Checkbox} from "../components/ui/checkbox";
import {Field,FieldLabel} from "../components/ui/field";
import {CaptureOptionSelect} from "../ui/capture-option-select";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import type {AIProvider,AIModelMetadata,AIModelPrice} from "./ai-configuration";
import {SettingsDisclosure,SettingsOptions} from "../components/settings-layout";

// Magpie a8ca790 app.js contextPicks/outputField; decimal K/M, not Ki/Mi.
// Percentage shortcuts are the user's explicit WinOne adaptation.
const contextPresets=[128000,200000,256000,1000000];
const outputPresets=[8000,16000,32000,64000,128000];
const compactionPresets=[50,60,70,80,90,95];
// Magpie provider/modelprefs.go Levels plus WinOne's retained ultra option.
const reasoningEfforts=["none","minimal","low","medium","high","xhigh","max","ultra"];
const tokenLabel=(value:number)=>value%1000000===0?`${value/1000000}M`:value%1000===0?`${value/1000}K`:String(value);
function PresetButtons<T extends number|string>({label,value,options,disabled,onSelect}:{label:string;value:T|undefined;options:{value:T;label:string}[];disabled:boolean;onSelect:(value:T)=>void}){
 return <div role="group" aria-label={label} className="settings-preset-actions">{options.map(option=><Button key={option.label} variant="secondary" disabled={disabled} aria-pressed={value===option.value} onClick={()=>onSelect(option.value)}>{option.label}</Button>)}</div>;
}

// Reuse the provider's save flow: editing a model never changes its routing ID
// and closing this sheet keeps the unsaved form draft visible to navigation.
export function AIModelSettings({provider,model,disabled,onChange,onClose}:{provider:AIProvider;model:string;disabled:boolean;onChange:(patch:Partial<AIProvider>)=>void;onClose:()=>void}){
 const {t}=useTranslation(),id=useId(),metadata=provider.modelMetadata?.[model]??{};
 function change(patch:Partial<AIModelMetadata>){onChange({modelMetadata:{...provider.modelMetadata,[model]:{...metadata,...patch}}});}
 const defaults={value:"",label:t("ai.modelDefault")};
 // A known vendor list governs the request choices; older or manual models
 // keep the full list. Retain a saved override so refreshing never hides it.
 const offeredEfforts=[...new Set([...(metadata.efforts?.length?metadata.efforts:reasoningEfforts),...(metadata.reasoningEffort?[metadata.reasoningEffort]:[])])];
 const offeredTiers=[...new Set(["auto","default",...(metadata.tiers?.length?metadata.tiers:["flex","priority","ultrafast"]),...(metadata.tier?[metadata.tier]:[])])];
 const configuredPercent=metadata.compactionPercent??0;
 const legacyThreshold=configuredPercent<=0?metadata.compactionThreshold??0:0;
 // Reading legacy metadata is not a migration. Its token count remains intact
 // until the user edits this field; unrelated settings keep the old behavior.
 const legacyPercent=legacyThreshold>0&&metadata.contextWindow?legacyThreshold/metadata.contextWindow*100:undefined;
 const percent=configuredPercent>0?configuredPercent:legacyPercent!==undefined&&legacyPercent<=100?Number(legacyPercent.toFixed(4)):undefined;
 const windowOptions=contextPresets.map(value=>({value,label:tokenLabel(value)}));
 if(metadata.maxContextWindow&&metadata.maxContextWindow>(metadata.listedContextWindow??0))windowOptions.push({value:metadata.maxContextWindow,label:t("ai.modelMaximumContext",{size:tokenLabel(metadata.maxContextWindow)})});
 return <Dialog open onOpenChange={open=>{if(!open)onClose();}}><DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-xl"><DialogHeader><DialogTitle className="flex items-center gap-2"><SlidersHorizontal size={18}/>{t("ai.modelSettings")}</DialogTitle><DialogDescription>{model}</DialogDescription></DialogHeader>
  <Field><FieldLabel htmlFor={id+"-name"}>{t("ai.modelDisplayName")}</FieldLabel><Input id={id+"-name"} value={provider.modelNames?.[model]??""} placeholder={model} maxLength={80} disabled={disabled} onChange={event=>{const modelNames={...provider.modelNames};if(event.target.value)modelNames[model]=event.target.value;else delete modelNames[model];onChange({modelNames});}}/></Field>
  <div className="grid gap-4 sm:grid-cols-2">{(["upstreamName","same"] as const).map(key=><Field key={key}><FieldLabel htmlFor={id+key}>{t("ai.model"+key)}</FieldLabel><Input id={id+key} value={metadata[key]??""} placeholder={model} disabled={disabled} spellCheck={false} onChange={event=>change({[key]:event.target.value})}/><p className="settings-help">{t("ai.model"+key+"Hint")}</p></Field>)}</div>
  <CaptureOptionSelect stacked label={t("ai.modelAPI")} value={metadata.api??""} disabled={disabled} items={[{value:"",label:t("ai.modelDefault")},{value:"chat",label:"OpenAI Chat Completions"},{value:"responses",label:"OpenAI Responses"},{value:"anthropic",label:"Anthropic Messages"}]} onChange={api=>change({api})}/>
  <div role="group" aria-label={t("ai.modelOfferedEfforts")} className="space-y-2"><h3 className="settings-group-title">{t("ai.modelOfferedEfforts")}</h3><div className="flex flex-wrap gap-x-4 gap-y-2">{reasoningEfforts.map(effort=><label key={effort} className="flex items-center gap-1.5"><Checkbox checked={metadata.efforts?.includes(effort)??false} disabled={disabled} onCheckedChange={checked=>change({efforts:reasoningEfforts.filter(value=>value===effort?checked===true:metadata.efforts?.includes(value))})}/>{effort}</label>)}</div><p className="settings-help">{t("ai.modelOfferedEffortsHint")}</p></div>
  <AIModelPricing value={metadata.price} disabled={disabled} onChange={price=>change({price})}/>
  <Button disabled={disabled} onClick={()=>{const next={...metadata},modelNames={...provider.modelNames};for(const key of ["same","upstreamName","api","efforts","price","vision"] as const)delete next[key];delete modelNames[model];onChange({modelNames,modelMetadata:{...provider.modelMetadata,[model]:next}});}}>{t("ai.modelRestoreDefaults")}</Button>
  <div className="grid gap-4 sm:grid-cols-2">
   <Field><FieldLabel htmlFor={id+"contextWindow"}>{t("ai.contextWindow")}</FieldLabel><Input id={id+"contextWindow"} type="number" min={0} max={100000000} step={1} placeholder={t("ai.modelDefault")} value={metadata.contextWindow||""} disabled={disabled} onChange={event=>change({contextWindow:Number(event.target.value)})}/><PresetButtons label={t("ai.contextPresets")} value={metadata.contextWindow} options={windowOptions} disabled={disabled} onSelect={contextWindow=>change({contextWindow})}/></Field>
   <Field><FieldLabel htmlFor={id+"maxTokens"}>{t("ai.maxTokens")}</FieldLabel><Input id={id+"maxTokens"} type="number" min={0} max={100000000} step={1} placeholder={t("ai.modelDefault")} value={metadata.maxTokens||""} disabled={disabled} onChange={event=>change({maxTokens:Number(event.target.value)})}/><PresetButtons label={t("ai.outputPresets")} value={metadata.maxTokens} options={outputPresets.map(value=>({value,label:tokenLabel(value)}))} disabled={disabled} onSelect={maxTokens=>change({maxTokens})}/></Field>
   <Field className="sm:col-span-2"><FieldLabel htmlFor={id+"compactionPercent"}>{t("ai.compactionPercent")}</FieldLabel><Input id={id+"compactionPercent"} type="number" min={0} max={100} step="any" placeholder={t("ai.modelDefault")} value={percent??""} disabled={disabled} onChange={event=>change({compactionPercent:Number(event.target.value),compactionThreshold:0})}/><PresetButtons label={t("ai.compactionPresets")} value={legacyThreshold?undefined:percent} options={compactionPresets.map(value=>({value,label:`${value}%`}))} disabled={disabled} onSelect={compactionPercent=>change({compactionPercent,compactionThreshold:0})}/><p className="settings-help">{legacyThreshold?t("ai.legacyCompactionHint",{tokens:legacyThreshold.toLocaleString()}):metadata.contextWindow&&percent?t("ai.compactionTokenPreview",{tokens:Math.floor(metadata.contextWindow*percent/100).toLocaleString(),percent}):t("ai.compactionPercentHint")}</p></Field>
   <div className="space-y-2 sm:col-span-2"><CaptureOptionSelect stacked label={t("ai.reasoningEffort")} value={metadata.reasoningEffort??""} disabled={disabled} items={[defaults,...offeredEfforts.map(value=>({value,label:value}))]} onChange={reasoningEffort=>change({reasoningEffort})}/><PresetButtons label={t("ai.reasoningPresets")} value={metadata.reasoningEffort??""} options={[defaults,...offeredEfforts.map(value=>({value,label:value}))]} disabled={disabled} onSelect={reasoningEffort=>change({reasoningEffort})}/></div>
   <CaptureOptionSelect stacked label={t("ai.modelTier")} value={metadata.tier??""} disabled={disabled} items={[defaults,...offeredTiers.map(value=>({value,label:value}))]} onChange={tier=>change({tier})}/>
   {(["reasoning","vision","tools"] as const).map(key=><SettingsOptions key={key} label={t("ai.capability"+key)} value={metadata[key]===undefined?"":String(metadata[key])} disabled={disabled} items={[defaults,{value:"true",label:t("ai.capabilityEnabled")},{value:"false",label:t("ai.capabilityDisabled")}]} onChange={value=>change({[key]:value===""?undefined:value==="true"})}/>)}
  </div><p className="settings-help">{t("ai.modelMetadataHint")}</p>
 </DialogContent></Dialog>;
}

// Magpie Names & levels keeps prices in dollars per million tokens, including
// an optional long-context tier. Empty cells retain list-price semantics.
function AIModelPricing({value,disabled,onChange}:{value?:AIModelPrice;disabled:boolean;onChange:(value:AIModelPrice|undefined)=>void}){
 const {t}=useTranslation(),id=useId();
 const parts=["input","output","cache_read","cache_write","cache_write_1h"] as const;
 function update(key:typeof parts[number],text:string,tier=false){const next={...value},part=tier?{...next.tiers?.[0]}:next;if(text==="")delete part[key];else part[key]=Number(text);if(tier)next.tiers=[{...part,above:next.tiers?.[0]?.above??0},...(next.tiers?.slice(1)??[])];onChange(Object.keys(next).length?next:undefined);}
 return <section className="space-y-3 border-t border-border pt-4"><h3 className="settings-group-title">{t("ai.modelPrice")}</h3><div className="grid gap-3 sm:grid-cols-2">{parts.map(key=><Field key={key}><FieldLabel htmlFor={id+key}>{t("ai.modelPrice"+key)}</FieldLabel><Input id={id+key} type="number" min={0} step="any" disabled={disabled} placeholder={t("ai.modelListPrice")} value={value?.[key]??""} onChange={event=>update(key,event.target.value)}/></Field>)}</div><details><summary className="cursor-pointer text-sm">{t("ai.modelLongContextPrice")}</summary><div className="mt-3 grid gap-3 sm:grid-cols-2"><Field><FieldLabel htmlFor={id+"above"}>{t("ai.modelPriceAbove")}</FieldLabel><Input id={id+"above"} type="number" min={1} step={1} disabled={disabled} value={value?.tiers?.[0]?.above||""} onChange={event=>onChange({...value,tiers:event.target.value?[{...value?.tiers?.[0],above:Number(event.target.value)},...(value?.tiers?.slice(1)??[])]:undefined})}/></Field>{parts.slice(0,4).map(key=><Field key={key}><FieldLabel htmlFor={id+"tier"+key}>{t("ai.modelLongContextPrice")} · {t("ai.modelPrice"+key)}</FieldLabel><Input id={id+"tier"+key} type="number" min={0} step="any" disabled={disabled} value={value?.tiers?.[0]?.[key]??""} placeholder={t("ai.modelListPrice")} onChange={event=>update(key,event.target.value,true)}/></Field>)}</div></details><p className="settings-help">{t("ai.modelPriceHint")}</p></section>;
}

export function AIRequestLimits({provider,disabled,onChange}:{provider:AIProvider;disabled:boolean;onChange:(patch:Partial<AIProvider>)=>void}){
 const {t}=useTranslation(),id=useId();
 return <SettingsDisclosure title={t("ai.requestLimits")} defaultOpen={!!(provider.maxConcurrency||provider.maxRPM||provider.queueLimit||provider.queueWait)}><div className="settings-field-grid">
  {(["maxConcurrency","maxRPM","queueLimit","queueWait"] as const).map(key=><Field key={key}><FieldLabel htmlFor={id+key}>{t("ai."+key)}</FieldLabel><Input id={id+key} type="number" min={0} max={key==="queueWait"?3600:key==="maxConcurrency"?1000:10000} step={1} placeholder={t("ai.noRequestLimit")} value={provider[key]||""} disabled={disabled} onChange={event=>onChange({[key]:Number(event.target.value)})}/></Field>)}
 </div><p className="settings-help">{t("ai.requestLimitsHint")}</p></SettingsDisclosure>;
}
