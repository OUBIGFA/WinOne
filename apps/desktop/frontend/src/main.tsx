import {Notice} from "./ui/notice";
import React, { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  getSnapshot,
  savePreferenceChange,
  setAdministratorPreference,restartAsAdministrator,
  type Snapshot,
  type Settings,
  captureCall,openCaptureWindow,
  startupProbePort,
  settingsClosePort,
  windowFramePort,
  settingsNavigationPort,
} from "./bridge";
import "./styles/shadcn.css";
import "./style.css";
import logo from "../../../../internal/branding/logo.png";
import { WinTrayPage } from "./features/wintray";
import { CapturePage } from "./features/capture";
import {ConveniencePage} from "./features/convenience";
import {SelectionPage} from "./features/selection";
import {AISettings} from "./features/ai-settings";
import {ChatPage} from "./features/chat";
import {createAccountReader} from "./features/ai-account-reader";
import { Home, ScanLine, Rocket, LaptopMinimal, Settings as SettingsIcon,RefreshCw,Palette,Activity,MousePointer2,Sparkles,Bot } from "lucide-react";
import {QuickAssistantIcon} from "./components/quick-assistant-icon";
import {moduleRank} from "./module-order";
import {IconLabel} from "./ui/icon-label";
import { Button } from "./ui/button";
import { ModulesPage } from "./features/modules";
import { Profiles } from "./features/profiles";
import { setApplicationLanguage, useTranslation } from "./i18n";
import {runCaptureJob,type CaptureDraft} from "./features/capture-draft-api";
import {finishStartupProbe} from "./startup-probe";
import {usePageNavigation,usePageActivity} from "./ui/page-navigation";
import {AppShell} from "./components/app-shell";
import {ModulePanel} from "./components/module-panel";
import {SettingsPanel} from "./components/settings-panel";
import {AppearanceSettings} from "./components/appearance-settings";
import {DefaultPageSetting} from './components/default-page-setting';
import {useDirectoryLinkIntents,type DirectoryLinkIntent} from './features/directory-link-intents';
import {StartupSettings,type StartupPreferences} from "./components/startup-settings";
import {Shortcuts} from "./features/shortcuts";
import {PermissionSettings} from "./components/permission-settings";
import {useSettingsNavigation} from "./ui/settings-navigation";

const pageDefinitions = [
  { id: "overview", title: "nav.overview", icon: Home },
  { id: "capture", title: "nav.capture", icon: ScanLine },
  { id: "selection", title: "nav.selection", icon: MousePointer2 },
  { id: "quick-assistant", title: "nav.quick-assistant", icon: QuickAssistantIcon },
  { id: "convenience", title: "nav.convenience", icon: Rocket },
  { id: "wintray", title: "nav.wintray", icon: LaptopMinimal },
  { id: "ai-services", title: "ai.serviceTitle", icon: Sparkles },
  { id: "settings", title: "nav.settings", icon: SettingsIcon },
];

function App() {
  const {t}=useTranslation();
  const pages=[pageDefinitions[0],...pageDefinitions.slice(1,-2).sort((a,b)=>moduleRank(a.id)-moduleRank(b.id)),{id:"chat",title:"chat.title",icon:Bot},...pageDefinitions.slice(-2)].map(page=>({...page,title:t(page.title)}));
  const [page, setPage] = useState("overview");
  const navigation=usePageNavigation(page,setPage,settingsClosePort);
  const [snapshot, setSnapshot] = useState<Snapshot>();
  const [homeReady,setHomeReady]=useState(false);
  const [windowReady,setWindowReady]=useState(false);
  const [formEpoch,setFormEpoch]=useState(0);
  // Explicit discard resets page drafts while retaining the native window and
  // IPC document. Ordinary background closes never rebuild these components.
  useEffect(()=>settingsClosePort.subscribeDiscard(()=>setFormEpoch(value=>value+1)),[]);
  const [error, setError] = useState("");
  const defaultPage=pages.some(page=>page.id===snapshot?.settings.defaultPage)?snapshot!.settings.defaultPage!:"overview";
  const initialPageApplied=useRef(false);
  const [linkIntent,setLinkIntent]=useState<DirectoryLinkIntent>();
  const [requestedConversationId,setRequestedConversationId]=useState<string>();
  const linkIntentPending=useRef(false);
  useDirectoryLinkIntents(!!snapshot,intent=>{initialPageApplied.current=true;navigation.requestPage('convenience',()=>{linkIntentPending.current=true;setLinkIntent(current=>({...intent,paths:[...(current?.paths??[]),...intent.paths]}));});},setError);
  useEffect(()=>{
    const open=(event:Event)=>{const id=(event as CustomEvent<{id?:unknown}>).detail?.id;if(typeof id!=="string"||!id||id.length>128)return;navigation.requestPage("chat",()=>setRequestedConversationId(id));};
    window.addEventListener("winone-open-chat",open);return()=>window.removeEventListener("winone-open-chat",open);
  },[navigation.requestPage]);
  useSettingsNavigation(settingsNavigationPort,page=>navigation.requestPage(linkIntentPending.current?'convenience':page),setError,defaultPage,!!snapshot);
  useEffect(()=>{if(snapshot&&!initialPageApplied.current){initialPageApplied.current=true;navigation.requestPage(defaultPage);}},[snapshot,defaultPage]);
  const [working, setBusy] = useState(false);
  const [loading,setLoading]=useState(false);
  const busy=working||loading;
  // Public account summaries live only as long as this settings window, so
  // returning to AI services does not restart discovery for every click.
  const [accountReader]=useState(createAccountReader);
  const [permissionRestarting,setPermissionRestarting]=useState(false);
  const [permissionError,setPermissionError]=useState("");
  usePageActivity(navigation.onActivity,false,working,"global-settings");
  const [notice, setNotice] = useState("");
  const opening=useRef(false);
  const [captureRefresh,setCaptureRefresh]=useState(0);
  async function editCapture(id:string) {
    if(opening.current)throw Error(t("capture.draftOpening"));
    opening.current=true;
    try {
      const existing=await captureCall<{draft:CaptureDraft|null}>("draft.get");
      let draftId=existing.draft?.id;
      if(!draftId) {
        const job=await runCaptureJob<{draftId:string}>("draft.begin",{expectedId:id});
        draftId=job.result?.draftId;
      }
      if(!draftId)throw Error(t("capture.draftUnavailable"));
      await openCaptureWindow(draftId);
    } finally {opening.current=false;setCaptureRefresh(value=>value+1);}
  }
  async function openCapturedDraft(id:string) {
    if(opening.current)throw Error(t("capture.draftOpening"));
    opening.current=true;
    try {await openCaptureWindow(id);}
    finally {opening.current=false;setCaptureRefresh(value=>value+1);}
  }
  async function refresh() {
    setLoading(true);
    setError("");
    try {
      setSnapshot(await getSnapshot());

    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    void refresh();
  }, []);
  useEffect(()=>{void setApplicationLanguage(snapshot?.settings.language||"zh-CN",snapshot?.locale);},[snapshot?.settings.language,snapshot?.locale]);
  useEffect(()=>{if(snapshot&&windowReady&&(page!=="overview"||homeReady))void finishStartupProbe(startupProbePort,document).catch(error=>setError(String(error)));},[snapshot,page,homeReady,windowReady]);
  useEffect(() => {
    const query = matchMedia("(prefers-color-scheme: dark)");
    const apply = () => {
      document.documentElement.dataset.theme =
        snapshot?.settings.theme === "system" || !snapshot
          ? query.matches
            ? "dark"
            : "light"
          : snapshot.settings.theme;
    };
    apply();
    query.addEventListener("change", apply);
    return () => query.removeEventListener("change", apply);
  }, [snapshot?.settings.theme]);
  async function changeAdministratorPreference(enabled:boolean){
    if(!snapshot||busy||navigation.activity.dirty||navigation.activity.busy)return;
    setBusy(true);setError("");setPermissionError("");setNotice("");
    try{const settings=await setAdministratorPreference(enabled);setSnapshot({...snapshot,settings});setNotice("common.saved");}
    catch(error){setPermissionError(error instanceof Error?error.message:String(error));}
    finally{setBusy(false);}
  }
  async function restartAdministrator(){
    if(!snapshot||busy||navigation.activity.dirty||navigation.activity.busy)return;
    setBusy(true);setPermissionRestarting(true);setError("");setPermissionError("");setNotice("");
    try{const closing=await restartAsAdministrator();if(!closing){setPermissionRestarting(false);setBusy(false);await refresh();}}
    catch(error){setPermissionRestarting(false);setBusy(false);setPermissionError(error instanceof Error?error.message:String(error));}
  }
  async function changeStartup(startup:StartupPreferences){
    if(!snapshot||busy)return;
    setBusy(true);setError("");setNotice("");
    try{const settings=await savePreferenceChange({startup:Object.fromEntries(Object.entries(startup).filter(([key,value])=>value!==snapshot.settings.startup?.[key as keyof StartupPreferences]))});setSnapshot({...snapshot,settings});setNotice("common.saved");}
    catch(error){setError(error instanceof Error?error.message:String(error));}
    finally{setBusy(false);}
  }
  async function changeTheme(theme: Settings["theme"]) {
    if (!snapshot) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const settings = await savePreferenceChange({theme});
      setSnapshot({ ...snapshot, settings });
      setNotice("common.saved");
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  async function changeLanguage(language:NonNullable<Settings["language"]>) {
    if(!snapshot)return;
    setBusy(true);setError("");setNotice("");
    try {await savePreferenceChange({language});setSnapshot(await getSnapshot());setNotice("common.saved");}
    catch(error){setError(String(error));}
    finally{setBusy(false);}
  }
  async function changeDefaultPage(defaultPage:string){
    if(!snapshot||busy)return;setBusy(true);setError("");setNotice("");
    try{const settings=await savePreferenceChange({defaultPage});setSnapshot({...snapshot,settings});setNotice("common.saved");}
    catch(error){setError(String(error));}finally{setBusy(false);}
  }
  const selected = pages.find((p) => p.id === page)!;
  const module = snapshot?.modules.find((m) => m.id === page);
  return (
    <AppShell key={formEpoch} page={page} pages={pages} logo={logo} onNavigate={id=>{navigation.requestPage(id);setNotice("");}} windowPort={windowFramePort} onWindowReady={()=>setWindowReady(true)}>

        {page!=="chat"&&<header className="page-actions">
          <Button
            icon={RefreshCw}
            busy={busy}
            onClick={() => void refresh()}
          >
            {t("common.refresh")}
          </Button>
        </header>}
        {error && (
          <Notice title={t("common.failed")} actions={<>
            <Button disabled={busy} onClick={() => void refresh()}>
              {t("common.reconnect")}
            </Button>
          </>}>{error}</Notice>
        )}
        {snapshot?.runtimeWarning&&<Notice tone="info">{snapshot.runtimeWarning}</Notice>}
        {page === "overview" && <ModulesPage onChanged={() => void refresh()} onActivity={navigation.onActivity} onReady={()=>setHomeReady(true)}/>}
        {page === "settings" && (
          <SettingsPanel onActivity={navigation.onActivity}><fieldset className="plain-fieldset settings-sections" disabled={busy}><section>
            <h2><IconLabel icon={Palette}>{t("shell.appearance")}</IconLabel></h2>
            <AppearanceSettings theme={snapshot?.settings.theme ?? "system"} language={snapshot?.settings.language || "zh-CN"} disabled={busy || !snapshot} onThemeChange={value=>void changeTheme(value)} onLanguageChange={value=>void changeLanguage(value)}/>
            <DefaultPageSetting value={defaultPage} pages={pages} disabled={busy||!snapshot} onChange={value=>void changeDefaultPage(value)}/>
            </section>
            <StartupSettings value={snapshot?.settings.startup??{runAtLogon:false,startMinimized:true}} disabled={busy||!snapshot} onChange={value=>void changeStartup(value)}/>
            <section><h2>{t("shortcuts.title")}</h2><Shortcuts moduleId="application" onSaved={()=>void refresh()} onActivity={navigation.onActivity}/></section>
            <PermissionSettings administrator={snapshot?.administrator} remembered={!!snapshot?.settings.startup?.preferAdministrator} disabled={busy||!snapshot||navigation.activity.dirty||navigation.activity.busy} restarting={permissionRestarting} onRemember={value=>void changeAdministratorPreference(value)} onRestart={()=>void restartAdministrator()} error={permissionError}/>
            <p className="saved" role="status">
              {notice?t(notice):""}
            </p>
            <Profiles onApplied={()=>void refresh()} onActivity={navigation.onActivity} />
            <section>
            <h2><IconLabel icon={Activity}>{t("shell.background")}</IconLabel></h2>
            <div className="setting-row">
              <div>
                <span>{t("shell.connection")}</span>
                <p>{t("shell.backgroundHint")}</p>
              </div>
              <span>{snapshot && !error ? t("shell.connected") : t("shell.disconnected")}</span>
            </div>
          </section></fieldset></SettingsPanel>
        )}
        {page === "ai-services" && <AISettings onActivity={navigation.onActivity} accountReader={accountReader}/>}
        {page === "chat" && <ChatPage onActivity={navigation.onActivity} onOpenAI={()=>navigation.requestPage("ai-services")} requestedConversationId={requestedConversationId} onRequestedConversationHandled={()=>setRequestedConversationId(undefined)}/>}
        <>
          {page === "wintray" && <WinTrayPage disabled={module?.state === "disabled"} onChanged={()=>void refresh()} onActivity={navigation.onActivity}/>}
          {page === "capture" && <ModulePanel moduleId="capture" onSaved={()=>void refresh()} onActivity={navigation.onActivity}><CapturePage disabled={module?.state === "disabled"} onEdit={editCapture} onOpenDraft={openCapturedDraft} refreshKey={captureRefresh} onActivity={navigation.onActivity}/></ModulePanel>}
          {page === "convenience" && <ConveniencePage disabled={snapshot?.modules.find(value=>value.id==="always-on-top")?.state === "disabled"} strokeDisabled={snapshot?.modules.find(value=>value.id==="stroke")?.state === "disabled"} actionsDisabled={busy} onChanged={()=>void refresh()} onActivity={navigation.onActivity} linkIntent={linkIntent} onLinkIntentConsumed={()=>{linkIntentPending.current=false;setLinkIntent(undefined);}}/>}
          {page === "quick-assistant" && <ModulePanel moduleId="quick-assistant" onSaved={()=>void refresh()} onActivity={navigation.onActivity}><SelectionPage key="quick-assistant" moduleId="quick-assistant" disabled={module?.state === "disabled"} onActivity={navigation.onActivity}/></ModulePanel>}
          {page === "selection" && <ModulePanel moduleId="selection" onSaved={()=>void refresh()} onActivity={navigation.onActivity}><SelectionPage disabled={module?.state === "disabled"} onActivity={navigation.onActivity}/></ModulePanel>}
        </>
        {page !== "overview" &&
          page !== "settings" &&
          page !== "wintray" &&
          page !== "capture" && page !== "quick-assistant" && page !== "selection" && page !== "ai-services" && page !== "chat" && page !== "convenience" && page !== "modules" && (
            <section className="empty">
              <span className="empty-icon" aria-hidden>
                <selected.icon size={28} strokeWidth={1.75} />
              </span>
              <h2>{module?.detail ?? t("shell.unavailable")}</h2>
              <p>{t("shell.unavailableHint")}</p>
              <Button variant="secondary" onClick={() => navigation.requestPage("overview")}>
                {t("common.back")}
              </Button>
            </section>
          )}

      {navigation.dialog}
    </AppShell>
  );
}

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
