import {InlineError} from "./notice";
import {useEffect,useRef,useState,type KeyboardEvent} from "react";
import {Input} from "../components/ui/input";
import {Button} from "../components/ui/button";
import {useTranslation} from "../i18n";
import {recordedShortcut} from "./shortcut-keys";

export interface ShortcutRecordingPort {begin():Promise<string>;end(token:string):Promise<void>}

// Native registration is released before accepting keys and restored before
// committing the draft. Blur, cancellation and unmount all share the same exit.
export function ShortcutRecorder({id,value,onChange,disabled=false,onRecordingChange,port,showHint=true}:{id:string;value:string;onChange:(value:string)=>void;disabled?:boolean;onRecordingChange?:(recording:boolean)=>void;port:ShortcutRecordingPort;showHint?:boolean}){
 const {t}=useTranslation();
 const [phase,setPhase]=useState("idle"),[preview,setPreview]=useState(""),[error,setError]=useState("");
 const [notice,setNotice]=useState("");
 const input=useRef<HTMLInputElement>(null),active=useRef(false),alive=useRef(true),finishing=useRef(false);
 const pending=useRef<Promise<string>|null>(null),timer=useRef<ReturnType<typeof setTimeout>|undefined>(undefined);
 const candidate=useRef<string|undefined>(undefined);
 const clearRequested=useRef(false);
 const callbacks=useRef({onChange,onRecordingChange});callbacks.current={onChange,onRecordingChange};
 async function finish(next?:string,message=""){
  if(finishing.current||!pending.current)return;
  finishing.current=true;active.current=false;clearTimeout(timer.current);
  if(alive.current)setPhase("finishing");
  try{const token=await pending.current;await port.end(token);if(alive.current){if(next!==undefined&&!clearRequested.current)callbacks.current.onChange(next);setError("");setNotice(message);}}
  catch(error){if(alive.current)setError(String(error));}
  finally{pending.current=null;finishing.current=false;if(alive.current){setPhase("idle");setPreview("");input.current?.blur();}callbacks.current.onRecordingChange?.(false);}
 }
 async function start(){
  if(disabled||active.current||finishing.current)return;
  active.current=true;candidate.current=undefined;clearRequested.current=false;setPhase("preparing");setError("");setNotice("");setPreview("");callbacks.current.onRecordingChange?.(true);
  const request=Promise.resolve().then(()=>port.begin());pending.current=request;
  try{await request;if(!alive.current||!active.current)return;setPhase("recording");timer.current=setTimeout(()=>void finish(undefined,t("shortcuts.recordTimeout")),20000);}
  catch(error){if(alive.current)setError(String(error));active.current=false;pending.current=null;if(alive.current)setPhase("idle");callbacks.current.onRecordingChange?.(false);}
 }
 useEffect(()=>{
  alive.current=true;
  const blur=()=>void finish();
  const visibility=()=>{if(document.hidden)void finish();};
  window.addEventListener("blur",blur);document.addEventListener("visibilitychange",visibility);
  return()=>{window.removeEventListener("blur",blur);document.removeEventListener("visibilitychange",visibility);alive.current=false;void finish();};
 },[port]);
 function keyDown(event:KeyboardEvent<HTMLInputElement>){
  if(event.key==="Tab"){void finish();return;}
  if(!active.current)return;
  event.preventDefault();event.stopPropagation();
  if(event.key==="Escape"){void finish();return;}
  if(phase!=="recording")return;
  if((event.key==="Delete"||event.key==="Backspace")&&!event.ctrlKey&&!event.altKey&&!event.shiftKey&&!event.metaKey){void finish("");return;}
  const result=recordedShortcut({...event.nativeEvent,key:event.key,code:event.code,ctrlKey:event.ctrlKey,altKey:event.altKey,shiftKey:event.shiftKey,metaKey:event.metaKey,repeat:event.repeat,isComposing:event.nativeEvent.isComposing,altGraph:event.getModifierState("AltGraph")});
  if(result.kind==="modifier"){setPreview(result.value);return;}
  if(result.kind==="shortcut"){candidate.current=result.value;setPreview(result.value);setError("");return;}
  if(result.kind==="invalid")setError(t(`shortcuts.recordError.${result.reason}`));
 }
 function keyUp(event:KeyboardEvent<HTMLInputElement>){
  // Re-register only after modifiers are released, so the recorded chord
  // cannot immediately trigger an existing WinOne action while still held.
  if(active.current&&candidate.current!==undefined&&!event.ctrlKey&&!event.altKey&&!event.shiftKey&&!event.metaKey){event.preventDefault();event.stopPropagation();void finish(candidate.current);}
 }
 return <div>
  <div className="flex max-w-lg items-center gap-2"><Input ref={input} id={id} readOnly disabled={disabled} aria-describedby={`${id}-record-help${error?` ${id}-record-error`:""}`} value={phase==="recording"?preview:phase==="preparing"?"":value} placeholder={t(phase==="recording"?"shortcuts.pressKeys":"shortcuts.recordPlaceholder")} onFocus={()=>void start()} onClick={()=>void start()} onBlur={()=>void finish()} onKeyDown={keyDown} onKeyUp={keyUp}/>
   <Button variant="outline" disabled={disabled||!value} onPointerDown={event=>event.preventDefault()} onClick={()=>{clearRequested.current=true;callbacks.current.onChange("");if(pending.current)void finish();}}>{t("shortcuts.clear")}</Button>
  </div>
  {/* Repeated forms share the idle hint; live recording feedback stays local. */}
  <p id={`${id}-record-help`} className={!showHint&&phase==="idle"&&!notice?"sr-only":"mt-2 text-sm text-muted-foreground"} role="status">{phase==="idle"&&notice?notice:t(phase==="preparing"?"shortcuts.recordPreparing":phase==="recording"?"shortcuts.recordInstructions":"shortcuts.recordHint")}</p>
  {error&&<InlineError id={`${id}-record-error`}>{error}</InlineError>}
 </div>;
}
