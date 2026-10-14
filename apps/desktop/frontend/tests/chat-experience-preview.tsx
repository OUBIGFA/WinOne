// Renderer fixture only: no live providers, credentials or user files.
import {createRoot} from "react-dom/client";
import {ChatPage} from "../src/features/chat";
import {setApplicationLanguage} from "../src/i18n";
import type {AIConfiguration} from "../src/features/ai-configuration";
import type {ChatMessage,Conversation} from "../src/features/chat-state";
import "../src/styles/shadcn.css";
import "../src/style.css";

const query=new URLSearchParams(location.search),language=query.get("language")??"zh-CN",running=query.get("running")==="true";
document.documentElement.classList.toggle("dark",query.get("theme")==="dark");
const ai:AIConfiguration={revision:1,providers:[{id:"fixture",name:"Local AI",authMode:"api-key",hasKey:false,models:["first","second"],modelNames:{first:"Example Model",second:"Another Model"},modelFree:{first:true,second:true},modelMetadata:{first:{rate:.5,rateWas:2,listedContextWindow:1000000},second:{rate:.5,rateWas:2,listedContextWindow:1000000}}}],tasks:{chat:{provider:"fixture",model:"first"}},engine:"magpie",ocrEngine:"",target:"zh-CN",prompt:""};
let conversation:Conversation={id:"experience",title:"整理项目并生成报告",projectId:"alpha",directory:"C:\\fixture\\Alpha",sessionDirectory:"C:\\fixture\\history",model:{provider:"fixture",model:"first"},permission:"ask",createdAt:"2026-10-11",updatedAt:"2026-10-11"};
const regression=query.has('regression');
if(regression){ai.providers[0].models=Array.from({length:90},(_,index)=>`model-${index}`);ai.tasks.chat={provider:'fixture',model:'model-72'};conversation.model=ai.tasks.chat;}
const start=Date.now()-30000,longPath="C:/fixture/"+"long-directory/".repeat(30)+"report.md";
const messages:ChatMessage[]=[
 {id:"user",role:"user",content:"检查项目文件，说明修改结果。",timestamp:start},
 {id:"first-work",turnId:"user",role:"assistant",timestamp:start+10,content:[{type:"thinking",thinking:Array.from({length:120},(_,i)=>`检查步骤 ${i+1}：读取项目配置，确认输入文件与已有结果。`).join("\n"),thinkingStartedAt:start+10,thinkingDurationMs:1500},{type:"toolCall",id:"read",name:"read",arguments:{path:longPath}}]},
 {id:"read-result",role:"toolResult",toolCallId:"read",toolName:"read",timestamp:start+2500,content:"Project read successfully"},
 {id:"progress",role:"assistant",timestamp:start+3000,content:"已读完配置。接下来整理需要修改的文件。"},
 {id:"second-work",role:"assistant",timestamp:start+4000,content:[{type:"toolCall",id:"shell",name:"powershell",arguments:{command:'Write-Output "'+"VeryLongToken".repeat(80)+'"'}}]},
 {id:"shell-result",role:"toolResult",toolCallId:"shell",toolName:"powershell",timestamp:start+6000,content:"Finished\n"+"log output ".repeat(100)},
];
if(!running)messages.push({id:"answer",role:"assistant",timestamp:start+7000,durationMs:2000,provider:"fixture",providerName:"Local AI",model:"first",modelName:"Example Model",content:"## 修改结果\n\n已检查项目配置。**结果已保存**，可以继续查看每一步的工具输入与输出。\n\n- 保留原有文件\n- 保存完整记录\n\n| 对象 | 结果 | 说明 | 文件 |\n| --- | --- | --- | --- |\n| 配置 | 已检查 | 保留原值 | "+longPath+" |\n\n```text\n"+"LongCodeToken".repeat(70)+"\n```\n\n> 每个工作段可以单独展开，查看具体操作。"});
if(query.get('sample')==='reading')messages.splice(0,messages.length,
 {id:'sample-user',role:'user',content:'请整理项目说明，并列出接下来的检查步骤。'},
 {id:'sample-answer',role:'assistant',providerName:'Local AI',modelName:'Example Model',content:'## 项目检查结果\n\n项目说明已经整理完成。下面保留了文件位置、处理状态和下一步操作，方便继续核查。\n\n### 已完成的工作\n\n- **项目说明**：补齐运行方式和目录结构。\n- **配置文件**：保留已有值，只检查必要字段。\n- **验证记录**：保存检查结果和可复现步骤。\n\n| 文件 | 状态 | 下一步 |\n| --- | --- | --- |\n| README.md | 已更新 | 核对使用说明 |\n| settings.json | 已检查 | 保留当前配置 |\n\n### 运行检查\n\n```typescript\nconst files = ["README.md", "settings.json"];\nfor (const file of files) {\n  console.log(`Checking: \u0024{file}`);\n}\n```\n\n> 本次检查保留原有文件。确认结果后，可以继续下一项工作。'});
// File responses use an isolated HTTP server supplied by the browser probe.
// No fixture action reads or writes a user's project.
const previewOrigin=query.get('previewOrigin')??location.origin;
const calls:{method:string;params:unknown}[]=[];
Object.assign(window,{chatExperienceCalls:calls,go:{main:{App:{
 SetFileDropTarget:async()=>{},
 PickChatFiles:async()=>Array.from({length:18},(_,i)=>`C:/fixture/attachment-${i}.txt`),
 Capture:async(method:string)=>{if(method!=="ai.configuration.get")throw Error("Unexpected fixture capture "+method);return JSON.stringify(ai);},
 Chat:async(method:string,raw:string)=>{
  const params=JSON.parse(raw);calls.push({method,params});
  if(method==="catalog")return JSON.stringify({projects:[{id:"alpha",name:"Alpha",path:"C:\\fixture\\Alpha"},{id:"beta",name:"Beta",path:"C:\\fixture\\Beta"}],conversations:[conversation]});
  if(method==="project.git")return JSON.stringify({projectId:params.projectId,repository:false});
  if(method==='attachments.add')return JSON.stringify({attachments:params.paths.map((path:string)=>({path,name:path.split(/[\\/]/).pop(),size:120,mimeType:'text/plain'}))});
  if(method==="files")return JSON.stringify({directory:conversation.directory,entries:['README.md','example.ts','picture.png','page.html','sample.pdf','sound.wav','clip.webm'].map(name=>({name,path:'C:\\fixture\\Alpha\\'+name,directory:false,size:120}))});
  if(method==='file.preview'||method==='file.save'){
   const name=String(params.path).split(/[\\/]/).pop()!;
   const kind=name.endsWith('.png')?'image':name.endsWith('.pdf')?'pdf':name.endsWith('.wav')?'audio':name.endsWith('.webm')?'video':'text';
   return JSON.stringify({path:params.path,name,kind,size:120,modified:1700000000000,revision:method==='file.save'?'saved':'original',url:previewOrigin+'/'+name,mimeType:kind==='image'?'image/png':'text/plain'});
  }
  if(method==="resources")return JSON.stringify({agents:[{path:'C:\\fixture\\Alpha\\AGENTS.md'}],skills:[{name:'Review project',description:'Inspect changes and check the project rules before continuing.',path:'C:\\fixture\\skills\\review\\SKILL.md'}],plugins:[{name:'Project tools',path:'C:\\fixture\\plugins\\project-tools',enabled:true}],mcpServers:[{name:'Local documents',enabled:true,status:'Connected'}],diagnostics:[]});
  if(method==="draft.get")return JSON.stringify({text:"保留这段草稿"});
  if(method==='settings.get')return JSON.stringify({workspaceDirectory:'C:\\fixture',skillDirectories:[],pluginDirectories:[],mcpServers:[],automations:[]});
  if(method==='settings.save')return JSON.stringify(params.settings);
  if(method==='title.regenerate')throw Error('管道已结束。 (os error 109) (transport)');
  if(method==='state'&&params.before){await new Promise(resolve=>setTimeout(resolve,240));return JSON.stringify({messages:Array.from({length:12},(_,index)=>({id:'older-'+index,role:index%2?'assistant':'user',content:'Earlier message '+index+'\n\n'+('Earlier context. '.repeat(30))})),operations:[],hasOlder:false});}
  if(method==="open"||method==="state")return JSON.stringify({messages,operations:[],hasOlder:regression,before:regression?'user':undefined,streaming:running});
  if(method==="poll")return JSON.stringify({events:[],cursor:0,approvals:[],running,status:running?"running":"idle",error:regression?"Request aborted":undefined});
  if(method==="configure"){conversation={...conversation,...params};return JSON.stringify(conversation);}
  if(["release","draft.save"].includes(method))return "{}";
  throw Error("Unexpected fixture Chat "+method);
 },
}}}});
await setApplicationLanguage(language);
document.documentElement.dataset.theme=query.get("theme")??"light";
createRoot(document.getElementById("root")!).render(<div style={{height:"100dvh"}}><ChatPage requestedConversationId={query.has("welcome")?undefined:"experience"}/></div>);
