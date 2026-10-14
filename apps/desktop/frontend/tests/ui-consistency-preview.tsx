import {useState} from "react";
import {createRoot} from "react-dom/client";
import {Home,Settings,MousePointer2} from "lucide-react";
import {AppShell} from "../src/components/app-shell";
import {AppearanceSettings} from "../src/components/appearance-settings";
import {MouseGesturesSettings} from "../src/features/mouse-gestures";
import {i18n,setApplicationLanguage} from "../src/i18n";
import defaults from "../../../../modules/stroke/defaults.json";
import logo from "../../../../internal/branding/logo.png";
import "../src/styles/shadcn.css";
import "../src/style.css";

const query=new URLSearchParams(location.search);
await setApplicationLanguage(query.get("language")??"zh-CN");
document.documentElement.dataset.theme=query.get("theme")??"light";
let settings=structuredClone(defaults);
// A deterministic bridge exercises the production editor without installing
// hooks, executing gestures, or touching the user's saved configuration.
window.go={main:{App:{Stroke:async(method:string,payload:string)=>{
 if(method==="settings.get")return JSON.stringify(settings);
 if(method==="settings.save"){settings=JSON.parse(payload);return JSON.stringify(settings);}
 throw Error("Unsupported fixture request: "+method);
}}}} as unknown as NonNullable<Window["go"]>;

function Fixture(){
 const [page,setPage]=useState("settings");
 const pages=[{id:"overview",title:i18n.t("nav.overview"),icon:Home},{id:"gestures",title:i18n.t("stroke.title"),icon:MousePointer2},{id:"settings",title:i18n.t("nav.settings"),icon:Settings}];
 return <AppShell page={page} pages={pages} logo={logo} onNavigate={setPage}>{page==="gestures"?<MouseGesturesSettings/>:<AppearanceSettings theme="system" language="zh-CN" disabled={false} onThemeChange={()=>{}} onLanguageChange={()=>{}}/>}</AppShell>;
}
createRoot(document.getElementById("root")!).render(<Fixture/>);
