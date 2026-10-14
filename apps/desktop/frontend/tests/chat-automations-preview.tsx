// Isolated visual fixture; all automation operations stay in memory.
import {createRoot} from "react-dom/client";
import {ChatAutomations} from "../src/features/chat-automations";
import {setApplicationLanguage} from "../src/i18n";
import {BUILTIN_TEMPLATES,TEMPLATE_CATEGORIES} from "../../../../third_party/cindy/scheduler/builtin-templates";
import enTemplates from "../../../../third_party/cindy/scheduler/en-templates.json";
import "../src/styles/shadcn.css";
import "../src/style.css";
const query=new URLSearchParams(location.search),model={provider:"fixture",model:"fixture-model"};
let schedule={id:"fixture",name:"Project checks",prompt:"Check project files",kind:"cron",cronExpr:"0 9 * * *",timezone:"Asia/Shanghai",recurring:true,manual:false,agentKind:"pi",modelAgentKind:"pi",providerId:model.provider,model:model.model,workspaceKind:"project",workingDir:"C:\\Projects\\Fixture",useWorktree:false,notify:{desktop:true,feishu:false},status:"active",createdAt:1,updatedAt:1};
Object.assign(window,{go:{main:{App:{PickDirectory:async()=>"C:\\Projects\\Fixture",Chat:async(method:string,payload:string)=>{
 const params=JSON.parse(payload);
 if(method==="automations.list")return JSON.stringify([schedule]);
 if(method==="automations.runtime")return JSON.stringify({slotsInUse:0,maxConcurrentRuns:2,inFlightRuns:[],waitingSchedules:[]});
 if(method==="automations.events")return new Promise(()=>{});
 if(method==="automations.get")return JSON.stringify(schedule);
 if(method==="automations.runs")return JSON.stringify([{id:"run",scheduleId:"fixture",status:"success",firedAt:Date.now()-10000,finishedAt:Date.now()-8000,resultText:"Project checks finished.",preRunHookResult:{status:"passed",decision:"run",exitCode:0,durationMs:25,stdout:"Check passed",stderr:""}}]);
 if(method==="automations.preview")return JSON.stringify({nextFireAt:Date.now()+86400000,description:"Daily"});
 if(method==="automations.update"){schedule={...schedule,...params.patch};Object.assign(window,{lastAutomationSave:params.patch});return JSON.stringify(schedule);}
 if(method==="automations.templates")return JSON.stringify({templates:BUILTIN_TEMPLATES.map(template=>params.language?.startsWith("en")?{...template,...(enTemplates.items as Record<string,unknown>)[template.id] as object}:template),categories:TEMPLATE_CATEGORIES.map(category=>params.language?.startsWith("en")?{...category,name:(enTemplates.categories as Record<string,string>)[category.id]}:category)});
 if(method==="automations.connections.get")return JSON.stringify({settings:{methods:{}},servers:[]});
 if(method==="automations.deleted")return JSON.stringify([{...schedule,name:"Deleted task",deletedAt:Date.now()}]);
 throw Error("Unsupported fixture call: "+method);
}}}}});
document.documentElement.dataset.theme=query.get("theme")??"light";document.body.style.margin="0";await setApplicationLanguage(query.get("language")??"zh-CN");
createRoot(document.getElementById("root")!).render(<div style={{height:"100dvh"}}><ChatAutomations providers={[{id:model.provider,name:"Fixture provider",authMode:"api-key",hasKey:true,models:[model.model]}]} defaultModel={model} conversations={[]} onBack={()=>{}} onConversation={()=>{}}/></div>);
