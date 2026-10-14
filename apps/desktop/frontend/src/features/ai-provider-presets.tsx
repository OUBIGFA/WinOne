import {useState} from "react";
import {PlugZap} from "lucide-react";
import {Button} from "../ui/button";
import {Input} from "../components/ui/input";
import {Dialog,DialogContent,DialogTitle} from "../components/ui/dialog";
import {Notice} from "../ui/notice";
import {useTranslation} from "../i18n";
import {AIProviderIcon} from "./ai-provider-icon";

type Endpoints={chat?:string;responses?:string;anthropic?:string};
export type AIServicePreset=Endpoints&{id:string;name:string;icon?:string;models?:string[];endpoint?:string;kind?:string;noKey?:boolean;regions?:({id:string;name:string}&Endpoints)[]};

// Preserve the preset's endpoint for the chosen API. A user can still edit
// the address afterwards; the worker treats that as an explicit destination.
export function presetConnection(preset:AIServicePreset,region?:string,preferred="chat"){
 const endpoints=preset.regions?.find(value=>value.id===region)??preset;
 const choices=["chat","responses","anthropic"] as const;
 const protocol=choices.find(value=>value===preferred&&endpoints[value])??choices.find(value=>endpoints[value])??"chat";
 return {protocol,baseUrl:endpoints[protocol]??preset.endpoint??""};
}

export function AIProviderPresets({presets,busy,error,disabled,load,onAdd}:{presets:AIServicePreset[];busy:boolean;error:string;disabled:boolean;load:()=>Promise<void>;onAdd:(preset:AIServicePreset)=>void}){
 const {t}=useTranslation();const [open,setOpen]=useState(false),[search,setSearch]=useState("");
 // Magpie also catalogs decision-only routers, which select another gateway
 // provider rather than generate text. This picker offers inference services.
 const entries=presets.filter(preset=>(preset.chat||preset.responses||preset.anthropic||preset.endpoint)&&(preset.name+" "+preset.id).toLocaleLowerCase().includes(search.toLocaleLowerCase().trim()));
 return <><Button icon={PlugZap} disabled={disabled} onClick={()=>{setOpen(true);setSearch("");if(!presets.length)void load();}}>{t("ai.addPreset")}</Button>
  <Dialog open={open} onOpenChange={setOpen}><DialogContent>
   <DialogTitle>{t("ai.choosePreset")}</DialogTitle>
   <Input aria-label={t("ai.searchProviders")} placeholder={t("ai.searchProviders")} value={search} onChange={event=>setSearch(event.target.value)}/>
   {error&&<Notice actions={<Button onClick={()=>void load()}>{t("common.retry")}</Button>}>{error}</Notice>}
   <div className="settings-choice-list" aria-busy={busy}>
    {busy?<p role="status">{t("capture.loading")}</p>:entries.map(preset=><Button key={preset.id} variant="ghost" className="settings-choice w-full min-h-11 justify-start gap-3 px-3 py-2.5 text-left" onClick={()=>{onAdd(preset);setOpen(false);}}><AIProviderIcon icon={preset.icon}/><span>{preset.name}</span></Button>)}
    {!busy&&!entries.length&&!error&&<p className="muted">{t("ai.noMatchingServices")}</p>}
   </div>
  </DialogContent></Dialog>
 </>;
}
