// Isolated UI fixture. All settings, accounts and model results stay in memory.
import {usageFixture} from "./ai-usage-fixture";
import {useState} from "react";
import {Sparkles,MousePointer2,Settings,ScanLine} from "lucide-react";
import {AISettings} from "../src/features/ai-settings";
import type {AIConfiguration} from "../src/features/ai-configuration";
import {CaptureAISettings} from "../src/features/capture-ai-settings";
import {createRoot} from "react-dom/client";
import {i18n,setApplicationLanguage} from "../src/i18n";
import {AppShell} from "../src/components/app-shell";
import {SettingsPanel} from "../src/components/settings-panel";
import {AppearanceSettings} from "../src/components/appearance-settings";
import logo from "../../../../internal/branding/logo.png";
import "../src/styles/shadcn.css";
import "../src/style.css";
const query=new URLSearchParams(location.search);
const language=query.get("language")||"zh-CN";
const theme=query.get("theme")||"light";
document.documentElement.dataset.theme=theme;
let preferences:AIConfiguration={revision:1,engine:"google",ocrEngine:"local",target:"auto",prompt:"",providers:[{id:"one",name:"示例服务",icon:"deepseek-color",authMode:"api-key",protocol:"chat",baseUrl:"https://example.invalid/v1",models:["shared-model","vision-model"],availableModels:["shared-model","vision-model","unchecked-model"],hasKey:true},{id:"account",name:"示例账号",icon:"openai",authMode:"account",protocol:"chat",baseUrl:"",agent:"codex",models:["shared-model"],hasKey:false}],tasks:{default:{provider:"one",model:"shared-model"}}};
if(query.has("manyProviders"))preferences.providers.push(...Array.from({length:9},(_,i)=>({...preferences.providers[0],id:`overview-${i}`,name:`Provider ${i+1} with a longer descriptive name`,models:["model-a","model-b","model-c"]})));
if(query.has("multipleKeys")){preferences.providers[0].keys=[{id:"primary",enabled:true,hasKey:true},{id:"second",enabled:true,hasKey:true},{id:"third",enabled:false,hasKey:true}];preferences.providers[0].keyPolicy={strategy:"round-robin",failover:true,maxAttempts:3,cooldownSeconds:60};}
if(query.has("manyModels"))preferences.providers[0].availableModels!.push(...Array.from({length:40},(_,index)=>`test-model-${index}`));
if(query.has("modelIcons")){preferences.providers[0].icon="openrouter-color";preferences.providers[0].models=["google/gemini-2.5-pro","anthropic/claude-sonnet-4","openai/gpt-5","deepseek/deepseek-r1"];preferences.providers[0].availableModels=[...preferences.providers[0].models];preferences.tasks.default.model=preferences.providers[0].models[0];}
if(query.has("modelSettings")){const provider=preferences.providers[0];provider.modelNames={"shared-model":"Gemini (free)"};provider.modelFree={"shared-model":true};provider.modelMetadata={"shared-model":{contextWindow:128000,maxTokens:8000,compactionThreshold:100000,reasoning:true}};}
const presets=[
 {id:"openai",name:"OpenAI",icon:"openai",chat:"https://example.invalid/openai/v1",models:["gpt-5"]},
 {id:"deepseek",name:"DeepSeek",icon:"deepseek-color",chat:"https://example.invalid/deepseek/v1",models:["deepseek-chat"]},
 {id:"regional",name:"Regional service with a longer descriptive name",icon:"openrouter-color",chat:"https://example.invalid/region/v1",regions:[{id:"one",name:"Region one",chat:"https://example.invalid/one/v1"},{id:"two",name:"Region two",chat:"https://example.invalid/two/v1"}]},
 ...Array.from({length:24},(_,i)=>({id:`fixture-${i}`,name:`Example provider ${i+1}`,chat:`https://example.invalid/${i}/v1`}))
];
// Each provider owns its login, just as the Host uses separate account stores.
const signedIn=new Set<string>();let loginProvider="";
const login=query.get("login");
// Synthetic sign-in states never open a browser or contact a vendor.
const pendingLogin={id:"fixture",status:{agent:"codex",state:login==="failure"?"failed":"waiting",url:"",...(login==="code"?{code:"ABCD-EFGH"}:{}),...(login==="callback"?{pasteCallback:true,instructions:"完成登录后，粘贴浏览器地址。"}:{}),...(login==="failure"?{error:"登录未完成，请重试。"}:{})}};
window.go={main:{App:{
 Chat:async(method:string,payload:string)=>{const value=usageFixture("chat."+method,payload);if(value===undefined)throw Error("Unsupported Chat fixture method: "+method);return JSON.stringify(value);},
 Capture:async(method:string,payload:string)=>{
  if(method==="ai.magpie.catalog")return JSON.stringify({gateway:"http://127.0.0.1:3426",visionModels:[],imageGenModels:[],searchChoices:[],searchVendors:[]});
  if(method==="ai.magpie.privacy.open"||method==="ai.magpie.privacy.state")return JSON.stringify({on:false,supported:false,installed:false,size:0});
  if(method==="ai.magpie.privacy.close")return '{}';
  const usage=usageFixture(method,payload);if(usage!==undefined)return JSON.stringify(usage);
  if(method==="ai.configuration.get"){if(query.has("failure"))throw Error("The AI settings file could not be read.");return JSON.stringify(preferences);}
  if(method==="ai.configuration.save"){const submission=JSON.parse(payload);Object.assign(window,{fixtureLastSave:submission});preferences={...submission.configuration,revision:preferences.revision+1};return JSON.stringify(preferences);}
  if(method==="ai.accounts.catalog")return JSON.stringify({presets,agents:[{id:"codex",name:"ChatGPT / Codex",icon:"openai"},{id:"claude",name:"Claude Code",icon:"claude-color"},{id:"copilot",name:"GitHub Copilot",icon:"githubcopilot"},{id:"chatgpt-api",name:"ChatGPT API",icon:"openai"},{id:"cursor",name:"Cursor",icon:"cursor"},{id:"workbuddy",name:"WorkBuddy",icon:"workbuddy-color"},{id:"zcode",name:"ZCode",icon:"zcode"}]});
  if(method==="ai.accounts.list"){const request=JSON.parse(payload);return JSON.stringify({accounts:signedIn.has(request.provider)?[{agent:request.agent,user:"test@example.invalid",plan:"Test plan"}]:[]});}
  // This settings fixture has no installed UI extensions. Return the real
  // empty discovery shape so unrelated pages do not inherit a fixture error.
  if(method==="ai.routing.extensions.open")return JSON.stringify({url:location.href,pages:[],panels:[]});
  if(method==="ai.routing.extensions.close")return JSON.stringify({closed:true});
  if(method==="ai.models.fetch")return JSON.stringify({models:[{id:"discovered-model"}]});
  if(method==="ai.accounts.models")return JSON.stringify({models:[{id:"shared-model",name:"Shared model"}]});
  if(method==="ai.accounts.signin.start"){const request=JSON.parse(payload);loginProvider=request.provider;pendingLogin.status.agent=request.agent;return JSON.stringify(pendingLogin);}
  if(method==="ai.accounts.signin.get"){if(login)return JSON.stringify(pendingLogin);signedIn.add(loginProvider);return JSON.stringify({id:"fixture",status:{agent:pendingLogin.status.agent,state:"done",url:""}});}
  if(method==="ai.accounts.signin.cancel")return JSON.stringify({id:"fixture",status:{state:"canceled"}});
  throw Error(`Unsupported fixture method: ${method}`);
 },
 ReadModuleLogs:async()=>({folderAvailable:false,entries:[]}),
 UpdateInfo:async()=>({currentVersion:"0.0.1",repository:"",configured:false}),
}}} as unknown as NonNullable<Window["go"]>;
await setApplicationLanguage(language);
function Preview(){
 const [page,setPage]=useState("ai-services");
 const pages=[{id:"selection",title:i18n.t("nav.selection"),icon:MousePointer2},{id:"capture",title:i18n.t("nav.capture"),icon:ScanLine},{id:"ai-services",title:i18n.t("ai.serviceTitle"),icon:Sparkles},{id:"settings",title:i18n.t("nav.settings"),icon:Settings}];
 const SelectedIcon=pages.find(p=>p.id===page)!.icon;
 return <AppShell page={page} pages={pages} logo={logo} onNavigate={setPage}><header><h1 className="flex items-center gap-3"><SelectedIcon className="size-6"/>{pages.find(p=>p.id===page)?.title}</h1><p className="settings-help">本地界面测试 · 全部数据为模拟数据，不访问真实服务</p></header>{page==="ai-services"?<AISettings/>:page==="capture"?<CaptureAISettings/>:<SettingsPanel><section><h2>{i18n.t("shell.appearance")}</h2><AppearanceSettings theme={theme as "light"|"dark"} language={language as "zh-CN"|"en-US"} disabled={false} onThemeChange={()=>{}} onLanguageChange={()=>{}}/></section></SettingsPanel>}</AppShell>;
}
createRoot(document.getElementById("root")!).render(<Preview/>);
