import {useEffect,useRef,useState} from "react";
import {Plus,RefreshCw,SlidersHorizontal,PlugZap,MoreHorizontal,Copy,Check,X} from "lucide-react";
import {captureCall} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {Input} from "../components/ui/input";
import {Checkbox} from "../components/ui/checkbox";
import {Notice} from "../ui/notice";
import {AIModelIcon} from "./ai-model-icon";
import {modelLabel} from "./ai-model-label";
import type {AIProvider,AIModelMetadata} from "./ai-configuration";
import {AIModelSelectionActions} from "./ai-model-selection-actions";
import {AIModelBadge} from "./ai-model-badge";
import {AIModelSettings,AIRequestLimits} from "./ai-model-settings";
import {DropdownMenu,DropdownMenuTrigger,DropdownMenuContent,DropdownMenuItem} from "../components/ui/dropdown-menu";

type ModelTestResult={account?:string;model?:string;ok:boolean;ms:number;protocol?:string;status?:number;error?:string};

// Discovery replaces the provider catalog; manual entry adds candidates.
// Checkboxes decide which candidates appear in feature/model selectors.
export function AIProviderModels({provider,canFetch,draftAccount=false,draftConnection=false,keyUpdates,disabled,onChange,onBusy}:{provider:AIProvider;canFetch:boolean;draftAccount?:boolean;draftConnection?:boolean;keyUpdates?:Record<string,string>;disabled:boolean;onChange:(patch:Partial<AIProvider>)=>void;onBusy:(value:boolean)=>void}){
 const {t}=useTranslation();const [query,setQuery]=useState(""),[fetching,setFetching]=useState(false),[error,setError]=useState("");
 const [editing,setEditing]=useState<string>();
 const [testing,setTesting]=useState(false),[tested,setTested]=useState<Record<string,ModelTestResult>>({}),[testNotice,setTestNotice]=useState(""),[menuFor,setMenuFor]=useState("");
 const [testClients,setTestClients]=useState<Record<string,string>>(); const working=fetching||testing;
 const list=useRef<HTMLDivElement>(null);
 const storageKey="winone.ai-model-height.v1."+provider.id;
 const [height]=useState(()=>{try{const value=Number(localStorage.getItem(storageKey));return value>=240&&value<=1200?value:400;}catch(error){console.warn("Model list size unavailable",error);return 400;}});
 // Native resizing updates inline height. Viewport clamping must not replace
 // the preference, so a larger window can restore the user's chosen height.
 useEffect(()=>{const element=list.current;if(!element||typeof ResizeObserver==="undefined")return;const observer=new ResizeObserver(()=>{const value=Number.parseFloat(element.style.height);if(Number.isFinite(value)&&value>=240)try{localStorage.setItem(storageKey,String(Math.min(1200,value)));}catch(error){console.warn("Model list size could not be saved",error);}});observer.observe(element);return ()=>observer.disconnect();},[storageKey]);
 const all=[...new Set([...(provider.availableModels??[]),...provider.models])];
 const visible=all.filter(model=>(model+" "+(provider.modelNames?.[model]??"")).toLowerCase().includes(query.trim().toLowerCase()));
 function toggle(model:string,checked:boolean){onChange({availableModels:all,models:checked?[...provider.models,model]:provider.models.filter(value=>value!==model)});}
 function add(){const model=query.trim();if(!model)return;if(model.length>256){setError(t("ai.modelNameTooLong"));return;}onChange({availableModels:[...new Set([...all,model])],models:[...new Set([...provider.models,model])]});setQuery("");setError("");}
 function connectionPayload(){return {provider:provider.id,...(draftAccount?{draft:{agent:provider.agent,agentBackend:provider.agentBackend}}:{}),...(draftConnection?{connection:{protocol:provider.protocol||"chat",baseUrl:provider.baseUrl,headers:provider.headers,limitRestOff:provider.limitRestOff,additionalTools:provider.additionalTools,preset:provider.preset,presetRegion:provider.presetRegion,keys:provider.keys??[{id:"primary",enabled:true,hasKey:provider.hasKey}],keyPolicy:provider.keyPolicy},keyUpdates}:{})};}
 // Like upstream provider/test, tests use the editor snapshot and never save
 // credentials. A result must exist for every requested model before success.
 async function testModels(models:string[],as?:string){if(working||disabled||!models.length)return;setTesting(true);onBusy(true);setError("");setTestNotice("");try{const response=await captureCall<{results:ModelTestResult[];clients?:Record<string,string>}>("ai.models.test",{...connectionPayload(),models,...(as?{as}:{}),metadata:provider.modelMetadata});if(!Array.isArray(response.results)||response.results.length!==models.length||response.results.some(result=>typeof result.ok!=="boolean"))throw Error(t("ai.modelTestIncomplete"));setTestClients(response.clients??{});setTested(current=>({...current,...Object.fromEntries(models.map((model,index)=>[model,response.results[index]]))}));const failed=response.results.filter(result=>!result.ok).length;setTestNotice(t(failed?"ai.modelTestsFailed":"ai.modelTestsPassed",{count:models.length,failed}));}catch(error){setError(String(error));}finally{setTesting(false);onBusy(false);}}
 async function fetchModels(){
  setFetching(true);onBusy(true);setError("");
  try{
   const value=await captureCall<{models:{id:string;name?:string;icon?:string;free?:boolean;metadata?:AIModelMetadata}[]}>("ai.models.fetch",connectionPayload());
   // A successful fetch is a current snapshot, including empty catalogs and
   // removed labels. Reusing old maps would revive retired models or pricing.
   const modelNames:Record<string,string>={},modelFree:Record<string,boolean>={},modelMetadata:Record<string,AIModelMetadata>={};
   for(const model of value.models){
    if(model.name?.trim())modelNames[model.id]=model.name;
    modelFree[model.id]=model.free===true;
    if(model.metadata)modelMetadata[model.id]={...model.metadata};
   }
   const availableModels=[...new Set(value.models.map(model=>model.id))];
   const restored=provider.authMode==="account"?[...(provider.delisted??[]),...provider.models]:provider.models;
   const models=[...new Set(restored)].filter(model=>availableModels.includes(model));
   // Preserve only user intent, not retired labels/pricing. A plugin list may
   // temporarily shrink to zero; Select none explicitly forgets these picks.
   const delisted=provider.authMode==="account"?[...new Set(restored)].filter(model=>!availableModels.includes(model)):[];
   onChange({availableModels,models,delisted,modelNames,modelFree,modelMetadata});
   // Tests describe the previous snapshot and must be rerun after discovery.
   setTested({});setTestNotice("");setTestClients(undefined);setEditing(undefined);setMenuFor("");
  }catch(error){setError(String(error));}finally{setFetching(false);onBusy(false);}
 }
 return <section className="settings-group">
  <div className="settings-model-heading"><div><h3 className="settings-group-title">{t("ai.models")}</h3><p className="settings-group-description">{t("ai.modelSelectionHint")}</p></div><div className="flex flex-wrap gap-2"><Button icon={PlugZap} disabled={disabled||working||!canFetch||!provider.models.length} busy={testing} onClick={()=>void testModels(provider.models)}>{t("ai.testModels")}</Button><Button icon={RefreshCw} disabled={disabled||working||!canFetch} busy={fetching} onClick={()=>void fetchModels()}>{t("ai.fetchModels")}</Button></div></div>
  <div className="flex flex-wrap items-center gap-2"><Input className="min-w-36 flex-1" aria-label={t("ai.findOrAddModel")} placeholder={t("ai.findOrAddModel")} value={query} disabled={disabled||working} onChange={event=>setQuery(event.target.value)}/><Button icon={Plus} disabled={disabled||working||!query.trim()} onClick={add}>{t("ai.addModel")}</Button></div>
  {!canFetch&&all.length>0&&<p className="settings-help">{t("ai.connectionForModels")}</p>}
  {error&&<Notice>{error}</Notice>}
  {testNotice&&<p role="status" className="settings-help">{testNotice}</p>}
  {testClients&&Object.entries(testClients).map(([client,reason])=><p key={client} className="settings-help">{t(client==="codex"?"ai.testAsCodex":"ai.testAsClaude")} · {reason||t("ai.clientTestAvailable")}</p>)}
  <div className="flex flex-wrap items-center justify-between gap-2"><p className="settings-model-count">{t("ai.enabledModels",{count:provider.models.length})}</p><AIModelSelectionActions total={all.length} selected={provider.models.length} hasPaused={!!provider.delisted?.length} disabled={disabled||working} onAll={()=>onChange({availableModels:all,models:all})} onNone={()=>onChange({availableModels:all,models:[],delisted:[]})}/></div>
  {!!provider.delisted?.length&&<p className="settings-help" role="status">{t("ai.delistedPicks",{count:provider.delisted.length})}</p>}
  <div ref={list} className="min-h-60 max-h-[75vh] resize-y overflow-y-auto overflow-x-hidden rounded-lg border border-border p-2" style={{height}} aria-label={t("ai.models")} title={t("ai.resizeModelList")}>
   {visible.length?visible.map(model=>{const enabled=provider.models.includes(model),name=modelLabel(provider,model),result=tested[model];return <div key={model} title={model} onContextMenu={event=>{event.preventDefault();if(!disabled&&!working)setMenuFor(model);}} onKeyDown={event=>{if(event.key==="ContextMenu"||event.shiftKey&&event.key==="F10"){event.preventDefault();if(!disabled&&!working)setMenuFor(model);}}} className={`flex flex-wrap items-center gap-1.5 rounded-md px-2 py-1 hover:bg-muted focus-within:bg-muted ${enabled?"":"text-muted-foreground"}`}><label className="flex min-w-0 flex-1 cursor-pointer items-center gap-1.5 py-1"><Checkbox checked={enabled} disabled={disabled||working} onCheckedChange={checked=>toggle(model,checked===true)}/><span className={enabled?"contents":"contents [&>*]:grayscale"}><AIModelIcon model={model} name={provider.modelNames?.[model]}/></span><span className="min-w-0 break-all">{name}</span><AIModelBadge provider={provider} id={model}/></label><Button variant="ghost" size="icon" icon={SlidersHorizontal} aria-label={t("ai.editModelSettings",{name})} title={t("ai.modelSettings")} disabled={disabled||working} onClick={()=>setEditing(model)}/><DropdownMenu open={menuFor===model} onOpenChange={open=>setMenuFor(open?model:"")}><DropdownMenuTrigger render={<Button variant="ghost" size="icon" icon={MoreHorizontal} aria-label={t("ai.modelActions",{name})} disabled={disabled||working}/>}/><DropdownMenuContent align="end"><DropdownMenuItem disabled={!canFetch} onClick={()=>void testModels([model])}><PlugZap/>{t("ai.testThisModel")}</DropdownMenuItem><DropdownMenuItem disabled={!canFetch||testClients!==undefined&&(!("codex" in testClients)||!!testClients.codex)} title={testClients?.codex} onClick={()=>void testModels([model],"codex")}>{t("ai.testAsCodex")}</DropdownMenuItem><DropdownMenuItem disabled={!canFetch||testClients!==undefined&&(!("claude-code" in testClients)||!!testClients["claude-code"])} title={testClients?.["claude-code"]} onClick={()=>void testModels([model],"claude-code")}>{t("ai.testAsClaude")}</DropdownMenuItem><DropdownMenuItem onClick={()=>void navigator.clipboard.writeText(model).catch(error=>setError(String(error)))}><Copy/>{t("ai.copyModelID")}</DropdownMenuItem><DropdownMenuItem onClick={()=>setEditing(model)}><SlidersHorizontal/>{t("ai.modelSettings")}</DropdownMenuItem></DropdownMenuContent></DropdownMenu>{result&&<p className={`flex basis-full items-start gap-1.5 break-all pl-6 text-sm ${result.ok?"text-muted-foreground":"text-destructive"}`}>{result.ok?<Check size={16}/>:<X size={16}/>}<span>{result.account&&`${result.account} · `}{result.model&&result.model!==model&&`${result.model} · `}{result.protocol&&`${result.protocol} · `}{result.ok?t("ai.modelTestTime",{ms:result.ms}):`${result.status||""} ${result.error||t("ai.modelTestFailed")}`}</span></p>}</div>}):<p className="settings-empty">{t(all.length?"ai.noMatchingModels":canFetch?"ai.fetchOrAddModels":"ai.connectionForModels")}</p>}
  </div>
  <AIRequestLimits provider={provider} disabled={disabled||working} onChange={onChange}/>
  {editing&&<AIModelSettings provider={provider} model={editing} disabled={disabled||working} onChange={onChange} onClose={()=>setEditing(undefined)}/>}
 </section>;
}
