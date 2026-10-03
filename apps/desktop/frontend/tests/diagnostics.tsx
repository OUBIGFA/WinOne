// Synthetic logs only; this fixture does not read files or open Explorer.
import {createRoot} from "react-dom/client";
import {Diagnostics} from "../src/features/diagnostics";
import "../src/styles/shadcn.css";
import "../src/style.css";

type NativeApp=NonNullable<NonNullable<Window["go"]>["main"]>["App"];
window.go={main:{App:{
  ReadModuleLogs:async(id:string)=>({folderAvailable:id==="wintray",entries:id==="wintray"?[
    {name:"activity",available:true,truncated:true,text:"2026-09-30 09:00:00 [INFO] Ready\n2026-09-30 09:00:05 [INFO] Backup started\n2026-09-30 09:00:08 [ERROR] Sample: backup destination unavailable"},
    {name:"process",available:true,truncated:false,text:"Sample process output"},
  ]:[{name:"process",available:false,truncated:false,text:""}]}),
  OpenModuleLogFolder:async()=>{throw Error("Fixture does not open folders");},
} as unknown as NativeApp}};
function Fixture(){return <main style={{maxWidth:860,margin:"auto"}}><h1>诊断日志样例</h1><Diagnostics moduleId="wintray"/></main>;}
createRoot(document.getElementById("root")!).render(<Fixture/>);
