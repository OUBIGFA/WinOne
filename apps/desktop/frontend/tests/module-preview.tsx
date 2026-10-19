import {i18n,setApplicationLanguage} from "../src/i18n";
import {SettingsFixtureShell} from "./settings-fixture-shell";
// This fixture uses the real panel components with synthetic, in-memory data.
import {createRoot} from "react-dom/client";
import {ModulePanel} from "../src/components/module-panel";
import {AlwaysOnTopPage} from "../src/features/always-on-top";
import "../src/styles/shadcn.css";
import "../src/style.css";
let preferences={schemaVersion:1,revision:0,frame:true,frameThickness:3,frameColor:"#0078d4",frameOpacity:90,useAccent:true,roundCorners:true,blockInGameMode:true,sound:false,excludedApps:[]};
let settings={schemaVersion:1,revision:0,theme:"system",language:"zh-CN",captureShortcut:"Shift+A",pinShortcut:"Alt+Q"};
type NativeApp=NonNullable<NonNullable<Window["go"]>["main"]>["App"];
window.go={main:{App:{
 BeginShortcutRecording:async()=>"preview-recording",
 EndShortcutRecording:async()=>{},
 Snapshot:async()=>({settings,modules:[]}),
 SaveSettings:async(value:typeof settings)=>{settings={...value,revision:settings.revision+1};return settings;},
 Shortcuts:async()=>JSON.stringify({bindings:[{id:"pin",shortcut:settings.pinShortcut,registered:!!settings.pinShortcut,error:""}],lastError:"",actionErrors:{}}),
 AlwaysOnTop:async(method:string,payload:string)=>{
 if(new URLSearchParams(location.search).has("failure")&&(method==="snapshot"||method==="settings.get"))throw Error(new URLSearchParams(location.search).get("error")||"Settings read failed");
  if(method==="settings.get")return JSON.stringify(preferences);
  if(method==="snapshot")return JSON.stringify({settings:preferences,administrator:false,windows:[]});
  if(method==="settings.save"){preferences={...JSON.parse(payload),revision:preferences.revision+1};return JSON.stringify(preferences);}
  throw Error("此验证页不执行系统操作");
 },
 ReadModuleLogs:async()=>({folderAvailable:false,entries:[{name:"process",available:false,truncated:false,text:""}]}),
 OpenModuleLogFolder:async()=>{},
} as unknown as NativeApp}};
await setApplicationLanguage(new URLSearchParams(location.search).get("language")||"zh-CN");
createRoot(document.getElementById("root")!).render(<SettingsFixtureShell page="always-on-top"><header><h1>{i18n.t("nav.always-on-top")}</h1></header><ModulePanel moduleId="always-on-top"><AlwaysOnTopPage/></ModulePanel></SettingsFixtureShell>);
