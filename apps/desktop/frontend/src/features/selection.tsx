import {SettingsSaveBar} from "../components/settings-save-bar";
import {useEffect,useLayoutEffect,useRef,useState} from "react";
import {Settings,Save,RotateCcw,Plus,Trash2,SlidersHorizontal,Copy,Languages,MessageCircleQuestion,ListCheck,Sparkles,PenLine,CodeXml,MessageCircleMore,PlugZap,BookOpen,Lightbulb,Search,FileText,CheckCheck,NotebookPen,Mail,Globe,GraduationCap,Calculator,Terminal,Braces,ListChecks,TextCursorInput,CaseSensitive,WandSparkles,Astroid,ChevronDown,SendHorizontal,MousePointerClick,Ban,AppWindow} from "lucide-react";
import {QuickAssistantIcon} from "../components/quick-assistant-icon";
import {Accordion} from "@base-ui/react/accordion";
import {selectionCall,quickAssistantCall,captureCall} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {Switch} from "../ui/switch";
import {Notice} from "../ui/notice";
import {IconLabel} from "../ui/icon-label";
import {Label} from "../components/ui/label";
import {usePathDrop} from "../ui/path-drop";
import {Textarea} from "../components/ui/textarea";
import {Input} from "../components/ui/input";
import {CaptureOptionSelect} from "../ui/capture-option-select";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "../components/ui/tabs";
import {SettingsSection,SettingsHelp,SettingsToggle} from "../components/settings-layout";

import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";

import {Shortcuts} from "./shortcuts";
import {AIModelSelect} from "./ai-model-select";
import type {AIConfiguration,AISelection} from "./ai-configuration";
import "./selection.css";

interface SelectionAction {selection?:AISelection;id:string;name:string;englishName?:string;icon:string;kind:"copy"|"translate"|"ai";prompt?:string;enabled:boolean}
interface Preferences {schemaVersion:number;revision:number;automatic:boolean;keyboard:boolean;excludedApps:string[];actions?:SelectionAction[];alwaysOnTop?:boolean;showReasoning?:boolean}
const actionIcons={copy:Copy,translate:Languages,explain:MessageCircleQuestion,summarize:ListCheck,polish:PenLine,code:CodeXml,conversation:MessageCircleMore,service:PlugZap,book:BookOpen,idea:Lightbulb,search:Search,document:FileText,check:CheckCheck,write:NotebookPen,mail:Mail,globe:Globe,learn:GraduationCap,calculate:Calculator,terminal:Terminal,braces:Braces,tasks:ListChecks,text:TextCursorInput,case:CaseSensitive,wand:WandSparkles,astroid:Astroid};
// Both assistant settings use the same editor; each saves its own ordered actions.
export function SelectionPage({disabled=false,onActivity,onSaved,moduleId="selection"}:{disabled?:boolean;onActivity?:PageActivityChanged;onSaved?:()=>void;moduleId?:"selection"|"quick-assistant"}) {
  const quick=moduleId==="quick-assistant";
  const call=quick?quickAssistantCall:selectionCall;
  const {t,i18n}=useTranslation();
  const [tab,setTab]=useState("toolbar"),[draft,setDraft]=useState<Preferences>(),[saved,setSaved]=useState<Preferences>();
  const [excluded,setExcluded]=useState(""),[working,setBusy]=useState(false),[loading,setLoading]=useState(false),[error,setError]=useState(""),[notice,setNotice]=useState("");
  const busy=working||loading;
  const [expanded,setExpanded]=useState<string[]>([]);
  const pendingFocus=useRef("");
  // Only a newly added action takes focus. Reopening other rows keeps normal
  // accordion keyboard navigation and does not overwrite the user's selection.
  useLayoutEffect(()=>{
    if(!pendingFocus.current)return;
    const input=document.getElementById(`selection-action-${pendingFocus.current}`) as HTMLInputElement|null;
    if(input){input.closest(".selection-action")?.scrollIntoView?.({block:"start"});input.focus({preventScroll:true});input.select();pendingFocus.current="";}
  },[expanded]);
  const excludedApps=excluded.split(/\r?\n/).map(value=>value.trim()).filter(Boolean);
  const exclusionDrop=usePathDrop({kind:"file",multiple:true,disabled:busy||!draft,onError:setError,onPaths:paths=>{setError("");setExcluded(value=>[...new Set([...value.split(/\r?\n/).filter(Boolean),...paths])].join("\n"));}});
  const dirty=!!draft&&JSON.stringify(quick?draft:{...draft,excludedApps})!==JSON.stringify(saved);
  // Discovery does not need to keep either assistant's settings page open.
  usePageActivity(onActivity,dirty,working,moduleId);
  async function load(){setLoading(true);setError("");setNotice("");try{const value=await call<Preferences>("settings.get");setDraft(value);setSaved(value);setExcluded((value.excludedApps??[]).join("\n"));}catch(error){setError(String(error));}finally{setLoading(false);}}
  useEffect(()=>{void load();},[moduleId]);
  async function save(){if(!draft)return;setBusy(true);setError("");setNotice("");try{const value=await call<Preferences>("settings.save",quick?draft:{...draft,excludedApps});setDraft(value);setSaved(value);setExcluded((value.excludedApps??[]).join("\n"));setNotice(t("common.saved"));}catch(error){setError(String(error));}finally{setBusy(false);}}
  const [ai,setAI]=useState<AIConfiguration>(),[aiError,setAIError]=useState("");
  async function loadAI(){setAIError("");try{setAI(await captureCall<AIConfiguration>("ai.configuration.get"));}catch(error){setAIError(String(error));}}
  useEffect(()=>{if(tab==="toolbar")void loadAI();},[tab]);
  const actions=draft?.actions??[];
  const changeAction=(index:number,patch:Partial<SelectionAction>)=>{if(draft)setDraft({...draft,actions:actions.map((action,i)=>i===index?{...action,...patch}:action)});};
  const moveAction=(from:number,to:number)=>{if(!draft)return;const next=[...actions];const [action]=next.splice(from,1);next.splice(to,0,action);setDraft({...draft,actions:next});};
  const saveActions=<SettingsSaveBar><Button icon={Save} variant="primary" disabled={!dirty||busy} onClick={()=>void save()}>{t("selection.save")}</Button>{dirty&&<Button icon={RotateCcw} disabled={busy} onClick={()=>void load()}>{t("selection.undo")}</Button>}</SettingsSaveBar>;
  return <Tabs value={tab} onValueChange={value=>setTab(String(value))}>
    <div className="settings-tab-bar">
    <TabsList variant="default" className="max-w-full flex-wrap justify-start gap-1 group-data-horizontal/tabs:h-auto" aria-label={t("nav."+moduleId)}>
      <TabsTrigger title={t("selection.toolbar")} value="toolbar"><IconLabel icon={SlidersHorizontal}>{t("selection.toolbar")}</IconLabel></TabsTrigger>
      <TabsTrigger title={t("selection.general")} value="general"><IconLabel icon={Settings}>{t("selection.general")}</IconLabel></TabsTrigger>
      <TabsTrigger title={t("shortcuts.title")} value="shortcuts"><IconLabel icon={SendHorizontal}>{t("shortcuts.title")}</IconLabel></TabsTrigger>
      
    </TabsList>
    {quick&&<Button icon={QuickAssistantIcon} disabled={disabled||busy} onClick={()=>{setError("");void quickAssistantCall("show").catch(error=>setError(String(error)));}}>{t("quickAssistant.open")}</Button>}
    </div>
    {!draft&&!error&&<p role="status">{t(quick?"quickAssistant.loading":"selection.loading")}</p>}
    {error&&<Notice actions={!dirty&&<Button disabled={busy} onClick={()=>void load()}>{t("common.retry")}</Button>}>{error}</Notice>}
    <TabsContent value="general">
      {disabled&&<p className="muted">{t("module.disabledTitle",{name:t("nav."+moduleId)})}</p>}
      {draft&&<fieldset className="plain-fieldset settings-section-stack" disabled={busy}>
        {quick?<SettingsSection icon={AppWindow} title={t("settingsDesign.windowBehavior")}>
          <SettingsToggle id="quick-always-on-top" label={t("quickAssistant.alwaysOnTop")} checked={draft.alwaysOnTop===true} onChange={alwaysOnTop=>setDraft({...draft,alwaysOnTop})}/>
          <SettingsToggle id="quick-show-reasoning" label={t("quickAssistant.showReasoning")} checked={draft.showReasoning!==false} onChange={showReasoning=>setDraft({...draft,showReasoning})}/>
        </SettingsSection>:<>
          <SettingsSection icon={MousePointerClick} title={t("settingsDesign.selectionTriggers")}>
            <SettingsToggle id="selection-automatic" label={t("selection.automatic")} checked={draft.automatic} onChange={automatic=>setDraft({...draft,automatic})}/>
            <div className="settings-dependent"><SettingsToggle id="selection-keyboard" label={t("selection.keyboard")} checked={draft.keyboard} disabled={!draft.automatic} description={!draft.automatic?t("settingsDesign.selectionKeyboardRequires"):undefined} onChange={keyboard=>setDraft({...draft,keyboard})}/></div>
          </SettingsSection>
          <SettingsSection icon={Ban} title={t("selection.excluded")}>
            <Textarea {...exclusionDrop} id="selection-exclusions" aria-label={t("selection.excluded")} aria-describedby="selection-exclusions-help" value={excluded} onChange={event=>setExcluded(event.target.value)} rows={5}/>
            <p id="selection-exclusions-help" className="settings-help">{t("selection.excludedHint")}</p>
          </SettingsSection>
        </>}
        {saveActions}
        {notice&&!dirty&&<p role="status">{notice}</p>}
      </fieldset>}
    </TabsContent>
    <TabsContent value="toolbar">
      {draft&&<fieldset className="plain-fieldset settings-stack" disabled={busy}>
        <div className="settings-group-heading"><h3 className="settings-group-title">{t("settingsDesign.toolbarActions")}</h3><p className="settings-group-description">{t("selection.toolbarHint")}</p></div>
        {aiError&&<Notice actions={<Button onClick={()=>void loadAI()}>{t("common.retry")}</Button>}>{aiError}</Notice>}
        {/* One editor at a time keeps names scannable. Drafts live above the
            accordion so collapse, sorting and tab changes never discard edits. */}
        <Accordion.Root className="selection-actions" value={expanded} onValueChange={setExpanded} multiple={false}>
          <div className="selection-actions-heading" aria-hidden="true"><span>{t("settingsDesign.action")}</span><span>{t("selection.position")}</span><span>{t("selection.show")}</span></div>
          {actions.map((action,index)=>{
            const name=i18n.language==="en-US"?(action.englishName||action.name):action.name;
            const Icon=actionIcons[action.icon as keyof typeof actionIcons]??Sparkles;
            return <Accordion.Item key={action.id} value={action.id} className="selection-action">
              <div className="selection-action-row">
                <Accordion.Header className="selection-action-heading"><Accordion.Trigger data-slot="action-trigger" className="selection-action-trigger" aria-label={t("selection.editAction",{name:name||t("selection.newAction")})} disabled={busy}>
                  <Icon className="size-4 shrink-0" aria-hidden="true"/><span>{name||t("selection.newAction")}</span><ChevronDown className="selection-action-chevron size-4" aria-hidden="true"/>
                </Accordion.Trigger></Accordion.Header>
                <div className="selection-action-controls">
                  <CaptureOptionSelect label={t("selection.position")} value={index} items={actions.map((_,i)=>({value:i,label:String(i+1)}))} disabled={busy} onChange={to=>moveAction(index,to)}/>
                  <div className="selection-action-visibility"><Label htmlFor={`selection-enabled-${action.id}`}>{t("selection.show")}</Label><Switch id={`selection-enabled-${action.id}`} aria-label={t("selection.showAction",{name})} checked={action.enabled} onCheckedChange={enabled=>changeAction(index,{enabled})}/></div>
                </div>
              </div>
              <Accordion.Panel className="selection-action-editor">
                <div className="settings-field-grid">
                  <div className="space-y-2"><Label htmlFor={`selection-action-${action.id}`}>{t("selection.actionName")}</Label><Input id={`selection-action-${action.id}`} value={name} onChange={event=>changeAction(index,i18n.language==="en-US"?{englishName:event.target.value}:{name:event.target.value})}/></div>
                  <CaptureOptionSelect stacked label={t("selection.icon")} value={action.icon} items={Object.entries(actionIcons).filter(([key])=>!quick||key!=="copy").map(([value,ActionIcon])=>({value,label:t(`selection.icons.${value}`),icon:<ActionIcon className="size-4"/>}))} disabled={busy} onChange={icon=>changeAction(index,{icon})}/>
                </div>
                {action.kind!=="copy"&&ai&&<AIModelSelect label={name} providers={ai.providers} value={action.selection?.provider?action.selection:undefined} disabled={busy} inheritLabel={t("ai.followDefaultService")} onChange={selection=>changeAction(index,{selection})}/>}
                {action.kind!=="copy"&&<div className="space-y-2"><Label htmlFor={`selection-prompt-${action.id}`}>{t("selection.prompt")}</Label><Textarea id={`selection-prompt-${action.id}`} rows={3} value={action.prompt??""} onChange={event=>changeAction(index,{prompt:event.target.value})}/></div>}
                {action.id.startsWith("custom-")&&<Button icon={Trash2} onClick={()=>{setDraft({...draft,actions:actions.filter((_,i)=>i!==index)});setExpanded([]);}}>{t("selection.remove")}</Button>}
              </Accordion.Panel>
            </Accordion.Item>;
          })}
        </Accordion.Root>
        <Button className="selection-add-action" icon={Plus} disabled={actions.length>=24} onClick={()=>{const id=`custom-${crypto.randomUUID()}`;pendingFocus.current=id;setDraft({...draft,actions:[...actions,{id,name:t("selection.newAction"),icon:"polish",kind:"ai",prompt:"",enabled:true}]});setExpanded([id]);}}>{t("selection.add")}</Button>
        {quick&&<SettingsHelp title={t("settingsDesign.quickToolbarHelp")}><ul><li>{t("settingsDesign.quickToolbarUse")}</li><li>{t("settingsDesign.quickToolbarDetails")}</li></ul></SettingsHelp>}
        {saveActions}
        {notice&&!dirty&&<p role="status">{notice}</p>}
      </fieldset>}
    </TabsContent>
    
    <TabsContent value="shortcuts" className="settings-section-stack"><Shortcuts moduleId={moduleId} onSaved={()=>onSaved?.()} onActivity={onActivity}/></TabsContent>
  </Tabs>;
}
