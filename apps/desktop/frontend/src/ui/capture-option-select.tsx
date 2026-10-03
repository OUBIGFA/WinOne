import {useId} from "react";
import {Label} from "../components/ui/label";
import {Select,SelectTrigger,SelectValue,SelectContent,SelectItem} from "../components/ui/select";

// Keep option labels and keyboard behavior identical across annotation controls.
export function CaptureOptionSelect<Value extends string|number>({label,value,items,disabled,onChange}:{label:string;value:Value;items:{value:Value;label:string}[];disabled:boolean;onChange:(value:Value)=>void}) {
  const id=useId();
  return <div className="flex items-center gap-2"><Label htmlFor={id}>{label}</Label>
    <Select value={value} items={items} disabled={disabled} onValueChange={next=>{if(next!==null)onChange(next);}}>
      <SelectTrigger id={id} className="min-w-20"><SelectValue/></SelectTrigger>
      <SelectContent>{items.map(item=><SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent>
    </Select>
  </div>;
}
