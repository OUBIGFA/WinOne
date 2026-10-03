import {useId} from "react";
import {useTranslation} from "../i18n";
import {Input} from "../components/ui/input";
import {Label} from "../components/ui/label";
import {InlineError} from "./notice";

export function validNumber(value:number,min:number,max?:number,integer=true){return Number.isFinite(value)&&(!integer||Number.isInteger(value))&&value>=min&&(max===undefined||value<=max);}

// Compose the shared shadcn controls. An unfinished input stays empty; saving
// pages must reject NaN instead of serializing it as null or silently using zero.
export function NumberField({id:providedId,label,value,min=0,max,integer=true,onChange}:{id?:string;label:string;value:number;min?:number;max?:number;integer?:boolean;onChange:(value:number)=>void}) {
  const generatedId=useId(),id=providedId??generatedId,{t}=useTranslation(),valid=validNumber(value,min,max,integer);
  const errorKey=max===undefined?(integer?"common.integerMinimum":"common.numberMinimum"):(integer?"common.numberRange":"common.decimalRange");
  return <div className="space-y-2"><Label htmlFor={id}>{label}</Label><Input id={id} type="number" min={min} max={max} step={integer?1:"any"} value={Number.isFinite(value)?value:""} aria-invalid={!valid} aria-describedby={valid?undefined:id+"-error"} onChange={event=>onChange(event.target.valueAsNumber)}/>{!valid&&<InlineError id={id+"-error"}>{t(errorKey,{min,max})}</InlineError>}</div>;
}
