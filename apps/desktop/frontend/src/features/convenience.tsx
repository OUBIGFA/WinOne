import {useCallback,useEffect,useRef,useState} from "react";
import {Pin,FolderSymlink,MousePointer2} from "lucide-react";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "../components/ui/tabs";
import {ModulePanel} from "../components/module-panel";
import {IconLabel} from "../ui/icon-label";
import type {PageActivityChanged} from "../ui/page-navigation";
import {useTranslation} from "../i18n";
import {AlwaysOnTopPage} from "./always-on-top";
import {ModulesPage} from "./modules";
import {DirectoryLinksPage} from './directory-links';
import {MouseGesturesSettings} from './mouse-gestures';
import {setFileDropTarget} from '../bridge';
import type {DirectoryLinkIntent} from './directory-link-intents';

// Group tools in navigation while retaining each tool's existing configuration,
// lifecycle and enable switch. Disabling must not discard an unfinished edit.
export function ConveniencePage({disabled,strokeDisabled=false,actionsDisabled=false,onChanged,onActivity,linkIntent,onLinkIntentConsumed}:{disabled:boolean;strokeDisabled?:boolean;actionsDisabled?:boolean;onChanged:()=>void;onActivity:PageActivityChanged;linkIntent?:DirectoryLinkIntent;onLinkIntentConsumed?:()=>void}){
 const {t}=useTranslation();
 const activity=useRef(new Map<string,boolean>());
 const [locked,setLocked]=useState(false);
 const [tab,setTab]=useState('always-on-top');
 useEffect(()=>{if(tab!=='directory-links')return;void setFileDropTarget(true).catch(console.error);return()=>{void setFileDropTarget(false).catch(console.error);};},[tab]);
 useEffect(()=>{if(linkIntent?.requested&&!locked)setTab('directory-links');},[linkIntent,locked]);
 useEffect(()=>{if(tab==='directory-links'&&linkIntent?.requested&&!locked){window.dispatchEvent(new CustomEvent('winone:directory-links-drop',{detail:linkIntent}));onLinkIntentConsumed?.();}},[tab,linkIntent,locked,onLinkIntentConsumed]);
 const report=useCallback<PageActivityChanged>((dirty,busy,source="page")=>{
  activity.current.set(source,dirty||busy);
  setLocked([...activity.current.values()].some(Boolean));
  onActivity(dirty,busy,source);
 },[onActivity]);
 return <Tabs value={tab} onValueChange={value=>{if(!locked)setTab(String(value));}}>
  <TabsList variant="default" aria-label={t("nav.convenience")}><TabsTrigger title={t("nav.always-on-top")} value="always-on-top"><IconLabel icon={Pin}>{t("nav.always-on-top")}</IconLabel></TabsTrigger><TabsTrigger title={t("stroke.title")} value="mouse-gestures"><IconLabel icon={MousePointer2}>{t("stroke.title")}</IconLabel></TabsTrigger><TabsTrigger title={t("links.title")} value="directory-links"><IconLabel icon={FolderSymlink}>{t("links.title")}</IconLabel></TabsTrigger></TabsList>
  <TabsContent value="always-on-top" className="settings-stack">
   <ModulesPage only="always-on-top" controlsDisabled={locked} onChanged={onChanged} onActivity={report}/>
   <ModulePanel moduleId="always-on-top"><AlwaysOnTopPage disabled={disabled} onActivity={report} onSaved={onChanged}/></ModulePanel>
  </TabsContent>
  <TabsContent value="directory-links" className="settings-stack"><DirectoryLinksPage disabled={actionsDisabled} onActivity={report}/></TabsContent>
  <TabsContent value="mouse-gestures" className="settings-stack"><ModulesPage only="stroke" controlsDisabled={locked} onChanged={onChanged} onActivity={report}/><ModulePanel moduleId="stroke"><MouseGesturesSettings disabled={strokeDisabled||actionsDisabled} onActivity={report}/></ModulePanel></TabsContent>
 </Tabs>;
}
