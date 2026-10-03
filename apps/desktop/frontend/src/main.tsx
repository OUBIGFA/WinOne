import {Notice} from "./ui/notice";
import React, { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  getSnapshot,
  saveSettings,
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
import { AlwaysOnTopPage } from "./features/always-on-top";
import { Home, ScanLine, Pin, PanelsTopLeft, Settings as SettingsIcon } from "lucide-react";
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
import {StartupSettings,type StartupPreferences} from "./components/startup-settings";
import {PermissionSettings} from "./components/permission-settings";
import {useSettingsNavigation} from "./ui/settings-navigation";

const pageDefinitions = [
  { id: "overview", title: "nav.overview", icon: Home },
  { id: "capture", title: "nav.capture", icon: ScanLine },
  { id: "always-on-top", title: "nav.always-on-top", icon: Pin },
  { id: "wintray", title: "nav.wintray", icon: PanelsTopLeft },
  { id: "settings", title: "nav.settings", icon: SettingsIcon },
];

function App() {
  const {t}=useTranslation();
  const pages=pageDefinitions.map(page=>({...page,title:t(page.title)}));
  const [page, setPage] = useState("overview");
  const navigation=usePageNavigation(page,setPage,settingsClosePort);
  const [snapshot, setSnapshot] = useState<Snapshot>();
  const [homeReady,setHomeReady]=useState(false);
  const [windowReady,setWindowReady]=useState(false);
  const [error, setError] = useState("");
  useSettingsNavigation(settingsNavigationPort,navigation.requestPage,setError);
  const [busy, setBusy] = useState(false);
  const [permissionRestarting,setPermissionRestarting]=useState(false);
  const [permissionError,setPermissionError]=useState("");
  usePageActivity(navigation.onActivity,false,busy,"global-settings");
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
    setBusy(true);
    setError("");
    try {
      setSnapshot(await getSnapshot());

    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
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
    try{const settings=await saveSettings({...snapshot.settings,startup});setSnapshot({...snapshot,settings});setNotice("common.saved");}
    catch(error){setError(error instanceof Error?error.message:String(error));}
    finally{setBusy(false);}
  }
  async function changeTheme(theme: Settings["theme"]) {
    if (!snapshot) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const settings = await saveSettings({ ...snapshot.settings, theme });
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
    try {await saveSettings({...snapshot.settings,language});setSnapshot(await getSnapshot());setNotice("common.saved");}
    catch(error){setError(String(error));}
    finally{setBusy(false);}
  }
  const selected = pages.find((p) => p.id === page)!;
  const module = snapshot?.modules.find((m) => m.id === page);
  return (
    <AppShell page={page} pages={pages} logo={logo} onNavigate={id=>{navigation.requestPage(id);setNotice("");}} windowPort={windowFramePort} onWindowReady={()=>setWindowReady(true)}>

        <header>
          <div>
            <h1>{selected.title}</h1>
          </div>
          <Button
            busy={busy}
            onClick={() => void refresh()}
          >
            {t("common.refresh")}
          </Button>
        </header>
        {error && (
          <Notice title={t("common.failed")} actions={<>
            <Button disabled={busy} onClick={() => void refresh()}>
              {t("common.reconnect")}
            </Button>
          </>}>{error}</Notice>
        )}
        {snapshot?.runtimeWarning&&<Notice tone="info">{snapshot.runtimeWarning}</Notice>}
        {page === "overview" && <ModulesPage onChanged={() => void refresh()} onActivity={navigation.onActivity} onReady={()=>setHomeReady(true)} startup={snapshot?.settings.startup} administrator={snapshot?.administrator}/>}
        {page === "settings" && (
          <SettingsPanel><fieldset className="plain-fieldset" disabled={busy}><section>
            <h2>{t("shell.appearance")}</h2>
            <AppearanceSettings theme={snapshot?.settings.theme ?? "system"} language={snapshot?.settings.language || "zh-CN"} disabled={busy || !snapshot} onThemeChange={value=>void changeTheme(value)} onLanguageChange={value=>void changeLanguage(value)}/>
            <StartupSettings value={snapshot?.settings.startup??{runAtLogon:false,startMinimized:true}} disabled={busy||!snapshot} onChange={value=>void changeStartup(value)}/>
            <PermissionSettings administrator={snapshot?.administrator} remembered={!!snapshot?.settings.startup?.preferAdministrator} disabled={busy||!snapshot||navigation.activity.dirty||navigation.activity.busy} restarting={permissionRestarting} onRemember={value=>void changeAdministratorPreference(value)} onRestart={()=>void restartAdministrator()} error={permissionError}/>
            <p className="saved" role="status">
              {notice?t(notice):""}
            </p>
            <Profiles onApplied={()=>void refresh()} onActivity={navigation.onActivity} />
            <h2>{t("shell.background")}</h2>
            <div className="setting-row">
              <div>
                <span>{t("shell.connection")}</span>
                <p>{t("shell.backgroundHint")}</p>
              </div>
              <span>{snapshot && !error ? t("shell.connected") : t("shell.disconnected")}</span>
            </div>
          </section></fieldset></SettingsPanel>
        )}
        <>
          {page === "wintray" && <WinTrayPage disabled={module?.state === "disabled"} onChanged={()=>void refresh()} onActivity={navigation.onActivity}/>}
          {page === "capture" && <ModulePanel moduleId="capture" onSaved={()=>void refresh()} onActivity={navigation.onActivity}><CapturePage disabled={module?.state === "disabled"} onEdit={editCapture} onOpenDraft={openCapturedDraft} refreshKey={captureRefresh} onActivity={navigation.onActivity}/></ModulePanel>}
          {page === "always-on-top" && <ModulePanel moduleId="always-on-top" onSaved={()=>void refresh()} onActivity={navigation.onActivity}><AlwaysOnTopPage disabled={module?.state === "disabled"} onActivity={navigation.onActivity}/></ModulePanel>}
        </>
        {page !== "overview" &&
          page !== "settings" &&
          page !== "wintray" &&
          page !== "capture" && page !== "always-on-top" && page !== "modules" && (
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
