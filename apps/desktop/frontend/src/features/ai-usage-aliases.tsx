import {useEffect,useRef,useState} from "react";
import {ArrowRight,Plus,Trash2} from "lucide-react";
import {captureCall} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {Notice} from "../ui/notice";
import {Input} from "../components/ui/input";
import {Label} from "../components/ui/label";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";

// Magpie's alias setter owns spelling, chains and cycle checks. This component
// only edits the shared WinOne draft; successful validation is not a saved value.
export function UsageAliases({aliases={},disabled=false,onChange,onActivity}:{aliases?:Record<string,string>;disabled?:boolean;onChange:(aliases:Record<string,string>)=>void;onActivity?:PageActivityChanged}){
 const {t}=useTranslation();const [from,setFrom]=useState(""),[to,setTo]=useState(""),[busy,setBusy]=useState(false),[error,setError]=useState("");
 const alive=useRef(true);useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
 usePageActivity(onActivity,!!from||!!to,busy,"ai-usage-aliases",()=>{setFrom("");setTo("");setError("");});
 async function apply(source:string,target:string,clearForm=false){
  if(busy||disabled)return;setBusy(true);setError("");
  try{const result=await captureCall<{usageAliases:Record<string,string>}>("ai.magpie.usage-aliases",{aliases,edit:{from:source,to:target}});if(!alive.current)return;onChange(result.usageAliases);if(clearForm){setFrom("");setTo("");}}
  catch(error){if(alive.current)setError(String(error));}finally{if(alive.current)setBusy(false);}
 }
 return <section className="settings-stack" aria-label={t("usage.aliases")}>
  <p className="settings-help">{t("usage.aliasHint")}</p>
  <form className="grid min-w-0 gap-3 sm:grid-cols-2" onSubmit={event=>{event.preventDefault();if(from.trim()&&to.trim())void apply(from,to,true);}}>
   <div className="grid min-w-0 gap-2"><Label htmlFor="usage-alias-from">{t("usage.aliasFrom")}</Label><Input id="usage-alias-from" value={from} disabled={busy||disabled} placeholder="deepseek-v4.1-flash" onChange={event=>setFrom(event.target.value)}/></div>
   <div className="grid min-w-0 gap-2"><Label htmlFor="usage-alias-to">{t("usage.aliasTo")}</Label><Input id="usage-alias-to" value={to} disabled={busy||disabled} placeholder="deepseek-flash" onChange={event=>setTo(event.target.value)}/></div>
   <Button type="submit" icon={Plus} disabled={busy||disabled||!from.trim()||!to.trim()} busy={busy}>{t("usage.aliasApply")}</Button>
  </form>
  {error&&<Notice>{error}</Notice>}
  <p className="settings-help">{t("usage.aliasSaveHint")}</p>
  {!Object.keys(aliases).length&&<p className="settings-empty">{t("usage.aliasEmpty")}</p>}
  {Object.entries(aliases).map(([source,target])=><div key={source} className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-3 border-b border-border pb-3"><div className="flex min-w-0 flex-wrap items-center gap-2"><span className="min-w-0 break-all">{source}</span><ArrowRight size={16} aria-hidden="true"/><span className="min-w-0 break-all">{target}</span></div><Button icon={Trash2} variant="ghost" disabled={busy||disabled} aria-label={t("usage.aliasRemoveNamed",{name:source})} onClick={()=>void apply(source,"")}>{t("usage.aliasRemove")}</Button></div>)}
 </section>;
}
