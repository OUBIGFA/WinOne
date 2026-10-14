import {useEffect,useId,useRef,useState,type ReactNode} from "react";
import {Check,Pencil} from "lucide-react";
import {Input} from "../components/ui/input";
import {Label} from "../components/ui/label";
import {Button} from "../ui/button";
import {CaptureOptionSelect} from "../ui/capture-option-select";
import {useTranslation} from "../i18n";
import type {AIProvider} from "./ai-configuration";

// Selection and naming share one field. Edits remain part of the page draft;
// Enter/blur only finish typing, while Escape restores the name before editing.
export function AIProviderSelect({provider,items,disabled,onSelect,onNameChange}:{provider:AIProvider;items:{value:string;label:string;icon?:ReactNode}[];disabled:boolean;onSelect:(id:string)=>void;onNameChange:(name:string)=>void}){
 const {t}=useTranslation(),id=useId();
 const [editing,setEditing]=useState(!provider.name);
 const original=useRef(provider.name),input=useRef<HTMLInputElement>(null),action=useRef<HTMLButtonElement>(null),restoreFocus=useRef(false);
 const account=provider.authMode==="account",label=t(account?"ai.account":"ai.service"),nameLabel=t(account?"ai.accountName":"ai.serviceName");
 const actionLabel=t(editing?"ai.finishRename":account?"ai.renameAccount":"ai.renameService");
 useEffect(()=>{if(editing){input.current?.focus();input.current?.select();}else if(restoreFocus.current){action.current?.focus();restoreFocus.current=false;}},[editing]);
 function finish(){restoreFocus.current=true;setEditing(false);}
 return <div className="flex min-w-0 items-end gap-2" onBlur={event=>{if(editing&&!event.currentTarget.contains(event.relatedTarget))setEditing(false);}}>
  <div className="min-w-0 flex-1">{editing?<div className="grid min-w-0 gap-2"><Label htmlFor={id}>{label}</Label><Input id={id} ref={input} aria-label={nameLabel} disabled={disabled} value={provider.name} onChange={event=>onNameChange(event.target.value)} onKeyDown={event=>{
   if(event.nativeEvent.isComposing)return;
   if(event.key==="Enter"||event.key==="Escape"){event.preventDefault();event.stopPropagation();if(event.key==="Escape")onNameChange(original.current);finish();}
  }}/></div>:<CaptureOptionSelect stacked label={label} value={provider.id} items={items} disabled={disabled} onChange={onSelect}/>}</div>
  <Button ref={action} className="shrink-0" variant="ghost" size="icon" icon={editing?Check:Pencil} disabled={disabled} aria-label={actionLabel} title={actionLabel} onClick={()=>{if(editing)finish();else{original.current=provider.name;setEditing(true);}}}/>
 </div>;
}
