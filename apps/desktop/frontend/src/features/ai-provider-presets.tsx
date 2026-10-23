import {useState} from "react";
import {PlugZap} from "lucide-react";
import {Button} from "../ui/button";
import {Input} from "../components/ui/input";
import {Dialog,DialogContent,DialogTitle} from "../components/ui/dialog";
import {Notice} from "../ui/notice";
import {useTranslation} from "../i18n";
import {AIProviderIcon} from "./ai-provider-icon";

type Endpoints={chat?:string;responses?:string;anthropic?:string};
export type AIServicePreset=Endpoints&{id:string;name:string;icon?:string;models?:string[];endpoint?:string;kind?:string;note?:string;short?:string;noKey?:boolean;sponsored?:boolean;website?:string;siteHost?:string;keysUrl?:string;langs?:string[];notes?:Record<string,string>;regions?:({id:string;name:string}&Endpoints)[]};

// Preserve the preset's endpoint for the chosen API. A user can still edit
// the address afterwards; the worker treats that as an explicit destination.
export function presetConnection(preset:AIServicePreset,region?:string,preferred="chat"){
 const endpoints=preset.regions?.find(value=>value.id===region)??preset;
 const choices=["chat","responses","anthropic"] as const;
 const protocol=choices.find(value=>value===preferred&&endpoints[value])??choices.find(value=>endpoints[value])??"chat";
 return {protocol,baseUrl:endpoints[protocol]??preset.endpoint??""};
}

export function AIProviderPresets({presets,busy,error,disabled,load,onAdd}:{presets:AIServicePreset[];busy:boolean;error:string;disabled:boolean;load:()=>Promise<void>;onAdd:(preset:AIServicePreset)=>void}){
 const {t,i18n}=useTranslation();const language=i18n.language.startsWith("zh")?"zh":"en";const [open,setOpen]=useState(false),[search,setSearch]=useState(""),[picture,setPicture]=useState<AIServicePreset>();
 // Magpie also catalogs decision-only routers, which select another gateway
 // provider rather than generate text. This picker offers inference services.
 const entries=presets.filter(preset=>(!preset.langs?.length||preset.langs.some(lang=>lang===language||lang===i18n.language))&&(preset.chat||preset.responses||preset.anthropic||preset.endpoint)&&(preset.name+" "+preset.id).toLocaleLowerCase().includes(search.toLocaleLowerCase().trim()));
 return <><Button icon={PlugZap} disabled={disabled} onClick={()=>{setOpen(true);setSearch("");void load();}}>{t("ai.addPreset")}</Button>
  <Dialog open={open} onOpenChange={setOpen}><DialogContent>
   <DialogTitle>{t("ai.choosePreset")}</DialogTitle>
   <Input aria-label={t("ai.searchProviders")} placeholder={t("ai.searchProviders")} value={search} onChange={event=>setSearch(event.target.value)}/>
   {error&&<Notice actions={<Button onClick={()=>void load()}>{t("common.retry")}</Button>}>{error}</Notice>}
   <div className="settings-choice-list" aria-busy={busy}>
    {busy?<p role="status">{t("capture.loading")}</p>:entries.map(preset=><div key={preset.id} className="min-w-0"><Button variant="ghost" className="settings-choice w-full min-h-11 justify-start gap-3 px-3 py-2.5 text-left" onClick={()=>{onAdd(preset);setOpen(false);}}><AIProviderIcon icon={preset.icon}/><span>{preset.name}{preset.sponsored&&<span className="muted ml-2">{t("ai.magpiePartner")}</span>}</span></Button>{preset.sponsored&&<div className="flex flex-wrap items-center gap-3 px-3 pb-2 text-sm">{preset.website&&<a href={preset.website} target="_blank" rel="noreferrer" className="underline break-all">{preset.siteHost||new URL(preset.website).hostname}</a>}{preset.keysUrl&&<a href={preset.keysUrl} target="_blank" rel="noreferrer" className="underline">{t("ai.openKeys")}</a>}</div>}<p className="muted px-3">{preset.sponsored?preset.notes?.[language]??preset.notes?.en:preset.note}</p></div>)}
    {!busy&&!entries.length&&!error&&<p className="muted">{t("ai.noMatchingServices")}</p>}
   </div>
	   {picture&&<div className="settings-choice-list" role="group" aria-label={picture.name}><p className="font-medium">{picture.name}</p><p className="settings-help break-all">{presetConnection(picture).baseUrl}</p>{picture.notes&&<p className="settings-help">{picture.notes[language]??picture.notes.en}</p>}<div className="flex flex-wrap gap-2"><Button onClick={()=>{onAdd(picture);setOpen(false);setPicture(undefined);}}>{t("ai.usePreset")}</Button>{picture.website&&<Button variant="ghost" onClick={()=>{window.open(picture.website,"_blank");}}>{picture.siteHost||t("ai.openWebsite")}</Button>}{picture.keysUrl&&<Button variant="ghost" onClick={()=>{window.open(picture.keysUrl,"_blank");}}>{t("ai.openKeys")}</Button>}</div></div>}
  </DialogContent></Dialog>
 </>;
}
