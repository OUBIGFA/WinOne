import {createRoot} from "react-dom/client";
import {DirectoryLinksPage,type DirectoryLinkRecord} from "../src/features/directory-links";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "../src/components/ui/tabs";
import {i18n,setApplicationLanguage} from "../src/i18n";
import {SettingsFixtureShell} from "./settings-fixture-shell";
import type {DirectoryEntry} from "../src/features/directory-links-browser";
import "../src/styles/shadcn.css";
import "../src/style.css";

// Real React pages and production shell, with in-memory native boundaries.
// No filesystem, Explorer menu or installed WinOne configuration is touched.
const source="C:\\source",destination="C:\\links";
const entries:DirectoryEntry[]=[{name:"项目",path:source+"\\项目",objectType:"directory"},...Array.from({length:35},(_,i)=>({name:`document-${i+1}.txt`,path:source+`\\document-${i+1}.txt`,objectType:"file" as const}))];
let records:DirectoryLinkRecord[]=["hardlink","junction","symlink","shortcut"].map((kind,i)=>({id:`existing-${i}`,name:`existing-${kind}`,linkPath:destination+`\\existing-${kind}`,targetPath:source+`\\existing-${kind}`,kind:kind as DirectoryLinkRecord["kind"],objectType:i%2?"directory":"file",targetText:source+`\\existing-${kind}`,relative:false,createdAt:"2026-10-10T10:00:00Z",checkedAt:"2026-10-11T10:00:00Z",status:"active",reason:"target-accessible",accessible:true,entryIdentity:{file:`known-${i}`}}));
let plan:{id:string;items:any[]}={id:"",items:[]};
const calls:{method:string;args:any}[]=[];
Object.assign(window,{directoryLinksFixture:{calls}});
type NativeApp=NonNullable<NonNullable<Window["go"]>["main"]>["App"];
window.go={main:{App:{
 PickDirectory:async()=>source,
 PickChatFiles:async()=>[source+"\\document-1.txt",source+"\\document-2.txt"],
 PickDirectories:async()=>[source+"\\项目","D:\\second-folder"],
 DirectoryLinks:async(method:string,payload:string)=>{
  const args=JSON.parse(payload||"{}");calls.push({method,args});
  if(method==="pending")return JSON.stringify({requested:false,paths:[]});
  if(method==="menu.get")return JSON.stringify({enabled:false,partial:false});
  if(method==="list"||method==="inspectBatch")return JSON.stringify(records);
  if(method==="browse"){
   if(args.directory.endsWith(".txt"))throw Error("请选择文件夹");
   const rows=args.directory===source?entries:args.directory===destination?[...records.map(row=>({name:row.name,path:row.linkPath,objectType:row.objectType,kind:row.kind,targetPath:row.targetPath})),{name:"ordinary.txt",path:destination+"\\ordinary.txt",objectType:"file"}]:[];
   return JSON.stringify({directory:args.directory,parent:"C:\\",entries:rows,total:rows.length,offset:0,hasMore:false});
  }
  if(method==="previewBatch"){
   const items=args.sources.map((path:string,index:number)=>{const name=args.linkName||path.split("\\").at(-1);if(records.some(row=>row.linkPath===args.directory+"\\"+name))throw Error("目标路径已被占用");return {id:String(index),source:path,linkPath:args.directory+"\\"+name,kind:args.kind==="auto"?"hardlink":args.kind,objectType:"file",targetText:path};});
   plan={id:"fixture-plan",items};return JSON.stringify(plan);
  }
  if(method==="createBatch"||method==="createBatch.elevated"){
   if(args.token!==plan.id)throw Error("Invalid fixture preview token");
   const added=plan.items.map((item,index)=>({...records[0],...item,id:`new-${records.length+index}`,name:item.linkPath.split("\\").at(-1),targetPath:item.source}));records=[...records,...added];return JSON.stringify({records:added,completed:added.length,total:added.length});
  }
  throw Error(`This fixture does not implement ${method}`);
 },
} as unknown as NativeApp}};
const params=new URLSearchParams(location.search);
await setApplicationLanguage(params.get("language")||"zh-CN");
document.documentElement.dataset.theme=params.get("theme")||"light";
document.documentElement.classList.toggle("dark",params.get("theme")==="dark");
createRoot(document.getElementById("root")!).render(<SettingsFixtureShell page="convenience"><Tabs value="links"><TabsList variant="default"><TabsTrigger value="pin">{i18n.t("nav.always-on-top")}</TabsTrigger><TabsTrigger value="stroke">{i18n.t("stroke.title")}</TabsTrigger><TabsTrigger value="links">{i18n.t("links.title")}</TabsTrigger></TabsList><TabsContent value="links" className="settings-stack"><DirectoryLinksPage/></TabsContent></Tabs></SettingsFixtureShell>);
