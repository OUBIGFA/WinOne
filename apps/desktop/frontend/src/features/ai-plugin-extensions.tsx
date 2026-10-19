import {useEffect,useLayoutEffect,useRef,useState} from "react";
import {Blocks} from "lucide-react";
import {captureCall} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {DropdownMenu,DropdownMenuTrigger,DropdownMenuContent,DropdownMenuGroup,DropdownMenuLabel,DropdownMenuItem} from "../components/ui/dropdown-menu";

type Named = {id:string;name:string};
type Title = {title:string;titles?:Record<string,string>};
type Action = Title & {id:string;menu:"provider"|"agent"};
export type PluginPage = Title & {key:string;replaces?:string;actions?:Action[]};
export type PluginExtensions = {url:string;pages:PluginPage[];panels:(Title & {key:string;slot:string})[]};
export type PluginPageRequest = {key:string;action?:string;menu?:"provider"|"agent";item?:Named;nonce:string};

// One owner lease covers all panels in this settings instance. Its unique ID
// keeps late replies/cleanup from closing a newer page's shared AI runtime.
export function useAIPluginExtensions(active:boolean,revision?:number,generation=0) {
 const {t}=useTranslation(),[state,setState]=useState<PluginExtensions>(),[error,setError]=useState("");
 useEffect(()=>{
  if(!active)return;
  const viewer=crypto.randomUUID();let live=true;setError("");setState(undefined);
  const close=()=>void captureCall("ai.routing.extensions.close",{viewer}).catch(console.error);
  void captureCall<PluginExtensions>("ai.routing.extensions.open",{viewer}).then(value=>{
   if(!live){close();return;}
   if(!value||!Array.isArray(value.pages)||!Array.isArray(value.panels)||!value.url)throw Error(t("aiPlugins.noPlugins"));
   setState(value);
  }).catch(error=>{if(live)setError(String(error));});
  return()=>{live=false;close();};
 },[active,revision,generation,t]);
 return {state,error};
}

export function pluginTitle(value:Title,language:string) {
 const locale=language.startsWith("zh-CN")?"zh":language.startsWith("en")?"en":language;
 return value.titles?.[locale]??value.titles?.[locale.split("-")[0]]??value.title;
}

// Context is deliberately reconstructed: never serialize a provider, account,
// draft, request header or credential into a plugin URL/message.
export function pluginPageURL(base:string,request?:PluginPageRequest) {
 const url=new URL(base);
 if(request){
  url.searchParams.set("page",request.key);url.searchParams.set("request",request.nonce);
  if(request.action&&request.menu&&request.item){
   url.searchParams.set("action",request.action);url.searchParams.set("menu",request.menu);
   url.searchParams.set("item-id",request.item.id);url.searchParams.set("item-name",request.item.name);
  }
 }
 return url.href;
}

export function AIPluginActions({extensions,provider,agent,disabled,onOpen,openRequest=0}:{extensions?:PluginExtensions;provider:Named;agent?:Named;disabled:boolean;onOpen:(request:PluginPageRequest)=>void;openRequest?:number}) {
 const {t,i18n}=useTranslation(),[open,setOpen]=useState(false),previousOpenRequest=useRef(openRequest);
 useEffect(()=>{if(previousOpenRequest.current!==openRequest&&!disabled)setOpen(true);previousOpenRequest.current=openRequest;},[openRequest]);
 useEffect(()=>{if(disabled)setOpen(false);},[disabled]);
 const groups=(["provider","agent"] as const).map(menu=>({menu,item:menu==="provider"?provider:agent,actions:(extensions?.pages??[]).flatMap(page=>(page.actions??[]).filter(action=>action.menu===menu).map(action=>({page,action})))})).filter(group=>group.item&&group.actions.length);
 if(!groups.length)return null;
 return <DropdownMenu open={open} onOpenChange={setOpen}><DropdownMenuTrigger render={<Button icon={Blocks} variant="ghost" size="icon" disabled={disabled} aria-label={t("aiPlugins.actions")} title={t("aiPlugins.actions")}/>}/><DropdownMenuContent align="end" className="w-auto max-w-80">
  {groups.map(group=><DropdownMenuGroup key={group.menu}><DropdownMenuLabel>{t(group.menu==="provider"?"aiPlugins.providerActions":"aiPlugins.agentActions")}</DropdownMenuLabel>{group.actions.map(({page,action})=><DropdownMenuItem key={page.key+":"+action.id} onClick={()=>onOpen({key:page.key,action:action.id,menu:group.menu,item:{id:group.item!.id,name:group.item!.name},nonce:crypto.randomUUID()})}>{pluginTitle(action,i18n.language)}</DropdownMenuItem>)}</DropdownMenuGroup>)}
 </DropdownMenuContent></DropdownMenu>;
}

// Reuse Magpie's entire P5 frame/hello/options/read/error implementation. This
// outer document supplies only a slot or a page, never the full application.
export function AIPluginSurface({extensions,slot,page,item,active,disabled=false}:{extensions?:PluginExtensions;slot?:string;page?:PluginPage;item?:Named;active:boolean;disabled?:boolean}) {
 const {t,i18n}=useTranslation(),frame=useRef<HTMLIFrameElement>(null),[height,setHeight]=useState(page?600:60);
 const available=!!extensions&&(page?true:extensions.panels.some(panel=>panel.slot===slot));
 const url=available?new URL(extensions!.url):undefined;
 if(url){
  url.searchParams.set("surface","1");
  if(page)url.searchParams.set("page",page.key);else if(slot)url.searchParams.set("slot",slot);
  if(item){url.searchParams.set("item-id",item.id);url.searchParams.set("item-name",item.name);}
 }
 const source=url?.href,origin=url?.origin;
 const sync=()=>{if(origin)frame.current?.contentWindow?.postMessage({type:"winone-plugin-theme",language:i18n.language,theme:document.documentElement.dataset.theme??(document.documentElement.classList.contains("dark")?"dark":"light")},origin);};
 useLayoutEffect(()=>{
  if(!active||!origin)return;
  const receive=(event:MessageEvent)=>{
   if(event.source!==frame.current?.contentWindow||event.origin!==origin||event.data?.type!=="winone-plugin-height")return;
   const next=event.data.height;if(typeof next==="number"&&Number.isFinite(next)&&next>0)setHeight(Math.ceil(next));
  };
  window.addEventListener("message",receive);return()=>window.removeEventListener("message",receive);
 },[active,origin]);
 useEffect(()=>{
  if(!active)return;
  const observer=new MutationObserver(sync);observer.observe(document.documentElement,{attributes:true,attributeFilter:["class","data-theme"]});sync();return()=>observer.disconnect();
 },[active,source,i18n.language]);
 if(!active||!source)return null;
 return <iframe key={source} ref={frame} src={source} title={page?pluginTitle(page,i18n.language):t("aiPlugins.panels")} onLoad={sync} inert={disabled} style={{width:"100%",height,border:0,display:"block"}}/>;
}
