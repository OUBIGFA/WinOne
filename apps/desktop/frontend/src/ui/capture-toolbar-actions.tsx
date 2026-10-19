import {useLayoutEffect,useRef,useState,type ReactNode} from "react";
import {ChevronDown,type LucideIcon} from "lucide-react";
import {Button} from "./button";
import {DropdownMenu,DropdownMenuTrigger,DropdownMenuContent,DropdownMenuItem} from "../components/ui/dropdown-menu";

export interface ToolbarAction {id:string;label:string;icon?:LucideIcon;disabled?:boolean;busy?:boolean;selected?:boolean;primary?:boolean;onClick:()=>void}

// Measure the actual local controls and move excess actions into a menu. There
// is no wrapping or horizontal scrolling, including after a language change.
export function CaptureToolbarActions({items,menuLabel,menuExtra,alwaysMenu=false,disabled=false}:{items:ToolbarAction[];menuLabel:string;menuExtra?:ReactNode;alwaysMenu?:boolean;disabled?:boolean}){
 const root=useRef<HTMLDivElement>(null),measure=useRef<HTMLDivElement>(null),menuMeasure=useRef<HTMLDivElement>(null);
 const [count,setCount]=useState(items.length);
 const signature=items.map(item=>`${item.id}:${item.label}:${item.busy}`).join("|")+menuLabel+alwaysMenu;
 useLayoutEffect(()=>{
  const update=()=>{
   if(!root.current||!measure.current||!menuMeasure.current)return;
   const available=root.current.clientWidth;
   if(!available){setCount(measure.current.children.length);return;}
   const widths=Array.from(measure.current.children).map(node=>Math.ceil(node.getBoundingClientRect().width));
   const total=widths.reduce((sum,width)=>sum+width,0)+Math.max(0,widths.length-1)*6;
   if(!alwaysMenu&&total<=available){setCount(widths.length);return;}
   const reserved=Math.ceil(menuMeasure.current.getBoundingClientRect().width)+6;
   let used=0,visible=0;
   for(const width of widths){if(used+width+(visible?6:0)>available-reserved)break;used+=width+(visible?6:0);visible++;}
   setCount(visible);
  };
  let frame=0;
  const schedule=()=>{if(!frame)frame=requestAnimationFrame(()=>{frame=0;update();});};
  schedule();
  const observer=new ResizeObserver(schedule);
  if(root.current)observer.observe(root.current);
  if(measure.current)observer.observe(measure.current);
  if(menuMeasure.current)observer.observe(menuMeasure.current);
  return()=>{observer.disconnect();if(frame)cancelAnimationFrame(frame);};
 },[signature]);
 const render=(item:ToolbarAction,measurement=false)=><Button key={item.id} disabled={item.disabled} busy={item.busy} aria-pressed={item.selected} variant={item.primary||item.selected?"primary":"secondary"} onClick={measurement?undefined:item.onClick}>{item.icon&&<item.icon size={18} aria-hidden/>}{item.label}</Button>;
 return <div ref={root} className="relative flex min-w-0 flex-1 items-center gap-1.5 overflow-hidden">
  <div aria-hidden inert className="pointer-events-none invisible absolute left-0 top-0 flex w-max gap-1.5"><div ref={measure} className="flex w-max gap-1.5">{items.map(item=>render(item,true))}</div><div ref={menuMeasure}><Button><ChevronDown size={16}/>{menuLabel}</Button></div></div>
  {items.slice(0,count).map(item=>render(item))}
  {(alwaysMenu||count<items.length)&&<DropdownMenu><DropdownMenuTrigger render={<Button disabled={disabled} className="min-w-0 max-w-full" aria-label={menuLabel}/> }><ChevronDown size={16}/><span className="truncate">{menuLabel}</span></DropdownMenuTrigger>
   <DropdownMenuContent className="w-56 max-w-(--available-width)">
    {items.slice(count).map(item=><DropdownMenuItem key={item.id} disabled={item.disabled||item.busy} onClick={item.onClick}>{item.icon&&<item.icon size={18}/>} {item.label}</DropdownMenuItem>)}
    {menuExtra}
   </DropdownMenuContent>
  </DropdownMenu>}
 </div>;
}
