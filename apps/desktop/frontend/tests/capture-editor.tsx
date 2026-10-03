// Synthetic source rendered by the production annotation renderer. Saving is
// simulated; this fixture does not capture the desktop or write user images.
import {useState} from "react";
import {createRoot} from "react-dom/client";
import CaptureEditor from "../src/features/capture-editor";
import {Button} from "../src/components/ui/button";
import type {AnnotationDocument} from "../src/features/capture-editor-model";
import "../src/styles/shadcn.css";
import "../src/style.css";

const surface=new URLSearchParams(location.search).has("surface")?{desktopPreviewId:"c".repeat(32),placement:{x:0,y:0,desktopX:0,desktopY:0,desktopWidth:3200,desktopHeight:1600},monitor:{left:0,top:0,right:3200,bottom:1600}}:undefined;
const initialDocument=new URLSearchParams(location.search).has("selection")?{version:2 as const,width:3200,height:1600,crop:{x:600,y:400,width:1600,height:800},marks:[]}:undefined;
const picking=new URLSearchParams(location.search).has("picking");
const longImage=new URLSearchParams(location.search).has("long");
const image=longImage?{id:"e".repeat(32),width:720,height:18000}:{id:"a".repeat(32),width:3200,height:1600};
const longDocument:AnnotationDocument={version:1,width:720,height:18000,marks:[{kind:"cover",x:100,y:17700,width:200,height:100,color:"#ff0000",strokeWidth:1}]};
function Fixture() {
  const [open,setOpen]=useState(true);
  const [selecting,setSelecting]=useState(picking);
  const output=async(action:string,document:AnnotationDocument)=>{
    window.dispatchEvent(new CustomEvent("capture-fixture-output",{detail:{action,document}}));
    return true;
  };

  return <>{!open&&<Button onClick={()=>setOpen(true)}>打开编辑器</Button>}{open&&<CaptureEditor selection={selecting?{mode:"region",candidates:async()=>({region:null,x:0,y:0,windows:[]}),commit:async()=>{await new Promise(resolve=>setTimeout(resolve,120));setSelecting(false);}}:undefined} onConfirm={picking?document=>output("confirm",document):undefined} surface={surface} saveError={new URLSearchParams(location.search).get("feedback")||undefined} initialZoom={longImage?"width":"fit"} initialDocument={longImage?longDocument:initialDocument} selectionRequired={!!initialDocument} sourceImage onSurfaceReady={async()=>{}} image={image} onClose={()=>setOpen(false)} onPreview={async document=>{
    const response=await fetch("/fixture/render",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(document)});
    if(!response.ok)throw Error(await response.text());
    return {id:"b".repeat(32),sourceId:image.id,width:document.crop?.width??document.width,height:document.crop?.height??document.height};
  }} onSave={document=>output("save",document)} onSaveAs={document=>output("saveAs",document)} onCopy={async document=>{await output("copy",document);}} onPin={async()=>{}} onSaveAndCopy={document=>output("saveAndCopy",document)} onRetake={async()=>{}}/>}</>;
}
createRoot(document.getElementById("root")!).render(<Fixture/>);
