import {useEffect,useLayoutEffect,useRef,useState} from "react";
import {createRoot} from "react-dom/client";
import {Square,MoveUpRight,Grid2X2,Blend,ScanLine,Undo2,Redo2,Save,X,Type,Check} from "lucide-react";
import {CaptureInlineText,type InlineTextHandle} from "../src/features/capture-inline-text";
import {Button} from "../src/components/ui/button";
import {InlineError} from "../src/ui/notice";
import "../src/styles/shadcn.css";
import "./native-toolbar.css";

type TextDraft={value:string;fontPixels:number;existing:boolean;id:number;x:number;y:number;width:number;height:number;imageWidth:number;imageHeight:number};
type State={nativeControls?:boolean;kind:"state";requestId:number;nativeWindow:string;annotations:number;canUndo:boolean;canRedo:boolean;tool:string;textTool:boolean;copied:boolean;error?:string;viewWidth:number;toolbar:{x:number;y:number;width:number;height:number};text?:TextDraft|null};
type Bridge={postMessage(message:string):void;addEventListener(type:"message",listener:(event:{data:unknown})=>void):void;removeEventListener(type:"message",listener:(event:{data:unknown})=>void):void};
const bridge=(window as unknown as {chrome:{webview:Bridge}}).chrome.webview;
const tools=[{id:"region",name:"选区",icon:ScanLine,native:"RegionRectangle"},{id:"rectangle",name:"矩形",icon:Square,native:"DrawingRectangle"},{id:"arrow",name:"箭头",icon:MoveUpRight,native:"DrawingArrow"},{id:"mosaic",name:"马赛克",icon:Grid2X2,native:"EffectPixelate"},{id:"blur",name:"模糊",icon:Blend,native:"EffectBlur"}];
function Toolbar(){
 const [state,setState]=useState<State>(),[busy,setBusy]=useState(false);
 const inline=useRef<InlineTextHandle>(null),afterText=useRef<string|undefined>(undefined);
 const sequence=useRef(0),pending=useRef<number|undefined>(undefined);
 function send(command:string,tool?:string,extra:Record<string,unknown>={}){const id=++sequence.current;pending.current=id;setBusy(true);bridge.postMessage(JSON.stringify({id,command,tool,...extra}));}
 useEffect(()=>{
  const receive=({data}:{data:unknown})=>{if((data as {kind?:string})?.kind==="commit-text"){afterText.current=(data as {after?:string}).after;inline.current?.commit();return;}const value=data as State;if(!value||value.kind!=="state"||typeof value.nativeWindow!=="string"||typeof value.canUndo!=="boolean"||typeof value.canRedo!=="boolean")return;setState(value);if(value.requestId===pending.current){pending.current=undefined;setBusy(false);}};
  bridge.addEventListener("message",receive);bridge.postMessage(JSON.stringify({id:0,command:"ready"}));
  return()=>bridge.removeEventListener("message",receive);
 },[]);
 const scale=state?.viewWidth?window.innerWidth/state.viewWidth:1;
 useLayoutEffect(()=>{
  if(!state?.text)return;
  const input=document.querySelector("textarea");if(!input)return;
  const session=state.text.id;
  const observer=new ResizeObserver(()=>bridge.postMessage(JSON.stringify({id:0,command:"text.resize",session,height:Math.ceil(input.getBoundingClientRect().height/scale)})));
  observer.observe(input);return()=>observer.disconnect();
 },[state?.text?.id,scale]);
 function finish(action:"save"|"confirm"){if(state?.text){afterText.current=action;inline.current?.commit();}else send(action);}
 function save(){finish("save");}
 function shortcut(event:KeyboardEvent){
  const input=event.target instanceof HTMLTextAreaElement;
  const control=event.ctrlKey&&!event.altKey&&!event.metaKey;
  const key=event.key.toLowerCase();
  if(control&&key==="s"){event.preventDefault();if(!busy&&!state?.copied)save();return;}
  // Text selection, IME and text undo remain owned by the inline editor.
  if(input||event.isComposing)return;
  if(event.key==="Escape"){event.preventDefault();if(!busy)send("cancel");}
  else if(control&&["z","y","c"].includes(key)){
   event.preventDefault();if(busy)return;
   if(key==="c")finish("confirm");
   else if(!state?.copied&&!state?.text){
    const redo=key==="y"||event.shiftKey;
    if(redo?state?.canRedo:state?.canUndo)send(redo?"redo":"undo");
   }
  }
 }
 // Commands belong to this capture window, including when a disabled button releases focus to body.
 useEffect(()=>{window.addEventListener("keydown",shortcut,true);return()=>window.removeEventListener("keydown",shortcut,true);},[state,busy]);
 if(!state)return null;
 return <div className="relative h-screen w-screen" data-native-window={state.nativeWindow} data-annotations={state.annotations} onContextMenu={event=>{event.preventDefault();send("cancel");}}>
 {!state.nativeControls&&<div className="absolute rounded-xl border bg-popover p-2 text-popover-foreground" style={{left:state.toolbar.x*scale,top:state.toolbar.y*scale,width:state.toolbar.width*scale}}>
  <div className="flex gap-2" role="group" aria-label="截图操作">
   <Button disabled={busy} onPointerDown={event=>event.preventDefault()} onClick={()=>finish("confirm")}><Check/>确认</Button>
   <Button disabled={!state||busy||state.copied} onPointerDown={event=>event.preventDefault()} onClick={save}><Save/>保存</Button>
   <Button variant="outline" disabled={!state?.canUndo||busy||!!state.text||state.copied} onClick={()=>send("undo")}><Undo2/>撤销</Button>
   <Button variant="outline" disabled={!state?.canRedo||busy||!!state.text||state.copied} onClick={()=>send("redo")}><Redo2/>重做</Button>
   <Button variant="ghost" disabled={busy} onClick={()=>send("cancel")}><X/>取消</Button>
  </div>
  <div className="mt-2 flex gap-2" role="group" aria-label="标注工具">{tools.map(tool=><Button key={tool.id} variant={!state.textTool&&state.tool===tool.native?"default":"outline"} aria-pressed={!state.textTool&&state.tool===tool.native} disabled={!state||busy||!!state.text||state.copied} onClick={()=>send("tool",tool.id)}><tool.icon/>{tool.name}</Button>)}<Button variant={state.textTool?"default":"outline"} aria-pressed={state.textTool} disabled={busy||!!state.text||state.copied} onClick={()=>send("text.begin")}><Type/>文字</Button></div>
  {state?.error&&<InlineError>{state.error}</InlineError>}
 </div>}
 {state.text&&<CaptureInlineText key={state.text.id} ref={inline} mark={{kind:"text",x:state.text.x,y:state.text.y,width:state.text.width,height:state.text.height,fontSize:state.text.fontPixels,strokeWidth:1,color:"#e23b30",text:state.text.value}} scale={scale} image={{width:state.text.imageWidth,height:state.text.imageHeight}} label="输入文字" hint="Ctrl+Enter 完成文字" onCommit={mark=>{// Empty text still completes the pending save or confirmation; only Escape cancels the edit.
 send("text.commit",undefined,{session:state.text!.id,text:mark?.text??"",height:mark?Math.ceil(mark.height!):state.text!.height,after:afterText.current});afterText.current=undefined;}} onCancel={()=>{afterText.current=undefined;send("text.cancel",undefined,{session:state.text!.id});}}/>}
 </div>;
}
createRoot(document.getElementById("root")!).render(<Toolbar/>);
