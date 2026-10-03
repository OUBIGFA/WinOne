import {InlineError} from "../ui/notice";
import {useEffect,useRef,useState,type CSSProperties} from "react";
import {Minus,Square,Copy,X} from "lucide-react";
import {Button} from "./ui/button";
import {Tooltip,TooltipContent,TooltipTrigger} from "./ui/tooltip";
import {useTranslation} from "../i18n";

export interface WindowFramePort {
 initialize():Promise<{maximized:boolean}>;
 read():Promise<{maximized:boolean}>;
 command(action:"minimize"|"toggle-maximize"|"close"):Promise<void>;
}

// Window actions use the same native close guard as Alt+F4. The draggable
// region is separate from buttons so a click never becomes a window drag.
export function WindowTitlebar({logo,port,onReady}:{logo:string;port?:WindowFramePort;onReady?:()=>void}){
 const {t}=useTranslation();const [maximized,setMaximized]=useState(false),[busy,setBusy]=useState(false),[error,setError]=useState("");
 const alive=useRef(true),ready=useRef(onReady);ready.current=onReady;
 useEffect(()=>{
  alive.current=true;if(!port){ready.current?.();return()=>{alive.current=false;};}
  void port.initialize().then(state=>{if(alive.current){setMaximized(state.maximized);ready.current?.();}}).catch(error=>{if(alive.current)setError(String(error));});
  let timer:ReturnType<typeof setTimeout>|undefined;
  const update=()=>{clearTimeout(timer);timer=setTimeout(()=>void port.read().then(state=>{if(alive.current)setMaximized(state.maximized);}).catch(error=>{if(alive.current)setError(String(error));}),75);};
  window.addEventListener("resize",update);window.addEventListener("focus",update);
  return()=>{alive.current=false;clearTimeout(timer);window.removeEventListener("resize",update);window.removeEventListener("focus",update);};
 },[port]);
 async function act(action:"minimize"|"toggle-maximize"|"close"){
  if(!port||busy)return;setBusy(true);setError("");
  try{await port.command(action);if(action!=="close"){const state=await port.read();if(alive.current)setMaximized(state.maximized);}}
  catch(error){if(alive.current)setError(String(error));}finally{if(alive.current)setBusy(false);}
 }
 const controls=[{action:"minimize",label:"window.minimize",Icon:Minus},{action:"toggle-maximize",label:maximized?"window.restore":"window.maximize",Icon:maximized?Copy:Square},{action:"close",label:"window.close",Icon:X}] as const;
 return <><div data-slot="window-titlebar" className="flex h-10 shrink-0 items-center border-b bg-background">
  <div className="flex h-full min-w-0 flex-1 select-none items-center gap-2 px-4" style={{"--wails-draggable":"drag"} as CSSProperties} onDoubleClick={()=>void act("toggle-maximize")}>
   <img className="mark size-6" src={logo} alt="" draggable={false}/><span className="text-sm font-medium">WinOne</span>
  </div>
  <div data-slot="window-controls" className="flex h-full shrink-0" style={{"--wails-draggable":"no-drag"} as CSSProperties}>
   {controls.map(({action,label,Icon})=><Tooltip key={action}><TooltipTrigger render={<Button variant="ghost" className={`h-10 w-11 rounded-none ${action==="close"?"hover:bg-destructive/10 hover:text-destructive":""}`} aria-label={t(label)} disabled={!port||busy} onClick={()=>void act(action)}/>}><Icon className="size-4" aria-hidden="true"/></TooltipTrigger><TooltipContent>{t(label)}</TooltipContent></Tooltip>)}
  </div>
 </div>{error&&<InlineError className="m-0 px-4 py-2 text-sm">{error}</InlineError>}</>;
}
