import {InlineError} from "../ui/notice";
import {useEffect,useState} from "react";
import {wintrayCall} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {DropdownMenu,DropdownMenuTrigger,DropdownMenuContent,DropdownMenuItem} from "../components/ui/dropdown-menu";

export interface CollectedIcon {id:string;name:string;exePath:string;image?:string;hosted?:boolean}
// One timer for the visible list; disabled modules never request icon data.
export function useCollectedIcons(enabled:boolean) {
 const [icons,setIcons]=useState<CollectedIcon[]>([]),[error,setError]=useState("");
 useEffect(()=>{
  if(!enabled){setIcons([]);setError("");return;}
  let stopped=false;let timer:ReturnType<typeof setTimeout>|undefined;
  async function refresh(){
   try{const next=await wintrayCall<CollectedIcon[]>("icons");if(!stopped){setIcons(next);setError("");}}
   catch(error){if(!stopped)setError(String(error));}
   finally{if(!stopped)timer=setTimeout(()=>void refresh(),1000);}
  }
  void refresh();return()=>{stopped=true;clearTimeout(timer);};
 },[enabled]);
 return {icons,error};
}
export function iconsForProgram(icons:CollectedIcon[],path:string) {
 const normalize=(value:string)=>value.replaceAll("/","\\").toLowerCase();
 return icons.filter(icon=>normalize(icon.exePath)===normalize(path));
}
// Keep native icon actions in the corresponding program row.
export function WinTrayIcons({icons,disabled=false}:{icons:CollectedIcon[];disabled?:boolean}) {
 const {t}=useTranslation();
 const [error,setError]=useState(""),[busy,setBusy]=useState(false);
 async function activate(id:string,right:boolean,action?:string){
  setBusy(true);setError("");
  try{
   const result=await wintrayCall<{delivered:boolean}>("icon.activate",{id,right,...(action?{action}:{})});
   if(result?.delivered!==true)throw Error(t("wintray.unknownResult"));
  }catch(error){setError(error instanceof Error?error.message:String(error));}finally{setBusy(false);}
 }
 return <>{error&&<InlineError>{error}</InlineError>}
  {icons.map(icon=><div className="flex w-full flex-wrap items-center gap-2 border-t pt-2" key={icon.id}>
    {icon.image&&<img src={`data:image/png;base64,${icon.image}`} alt="" width={20} height={20}/>}
    <span className="min-w-0 flex-1 break-words text-sm">{icon.name}</span>
    <Button size="sm" variant="ghost" aria-label={t("wintray.openCollected",{name:icon.name})} disabled={busy||disabled} onClick={()=>void activate(icon.id,false)}>{t("common.open")}</Button>
    {icon.hosted?<DropdownMenu><DropdownMenuTrigger render={<Button size="sm" variant="ghost" disabled={busy||disabled}/>}>{t("wintray.programActions")}</DropdownMenuTrigger>
      <DropdownMenuContent>{(["hide","release","quit"] as const).map(action=><DropdownMenuItem key={action} disabled={busy||disabled} onClick={()=>void activate(icon.id,false,action)}>{t(`wintray.hosted.${action}`)}</DropdownMenuItem>)}</DropdownMenuContent>
    </DropdownMenu>:<Button size="sm" variant="ghost" aria-label={t("wintray.originalMenu",{name:icon.name})} disabled={busy||disabled} onClick={()=>void activate(icon.id,true)}>{t("wintray.menu")}</Button>}
  </div>)}
 </>;
}
