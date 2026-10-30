import {useEffect,useId,useState} from "react";
import {Network} from "lucide-react";
import {useTranslation} from "../i18n";
import {SettingsSection,SettingsOptions} from "../components/settings-layout";
import {Field,FieldLabel} from "../components/ui/field";
import {Input} from "../components/ui/input";
import {Button} from "../ui/button";
import {Notice} from "../ui/notice";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";
import {proxyFields,proxyAddress} from "./magpie-proxy-fields";
import type {MagpieSettings} from "./ai-configuration";

function proxyDraft(settings:MagpieSettings){
 const value=settings.proxy??"",mode=!value?"auto":value==="direct"?"direct":"custom";
 const fields=proxyFields(mode==="custom"?value:settings.proxyCustom??"");
 return {mode,fields,address:fields.address,username:fields.username,password:fields.password};
}

// The upstream helpers own userinfo parsing/encoding. This form stages its
// edits in WinOne's existing save transaction and protects unfinished fields.
export function AIProxySettings({settings,disabled,onChange,onActivity}:{settings:MagpieSettings;disabled:boolean;onChange:(patch:Partial<MagpieSettings>)=>void;onActivity?:PageActivityChanged}){
 const {t}=useTranslation(),id=useId();const [draft,setDraft]=useState(()=>proxyDraft(settings)),[error,setError]=useState("");
 const reset=()=>{setDraft(proxyDraft(settings));setError("");};
 useEffect(reset,[settings.proxy,settings.proxyCustom]);
 const original=proxyDraft(settings),dirty=draft.mode!==original.mode||draft.mode==="custom"&&(draft.address!==original.address||draft.username!==original.username||draft.password!==original.password);
 usePageActivity(onActivity,dirty,false,"ai-proxy",reset);
 function apply(){
  setError("");
  if(draft.mode!=="custom"){onChange({proxy:draft.mode==="direct"?"direct":""});return;}
  if(!draft.address.trim()){setError(t("ai.proxyAddressRequired"));return;}
  try{const proxy=proxyAddress(draft.address,draft.fields,draft.username,draft.password);onChange({proxy,proxyCustom:proxy});}
  catch{setError(t("ai.proxyEncodingError"));}
 }
 return <SettingsSection icon={Network} title={t("ai.proxyTitle")}>
  <p className="settings-help">{t("ai.proxyHint")}</p>
  <SettingsOptions label={t("ai.proxyMode")} value={draft.mode} disabled={disabled} items={[{value:"auto",label:t("ai.proxyAuto")},{value:"direct",label:t("ai.proxyDirect")},{value:"custom",label:t("ai.proxyCustom")}]} onChange={mode=>setDraft(old=>({...old,mode}))}/>
  {draft.mode==="custom"&&<div className="grid min-w-0 gap-3 sm:grid-cols-2">
   <Field className="sm:col-span-2"><FieldLabel htmlFor={id+"-address"}>{t("ai.proxyAddress")}</FieldLabel><Input id={id+"-address"} value={draft.address} disabled={disabled} placeholder="http://127.0.0.1:7890" onChange={event=>{const address=event.target.value,fields=proxyFields(address);setDraft(old=>fields.username||fields.password?{...old,fields,address:fields.address,username:fields.username,password:fields.password}:{...old,address});}}/></Field>
   <Field><FieldLabel htmlFor={id+"-username"}>{t("ai.proxyUsername")}</FieldLabel><Input id={id+"-username"} value={draft.username} disabled={disabled} autoComplete="off" onChange={event=>setDraft(old=>({...old,username:event.target.value}))}/></Field>
   <Field><FieldLabel htmlFor={id+"-password"}>{t("ai.proxyPassword")}</FieldLabel><Input id={id+"-password"} type="password" value={draft.password} disabled={disabled} autoComplete="new-password" onChange={event=>setDraft(old=>({...old,password:event.target.value}))}/></Field>
  </div>}
  {error&&<Notice>{error}</Notice>}
  <div className="flex flex-wrap items-center gap-3"><Button disabled={disabled||!dirty} onClick={apply}>{t("ai.proxyApply")}</Button><span className="settings-help">{t("ai.proxySaveHint")}</span></div>
 </SettingsSection>;
}
