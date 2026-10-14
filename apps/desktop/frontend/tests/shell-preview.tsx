import {Notice} from "../src/ui/notice";
// Visual fixture only: no processes, files or system settings are changed.
import {useState} from "react";
import {createRoot} from "react-dom/client";
import {Home,ScanLine,Pin,PanelsTopLeft,Settings,MousePointer2} from "lucide-react";
import {AppShell} from "../src/components/app-shell";
import {ResizablePanelGroup,ResizablePanel,ResizableHandle} from "../src/components/ui/resizable";
import {ModulesPage} from "../src/features/modules";
import {AppearanceSettings} from "../src/components/appearance-settings";
import {useTranslation,setApplicationLanguage} from "../src/i18n";
import type {Settings as Preferences} from "../src/bridge";
import "../src/styles/shadcn.css";
import "../src/style.css";
import logo from "../../../../internal/branding/logo.png";

const pages=[{id:"overview",title:"首页",icon:Home},{id:"capture",title:"截图",icon:ScanLine},{id:"selection",title:"划词助手",icon:MousePointer2},{id:"always-on-top",title:"置顶功能",icon:Pin},{id:"wintray",title:"程序管理",icon:PanelsTopLeft},{id:"chat",title:"Chat",icon:PanelsTopLeft},{id:"settings",title:"设置",icon:Settings}];
let revision=0;
const englishNames:Record<string,string>={capture:"Capture",selection:"Text selection","always-on-top":"Always on Top",wintray:"Program management"};
const entries=pages.slice(1,5).map(page=>({manifest:{id:page.id,name:{"zh-CN":page.title,"en-US":englishNames[page.id]},distribution:"bundled"},enabled:true,pendingStop:false,runtime:{id:page.id,name:page.title,state:"ready",detail:""}}));
if(new URLSearchParams(location.search).has("optional"))entries.push({manifest:{id:"example.tool",name:{"zh-CN":"扩展示例","en-US":"Example tool"},distribution:"optional"},enabled:false,pendingStop:false,runtime:{id:"example.tool",name:"Example tool",state:"disabled",detail:""}});
window.go={main:{App:{Shortcuts:async()=>JSON.stringify({bindings:[{id:"capture",shortcut:"Shift+A",registered:true},{id:"pin",shortcut:"Alt+Q",registered:true}]}),Modules:async(method:string,payload:string)=>{
 if(method==="snapshot")return JSON.stringify({revision,modules:entries});
 if(method==="setEnabled"){const value=JSON.parse(payload);const entry=entries.find(entry=>entry.manifest.id===value.id)!;entry.enabled=value.enabled;entry.runtime.state=value.enabled?"ready":"disabled";revision++;return JSON.stringify({state:"completed"});}
 throw Error("Fixture command unavailable");
}}}} as NonNullable<Window["go"]>;
// Exercise nested panel registration like Chat without starting a provider or
// loading a user's conversations. The outer shell is the production component.
function NestedPanelPreview(){
 const [left,setLeft]=useState(true),[right,setRight]=useState(false);
 return <div className="chat-workspace"><ResizablePanelGroup orientation="horizontal">
  {left&&<><ResizablePanel id="sidebar" defaultSize={230} minSize={170}>Chat 左栏</ResizablePanel><ResizableHandle/></>}
  <ResizablePanel id="workbench" minSize="30%"><ResizablePanelGroup orientation="horizontal"><ResizablePanel id="conversation" minSize="20%"><button onClick={()=>setLeft(value=>!value)}>切换 Chat 左栏</button><button onClick={()=>setRight(value=>!value)}>切换 Chat 右栏</button></ResizablePanel>{right&&<><ResizableHandle/><ResizablePanel id="details" defaultSize="42%" minSize={280}>Chat 右栏</ResizablePanel></>}</ResizablePanelGroup></ResizablePanel>
 </ResizablePanelGroup></div>;
}
function Preview(){
 const {t}=useTranslation();
 const navigation=pages.map(page=>({...page,title:page.id==="chat"?"Chat":t(`nav.${page.id}`)}));
 const [page,setPage]=useState("overview");
 const [theme,setTheme]=useState<Preferences["theme"]>("system");
 const [language,setLanguage]=useState<NonNullable<Preferences["language"]>>("zh-CN");
 return <AppShell page={page} pages={navigation} logo={logo} onNavigate={setPage}>
  <header><h1>{navigation.find(item=>item.id===page)?.title}</h1></header>
  {new URLSearchParams(location.search).get("warning")&&<Notice tone="info">{new URLSearchParams(location.search).get("warning")}</Notice>}
  {page==="overview"?<ModulesPage onChanged={()=>{}}/>:page==="chat"?<NestedPanelPreview/>:page==="settings"?<section><h2>外观</h2><AppearanceSettings theme={theme} language={language} disabled={false} onThemeChange={value=>{setTheme(value);document.documentElement.dataset.theme=value==="system"?(matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light'):value;}} onLanguageChange={value=>{setLanguage(value);void setApplicationLanguage(value);}}/></section>:<section>{Array.from({length:35},(_,i)=><p key={i} className="border-b py-5">滚动验证内容 {i+1}</p>)}</section>}
 </AppShell>;
}
void setApplicationLanguage(new URLSearchParams(location.search).get("language")||"zh-CN");
createRoot(document.getElementById("root")!).render(<Preview/>);
