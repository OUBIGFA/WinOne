// In-memory import workflow. File dialogs and writes are never invoked.
import {createRoot} from "react-dom/client";
import {ProfileImport} from "../src/features/profile-import";
import {setApplicationLanguage} from "../src/i18n";
import "../src/styles/shadcn.css";
import "../src/style.css";
let picks=0;
window.go={main:{App:{
 PickProfile:async()=>++picks===1?"C:\\Backups\\Home.winone":"C:\\Backups\\Broken.winone",
 PickProfileLocation:async()=>"D:\\Pictures\\WinOne",
 Profiles:async(method:string,payload:string)=>{
  const args=JSON.parse(payload);
  if(args.path.endsWith("Broken.winone"))throw Error("Cannot read selected file");
  if(method==="import.apply"){
   if(args.bindings["capture.output"]!=="D:\\Pictures\\WinOne")throw Error("Missing mapping");
   document.getElementById("result")!.textContent=JSON.stringify(args);return JSON.stringify({revision:5});
  }
  if(method!=="import.preview")throw Error("Unsupported fixture action");
  const mapped=!!args.bindings["capture.output"];
  return JSON.stringify({name:"Home",sourceHash:"fixture-reviewed",configurationRevision:4,moduleRevision:2,
   preferences:{theme:"dark",language:"en-US",runAtLogon:false,startMinimized:true,enabled:{capture:true,"always-on-top":false},shortcuts:{}},currentShortcuts:{},
   plan:{modules:[{id:"capture",state:mapped?"ready":"needs_mapping",requiredPaths:["capture.output"],settings:{openEditor:true,scrollDelay:300}},{id:"always-on-top",state:"ready",requiredPaths:[],settings:{frame:true,frameThickness:3}}],shortcutIssues:[]},
   paths:[{id:"capture.output",label:"Capture folder",kind:"directory",value:args.bindings["capture.output"]||"",state:mapped?"ready":"needs_mapping"}]});
 }
}}} as unknown as NonNullable<Window["go"]>;
void setApplicationLanguage(new URLSearchParams(location.search).get("language")||"zh-CN");
createRoot(document.getElementById("root")!).render(<main className="mx-auto max-w-4xl p-8"><ProfileImport onApplied={()=>{}}/><output hidden id="result"/></main>);
