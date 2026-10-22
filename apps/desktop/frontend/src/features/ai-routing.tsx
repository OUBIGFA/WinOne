import {useEffect,useLayoutEffect,useRef,useState} from "react";
import {Waypoints,RefreshCw} from "lucide-react";
import {captureCall} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {Notice} from "../ui/notice";
import {Switch} from "../ui/switch";

// Magpie's complete routing page owns its editor, rules and live trace. The
// parent supplies WinOne navigation, language and theme through the adapter.
export function AIRouting({active,enabled,revision,onEnabled,onEnableGroups,onOpenMasking,disabled=false}:{onOpenMasking?:(key:string)=>void;active:boolean;enabled:boolean;revision?:number;onEnabled:(value:boolean)=>void;onEnableGroups?:()=>Promise<void>;disabled?:boolean}){
 const {t,i18n}=useTranslation(),frame=useRef<HTMLIFrameElement>(null),[url,setURL]=useState(""),[error,setError]=useState(""),[busy,setBusy]=useState(false),[retry,setRetry]=useState(0),[height,setHeight]=useState(600);
 // Saving AI settings replaces the backend snapshot. Reopen only after the
 // saved revision changes, so toggling an unsaved switch keeps the editor.
 useEffect(()=>{if(!active)return;let alive=true;setBusy(true);setError("");setURL("");void captureCall<{url:string}>("ai.routing.open").then(value=>{if(alive)setURL(value.url);else void captureCall("ai.routing.close").catch(console.error);}).catch(error=>{if(alive)setError(String(error));}).finally(()=>{if(alive)setBusy(false);});return()=>{alive=false;};},[active,retry,revision]);
 useEffect(()=>{if(!active)return;return()=>{void captureCall("ai.routing.close").catch(console.error);};},[active]);
 const masking=useRef(onOpenMasking);masking.current=onOpenMasking;
 useLayoutEffect(()=>{if(!active||!url)return;const origin=new URL(url).origin;const receive=(event:MessageEvent)=>{if(event.source===frame.current?.contentWindow&&event.origin===origin&&event.data?.type==='winone-routing-masking'&&typeof event.data.key==='string'&&!disabled)masking.current?.(event.data.key);};window.addEventListener('message',receive);return()=>window.removeEventListener('message',receive);},[active,url,disabled]);
 const enableGroups=useRef(onEnableGroups);enableGroups.current=onEnableGroups;
 // This capability belongs to the authenticated routing document only. The
 // parent saves the one plugin choice while retaining its other drafts.
 useLayoutEffect(()=>{if(!active||!url)return;const origin=new URL(url).origin;const receive=async(event:MessageEvent)=>{
  if(event.source!==frame.current?.contentWindow||event.origin!==origin||event.data?.type!=="winone-routing-native"||event.data.method!=="enable-groups"||typeof event.data.id!=="string"||disabled||!enableGroups.current)return;
  const child=frame.current.contentWindow,id=event.data.id;
  try{await enableGroups.current();child?.postMessage({type:"winone-routing-native-result",id},origin);}
  catch(error){child?.postMessage({type:"winone-routing-native-result",id,error:String(error)},origin);}
 };window.addEventListener("message",receive);return()=>window.removeEventListener("message",receive);},[active,url,disabled]);
 // Attach before the child's first paint: a cached editor can report its
 // height before passive effects run. Only this authenticated child may resize.
 useLayoutEffect(()=>{if(!active||!url)return;const origin=new URL(url).origin;const resize=(event:MessageEvent)=>{if(event.source!==frame.current?.contentWindow||event.origin!==origin||event.data?.type!=="winone-routing-height")return;const next=event.data.height;if(typeof next==="number"&&Number.isFinite(next)&&next>0)setHeight(Math.ceil(next));};window.addEventListener("message",resize);return()=>window.removeEventListener("message",resize);},[active,url]);
 useEffect(()=>{if(!active)return;const sync=()=>frame.current?.contentWindow?.postMessage({type:"winone-routing-theme",language:i18n.language,theme:document.documentElement.dataset.theme??(document.documentElement.classList.contains("dark")?"dark":"light")},new URL(url||location.href).origin);const observer=new MutationObserver(sync);observer.observe(document.documentElement,{attributes:true,attributeFilter:["class","data-theme"]});sync();return()=>observer.disconnect();},[active,url,i18n.language]);
 return <section className="settings-stack ai-routing" aria-label={t("routing.title")}><div className="settings-switch-row"><label className="ai-routing-toggle-label" htmlFor="winone-routing-enabled"><Waypoints size={18} aria-hidden="true"/><span>{t("routing.enabled")}</span></label><Switch id="winone-routing-enabled" checked={enabled} disabled={busy||disabled} onCheckedChange={onEnabled}/></div><p className="settings-help">{t("routing.hint")}</p>{error&&<Notice actions={<Button icon={RefreshCw} onClick={()=>setRetry(value=>value+1)}>{t("common.retry")}</Button>}>{error}</Notice>}{busy&&!url&&<p role="status">{t("capture.loading")}</p>}{active&&url&&<iframe ref={frame} title={t("routing.title")} src={url} onLoad={()=>frame.current?.contentWindow?.postMessage({type:"winone-routing-theme",language:i18n.language,theme:document.documentElement.dataset.theme??(document.documentElement.classList.contains("dark")?"dark":"light")},new URL(url).origin)} style={{width:"100%",height,border:0,display:"block"}}/>}</section>;
}
