import {useId} from "react";
import {useTranslation} from "../i18n";
import {Input} from "../components/ui/input";
import {Label} from "../components/ui/label";
import {InlineError} from "./notice";

export function validNumber(value:number,min:number,max?:number,integer=true){return Number.isFinite(value)&&(!integer||Number.isInteger(value))&&value>=min&&(max===undefined||value<=max);}

// Compose the shared shadcn controls. An unfinished input stays empty; saving
// pages must reject NaN instead of serializing it as null or silently using zero.
export function NumberField({id:providedId,label,inlineLabel,value,min=0,max,integer=true,disabled=false,onChange}:{id?:string;label:string;inlineLabel?:string;value:number;min?:number;max?:number;integer?:boolean;disabled?:boolean;onChange:(value:number)=>void}) {
  const generatedId=useId(),id=providedId??generatedId,{t}=useTranslation(),valid=validNumber(value,min,max,integer);
  const errorKey=max===undefined?(integer?"common.integerMinimum":"common.numberMinimum"):(integer?"common.numberRange":"common.decimalRange");
  // Short units sit at the right edge of the existing Input, without a second
  // control border. Keep the full accessible label and native label-click focus.
  return <div className="min-w-0 space-y-2"><div className={inlineLabel?"relative":"space-y-2"}>
    <Label htmlFor={id} className={inlineLabel?"absolute inset-y-0 end-2.5 z-10 text-muted-foreground font-normal":undefined}>{inlineLabel?<><span aria-hidden="true">{inlineLabel}</span><span className="sr-only">{label}</span></>:label}</Label>
    <Input id={id} className={inlineLabel?"pe-11":undefined} type="number" disabled={disabled} min={min} max={max} step={integer?1:"any"} value={Number.isFinite(value)?value:""} aria-invalid={!valid} aria-describedby={valid?undefined:id+"-error"} onChange={event=>onChange(event.target.valueAsNumber)}/>
  </div>{!valid&&<InlineError id={id+"-error"}>{t(errorKey,{min,max})}</InlineError>}</div>;
}
