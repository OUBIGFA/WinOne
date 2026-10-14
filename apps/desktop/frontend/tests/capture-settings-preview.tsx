import {i18n,setApplicationLanguage} from "../src/i18n";
import {SettingsFixtureShell} from "./settings-fixture-shell";
// Visual fixture only. Validation and output parity use the shared native/Go fixtures.
import {createRoot} from "react-dom/client";
import {CapturePage} from "../src/features/capture";
import {ModulePanel} from "../src/components/module-panel";
import "../src/styles/shadcn.css";
import "../src/style.css";
let settings={schemaVersion:1,revision:0,outputDirectory:"C:\\Pictures\\WinOne",copyToClipboard:true,askWhereToSave:false,openEditor:true,pinAfterCapture:false,theme:"system",scrollDelay:300,scrollFromTop:false,fileNameTemplate:""};
let globalSettings={schemaVersion:1,revision:0,theme:"system",language:"zh-CN",captureShortcut:"Shift+A",pinShortcut:"Alt+Q"};
window.go={main:{App:{
 PickDirectory:async()=>{
  if(!new URLSearchParams(location.search).has("folderTest"))return "D:\\Pictures\\WinOne";
  document.documentElement.dataset.folderCalls=String(Number(document.documentElement.dataset.folderCalls||"0")+1);
  return new Promise<string>((resolve,reject)=>document.addEventListener("fixture:folder-result",event=>{const result=(event as CustomEvent<{path?:string;error?:string}>).detail;if(result.error)reject(Error(result.error));else resolve(result.path||"");},{once:true}));
 },
 Snapshot:async()=>({settings:globalSettings,modules:[]}),
 SaveSettings:async(value:typeof globalSettings)=>{globalSettings={...value,revision:globalSettings.revision+1};return globalSettings;},
 Shortcuts:async()=>JSON.stringify({bindings:[{id:"capture",shortcut:globalSettings.captureShortcut,registered:true}],lastError:""}),
 BeginShortcutRecording:async()=>"preview-recording",EndShortcutRecording:async()=>{},
 ReadModuleLogs:async()=>({folderAvailable:false,entries:[]}),OpenModuleLogFolder:async()=>{},
 Capture:async(method:string,payload:string)=>{
 if(method==="ai.configuration.get")return JSON.stringify({revision:1,engine:"google",ocrEngine:"local",target:"auto",prompt:"",providers:[],tasks:{}});
 if(new URLSearchParams(location.search).has("failure")&&(method==="snapshot"||method==="settings.get"))throw Error(new URLSearchParams(location.search).get("error")||"Settings read failed");
 if(method==="snapshot")return JSON.stringify({settings,busy:false,pins:0});
 if(method==="settings.get")return JSON.stringify(settings);
 if(method==="settings.save"){settings={...JSON.parse(payload),revision:settings.revision+1};return JSON.stringify(settings);}
 if(method==="filename.preview"){
  const {template}=JSON.parse(payload);
  let name=(template||"WinOne-{timestamp}-OK").replaceAll("{timestamp}","20261001090807").replaceAll("{date}","20261001").replaceAll("{time}","090807").replaceAll("{sequence}","01");
  if(name.includes("/")||name.includes(String.fromCharCode(92))||/[{}<>:"|?*]/.test(name))throw Error("文件名模板无效，请检查占位符、长度和特殊字符");
  if(!name.toLowerCase().endsWith(".png"))name+=".png";
  return JSON.stringify({name});
 }
 throw Error("此预览不执行系统操作");
}}}} as unknown as NonNullable<Window["go"]>;
await setApplicationLanguage(new URLSearchParams(location.search).get("language")||"zh-CN");
createRoot(document.getElementById("root")!).render(<SettingsFixtureShell page="capture"><header><h1>{i18n.t("nav.capture")}</h1></header><ModulePanel moduleId="capture" onSaved={()=>{}} onActivity={()=>{}}><CapturePage/></ModulePanel></SettingsFixtureShell>);
