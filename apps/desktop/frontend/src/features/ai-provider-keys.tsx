import {Plus,Trash2} from "lucide-react";
import {useTranslation} from "../i18n";
import {Input} from "../components/ui/input";
import {Field,FieldLabel} from "../components/ui/field";
import {Button} from "../ui/button";
import {Switch} from "../ui/switch";
import {CaptureOptionSelect} from "../ui/capture-option-select";
import type {AIKeyPolicy,AIKeySlot,AIProvider} from "./ai-configuration";

const defaultPolicy:AIKeyPolicy={strategy:"round-robin",failover:true,maxAttempts:3,cooldownSeconds:60};

// Legacy snapshots show their existing key as the stable primary slot. Merely
// displaying that slot does not dirty the form or ask for the secret again.
function slotsFor(provider:AIProvider):AIKeySlot[]{
 return provider.keys??[{id:"primary",enabled:true,hasKey:provider.hasKey}];
}

export function AIProviderKeys({provider,edits,disabled,onChange,onEdit}:{
 provider:AIProvider;edits:Record<string,string>;disabled:boolean;
 onChange:(patch:Partial<AIProvider>)=>void;onEdit:(id:string,value:string|undefined)=>void;
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
   {slots.map((slot,index)=><div className="flex min-w-0 items-center gap-3" key={slot.id}>
    {slots.length>1&&<span className="w-4 shrink-0 text-right text-xs tabular-nums text-muted-foreground" aria-hidden="true">{index+1}</span>}
    <Input id={`${prefix}-${slot.id}`} className={`min-w-0 flex-1 ${slot.enabled?"":"opacity-60"}`} aria-label={slots.length===1?t("ai.key"):t("ai.keyNumber",{index:index+1})} type="password" autoComplete="new-password" value={edits[slot.id]??""} disabled={disabled} placeholder={t(slot.hasKey?"ai.keySaved":"ai.keyOptional")} onChange={event=>{onChange({keys:slots});onEdit(slot.id,event.target.value||undefined);}}/>
    <Switch aria-label={t("ai.enableKey",{index:index+1})} title={t("ai.enableKey",{index:index+1})} checked={slot.enabled} disabled={disabled} onCheckedChange={enabled=>onChange({keys:slots.map(current=>current.id===slot.id?{...current,enabled}:current)})}/>
    <Button icon={Trash2} variant="ghost" className="shrink-0 px-2" aria-label={t("ai.removeKey",{index:index+1})} title={t("ai.removeKey",{index:index+1})} disabled={disabled} onClick={()=>remove(slot.id)}/>
   </div>)}
  </div>
  {slots.length>1&&<div className="settings-stack pt-2">
   <CaptureOptionSelect stacked label={t("ai.keyStrategy")} value={policy.strategy} disabled={disabled} items={[{value:"round-robin",label:t("ai.roundRobinKeys")},{value:"ordered",label:t("ai.orderedKeys")}]} onChange={strategy=>setPolicy({strategy:strategy as AIKeyPolicy["strategy"]})}/>
   <div className="settings-switch-row"><label htmlFor={`${prefix}-failover`}>{t("ai.keyFailover")}</label><Switch id={`${prefix}-failover`} checked={policy.failover} disabled={disabled} onCheckedChange={failover=>setPolicy({failover})}/></div>
   <div className="settings-field-grid">
    <Field><FieldLabel htmlFor={`${prefix}-attempts`}>{t("ai.keyMaxAttempts")}</FieldLabel><Input id={`${prefix}-attempts`} type="number" min={1} max={16} step={1} value={policy.maxAttempts||""} disabled={disabled||!policy.failover} onChange={event=>setPolicy({maxAttempts:Number(event.target.value)})}/></Field>
    <Field><FieldLabel htmlFor={`${prefix}-cooldown`}>{t("ai.keyCooldown")}</FieldLabel><Input id={`${prefix}-cooldown`} type="number" min={1} max={3600} step={1} value={policy.cooldownSeconds||""} disabled={disabled} onChange={event=>setPolicy({cooldownSeconds:Number(event.target.value)})}/></Field>
   </div>
   <p className="settings-help">{t("ai.keyFailoverHint")}</p>
  </div>}
 </Field>;
}
