import {i18n,setApplicationLanguage} from "../src/i18n";
// In-memory fixture: no user programs or startup settings are changed.
import {createRoot} from "react-dom/client";
import {WinTrayPage} from "../src/features/wintray";

import "../src/styles/shadcn.css";
import "../src/style.css";
let settings={schemaVersion:3,language:"zh-CN",runAtLogon:true,startMinimizedToTray:true,exitAfterManagedAppsCompleted:false,closeWindowRetrySeconds:10,startupIntervalSeconds:3,managedApps:["浏览器","音乐播放器","文件同步"].map((name,i)=>({id:String(i),name,exePath:`C:\\Apps\\example${i}.exe`,args:"",runOnStartup:i!==1,collectTrayIcon:i===2,launchViaLogonTask:false,launchHiddenInBackground:false,trayBehavior:{autoMinimizeAndHideOnLaunch:true,closeDelaySeconds:3},schedule:{enabled:i===0,startDelayMinutes:5,autoExitMinutes:30,frequencyEnabled:false,frequencyDays:1,frequencyRuns:1}}))};
let revision=1;
window.go={main:{App:{
 WinTray:async(method:string,payload:string)=>{
 if(new URLSearchParams(location.search).has("failure")&&(method==="snapshot"||method==="settings.get"))throw Error(new URLSearchParams(location.search).get("error")||"Settings read failed");
  if(method==="snapshot"||method==="settings.get")return JSON.stringify({revision:String(revision),settings});
  if(method==="settings.save"){settings=JSON.parse(payload).settings;revision++;return JSON.stringify({state:"completed"});}
  if(method==="icons")return JSON.stringify([{id:"sync-icon",name:"文件同步",exePath:"C:\\Apps\\example2.exe"}]);
  if(method==="icon.activate")return JSON.stringify({delivered:true});
  throw Error("此预览不执行系统操作");
 },
 PickProgram:async()=>"C:\\Apps\\replacement.exe",
 ProgramInfo:async(path:string)=>({name:new URLSearchParams(location.search).has("friendlyNames")?"Detected application":path.split(/[\\/]/).pop()!.replace(/\.[^.]+$/,"")}),
 PickPrograms:async()=>["C:\\Apps\\new-program.exe"],
 Modules:async(method:string)=>{
  if(method==="resetWinTray.preview")return JSON.stringify({configurationRevision:revision,moduleRevision:1,programCount:settings.managedApps.length});
  throw Error("此预览不执行系统操作");
 },
 Residency:async()=>JSON.stringify({requested:false,reasons:[]}),
 ReadModuleLogs:async()=>({folderAvailable:false,entries:[]}),
}}} as unknown as NonNullable<Window["go"]>;
void setApplicationLanguage(new URLSearchParams(location.search).get("language")||"zh-CN");
createRoot(document.getElementById("root")!).render(<main className="mx-auto max-w-6xl p-8"><header><h1>{i18n.t("nav.wintray")}</h1></header><WinTrayPage/></main>);
