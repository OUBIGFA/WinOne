// Synthetic native boundary for validating the real workspace and controller.
// Ctrl+Shift+X simulates a native window close request; first Save is cancelled.
import {useEffect,useRef,useState} from "react";
import {createRoot} from "react-dom/client";
import {CaptureDraftWorkspace} from "../src/features/capture-draft-workspace";
import type {CaptureDraft} from "../src/features/capture-draft-api";
import "../src/style.css";

const initial:CaptureDraft={id:"a".repeat(32),revision:3,width:3200,height:1600,previewReady:true,document:{version:1,width:3200,height:1600,marks:[{kind:"rectangle",x:200,y:200,width:800,height:400,color:"#ff0000",strokeWidth:12}]}};
type NativeApp=NonNullable<NonNullable<Window["go"]>["main"]>["App"];
function Fixture() {
 const current=useRef<CaptureDraft|null>(structuredClone(initial));
 const closePending=useRef(false),listeners=useRef(new Set<()=>void>()),saveCount=useRef(0),copyCount=useRef(0);
 const [open,setOpen]=useState(true),[report,setReport]=useState("");
 useState(()=>{
  window.runtime={EventsOn:(_name,listener)=>{listeners.current.add(listener);return()=>listeners.current.delete(listener);}};
  window.go={main:{App:{
   Capture:async(method:string,payload:string)=>{
    const params=JSON.parse(payload);
    if(method==="draft.get")return JSON.stringify({draft:current.current});
    if(!current.current||params.expectedId!==current.current.id)throw Error("Unknown fixture draft");
    if(method==="draft.update") {
     if(params.revision!==current.current.revision)throw Error("Revision conflict");
     current.current={...current.current,revision:params.revision+1,document:params.document};
     setReport(JSON.stringify(current.current));
     return JSON.stringify({state:"completed",result:{draftId:initial.id,revision:current.current.revision}});
    }
    if(method==="edit.preview")return JSON.stringify({state:"completed",result:{id:"b".repeat(32),sourceId:initial.id,width:3200,height:1600}});
    if(method==="draft.copy"||method==="draft.pin") {
     if(method==="draft.copy"&&copyCount.current++===0)return JSON.stringify({state:"failed",error:"Clipboard fixture is busy"});
     if(params.revision!==current.current.revision)throw Error("Revision conflict");
     setReport(method+" copyAttempt="+copyCount.current+" "+JSON.stringify(current.current));
     return JSON.stringify({state:"completed",result:{draftId:initial.id,revision:current.current.revision,copied:method==="draft.copy",pinned:method==="draft.pin"}});
    }
    if(method==="draft.complete") {
     if(saveCount.current++===0)return JSON.stringify({state:"cancelled"});
     setReport("SAVED "+JSON.stringify(current.current));current.current=null;
     return JSON.stringify({state:"completed",result:{id:"saved"}});
    }
    if(method==="draft.cancel"){current.current=null;setReport("DISCARDED");return JSON.stringify({state:"completed"});}
    throw Error(method);
   },
   SetCaptureEditing:async()=>{},CaptureClosePending:async()=>closePending.current,CancelCaptureClose:async()=>{closePending.current=false;},
  } as unknown as NativeApp}};
  return true;
 });
 useEffect(()=>{
  const close=(event:KeyboardEvent)=>{if(event.ctrlKey&&event.shiftKey&&event.key.toLowerCase()==="x"){event.preventDefault();closePending.current=true;listeners.current.forEach(listener=>listener());}};
  document.addEventListener("keydown",close);return()=>document.removeEventListener("keydown",close);
 },[]);
 return <><pre id="draft-result">{report}</pre>{open?<CaptureDraftWorkspace draft={initial} onDone={()=>setOpen(false)}/>:<p>WORKSPACE CLOSED</p>}</>;
}
createRoot(document.getElementById("root")!).render(<Fixture/>);
