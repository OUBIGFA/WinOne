import {useId,type ReactNode} from "react";
import {Label} from "../components/ui/label";
import {Select,SelectTrigger,SelectValue,SelectContent,SelectItem} from "../components/ui/select";

// Keep option labels and keyboard behavior identical across annotation controls.
export function CaptureOptionSelect<Value extends string|number>({label,accessibleLabel,value,items,disabled,onChange,stacked=false,placeholder,contentClassName}:{label:string;accessibleLabel?:string;value:Value;items:{value:Value;label:string;icon?:ReactNode;badge?:ReactNode}[];disabled:boolean;onChange:(value:Value)=>void;stacked?:boolean;placeholder?:string;contentClassName?:string}) {
  const id=useId();
  const selected=items.find(item=>item.value===value);
  return <div className={stacked?"grid min-w-0 gap-2":"flex items-center gap-2"}><Label htmlFor={id}>{label}</Label>
    <Select value={value} items={items} disabled={disabled} onValueChange={next=>{if(next!==null)onChange(next);}}>
      <SelectTrigger id={id} aria-label={accessibleLabel} title={selected?.label} className={stacked?"w-full min-w-0":"min-w-20"}><SelectValue placeholder={placeholder}>{selected?.icon||selected?.badge?<>{selected.icon}<span className="truncate">{selected.label}</span>{selected.badge}</>:undefined}</SelectValue></SelectTrigger>
      <SelectContent className={contentClassName}>{items.map(item=><SelectItem key={item.value} value={item.value} className={item.icon?"[&>:first-child]:gap-1.5":undefined}>{item.icon}<span className="min-w-0 break-words whitespace-normal">{item.label}</span>{item.badge}</SelectItem>)}</SelectContent>
    </Select>
  </div>;
}
