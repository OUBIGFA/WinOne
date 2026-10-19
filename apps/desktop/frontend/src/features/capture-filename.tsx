import {InlineError} from "../ui/notice";
import {FileText} from "lucide-react";
import {useEffect,useRef,useState} from "react";
import {captureCall} from "../bridge";
import {Input} from "../components/ui/input";
import {SettingsSection} from "../components/settings-layout";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";

export function CaptureFileName({value,onChange}:{value:string;onChange:(value:string)=>void}) {
 const {t}=useTranslation();
 const [requested,setRequested]=useState(value!==""),[name,setName]=useState(""),[error,setError]=useState("");
 const [attempt,setAttempt]=useState(0),input=useRef<HTMLInputElement>(null);
 useEffect(()=>{
  if(!requested)return;
  let active=true;setName("");setError("");
  const timer=setTimeout(()=>{void captureCall<{name:string}>("filename.preview",{template:value}).then(result=>{
   if(typeof result?.name!=="string"||!result.name.trim())throw Error(t("capture.fileNameFailed"));if(active)setName(result.name);
  }).catch(error=>{if(active)setError(error instanceof Error?error.message:String(error));});},150);
  return()=>{active=false;clearTimeout(timer);};
 },[value,requested,t,attempt]);
 return <SettingsSection icon={FileText} title={t("capture.fileNameTemplate")} headingId="capture-filename-heading">
  <Input ref={input} id="capture-file-name" aria-labelledby="capture-filename-heading" value={value} placeholder="WinOne-{timestamp}-OK" aria-describedby={`capture-file-name-hint${error?" capture-file-name-error":""}`} aria-invalid={!!error} onFocus={()=>setRequested(true)} onChange={event=>{setRequested(true);onChange(event.target.value);}}/>
  <p id="capture-file-name-hint" className="text-sm text-muted-foreground">{t("capture.fileNameHint")}</p>
  {name&&<p className="break-all text-sm" role="status">{t("capture.fileNameExample")}: {name}</p>}
  {error&&<><InlineError id="capture-file-name-error">{error}</InlineError><Button onClick={()=>{setAttempt(value=>value+1);input.current?.focus({preventScroll:true});}}>{t("common.retry")}</Button></>}
 </SettingsSection>;
}
