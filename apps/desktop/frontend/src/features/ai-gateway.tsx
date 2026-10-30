import {SettingsSaveBar} from "../components/settings-save-bar";
import {useEffect,useState} from "react";
import {Copy,Eye,EyeOff,RefreshCw,RotateCcw,Save,Server,Plug} from "lucide-react";
import {captureCall} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {Notice} from "../ui/notice";
import {Input} from "../components/ui/input";
import {Label} from "../components/ui/label";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";
import type {AIProvider,AISelection} from "./ai-configuration";
import {AIGatewayModels} from "./ai-gateway-models";
import {SettingsSection,SettingsDisclosure,SettingsToggle} from "../components/settings-layout";

interface GatewayStatus {revision:number;enabled:boolean;port:number;running:boolean;hasKey:boolean;baseUrl:string;error?:string;models?:AISelection[]|null}
export function AIGateway({active,providers,onActivity}:{active:boolean;providers:AIProvider[];onActivity?:PageActivityChanged}){
 const {t}=useTranslation();
 const [saved,setSaved]=useState<GatewayStatus>(),[enabled,setEnabled]=useState(false),[port,setPort]=useState("3426");
 const [models,setModels]=useState<AISelection[]|null>(null);
 const [working,setBusy]=useState(false),[loading,setLoading]=useState(false),[error,setError]=useState(""),[notice,setNotice]=useState(""),[key,setKey]=useState("");
 const busy=working||loading;
 const dirty=!!saved&&(saved.enabled!==enabled||String(saved.port)!==port||JSON.stringify(models)!==JSON.stringify(saved.models??null));
 usePageActivity(onActivity,dirty,working,"ai-gateway",()=>{if(saved)accept(saved);});
 function accept(value:GatewayStatus){setSaved(value);setEnabled(value.enabled);setPort(String(value.port));setModels(value.models??null);setKey("");}
 // Reading status blocks this form, but never owns navigation or window close.
 async function load(){setLoading(true);setError("");try{accept(await captureCall<GatewayStatus>("ai.gateway.get"));}catch(e){setError(String(e));}finally{setLoading(false);}}
 useEffect(()=>{if(active&&!saved&&!busy&&!error)void load();},[active,saved,busy,error]);
 async function save(resetKey=false){
  if(!saved)return;
  const number=Number(port);
  if(!/^[0-9]+$/.test(port)||!Number.isInteger(number)||number<1024||number>65535){setError(t("gateway.portError"));return;}
  setBusy(true);setError("");setNotice("");
  try{accept(await captureCall<GatewayStatus>("ai.gateway.save",{settings:{revision:saved.revision,enabled,port:number,models},resetKey}));setNotice(t("common.saved"));}catch(e){setError(String(e));}finally{setBusy(false);}
 }
 async function accessKey(copy=false){setBusy(true);setError("");setNotice("");try{const value=await captureCall<{key:string}>("ai.gateway.key");if(copy){try{await navigator.clipboard.writeText(value.key);setNotice(t("gateway.copied"));}catch{setKey(value.key);setError(t("gateway.copyFailed"));}}else setKey(value.key);}catch(e){setError(String(e));}finally{setBusy(false);}}
 return <div className="settings-stack">
  <p className="muted">{t("gateway.hint")}</p>
  {error&&<Notice actions={!saved&&<Button disabled={busy} onClick={()=>void load()}>{t("common.retry")}</Button>}>{error}</Notice>}
  {!saved&&!error&&<p role="status">{t("gateway.loading")}</p>}
  {saved&&<>
   {saved.error&&<Notice>{saved.error}</Notice>}
   <fieldset className="plain-fieldset settings-section-stack" disabled={busy}>
    <SettingsSection icon={Server} title={t("settingsDesign.gatewayRun")}>
    <SettingsToggle id="gateway-enabled" label={t("gateway.enable")} checked={enabled} onChange={setEnabled}/>
    <div className="space-y-2"><Label htmlFor="gateway-port">{t("gateway.port")}</Label><Input id="gateway-port" className="max-w-40" inputMode="numeric" value={port} onChange={e=>setPort(e.target.value)}/></div>
    <p className="muted">{t("gateway.localHint")}</p>
    <div className="settings-switch-row"><span>{t("gateway.status")}: {t(saved.running?"gateway.running":"gateway.stopped")}</span><Button icon={RefreshCw} disabled={dirty} onClick={()=>void load()}>{t("common.refresh")}</Button></div>
    </SettingsSection>
    <SettingsSection icon={Plug} title={t("settingsDesign.gatewayConnect")}>
    <div className="space-y-2"><Label htmlFor="gateway-address">{t("gateway.address")}</Label><Input id="gateway-address" readOnly value={saved.baseUrl}/></div>
    <div className="space-y-2"><Label htmlFor="gateway-key">{t("gateway.key")}</Label><Input id="gateway-key" readOnly value={key} placeholder={saved.hasKey?"••••••••":""}/><div className="flex flex-wrap gap-2"><Button icon={key?EyeOff:Eye} disabled={!saved.hasKey} onClick={()=>key?setKey(""):void accessKey()}>{t(key?"gateway.hideKey":"gateway.showKey")}</Button><Button icon={Copy} disabled={!saved.hasKey} onClick={()=>void accessKey(true)}>{t("gateway.copyKey")}</Button><Button icon={RotateCcw} disabled={dirty} onClick={()=>void save(true)}>{t("gateway.resetKey")}</Button></div><p className="muted">{t("gateway.keyHint")}</p></div>
    </SettingsSection>
    <AIGatewayModels providers={providers} value={models} disabled={busy} onChange={setModels}/>
    <SettingsSaveBar undo={{disabled:!dirty,onClick:()=>accept(saved)}}><Button icon={Save} variant="primary" disabled={!dirty&&saved.running===saved.enabled&&!saved.error} onClick={()=>void save()}>{t("gateway.save")}</Button></SettingsSaveBar>
   </fieldset>
   <SettingsDisclosure title={t("settingsDesign.gatewayInterfaces")}><p className="settings-help">{t("gateway.protocols")}</p></SettingsDisclosure>
  </>}
  {notice&&!dirty&&<p role="status">{notice}</p>}
 </div>;
}
