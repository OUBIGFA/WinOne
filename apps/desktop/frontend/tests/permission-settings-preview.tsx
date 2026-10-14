// Layout fixture only. All controls change in-memory state; no UAC or startup
// task operations are performed by this page.
import {useState} from "react";
import {createRoot} from "react-dom/client";
import {Home,ScanLine,Pin,PanelsTopLeft,Settings} from "lucide-react";
import {AppShell} from "../src/components/app-shell";
import {SettingsPanel} from "../src/components/settings-panel";
import {AppearanceSettings} from "../src/components/appearance-settings";
import {StartupSettings} from "../src/components/startup-settings";
import {PermissionSettings} from "../src/components/permission-settings";
import {Profiles} from "../src/features/profiles";
import {setApplicationLanguage,useTranslation} from "../src/i18n";
import "../src/styles/shadcn.css";
import "../src/style.css";
import logo from "../../../../internal/branding/logo.png";

const query=new URLSearchParams(location.search);
window.go={main:{App:{UpdateInfo:async()=>({version:"0.0.1",configured:false}),ReadModuleLogs:async()=>({folderAvailable:false,entries:[]})}}} as unknown as NonNullable<Window["go"]>;
function Preview(){
 const {t}=useTranslation();const [theme,setTheme]=useState<"light"|"dark"|"system">(matchMedia("(prefers-color-scheme: dark)").matches?"dark":"light");const [language,setLanguage]=useState<"system"|"zh-CN"|"en-US">(query.get("language")==="en-US"?"en-US":"zh-CN");
 const [startup,setStartup]=useState({runAtLogon:true,startMinimized:true});const [remember,setRemember]=useState(false);
 const pages=[{id:"overview",title:t("nav.overview"),icon:Home},{id:"capture",title:t("nav.capture"),icon:ScanLine},{id:"always-on-top",title:t("nav.always-on-top"),icon:Pin},{id:"wintray",title:t("nav.wintray"),icon:PanelsTopLeft},{id:"settings",title:t("nav.settings"),icon:Settings}];
 return <AppShell page="settings" pages={pages} logo={logo} onNavigate={()=>{}}><header><h1>{t("nav.settings")}</h1></header><SettingsPanel><div className="settings-sections"><section>
  <h2>{t("shell.appearance")}</h2><AppearanceSettings theme={theme} language={language} disabled={false} onThemeChange={value=>{setTheme(value);document.documentElement.dataset.theme=value;}} onLanguageChange={value=>{setLanguage(value);void setApplicationLanguage(value);}}/>
  </section><StartupSettings value={startup} disabled={false} onChange={setStartup}/>
  <PermissionSettings administrator={query.get("administrator")==="1"} remembered={remember} disabled={false} restarting={false} onRemember={setRemember} onRestart={()=>{}}/>
  <Profiles/><section><h2>{t("shell.background")}</h2><div className="setting-row"><div><span>{t("shell.connection")}</span><p>{t("shell.backgroundHint")}</p></div><span>{t("shell.connected")}</span></div>
 </section></div></SettingsPanel></AppShell>;
}
void setApplicationLanguage(query.get("language")==="en-US"?"en-US":"zh-CN").then(()=>createRoot(document.getElementById("root")!).render(<Preview/>));
