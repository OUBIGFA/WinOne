// Renderer fixture only: no live providers, credentials or user files.
import {createRoot} from "react-dom/client";
import {ChatPage} from "../src/features/chat";
import {AppShell} from '../src/components/app-shell';
import {MessageSquare} from 'lucide-react';
import {setApplicationLanguage} from "../src/i18n";
import type {AIConfiguration} from "../src/features/ai-configuration";
import type {ChatMessage,Conversation} from "../src/features/chat-state";
import resilienceDocument from './fixtures/chat-resilience.md?raw';
import "../src/styles/shadcn.css";
import "../src/style.css";

const query=new URLSearchParams(location.search),language=query.get("language")??"zh-CN",running=query.get("running")==="true";
// Use the same data-theme contract as the native App; no extra .dark fixture class.
const ai:AIConfiguration={revision:1,providers:[{id:"fixture",name:"Local AI",authMode:"api-key",hasKey:false,models:["first","second"],modelNames:{first:"Example Model",second:"Another Model"},modelFree:{first:true,second:true},modelMetadata:{first:{rate:.5,rateWas:2,listedContextWindow:1000000},second:{rate:.5,rateWas:2,listedContextWindow:1000000}}}],tasks:{chat:{provider:"fixture",model:"first"}},engine:"magpie",ocrEngine:"",target:"zh-CN",prompt:""};
let conversation:Conversation={id:"experience",title:"整理项目并生成报告",projectId:"alpha",directory:"C:\\fixture\\Alpha",sessionDirectory:"C:\\fixture\\history",model:{provider:"fixture",model:"first"},permission:"ask",createdAt:"2026-10-11",updatedAt:"2026-10-11"};
const regression=query.has('regression'),resilience=query.has('resilience'),links=query.has('links');
const planItems=[{step:'读取项目说明',status:'completed'},{step:'验证工作过程和文件预览',status:query.has('planDone')?'completed':'in_progress'},{step:'检查窄栏与尺寸拖动',status:query.has('planDone')?'completed':'pending'}];
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
const ioHolds=new Set<string>(),ioWaiters=new Map<string,Set<()=>void>>(),ioFailures=new Map<string,string>();
const fixtureIO={hold(method:string){ioHolds.add(method);},release(method:string){ioHolds.delete(method);for(const release of ioWaiters.get(method)??[])release();ioWaiters.delete(method);},fail(method:string,message?:string){if(message)ioFailures.set(method,message);else ioFailures.delete(method);}};
const alignment=query.has('alignment'),changedFiles=['README.md','example.ts','notes.md','guide.md','plan.md'].map(path=>({path,status:'M'}));
const patch='diff --git a/README.md b/README.md\n--- a/README.md\n+++ b/README.md\n@@ -1 +1 @@\n-old\n+new\n';
let workspaceState='applied',promptRevision=0;
const reviewMode=query.has('review'),reviewStaged=new Set<string>(),reviewReverted=new Set<string>();let reviewRevision=1;
const reviewFiles=[{path:'example.ts',status:'M',additions:2,deletions:1,size:96},{path:'README.md',status:'M',additions:3,deletions:1,size:180},{path:'picture.png',status:'M',additions:0,deletions:0,size:320,binary:true}];
if(alignment){
 messages.splice(0,messages.length,
  {id:'user',role:'user',content:'Update the project and explain the changes.',timestamp:start},
  {id:'opening',role:'assistant',providerName:'Local AI',modelName:'Example Model',timestamp:start+10,content:[{type:'text',text:'I will check the project first.'},{type:'thinking',thinking:'Inspect the existing files.'},{type:'toolCall',id:'read',name:'read',arguments:{path:'README.md'}}]},
  {role:'toolResult',toolCallId:'read',timestamp:start+20,content:'Read complete'},
  {id:'middle',role:'assistant',providerName:'Local AI',modelName:'Example Model',timestamp:start+30,content:[{type:'text',text:'The configuration is ready. I will update the files.'},{type:'toolCall',id:'write',name:'write',arguments:{path:'README.md'}}]},
  {role:'toolResult',toolCallId:'write',timestamp:start+40,content:'Saved'},
 );
 if(!running)messages.push({id:'answer',role:'assistant',providerName:'Local AI',modelName:'Example Model',timestamp:start+50,content:'## Changes completed\n\nAll files have been checked.\n\n- Kept existing configuration\n- Saved the report'});
}
if(query.has('repair')){
 if(running){for(let i=0;i<8;i++)messages.push({id:'repair-tool-'+i,role:'assistant',timestamp:start+8000+i,content:[{type:'toolCall',id:'repair-call-'+i,name:'powershell',arguments:{command:'Write-Output repair-'+i}}]},{role:'toolResult',toolCallId:'repair-call-'+i,timestamp:start+8100+i,content:'done'});}
 else {const answer=messages.at(-1)!;answer.content=String(answer.content)+'\n\n[Open project notes](./README.md) · [Browse docs](docs/)\n\n`userId` and `getValue()` are identifiers.';}
}
if(query.has('liveThinking'))messages.splice(0,messages.length,{id:'thinking-user',role:'user',content:'Inspect the plan',timestamp:start},{id:'thinking-stream',role:'assistant',timestamp:start+1,content:[{type:'thinking',thinking:Array.from({length:query.has('growThinking')?2:150},(_,index)=>`Real thinking line ${index+1}: preserve every detail.`).join('\n')}]});
if(resilience)messages.splice(0,messages.length,
 {id:'user',turnId:'user',role:'user',timestamp:start,content:'验证 Markdown 预览和计划。'},
 {id:'plan-call',turnId:'user',role:'assistant',timestamp:start+1,content:[{type:'toolCall',id:'plan',name:'update_plan',arguments:{plan:planItems,explanation:'所有内容在隔离夹具中验证。'}}]},
 {role:'toolResult',turnId:'user',toolCallId:'plan',timestamp:start+2,content:JSON.stringify({items:planItems,explanation:'所有内容在隔离夹具中验证。'})},
 {id:'read-call',turnId:'user',role:'assistant',timestamp:start+3,content:[{type:'toolCall',id:'document-read',name:'read',arguments:{path:'README.md'}}]},
 {role:'toolResult',turnId:'user',toolCallId:'document-read',timestamp:start+4,content:[{type:'text',text:resilienceDocument}]},
 {id:'write-call',turnId:'user',role:'assistant',timestamp:start+5,content:[{type:'toolCall',id:'document-write',name:'write',arguments:{path:'notes.md',content:resilienceDocument}}]},
 {role:'toolResult',turnId:'user',toolCallId:'document-write',timestamp:start+6,content:'Saved notes.md'},
 {id:'long-reply',turnId:'user',role:'assistant',timestamp:start+7,content:Array.from({length:8},(_,index)=>`### 验证段落 ${index+1}\n\n`+'保留实际过程，检查完整内容，不移动或修改用户文件。'.repeat(6)).join('\n\n')+'\n\n'+resilienceDocument},
 ...(running?[{id:'active-tool',turnId:'user',role:'assistant',timestamp:start+8,content:[{type:'toolCall',id:'active',name:'powershell',arguments:{command:'Write-Output "isolated fixture"'}}]} as ChatMessage]:[])
);
if(links){
 ai.providers[0].models=['replacement-model'];
 messages.splice(0,messages.length,{id:'user',role:'user',content:'Keep the original request.'},{id:'answer',role:'assistant',provider:'fixture',model:'first',providerName:'Local AI',modelName:'Example Model',content:'[source](README.md:12:3) · [folder](docs/) · [HTML](page.html) · [denied](../private.txt)'});
}
if(query.has('editWidths'))messages.splice(0,messages.length,
 {id:'short-user',role:'user',content:'简短消息'},
 {id:'short-answer',role:'assistant',content:'Short response.'},
 {id:'long-user',role:'user',content:query.get('editWidths')==='short'?'简短消息':'这是一条较长的原始消息，点击编辑时应保留气泡原有宽度。'.repeat(16)},
 {id:'long-answer',role:'assistant',content:Array.from({length:24},(_,index)=>`### 检查段落 ${index+1}\n\n这里保留正文，以核对输入区域升高或缩小之后的返回底部位置。`).join('\n\n')+'\n\n最后一段内容应完整显示在输入区域上方。'},
);
let streamRevision=0;
const growThinking=()=>{const message=messages.find(message=>message.id==='thinking-stream');if(message){message.content=[{type:'thinking',thinking:Array.from({length:150},(_,index)=>`Real thinking line ${index+1}: preserve every detail.`).join('\n')}];streamRevision++;}};
const browserStates=new Map<string,Record<string,unknown>>();
Object.assign(window,{chatExperienceGrowThinking:growThinking,chatExperienceCalls:calls,chatExperienceIO:fixtureIO,go:{main:{App:{
 SetFileDropTarget:async()=>{},
 PickChatFiles:async()=>Array.from({length:18},(_,i)=>`C:/fixture/attachment-${i}.txt`),
 ChatMedia:async(method:string,raw:string)=>{calls.push({method:'media.native.'+method,params:JSON.parse(raw)});return JSON.stringify(method==='save-as'?{canceled:false,path:'C:/fixture/saved-image.png'}:{success:true});},
 ChatBrowser:async(method:string,raw:string)=>{const params=JSON.parse(raw);calls.push({method:'browser.native.'+method,params});if(method==='create'){const state={...params,title:'HTML preview',loading:false,canGoBack:false,canGoForward:false,error:'',visible:false,favicon:'',isAudible:false,muted:false,crashed:false};browserStates.set(params.tabId,state);return JSON.stringify(state);}if(method==='close'){browserStates.delete(params.tabId);return '{}';}return JSON.stringify(browserStates.get(params.tabId)??{});},
 Capture:async(method:string)=>{if(method!=="ai.configuration.get")throw Error("Unexpected fixture capture "+method);return JSON.stringify(ai);},
 Chat:async(method:string,raw:string)=>{
  const params=JSON.parse(raw);calls.push({method,params});
  if(ioFailures.has(method))throw Error(ioFailures.get(method));
  if(ioHolds.has(method))await new Promise<void>(resolve=>{const waiters=ioWaiters.get(method)??new Set();waiters.add(resolve);ioWaiters.set(method,waiters);});
  if(query.has('editWidths')&&method==='review.turn')return JSON.stringify({snapshot:{files:[],isReversible:false},patches:{}});
  if(method==='media.upload')return JSON.stringify({url:previewOrigin+'/image-upload?name='+encodeURIComponent(params.name||'image.png')});
  if(method==='media.source')return JSON.stringify({attachment:{name:'picture.png',path:'C:/fixture/Alpha/picture.png',mimeType:'image/png',size:320},preview:{name:'picture.png',path:'C:/fixture/Alpha/picture.png',kind:'image',mimeType:'image/png',size:320,revision:'image',url:previewOrigin+'/picture.png'}});
  if(method==='media.attach')return '{}';
  if(alignment&&method==='review.metadata')return JSON.stringify({repository:true,root:conversation.directory,branch:'feature/review',headOid:'a'.repeat(40),detached:false,unborn:false,upstream:'origin/feature/review',ahead:1,behind:0,candidates:[{refName:'main',shortName:'main',kind:'local',oid:'b'.repeat(40),isDefaultBranch:true}],inProgress:[]});
  if(alignment&&method==='review.commits')return JSON.stringify([{id:'a'.repeat(40),title:'Update example',date:'2026-10-15T08:00:00Z'}]);
  if(alignment&&method==='review.diff.preview')return JSON.stringify({url:query.has('previewOrigin')?previewOrigin+'/review.diff?path='+encodeURIComponent(params.path):'data:text/plain;base64,'+btoa(patch)});
  if(alignment&&method==='review.file.preview')return JSON.stringify({present:true,url:previewOrigin+'/'+params.path+'?side='+params.side,size:180,mime:params.path.endsWith('.png')?'image/png':'text/markdown',root:conversation.directory});
  if(reviewMode&&method==='review.snapshot'&&!params.turnId)return JSON.stringify({root:conversation.directory,branch:'feature/review',scope:params.scope,base:params.base,revision:'review-'+reviewRevision,files:reviewFiles.filter(file=>!reviewReverted.has(file.path)&&(params.scope==='staged'?reviewStaged.has(file.path):params.scope==='working'?!reviewStaged.has(file.path):true))});
  if(reviewMode&&method==='review.mutate'){for(const path of params.mutation.paths??[]){if(params.mutation.action==='stage')reviewStaged.add(path);else if(params.mutation.action==='unstage')reviewStaged.delete(path);else if(params.mutation.action==='revert')reviewReverted.add(path);}reviewRevision++;return '{}';}
  if(query.has('statusRows')&&method==='activity')return JSON.stringify({experience:{phase:'running',attention:false},empty:{phase:'idle',attention:false},awaiting:{phase:'needs-interaction',attention:false},failed:{phase:'error',attention:true},interrupted:{phase:'interrupted',attention:false}});
  if(query.has('statusRows')&&method==='catalog')return JSON.stringify({projects:[{id:'alpha',name:'Alpha',path:'C:/fixture/Alpha'}],conversations:['experience','empty','awaiting','failed','interrupted'].map(id=>({...conversation,id,title:id,projectId:id==='experience'?'alpha':undefined}))});
  if(method==='activity')return JSON.stringify(resilience||links?{experience:{phase:links?'interrupted':running?'running':'completed',attention:!links},empty:{phase:'idle',attention:false},background:{phase:'running',attention:false}}:{});
  if(method==="catalog")return JSON.stringify({projects:[{id:"alpha",name:"Alpha",path:"C:\\fixture\\Alpha"},{id:"beta",name:"Beta",path:"C:\\fixture\\Beta"}],conversations:resilience||links?[{...conversation,hasMessages:true},{...conversation,id:'empty',title:'尚无任务的会话',hasMessages:false},{...conversation,id:'background',title:'后台正在执行的会话',hasMessages:true}]:[conversation]});
  if(method==='conversation.new'){conversation={...conversation,id:'created',title:'Chat'};messages.splice(0,messages.length);return JSON.stringify(conversation);}
  if(method==='title.generate'){conversation={...conversation,title:'New conversation'};return JSON.stringify(conversation);}
  if(method==="project.git")return JSON.stringify({projectId:params.projectId,repository:false});
  if(method==='attachments.add')return JSON.stringify({attachments:params.paths.map((path:string)=>({path,name:path.split(/[\\/]/).pop(),size:120,mimeType:'text/plain'}))});
  if(['files.list','files.index','files.search'].includes(method))return JSON.stringify({url:previewOrigin+'/'+method+'.json?path='+encodeURIComponent(params.path||'')});
  if(method==='files.cancel')return '{}';
  if(method==="files")return JSON.stringify({directory:conversation.directory,entries:['README.md','example.ts','picture.png','page.html','sample.pdf','sound.wav','clip.webm'].map(name=>({name,path:'C:\\fixture\\Alpha\\'+name,directory:false,size:120}))});
  if(links&&method==='file.stat')return JSON.stringify({files:params.paths.map((path:string)=>path.endsWith('private.txt')?{requested:path,error:'文件不在当前会话允许访问的范围内。'}:{requested:path,file:{path,name:path.split(/[\\/]/).filter(Boolean).pop(),directory:/[\\/]docs[\\/]?$/.test(path),size:120,modified:1700000000000}})});
  if(links&&['file.open','file.reveal','link.open'].includes(method))return '{}';
  if(method==='file.stat')return JSON.stringify({files:params.paths.map((path:string)=>({requested:path,file:{path:/^[A-Za-z]:/.test(path)?path:conversation.directory+'\\'+path,name:path.split(/[\\/]/).pop(),directory:false,size:120,modified:1700000000000}}))});
  if(method==='file.preview'||method==='file.save'){
   const name=String(params.path).split(/[\\/]/).pop()!;
   const kind=name.endsWith('.png')?'image':name.endsWith('.pdf')?'pdf':name.endsWith('.wav')?'audio':name.endsWith('.webm')?'video':'text';
   return JSON.stringify({path:params.path,name,kind,size:120,modified:1700000000000,revision:method==='file.save'?'saved':'original',url:previewOrigin+'/'+name,mimeType:kind==='image'?'image/png':'text/plain'});
  }
  if(method==="resources")return JSON.stringify({agents:[{path:'C:\\fixture\\Alpha\\AGENTS.md'}],skills:[{name:'Review project',description:'Inspect changes and check the project rules before continuing.',path:'C:\\fixture\\skills\\review\\SKILL.md'}],plugins:[{name:'Project tools',path:'C:\\fixture\\plugins\\project-tools',enabled:true}],mcpServers:[{name:'Local documents',enabled:true,status:'Connected'}],diagnostics:[]});
  if(method==="draft.get")return JSON.stringify({text:"保留这段草稿"});
  if(method==='settings.get')return JSON.stringify({workspaceDirectory:'C:\\fixture',skillDirectories:[],pluginDirectories:[],mcpServers:[],automations:[],...(alignment&&!query.has('titleInherit')?{titleModel:conversation.model}:{})});
  if(alignment&&method==='review.turn')return JSON.stringify({snapshot:{root:conversation.directory,revision:'r1',isReversible:true,workspaceState,files:changedFiles},patches:Object.fromEntries(changedFiles.map(file=>[file.path,patch]))});
  if(alignment&&method==='review.turn.apply'){workspaceState=params.mutation.action==='undo'?'undone':'applied';return JSON.stringify({root:conversation.directory,revision:'r1',isReversible:true,workspaceState,files:changedFiles});}
  if(alignment&&method==='review.snapshot')return JSON.stringify({root:conversation.directory,revision:'r1',source:'session',files:changedFiles});
  if(alignment&&method==='review.diff')return JSON.stringify(patch);
  if(alignment&&method==='prompt'){messages.push({id:params.requestId,requestId:params.requestId,role:'user',content:params.text},{id:'next-answer',role:'assistant',content:'The follow-up is complete.'});promptRevision++;return '{}';}
  if(alignment&&method==='poll'&&promptRevision)return JSON.stringify({events:params.cursor<promptRevision?[{sequence:promptRevision,data:{type:'settled',state:{messages,hasOlder:false}}}]:[],cursor:promptRevision,approvals:[],running:false,status:'idle'});
  if(method==='settings.save')return JSON.stringify(params.settings);
  if(method==='title.regenerate')throw Error('管道已结束。 (os error 109) (transport)');
  if(method==='state'&&params.before){await new Promise(resolve=>setTimeout(resolve,240));return JSON.stringify({messages:Array.from({length:12},(_,index)=>({id:'older-'+index,role:index%2?'assistant':'user',content:'Earlier message '+index+'\n\n'+('Earlier context. '.repeat(30))})),operations:[],hasOlder:false});}
  if((method==='open'||method==='state')&&params.id==='empty')return JSON.stringify({messages:[],operations:[],hasOlder:false,streaming:false});
  if(method==="open"||method==="state")return JSON.stringify({messages,operations:[],hasOlder:regression,before:regression?'user':undefined,streaming:running,...(resilience?{workflow:{items:planItems},taskGraph:{scheduling:'running',total:running?2:0,tasks:running?[{id:1,kind:'pi.generation',conversationId:1,state:'waiting',background:false},{id:2,kind:'pi.tool',conversationId:1,owner:1,state:'running',background:false}]:[]}}:{}),...(query.has('audit')?{usage:{input:1000,output:200,cacheRead:400,cacheWrite:50,totalTokens:1650,cost:0,costKnown:false},contextUsage:{tokens:1650,contextWindow:100000,percent:1.65}}:{})});
  if(method==='poll'&&query.has('growThinking'))return JSON.stringify({events:[],cursor:streamRevision,reset:params.cursor<streamRevision,approvals:[],running,status:'running'});
  if(method==="poll")return JSON.stringify({events:[],cursor:0,approvals:[],running,status:links?'stopped':running?"running":"idle",error:regression&&!alignment?"Request aborted":undefined});
  if(method==="configure"){conversation={...conversation,...params};return JSON.stringify(conversation);}
  if(["release","draft.save"].includes(method))return "{}";
  throw Error("Unexpected fixture Chat "+method);
 },
}}}});
await setApplicationLanguage(language);
document.documentElement.dataset.theme=query.get("theme")??"light";
const page=<ChatPage requestedConversationId={query.has("welcome")?undefined:"experience"}/>;
createRoot(document.getElementById("root")!).render(query.has('shell')?<AppShell page="chat" pages={[{id:'chat',title:'Chat',icon:MessageSquare}]} logo="" onNavigate={()=>{}}>{page}</AppShell>:<div style={{height:"100dvh"}}>{page}</div>);
