import "./ai-model-menu.css";
import {CaptureOptionSelect} from "../ui/capture-option-select";
import {useTranslation} from "../i18n";
import {AIModelIcon} from "./ai-model-icon";
import {modelLabel} from "./ai-model-label";
import {AIProviderIcon} from "./ai-provider-icon";
import {AIModelBadge} from "./ai-model-badge";
import type {AIProvider,AISelection} from "./ai-configuration";

// Feature settings store references only. Disabled historical choices remain
// visible as a warning, never silently switch to another provider.
export function AIModelSelect({providers,value,onChange,disabled=false,label,inheritLabel}:{providers:AIProvider[];value?:AISelection;onChange:(value:AISelection|undefined)=>void;disabled?:boolean;label:string;inheritLabel?:string}) {
 const {t}=useTranslation();
 const provider=providers.find(p=>p.id===value?.provider);
 const invalid=!!value?.provider&&(!provider||!provider.models.includes(value.model));
 const providerItems=[...(inheritLabel?[{value:"__inherit",label:inheritLabel},...(value&&!value.provider?[{value:"",label:t("ai.unconfigured")}]:[])]:[{value:"",label:t("ai.chooseProvider")}]),...providers.map(p=>({value:p.id,label:p.name,icon:<AIProviderIcon icon={p.avatar||p.icon}/>}))];
 if(value?.provider&&!provider)providerItems.push({value:value.provider,label:value.provider});
 const modelItems=(provider?.models??[]).map(model=>({value:model,label:modelLabel(provider!,model),icon:<AIModelIcon model={model} name={provider?.modelNames?.[model]}/>,badge:<AIModelBadge provider={provider!} id={model}/>}));
 return <div className="space-y-2">
  <div className="settings-field-grid">
   <CaptureOptionSelect stacked label={t("ai.supplier")} accessibleLabel={`${label} · ${t("ai.supplier")}`} value={value===undefined&&inheritLabel?"__inherit":value?.provider||""} items={providerItems} disabled={disabled} onChange={id=>{const next=providers.find(p=>p.id===id);onChange(next?{provider:id,model:next.models[0]??""}:inheritLabel&&id===""?{provider:"",model:""}:undefined);}}/>
   <CaptureOptionSelect contentClassName="ai-model-menu" stacked label={t("ai.model")} accessibleLabel={`${label} · ${t("ai.model")}`} value={value?.model||""} placeholder={t(value===undefined&&inheritLabel?"ai.inherit":provider?"ai.noEnabledModels":"ai.chooseProviderFirst")} items={modelItems} disabled={disabled||!provider||!modelItems.length} onChange={model=>onChange({provider:provider!.id,model})}/>
  </div>
  {invalid&&<p role="status" className="muted">{t("ai.selectionUnavailable",{model:value?.model||t("ai.unconfigured")})}</p>}
 </div>;
}
