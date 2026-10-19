import {createRoot} from "react-dom/client";
import {setApplicationLanguage,i18n} from "../src/i18n";
import {SelectionPage} from "../src/features/selection";
import {SettingsFixtureShell} from "./settings-fixture-shell";
import {ModulePanel} from "../src/components/module-panel";
import "../src/styles/shadcn.css";
import "../src/style.css";
const query=new URLSearchParams(location.search),language=query.get("language")||"zh-CN",theme=query.get("theme")||"light";
document.documentElement.dataset.theme=theme;
const moduleId=query.get("assistant")==="quick"?"quick-assistant":"selection";
let preferences={schemaVersion:1,revision:1,automatic:true,keyboard:true,excludedApps:["notepad.exe"],actions:[
  {id:"copy",name:"复制",englishName:"Copy",icon:"copy",kind:"copy",enabled:true},
  {id:"translate",name:"翻译",englishName:"Translate",icon:"translate",kind:"translate",enabled:true},
  {id:"explain",name:"解释",englishName:"Explain",icon:"explain",kind:"ai",prompt:"解释选中文字，不要重复原文。",enabled:true},
  {id:"summarize",name:"总结",englishName:"Summarize",icon:"summarize",kind:"ai",prompt:"总结选中文字。",enabled:true},
  {id:"polish",name:"润色",englishName:"Polish",icon:"polish",kind:"ai",prompt:"润色选中文字。",enabled:true},
  {id:"code",name:"代码",englishName:"Code",icon:"code",kind:"ai",prompt:"帮助处理选中的代码或编程问题。",enabled:true},
]};
window.go={main:{App:{
  Snapshot:async()=>({settings:{schemaVersion:1,revision:1,theme,language,captureShortcut:"",pinShortcut:"",selectionShortcut:"Alt+Shift+S"},modules:[]}),
  Shortcuts:async()=>JSON.stringify({bindings:[{id:"selection",shortcut:"Alt+Shift+S",registered:true}]}),
  Selection:async(method:string,payload:string)=>{if(method==="settings.get")return JSON.stringify(preferences);if(method==="settings.save"){preferences={...JSON.parse(payload),revision:preferences.revision+1};return JSON.stringify(preferences);}throw Error("No system actions in preview");},
  QuickAssistant:async(method:string,payload:string)=>{if(method==="settings.get")return JSON.stringify({...preferences,actions:preferences.actions.filter(action=>action.kind!=="copy")});if(method==="settings.save"){preferences={...JSON.parse(payload),revision:preferences.revision+1};return JSON.stringify(preferences);}if(method==="show")return JSON.stringify({shown:true});throw Error("No system actions in preview");},
  Capture:async(method:string)=>{if(method==="ai.configuration.get")return JSON.stringify({providers:[{id:"test-provider",name:"示例服务",icon:"openai",models:["gpt-5","claude-sonnet-4"]}],tasks:{}});throw Error("No system actions in preview");},
  ReadModuleLogs:async()=>({folderAvailable:false,entries:[]}),
}}} as unknown as NonNullable<Window["go"]>;
await setApplicationLanguage(language);
createRoot(document.getElementById("root")!).render(<SettingsFixtureShell page={moduleId}><header><h1>{i18n.t("nav."+moduleId)}</h1></header><ModulePanel moduleId={moduleId}><SelectionPage moduleId={moduleId}/></ModulePanel></SettingsFixtureShell>);
