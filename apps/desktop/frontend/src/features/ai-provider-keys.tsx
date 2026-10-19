import {Eye,EyeOff,Plus,Trash2} from "lucide-react";
import {useEffect,useRef,useState,type ClipboardEvent} from "react";
import {captureCall} from "../bridge";
import {useTranslation} from "../i18n";
import {Input} from "../components/ui/input";
import {Field,FieldLabel} from "../components/ui/field";
import {Button} from "../ui/button";
import {Switch} from "../ui/switch";
import {InlineError} from "../ui/notice";
import {SettingsOptions, SettingsToggle} from "../components/settings-layout";
import type {AIKeyPolicy,AIKeySlot,AIProvider} from "./ai-configuration";

const defaultPolicy:AIKeyPolicy={strategy:"round-robin",failover:true,maxAttempts:3,cooldownSeconds:60};

// Legacy snapshots show their existing key as the stable primary slot. Merely
// displaying that slot does not dirty the form or ask for the secret again.
function slotsFor(provider:AIProvider):AIKeySlot[]{
 return provider.keys??[{id:"primary",enabled:true,hasKey:provider.hasKey}];
}

// A row owns only its reveal state. Saved plaintext never becomes a form edit,
// and hiding/removing/resetting the row invalidates any late read response.
function ProviderKeyRow({provider,slot,index,count,edit,resetToken,disabled,onEdit,onEnabled,onRemove,onPaste}:{
 provider:string;slot:AIKeySlot;index:number;count:number;edit?:string;resetToken?:unknown;disabled:boolean;
 onEdit:(value:string|undefined)=>void;onEnabled:(enabled:boolean)=>void;onRemove:()=>void;
 onPaste?:(event:ClipboardEvent<HTMLInputElement>)=>void;
}){
 const {t}=useTranslation();
 const [visible,setVisible]=useState(false),[savedKey,setSavedKey]=useState(""),[reading,setReading]=useState(false),[error,setError]=useState("");
 const generation=useRef(0);
 useEffect(()=>{setVisible(false);setSavedKey("");setReading(false);setError("");return()=>{generation.current++;};},[resetToken]);
 // A cURL paste in another field can replace this draft while a read is pending.
 useEffect(()=>{generation.current++;setSavedKey("");setReading(false);setError("");},[edit]);
 async function toggle(){
  setError("");
  if(visible){generation.current++;setVisible(false);setSavedKey("");return;}
  if(edit!==undefined){setVisible(true);return;}
  const request=++generation.current;setReading(true);
  try{const result=await captureCall<{key:string}>("ai.keys.reveal",{provider,id:slot.id});if(generation.current===request){setSavedKey(result.key);setVisible(true);}}
  catch(error){if(generation.current===request)setError(String(error));}
  finally{if(generation.current===request)setReading(false);}
 }
 const label=count===1?t("ai.key"):t("ai.keyNumber",{index:index+1});
 const action=t(visible?"ai.hideKey":"ai.showKey",{name:label});
 return <div className="flex min-w-0 flex-col gap-2">
  <div className="flex min-w-0 items-center gap-3">
   {count>1&&<span className="w-4 shrink-0 text-right text-xs tabular-nums text-muted-foreground" aria-hidden="true">{index+1}</span>}
   <Input id={`ai-keys-${provider}-${slot.id}`} className={`min-w-0 flex-1 ${slot.enabled?"":"opacity-60"}`} aria-label={label} type={visible?"text":"password"} autoComplete="new-password" spellCheck={false} value={edit??(visible?savedKey:"")} disabled={disabled||reading} placeholder={t(slot.hasKey?"ai.keySaved":"ai.keyOptional")} onPaste={onPaste} onChange={event=>onEdit(event.target.value||undefined)}/>
   <Button icon={visible?EyeOff:Eye} variant="ghost" className="shrink-0 px-2" aria-label={action} title={action} busy={reading} disabled={disabled||(!edit&&!slot.hasKey)} onClick={()=>void toggle()}/>
   <Switch aria-label={t("ai.enableKey",{index:index+1})} title={t("ai.enableKey",{index:index+1})} checked={slot.enabled} disabled={disabled} onCheckedChange={onEnabled}/>
   <Button icon={Trash2} variant="ghost" className="shrink-0 px-2" aria-label={t("ai.removeKey",{index:index+1})} title={t("ai.removeKey",{index:index+1})} disabled={disabled} onClick={onRemove}/>
  </div>
  {error&&<InlineError>{error}</InlineError>}
 </div>;
}

export function AIProviderKeys({provider,edits,disabled,resetToken,onChange,onEdit,onPaste}:{
 provider:AIProvider;edits:Record<string,string>;disabled:boolean;resetToken?:unknown;
 onChange:(patch:Partial<AIProvider>)=>void;onEdit:(id:string,value:string|undefined)=>void;
 onPaste?:(event:ClipboardEvent<HTMLInputElement>,id:string)=>void;
}){
 const {t}=useTranslation();const slots=slotsFor(provider),policy=provider.keyPolicy??defaultPolicy;
 const prefix=`ai-keys-${provider.id}`;
 function setPolicy(patch:Partial<AIKeyPolicy>){onChange({keyPolicy:{...policy,...patch}});}
 function add(){onChange({keys:[...slots,{id:crypto.randomUUID(),enabled:true,hasKey:false}]});}
 function remove(id:string){onChange({keys:slots.filter(slot=>slot.id!==id)});onEdit(id,undefined);}
 return <Field>
  <div className="flex items-center justify-between gap-3">
   <FieldLabel htmlFor={slots.length?`${prefix}-${slots[0].id}`:undefined}>{t("ai.key")}</FieldLabel>
   <Button icon={Plus} variant="ghost" className="shrink-0 px-2" aria-label={t("ai.addKey")} title={t("ai.addKey")} disabled={disabled||slots.length>=16} onClick={add}/>
  </div>
  <div className="flex flex-col gap-3">
   {slots.map((slot,index)=><ProviderKeyRow key={`${provider.id}:${slot.id}`} provider={provider.id} slot={slot} index={index} count={slots.length} edit={edits[slot.id]} resetToken={resetToken} disabled={disabled} onPaste={event=>onPaste?.(event,slot.id)} onEdit={value=>{onChange({keys:slots});onEdit(slot.id,value);}} onEnabled={enabled=>onChange({keys:slots.map(current=>current.id===slot.id?{...current,enabled}:current)})} onRemove={()=>remove(slot.id)}/>)}
  </div>
  {slots.length>1&&<div className="settings-stack pt-2">
   <SettingsOptions label={t("ai.keyStrategy")} value={policy.strategy} disabled={disabled} items={[{value:"round-robin",label:t("ai.roundRobinKeys")},{value:"ordered",label:t("ai.orderedKeys")}]} onChange={strategy=>setPolicy({strategy:strategy as AIKeyPolicy["strategy"]})}/>
   <SettingsToggle id={`${prefix}-failover`} label={t("ai.keyFailover")} checked={policy.failover} disabled={disabled} onChange={failover=>setPolicy({failover})}/>
   <div className="settings-field-grid">
    <Field><FieldLabel htmlFor={`${prefix}-attempts`}>{t("ai.keyMaxAttempts")}</FieldLabel><Input id={`${prefix}-attempts`} type="number" min={1} max={16} step={1} value={policy.maxAttempts||""} disabled={disabled||!policy.failover} onChange={event=>setPolicy({maxAttempts:Number(event.target.value)})}/></Field>
    <Field><FieldLabel htmlFor={`${prefix}-cooldown`}>{t("ai.keyCooldown")}</FieldLabel><Input id={`${prefix}-cooldown`} type="number" min={1} max={3600} step={1} value={policy.cooldownSeconds||""} disabled={disabled} onChange={event=>setPolicy({cooldownSeconds:Number(event.target.value)})}/></Field>
   </div>
   <p className="settings-help">{t("ai.keyFailoverHint")}</p>
  </div>}
 </Field>;
}
