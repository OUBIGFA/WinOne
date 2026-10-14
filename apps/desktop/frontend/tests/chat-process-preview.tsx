// A local browser fixture. No installed profile, provider or remote service is used.
import {useState} from "react";
import {createRoot} from "react-dom/client";
import {flushSync} from "react-dom";
import {setApplicationLanguage} from "../src/i18n";
import {WorkProcess} from "../src/features/cindy/WorkProcess";
import {ProcessNavigationProvider,ProcessCollapseAction} from "../src/features/process-navigation";
import {ChatDesktopSurface} from '../src/features/chat-desktop-kit';
import {ChatMessageView} from "../src/features/chat-messages";
import {AIProviderAvatar} from "../src/features/ai-provider-avatar";
import type {AIProvider} from "../src/features/ai-configuration";
import type {ProcessResponse} from "../src/features/chat-process";
import "../src/styles/shadcn.css";
import "../src/style.css";
import "../src/features/chat.css";
import "../src/features/cindy/theme.css";

const root=createRoot(document.getElementById("root")!);
const pictures=new Map<string,Uint8Array>();let upload=new Uint8Array();
const base64=(bytes:Uint8Array)=>{let binary="";for(const byte of bytes)binary+=String.fromCharCode(byte);return btoa(binary);};
Object.assign(window,{go:{main:{App:{Capture:async(method:string,payload:string)=>{
 const value=JSON.parse(payload);
 if(method==="ai.icons.store"){
  const part=Uint8Array.from(atob(value.data),c=>c.charCodeAt(0));if(value.offset===0)upload=new Uint8Array(value.size);upload.set(part,value.offset);
  if(value.offset+part.length<value.size)return JSON.stringify({upload:"fixture-upload",offset:value.offset+part.length});
  const hash=new Uint8Array(await crypto.subtle.digest("SHA-256",upload));const icon="file:"+[...hash.slice(0,8)].map(n=>n.toString(16).padStart(2,"0")).join("")+".png";pictures.set(icon,upload);return JSON.stringify({icon});
 }
 if(method==="ai.icons.read"){const bytes=pictures.get(value.icon);if(!bytes)throw Error("Fixture picture missing");return JSON.stringify({data:base64(bytes.subarray(value.offset,value.offset+(256<<10))),size:bytes.length,mimeType:"image/png"});}
 if(method==="ai.icons.cancel")return "{}";
 if(method==="ai.icons.fetch"||method==="ai.icons.website"){const icon=[...pictures.keys()][0];if(!icon)throw Error("Upload the local picture fixture first");return JSON.stringify({icon});}
 throw Error("Unsupported preview operation: "+method);
}}}}});

function Avatar(){
 const [provider,setProvider]=useState<AIProvider>({id:"preview",name:"Preview",authMode:"api-key",baseUrl:"https://example.invalid/v1",icon:"deepseek-color",models:[],hasKey:false});
 const [busy,setBusy]=useState(false);
 return <div className="settings-stack" style={{padding:20}}><AIProviderAvatar provider={provider} disabled={busy} onBusy={setBusy} onChange={avatar=>setProvider(p=>({...p,avatar}))}/><output id="avatar-reference">{provider.avatar??""}</output></div>;
}

function Process({id,lines,running}:{id:string;lines:number;running:boolean}){
 const group:ProcessResponse={kind:"response",key:id,messages:[],running,startedAt:Date.now()-1200,durationMs:1200,hasTools:false,entries:[{kind:"thinking",key:"thought",part:{type:"thinking",thinking:Array.from({length:lines},(_,i)=>`Thought ${i+1}: read and check the project.`).join("\n"),thinkingDurationMs:1000}},{kind:"operation",key:"retry",operation:{id:"retry",turnId:id,kind:"retry",status:"completed",startedAt:10,durationMs:200,attempt:1,maxAttempts:2}}]};
 return <ProcessNavigationProvider><div className="chat-main" style={{height:700,minHeight:0}}><div className="chat-transcript" data-preview-transcript=""><div className="client-thread client-thread--chat"><ChatMessageView message={{id:"answer",role:"assistant",content:"Final answer."}} onCopy={()=>{}} canFork={false} onFork={()=>{}} process={<WorkProcess group={group} conversationId={id} onCopy={()=>{}}/>}/><div style={{height:900}} aria-hidden="true"/></div></div><footer className="chat-input-area"><div className="chat-input-content"><ProcessCollapseAction/><div style={{height:40}}>Composer</div></div></footer></div></ProcessNavigationProvider>;
}

Object.assign(window,{renderProcessPreview:async(value:{id:string;width:number;lines:number;running:boolean;dark:boolean;language:string;avatar?:boolean})=>{
 await setApplicationLanguage(value.language);document.documentElement.classList.toggle("dark",value.dark);document.body.style.margin="0";
 document.getElementById("root")!.style.width=value.width+"px";
 flushSync(()=>root.render(<ChatDesktopSurface>{value.avatar?<Avatar/>:<Process key={value.id} id={value.id} lines={value.lines} running={value.running}/>}</ChatDesktopSurface>));
}});
