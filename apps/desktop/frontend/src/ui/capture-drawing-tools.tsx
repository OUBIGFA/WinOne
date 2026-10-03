import {useEffect,useState} from "react";
import {InlineError} from "./notice";
import {Settings2,type LucideIcon} from "lucide-react";
import {Button} from "./button";
import {useTranslation} from "../i18n";
import {CaptureToolbarActions} from "./capture-toolbar-actions";
import {DropdownMenu,DropdownMenuTrigger,DropdownMenuContent,DropdownMenuCheckboxItem} from "../components/ui/dropdown-menu";

export const defaultToolbarTools=["select","crop","rectangle","ellipse","arrow","pen","cover","text","mosaic","blur"];
export interface ToolbarPreferencesPort {read():Promise<string[]|null>;save(tools:string[]):Promise<void>}

export function CaptureDrawingTools({items,selected,disabled,onSelect,port}:{items:readonly {kind:string;icon:LucideIcon}[];selected:string;disabled:boolean;onSelect:(tool:string)=>void;port?:ToolbarPreferencesPort}) {
 const {t}=useTranslation();
 const [pinned,setPinned]=useState(defaultToolbarTools);
 const [ready,setReady]=useState(!port),[saving,setSaving]=useState(false),[error,setError]=useState("");
 const [attempt,setAttempt]=useState(0);
 useEffect(()=>{
  if(!port)return;
  let active=true;
  void port.read().then(tools=>{if(active){setPinned(tools??defaultToolbarTools);setReady(true);}}).catch(error=>{if(active)setError(String(error));});
  return()=>{active=false;};
 },[port,attempt]);
 async function toggle(tool:string){
  const next=pinned.includes(tool)?pinned.filter(value=>value!==tool):[...pinned,tool];
  setSaving(true);setError("");
  try{await port?.save(next);setPinned(next);}catch(error){setError(String(error));}finally{setSaving(false);}
 }
 return <div className="flex min-w-0 flex-1 gap-1.5" role="group" aria-label={t("capture.drawingTools")}>
  <CaptureToolbarActions disabled={disabled} menuLabel={t("capture.moreTools")}
   items={items.filter(item=>pinned.includes(item.kind)).map(item=>({id:item.kind,label:t(`capture.${item.kind}`),icon:item.icon,selected:selected===item.kind,disabled,onClick:()=>onSelect(item.kind)}))}
  />
  <DropdownMenu><DropdownMenuTrigger render={<Button disabled={disabled||saving}/> }><Settings2 size={16}/>{t("capture.customizeToolbar")}</DropdownMenuTrigger>
   <DropdownMenuContent className="max-h-[var(--available-height)] w-56 max-w-(--available-width) overflow-y-auto">
    {items.map(item=><DropdownMenuCheckboxItem key={item.kind} checked={pinned.includes(item.kind)} disabled={disabled||saving||!ready} onCheckedChange={()=>void toggle(item.kind)}>{t(`capture.${item.kind}`)}</DropdownMenuCheckboxItem>)}
    {error&&<InlineError className="p-2">{error}{!ready&&<div className="mt-2"><Button onClick={()=>{setError("");setAttempt(value=>value+1);}}>{t("common.retry")}</Button></div>}</InlineError>}
   </DropdownMenuContent>
  </DropdownMenu>
 </div>;
}
