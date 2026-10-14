// In-memory project fixture; no real project, session or provider is changed.
import {createRoot} from "react-dom/client";
import {ChatPage} from "../src/features/chat";
import {setApplicationLanguage} from "../src/i18n";
import "../src/styles/shadcn.css";
import "../src/style.css";
const query=new URLSearchParams(location.search);
const projects=[{id:"opaque-alpha",name:"Alpha Project",path:"C:\\Projects\\Alpha"},{id:"opaque-beta",name:"Beta Project",path:"C:\\Projects\\Beta"}];
Object.assign(window,{go:{main:{App:{
 Capture:async()=>JSON.stringify({providers:[],tasks:{}}),SetFileDropTarget:async()=>{},PickDirectory:async()=>"C:\\Projects\\Opened",
 Chat:async(method:string,payload:string)=>{
  const params=JSON.parse(payload);
  if(method==="catalog")return JSON.stringify({projects,conversations:[]});
  if(method==="project.git")return JSON.stringify({projectId:params.projectId,repository:true,branch:params.projectId==="opaque-alpha"?"feature/alpha":"feature/beta"});
  if(method==="project.add"||method==="project.create"){const project={id:"opaque-opened",name:params.name,path:params.path??params.parent+"\\"+params.name};projects.push(project);return JSON.stringify(project);}
  throw Error("Unsupported fixture call: "+method);
 }
}}}});
document.documentElement.dataset.theme=query.get("theme")??"light";document.body.style.margin="0";
await setApplicationLanguage(query.get("language")??"zh-CN");
createRoot(document.getElementById("root")!).render(<div style={{height:"100dvh"}}><ChatPage/></div>);
