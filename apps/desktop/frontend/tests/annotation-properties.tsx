// Isolated real properties panel and model; no native capture or user settings.
import {createRoot} from "react-dom/client";
import {useState} from "react";
import {setApplicationLanguage} from "../src/i18n";
import {AnnotationProperties} from "../src/features/capture-annotation-properties";
import {createHistory,commitMark,replaceMark,type AnnotationMark} from "../src/features/capture-editor-model";
import "../src/styles/shadcn.css";
import "../src/style.css";
const parameters=new URLSearchParams(location.search);
const mark:AnnotationMark={kind:parameters.has("effect")?"mosaic":"text",x:10,y:20,width:100,height:50,color:"#ff0000",strokeWidth:3,text:"WinOne",fontSize:24,effectSize:12};
function Fixture(){
 const [history,setHistory]=useState(()=>commitMark(createHistory(400,300),mark)),[open,setOpen]=useState(true);
 return <main className="flex justify-end p-4">{open&&<AnnotationProperties mark={mark} onApply={next=>setHistory(current=>replaceMark(current,0,next))} onClose={()=>setOpen(false)}/>}<output hidden id="result">{JSON.stringify(history.document.marks[0])}</output></main>;
}
void setApplicationLanguage(parameters.get("language")||"zh-CN");
createRoot(document.getElementById("root")!).render(<Fixture/>);
