import "./ai-model-menu.css";
import {useState} from "react";
import {Ban,Bot,ChevronDown,Sparkles} from "lucide-react";
import {useTranslation} from "../i18n";
import {Popover,PopoverTrigger,PopoverContent} from "../components/ui/popover";
import {modelLabel} from "./ai-model-label";
import {AIProviderIcon} from "./ai-provider-icon";
import {AIModelBadge} from "./ai-model-badge";
import {AIModelMenuContent,type AIModelChoice} from "./ai-model-menu";
import type {AIProvider,AISelection} from "./ai-configuration";

const inherit="__inherit";

// Feature settings store references only. Disabled historical choices remain
// visible as a warning, never silently switch to another provider. The field
// opens the shared model menu (AIModelMenuContent), used throughout WinOne.
export function AIModelSelect({providers,value,onChange,disabled=false,label,inheritLabel}:{providers:AIProvider[];value?:AISelection;onChange:(value:AISelection|undefined)=>void;disabled?:boolean;label:string;inheritLabel?:string}) {
 const {t}=useTranslation();
 const [open,setOpen]=useState(false);
 const provider=providers.find(p=>p.id===value?.provider);
 const invalid=!!value?.provider&&(!provider||!provider.models.includes(value.model));
 const choices:AIModelChoice[]=inheritLabel?[{value:inherit,label:inheritLabel,icon:<Sparkles size={15}/>},...(value&&!value.provider?[{value:"",label:t("ai.unconfigured"),icon:<Ban size={15}/>}]:[])]:value?[{value:"",label:t("ai.unconfigured"),icon:<Ban size={15}/>}]:[];
 const choice=value===undefined&&inheritLabel?inherit:value&&!value.provider?"":undefined;
 const shownChoice=choices.find(item=>item.value===choice);
 const name=provider&&value?.model?modelLabel(provider,value.model):value?.model??"";
 const text=shownChoice?shownChoice.label:value?.provider?`${provider?.name??value.provider}${name?" · "+name:""}`:t("aiShared.chooseModel");
 const accessible=`${label} · ${t("aiShared.model")}`;
 return <div className="space-y-2">
  <label className="ai-model-field-label">{t("aiShared.model")}</label>
  <Popover open={open} onOpenChange={next=>{if(!disabled)setOpen(next);}}>
   <PopoverTrigger render={<button type="button" className="ai-model-field" disabled={disabled} aria-label={accessible} title={text}/>}>
    <span className="ai-model-field-value">
     {shownChoice?shownChoice.icon:provider?<AIProviderIcon icon={provider.avatar||provider.icon}/>:value?.provider?null:<Bot size={15}/>}
     {provider&&!shownChoice?<><span className="chat-provider-name">{provider.name}</span><span aria-hidden="true">·</span><span className="chat-model-name">{name}</span>{value?.model&&provider.models.includes(value.model)&&<AIModelBadge provider={provider} id={value.model}/>}</>:<span className={value===undefined&&!inheritLabel?"chat-model-name muted":"chat-model-name"}>{text}</span>}
    </span>
    <ChevronDown size={14}/>
   </PopoverTrigger>
   <PopoverContent align="start" className="ai-model-menu chat-model-menu">
    <AIModelMenuContent providers={providers} selected={value} choice={choice} choices={choices} open={open} label={accessible}
     onModel={next=>{setOpen(false);if(next.provider!==value?.provider||next.model!==value?.model||choice!==undefined)onChange(next);}}
     onChoice={next=>{setOpen(false);if(next===choice)return;onChange(next===inherit?undefined:inheritLabel?{provider:"",model:""}:undefined);}}/>
   </PopoverContent>
  </Popover>
  {invalid&&<p role="status" className="muted">{t("ai.selectionUnavailable",{model:value?.model||t("ai.unconfigured")})}</p>}
 </div>;
}
