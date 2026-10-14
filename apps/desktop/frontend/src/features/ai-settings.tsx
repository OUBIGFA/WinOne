import {useEffect,useRef,useState} from "react";
import {captureCall} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {Switch} from "../ui/switch";
import {Notice} from "../ui/notice";
import {Input} from "../components/ui/input";
import {Field,FieldLabel} from "../components/ui/field";
import {CaptureOptionSelect} from "../ui/capture-option-select";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";
import {AIAccounts} from "./ai-accounts";
import {createAccountReader,type AccountReader} from "./ai-account-reader";
import {AIUsage} from "./ai-usage";
import {AIRouting} from "./ai-routing";
import {AIGateway} from "./ai-gateway";
import {AIAgentPicker} from "./ai-agent-picker";
import {AIProviderOverview} from "./ai-provider-overview";
import {AIProviderIcon} from "./ai-provider-icon";
import {AIProviderSelect} from "./ai-provider-select";
import {AIProviderModels} from "./ai-provider-models";
import {AIProviderHeaders} from "./ai-provider-headers";
import {AIProviderAvatar} from "./ai-provider-avatar";
import {refreshProviderAvatars} from "./provider-avatar-cache";
import {AIModelSelect} from "./ai-model-select";
import {AIMagpieSettings} from "./ai-magpie-settings";
import {AIProviderKeys} from "./ai-provider-keys";
import {AIProviderPresets,presetConnection,type AIServicePreset} from "./ai-provider-presets";
import {LayersPlus,UserPlus,Plus,Save,RotateCcw,Trash2,Brain,Share2,ChartNoAxesCombined,SlidersHorizontal,Waypoints} from "lucide-react";
import {LayoutGridCircles} from "../components/layout-grid-circles-icon";
import {IconLabel} from "../ui/icon-label";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "../components/ui/tabs";
import type {AIConfiguration,AIProvider,AIAgent} from "./ai-configuration";

// AI services owns connections and enabled models. Feature bindings are edited
// where the feature is used; legacy bindings are preserved by this form.
export function AISettings({onActivity,accountReader:sharedReader}:{onActivity?:PageActivityChanged;accountReader?:AccountReader}) {
 const {t}=useTranslation();
 const [draft,setDraft]=useState<AIConfiguration>(),[saved,setSaved]=useState<AIConfiguration>();
 const [keyUpdates,setKeyUpdates]=useState<Record<string,Record<string,string>>>({}),[selected,setSelected]=useState("");
 const [busy,setBusy]=useState(false),[accountBusy,setAccountBusy]=useState(false),[modelBusy,setModelBusy]=useState(false),[error,setError]=useState(""),[notice,setNotice]=useState("");
 const [accountLoading,setAccountLoading]=useState(false);
 const [avatarBusy,setAvatarBusy]=useState(false);
 const [section,setSection]=useState("custom");
 const [providerTab,setProviderTab]=useState("connection");
 const [advancedTab,setAdvancedTab]=useState("quota");
 const modelSection=useRef<HTMLElement>(null);
 const [modelNavigation,setModelNavigation]=useState(0);
 // Run after the destination tab mounts. Explicit requests also focus a model
 // list already on screen, without moving focus during ordinary form edits.
 useEffect(()=>{if(!modelNavigation)return;modelSection.current?.focus({preventScroll:true});modelSection.current?.scrollIntoView({block:"start"});},[modelNavigation]);
 // A tab switch changes the kind of connection, not the account being edited.
 const selectedBySection=useRef<Record<string,string>>({});
 const [choosingAgent,setChoosingAgent]=useState(false);
 const [agents,setAgents]=useState<AIAgent[]>([]),[catalogBusy,setCatalogBusy]=useState(false),[catalogError,setCatalogError]=useState("");
 const [presets,setPresets]=useState<AIServicePreset[]>([]);
 const [catalogLoaded,setCatalogLoaded]=useState(false);
 const [localReader]=useState(createAccountReader);
 const accountReader=sharedReader??localReader;
 const catalogRequest=useRef<Promise<void>|undefined>(undefined);
 const [loading,setLoading]=useState(false);
 function loadCatalog():Promise<void>{
  if(catalogRequest.current)return catalogRequest.current;
  setCatalogBusy(true);setCatalogError("");
  catalogRequest.current=(async()=>{try{const result=await accountReader.catalog();setAgents(result.agents);setPresets(result.presets??[]);setCatalogLoaded(true);}catch(error){setCatalogError(String(error));}finally{setCatalogBusy(false);catalogRequest.current=undefined;}})();
  return catalogRequest.current;
 }
 useEffect(()=>{if((section==="accounts"||draft?.providers.some(provider=>provider.preset))&&!catalogLoaded)void loadCatalog();},[section,catalogLoaded,draft?.revision]);
 const dirty=!!draft&&(JSON.stringify(draft)!==JSON.stringify(saved)||Object.keys(keyUpdates).length>0);
 const navigationLocked=busy||accountBusy||modelBusy||avatarBusy;
 const locked=navigationLocked||loading;
 // Reads do not own unsaved work. Keep navigation usable while they settle.
 usePageActivity(onActivity,dirty,navigationLocked,"ai-settings");
 async function load(){setLoading(true);setError("");setNotice("");try{const value=await captureCall<AIConfiguration>("ai.configuration.get");refreshProviderAvatars();setDraft(value);setSaved(value);setKeyUpdates({});setSelected(current=>value.providers.some(p=>p.id===current)?current:value.providers[0]?.id??"");}catch(error){setError(String(error));}finally{setLoading(false);}}
 useEffect(()=>{void load();},[]);
 async function save(){if(!draft)return;setBusy(true);setError("");setNotice("");try{const value=await captureCall<AIConfiguration>("ai.configuration.save",{configuration:draft,...(Object.keys(keyUpdates).length?{keyUpdates}:{})});refreshProviderAvatars();setDraft(value);setSaved(value);setKeyUpdates({});setNotice(t("common.saved"));}catch(error){setError(String(error));}finally{setBusy(false);}}
 function change(p:AIProvider,patch:Partial<AIProvider>){
  setDraft(value=>{
   if(!value)return value;
   const providers=value.providers.map(current=>current.id===p.id?{...current,...patch}:current);
   const updated=providers.find(current=>current.id===p.id);
   if(!updated||patch.availableModels===undefined)return {...value,providers};
   // The Host rejects references to removed catalog entries. Clear only those
   // references; disabling a still-listed model keeps its existing assignment.
   const models=new Set([...(updated.availableModels??[]),...updated.models]);
   const tasks=Object.fromEntries(Object.entries(value.tasks).map(([task,selection])=>[task,selection.provider===p.id&&!models.has(selection.model)?{provider:"",model:""}:selection]));
   return {...value,providers,tasks};
  });setNotice("");
 }
 function changeKey(provider:string,id:string,value:string|undefined){setKeyUpdates(current=>{const next={...current},edits={...next[provider]};if(value===undefined)delete edits[id];else edits[id]=value;if(Object.keys(edits).length)next[provider]=edits;else delete next[provider];return next;});}
 // Names are derived from the shared catalog, with suffixes only for duplicate
 // accounts. Naming another connection must never rename an existing account.
 function uniqueName(base:string,id?:string){const names=new Set(draft?.providers.filter(p=>p.id!==id).map(p=>p.name.trim().toLocaleLowerCase()));let name=base;for(let i=2;names.has(name.toLocaleLowerCase());i++)name=`${base} ${i}`;return name;}
 function add(agent?:AIAgent){const authMode=agent?"account":"api-key";
  const p:AIProvider={id:crypto.randomUUID(),name:agent?uniqueName(agent.name):"",authMode,protocol:"chat",baseUrl:"",agent:agent?.id??"",agentBackend:agent?(agent.backend??"builtin"):undefined,icon:agent?.icon,models:[],availableModels:[],hasKey:false};
  setDraft(value=>value?{...value,providers:[...value.providers,p]}:value);setSelected(p.id);setNotice("");
 }
 function addPreset(preset:AIServicePreset){const current=draft?.providers.find(p=>p.id===selected);const empty=current?.authMode==="api-key"&&!saved?.providers.some(p=>p.id===current.id)&&!current.name.trim()&&!current.baseUrl?.trim()&&!current.models.length&&!current.availableModels?.length&&!keyUpdates[current.id]&&!current.keys?.some(key=>key.hasKey)?current:undefined;const region=preset.regions?.[0]?.id;const p:AIProvider={id:empty?.id??crypto.randomUUID(),name:uniqueName(preset.name,empty?.id),authMode:"api-key",preset:preset.id,presetRegion:region,...presetConnection(preset,region),models:[],availableModels:preset.models??[],hasKey:false,icon:preset.icon};setDraft(value=>value?{...value,providers:empty?value.providers.map(value=>value.id===empty.id?p:value):[...value.providers,p]}:value);setSelected(p.id);setNotice("");}
 function changeProtocol(provider:AIProvider,protocol:string){const preset=presets.find(item=>item.id===provider.preset);if(!preset){change(provider,{protocol});return;}const previous=presetConnection(preset,provider.presetRegion,provider.protocol),next=presetConnection(preset,provider.presetRegion,protocol);change(provider,{protocol,baseUrl:provider.baseUrl===previous.baseUrl&&next.protocol===protocol?next.baseUrl:provider.baseUrl});}

 const providers=section!=="custom"&&section!=="accounts"?[]:(draft?.providers??[]).filter(p=>p.authMode===(section==="custom"?"api-key":"account"));
 const provider=providers.find(p=>p.id===selected)??providers[0];
 const preset=presets.find(item=>item.id===provider?.preset);
 const old=provider&&saved?.providers.find(p=>p.id===provider.id);
 const connectionSaved=!!provider&&!!old&&provider.authMode===old.authMode&&provider.agent===old.agent&&provider.agentBackend===old.agentBackend&&provider.preset===old.preset&&provider.presetRegion===old.presetRegion&&provider.baseUrl===old.baseUrl&&provider.protocol===old.protocol&&keyUpdates[provider.id]===undefined&&JSON.stringify(provider.keys)===JSON.stringify(old.keys)&&JSON.stringify(provider.keyPolicy)===JSON.stringify(old.keyPolicy)&&JSON.stringify(provider.headers)===JSON.stringify(old.headers);
 const serviceItems=providers.map(p=>({value:p.id,label:p.name||t("ai.unnamed"),icon:<AIProviderIcon icon={p.avatar||p.icon}/>}));
 function openProviderModels(id:string){
  const target=draft?.providers.find(item=>item.id===id);if(!target||locked)return;
  // Navigate inside the same draft; loading settings here would lose unsaved
  // connection/key edits. Removed draft providers cannot be opened via summary.
  if(provider)selectedBySection.current[section]=provider.id;
  const next=target.authMode==="account"?"accounts":"custom";
  selectedBySection.current[next]=id;setSection(next);setSelected(id);setProviderTab("connection");setModelNavigation(value=>value+1);
 }
 function remove(p:AIProvider){const tasks=Object.fromEntries(Object.entries(draft!.tasks).map(([task,selection])=>[task,selection.provider===p.id?{provider:"",model:""}:selection]));setDraft({...draft!,providers:draft!.providers.filter(value=>value.id!==p.id),tasks});setKeyUpdates(current=>{const next={...current};delete next[p.id];return next;});setSelected("");}
  const removedOverviewProviders=draft&&saved?saved.providers.filter(item=>!draft.providers.some(current=>current.id===item.id)):[];
  const overviewProviders=draft?[...draft.providers,...removedOverviewProviders]:saved?.providers??[];
  return <section className="settings-stack">
  <p className="muted">{t("ai.shared")}</p>
   <AIProviderOverview providers={overviewProviders} disabled={locked} unavailableProviders={removedOverviewProviders.map(item=>item.id)} onOpenProvider={openProviderModels}/>
  <fieldset disabled={busy} className="plain-fieldset settings-stack">

   <Tabs value={section} onValueChange={value=>{if(provider)selectedBySection.current[section]=provider.id;const next=String(value);setSection(next);setSelected(selectedBySection.current[next]??"");}}>
    <TabsList variant="default" className="max-w-full flex-wrap group-data-horizontal/tabs:h-auto" aria-label={t("ai.serviceTitle")}><TabsTrigger value="custom" disabled={navigationLocked}><IconLabel icon={LayersPlus}>{t("ai.customProviders")}</IconLabel></TabsTrigger><TabsTrigger value="accounts" disabled={navigationLocked}><IconLabel icon={UserPlus}>{t("ai.addProvidersTab")}</IconLabel></TabsTrigger><TabsTrigger value="defaults" disabled={navigationLocked}><IconLabel icon={LayoutGridCircles}>{t("ai.defaultService")}</IconLabel></TabsTrigger><TabsTrigger value="gateway" disabled={navigationLocked}><IconLabel icon={Share2}>{t("gateway.title")}</IconLabel></TabsTrigger><TabsTrigger value="routing" disabled={navigationLocked}><IconLabel icon={Waypoints}>{t("routing.title")}</IconLabel></TabsTrigger><TabsTrigger value="usage" disabled={navigationLocked}><IconLabel icon={ChartNoAxesCombined}>{t("usage.title")}</IconLabel></TabsTrigger><TabsTrigger value="advanced" disabled={navigationLocked}><IconLabel icon={SlidersHorizontal}>{t("ai.advanced")}</IconLabel></TabsTrigger></TabsList>
    {error&&<Notice actions={!draft&&<Button disabled={busy} onClick={()=>void load()}>{t("common.retry")}</Button>}>{error}</Notice>}
    {section==="accounts"&&catalogError&&<Notice actions={<Button onClick={()=>void loadCatalog()}>{t("common.retry")}</Button>}>{catalogError}</Notice>}
    {section==="accounts"&&catalogBusy&&<p role="status">{t("capture.loading")}</p>}
    {!draft&&!error&&section!=="gateway"&&section!=="usage"&&<TabsContent value={section}><p role="status">{t("capture.loading")}</p></TabsContent>}
    {draft&&(section==="custom"||section==="accounts")&&<TabsContent value={section} className="settings-stack">
     <div className="settings-provider-toolbar">
      {provider&&<div className="min-w-0 flex-1 basis-48"><AIProviderSelect key={provider.id} provider={provider} items={serviceItems} disabled={locked} onSelect={setSelected} onNameChange={name=>change(provider,{name})}/></div>}
      <Button icon={Plus} disabled={locked||draft.providers.length>=32||section==="accounts"&&!agents.length} onClick={()=>section==="accounts"?setChoosingAgent(true):add()}>{t(section==="custom"?"ai.addCustomProvider":"ai.addAccountProvider")}</Button>
      {section==="custom"&&<AIProviderPresets presets={presets} busy={catalogBusy} error={catalogError} disabled={locked||draft.providers.length>=32} load={loadCatalog} onAdd={addPreset}/>}
     </div>
     {!providers.length&&!(section==="accounts"&&catalogBusy)&&<p className="muted">{t(section==="accounts"?"ai.noAccounts":"ai.noServices")}</p>}
     {provider&&<div className="settings-provider" key={provider.id}>
      {provider.authMode==="account"&&<AIAccounts key={`${provider.id}:${provider.agent}:${provider.agentBackend??"builtin"}`} reader={accountReader} provider={provider.id} agent={provider.agent||"codex"} draft={!connectionSaved?{agent:provider.agent||"codex",agentBackend:provider.agentBackend}:undefined} disabled={busy||loading||modelBusy} catalog={agents} onBusy={setAccountBusy} onLoading={setAccountLoading}/>}
      {provider.authMode==="api-key"&&<Tabs value={providerTab} onValueChange={value=>setProviderTab(String(value))}>
       <TabsList variant="line" aria-label={t("ai.providerSettings")}><TabsTrigger value="connection" disabled={navigationLocked}>{t("ai.connection")}</TabsTrigger><TabsTrigger value="advanced" disabled={navigationLocked}>{t("ai.advanced")}</TabsTrigger></TabsList>
       <TabsContent value="connection" keepMounted className="settings-group" aria-label={t("ai.connection")}>
       <CaptureOptionSelect stacked label={t("ai.protocol")} value={provider.protocol||"chat"} disabled={locked} items={[{value:"chat",label:"OpenAI Chat Completions"},{value:"responses",label:"OpenAI Responses"},{value:"anthropic",label:"Anthropic Messages"}]} onChange={protocol=>changeProtocol(provider,protocol)}/>
       {!!preset?.regions?.length&&<CaptureOptionSelect stacked label={t("ai.presetRegion")} value={provider.presetRegion??preset.regions[0].id} items={preset.regions.map(region=>({value:region.id,label:region.name}))} disabled={locked} onChange={region=>change(provider,{presetRegion:region,...presetConnection(preset,region,provider.protocol),models:[],availableModels:preset.models??[]})}/>}
       <Field><FieldLabel htmlFor="ai-url">{t("ai.address")}</FieldLabel><Input id="ai-url" value={provider.baseUrl??""} disabled={locked} placeholder={preset?.endpoint??"https://api.example.com/v1"} onChange={event=>change(provider,{baseUrl:event.target.value})}/></Field>
       <AIProviderKeys provider={provider} edits={keyUpdates[provider.id]??{}} disabled={locked} onChange={patch=>change(provider,patch)} onEdit={(id,value)=>changeKey(provider.id,id,value)}/>
       {preset?.kind==="local"&&<p className="settings-help">{t("ai.localServerHint")}</p>}
       </TabsContent>
       <TabsContent value="advanced" keepMounted className="settings-stack">
        <AIProviderAvatar provider={provider} disabled={locked} onChange={avatar=>change(provider,{avatar})} onBusy={setAvatarBusy}/>
        <AIProviderHeaders key={provider.id} value={provider.headers} resetToken={saved} disabled={locked} onChange={headers=>change(provider,{headers})}/>
       </TabsContent>
      </Tabs>}
      <section ref={modelSection} tabIndex={-1} aria-label={t("ai.modelSettings")}>
       <AIProviderModels key={`models-${provider.id}:${provider.agent}`} provider={provider} canFetch={provider.authMode==="account"||!!provider.baseUrl?.trim()} draftAccount={provider.authMode==="account"&&!connectionSaved} draftConnection={provider.authMode==="api-key"&&!connectionSaved} keyUpdates={keyUpdates[provider.id]} disabled={busy||loading||accountBusy||provider.authMode==="account"&&accountLoading} onChange={patch=>change(provider,patch)} onBusy={setModelBusy}/>
      </section>
     </div>}
    </TabsContent>}
    <TabsContent value="defaults" className="settings-stack">{draft&&<>
   <section className="settings-group" aria-label={t("ai.defaultService")}>
    <h3 className="settings-group-title">{t("ai.defaultService")}</h3><p className="settings-group-description">{t("ai.defaultServiceHint")}</p>
    <AIModelSelect label={t("ai.defaultService")} providers={draft.providers.filter(p=>p.models.length>0||p.id===draft.tasks.default?.provider)} value={draft.tasks.default} disabled={locked} onChange={value=>{const tasks={...draft.tasks};if(value)tasks.default=value;else delete tasks.default;setDraft({...draft,tasks});}}/>
   </section>
   <section className="settings-group settings-response-options"><div className="settings-switch-row"><label htmlFor="ai-auto-retranslate">{t("ai.autoRetranslate")}</label><Switch id="ai-auto-retranslate" checked={draft.autoRetranslate!==false} disabled={locked} onCheckedChange={autoRetranslate=>setDraft({...draft,autoRetranslate})}/></div><div className="settings-switch-row"><label htmlFor="ai-show-reasoning"><IconLabel icon={Brain}>{t("ai.showReasoning")}</IconLabel></label><Switch id="ai-show-reasoning" checked={draft.showReasoning!==false} disabled={locked} onCheckedChange={showReasoning=>setDraft({...draft,showReasoning})}/></div></section>
    </>}</TabsContent>
    {draft&&section==="advanced"&&<TabsContent value="advanced" className="settings-stack"><Tabs value={advancedTab} onValueChange={value=>setAdvancedTab(String(value))}><TabsList variant="line" className="max-w-full flex-wrap group-data-horizontal/tabs:h-auto" aria-label={t("ai.advanced")}><TabsTrigger value="quota" disabled={navigationLocked}>{t("ai.quotaSettings")}</TabsTrigger><TabsTrigger value="model-options" disabled={navigationLocked}>{t("ai.modelSettings")}</TabsTrigger><TabsTrigger value="privacy" disabled={navigationLocked}>{t("ai.privacySettings")}</TabsTrigger></TabsList><TabsContent value="quota" keepMounted><AIMagpieSettings section="quota" settings={draft.magpieSettings??{}} providers={draft.providers} disabled={locked} onChange={patch=>{setDraft({...draft,magpieSettings:{...draft.magpieSettings,...patch}});setNotice("");}}/></TabsContent><TabsContent value="model-options" keepMounted><AIMagpieSettings section="model-options" settings={draft.magpieSettings??{}} providers={draft.providers} disabled={locked} onChange={patch=>{setDraft({...draft,magpieSettings:{...draft.magpieSettings,...patch}});setNotice("");}}/></TabsContent><TabsContent value="privacy" keepMounted><AIMagpieSettings section="privacy" settings={draft.magpieSettings??{}} providers={draft.providers} disabled={locked} onChange={patch=>{setDraft({...draft,magpieSettings:{...draft.magpieSettings,...patch}});setNotice("");}}/></TabsContent></Tabs></TabsContent>}
    <TabsContent value="gateway" keepMounted><AIGateway active={section==="gateway"} providers={saved?.providers??[]} onActivity={onActivity}/></TabsContent>
    <TabsContent value="routing" keepMounted><AIRouting active={section==="routing"} enabled={!!draft?.routingEnabled} revision={saved?.revision} onEnabled={routingEnabled=>{if(draft)setDraft({...draft,routingEnabled});}}/></TabsContent>
    <TabsContent value="usage" keepMounted><AIUsage active={section==="usage"} providers={saved?.providers??[]} preferences={draft?.magpieSettings} onPreferencesChange={patch=>setDraft(value=>value?{...value,magpieSettings:{...value.magpieSettings,...patch}}:value)}/></TabsContent>
   </Tabs>
   {draft&&section!=="gateway"&&(section!=="usage"||dirty)&&<div className="settings-save-bar"><Button icon={Save} variant="primary" disabled={!dirty||locked} onClick={()=>void save()}>{t("ai.save")}</Button>{dirty&&<Button icon={RotateCcw} disabled={locked} onClick={()=>void load()}>{t("shortcuts.undo")}</Button>}{provider&&<Button className="settings-remove-action" icon={Trash2} disabled={locked} onClick={()=>remove(provider)}>{t("ai.removeService")}</Button>}</div>}
  </fieldset>
  {notice&&!dirty&&<p role="status">{notice}</p>}
  <AIAgentPicker open={choosingAgent} onOpenChange={setChoosingAgent} agents={agents} onChoose={add}/>
 </section>;
}
