// Isolated settings composition; no network requests or browser windows.
import {createRoot} from "react-dom/client";
import {SettingsPanel} from "../src/components/settings-panel";
import {setApplicationLanguage} from "../src/i18n";
import "../src/styles/shadcn.css";
import "../src/style.css";
const params=new URLSearchParams(location.search),scenario=params.get("case")||"available";
window.go={main:{App:{
 UpdateInfo:async()=>({version:"1.0.0",configured:scenario!=="unconfigured"}),
 CheckForUpdates:async()=>scenario==="failure"?{state:"failed",errorCode:"network"}:scenario==="current"?{state:"up_to_date"}:{state:"available",latestVersion:"1.1.0"},
 CancelUpdateCheck:async()=>{},OpenUpdateRelease:async()=>{document.getElementById("opened")!.textContent="yes";},OpenRepository:async()=>{},
 ReadModuleLogs:async()=>({folderAvailable:false,entries:[]})
}}} as unknown as NonNullable<Window["go"]>;
void setApplicationLanguage(params.get("language")||"zh-CN");
createRoot(document.getElementById("root")!).render(<main className="mx-auto max-w-4xl p-8"><SettingsPanel><></></SettingsPanel><output hidden id="opened"/></main>);
