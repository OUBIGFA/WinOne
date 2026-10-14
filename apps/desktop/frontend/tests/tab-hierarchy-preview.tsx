import {createRoot} from "react-dom/client";
import {ConveniencePage} from "../src/features/convenience";
import {setApplicationLanguage} from "../src/i18n";
import defaults from "../../../../modules/stroke/defaults.json";
import "../src/styles/shadcn.css";
import "../src/style.css";
let penPreferences={...structuredClone(defaults),revision:1};
const pinPreferences={schemaVersion:1,revision:1,frame:true,frameThickness:3,frameColor:"#0078d4",frameOpacity:90,useAccent:true,excludedApps:[]};
Object.assign(window,{go:{main:{App:{
 SetFileDropTarget:async()=>{},
 Snapshot:async()=>({settings:{schemaVersion:1,revision:1,pinShortcut:"Alt+Q"},modules:[]}),
 Shortcuts:async()=>JSON.stringify({bindings:[],actionErrors:{}}),
 Modules:async()=>JSON.stringify({revision:1,modules:[]}),
 AlwaysOnTop:async(method:string)=>JSON.stringify(method==="snapshot"?{settings:pinPreferences,windows:[]}:pinPreferences),
 Stroke:async(method:string,raw:string)=>{if(method==="settings.save")penPreferences={...JSON.parse(raw),revision:penPreferences.revision+1};return JSON.stringify(penPreferences);},
 DirectoryLinks:async(method:string)=>JSON.stringify(method==="list"?[]:method==="browse"?{path:"",entries:[]}:method==="preferences.get"?{revision:1,sourceDirectory:"",targetDirectory:""}:{requested:false}),
}}}});
const query=new URLSearchParams(location.search);
await setApplicationLanguage(query.get("language")||"zh-CN");
document.documentElement.classList.toggle("dark",query.get("theme")==="dark");
document.documentElement.dataset.theme=query.get("theme")||"light";
createRoot(document.getElementById("root")!).render(<main style={{padding:24}}><ConveniencePage disabled={false} onChanged={()=>{}} onActivity={()=>{}}/></main>);

