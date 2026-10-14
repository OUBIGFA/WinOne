import {useEffect,useState} from "react";
import {Save,RotateCcw} from "lucide-react";
import {captureCall} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {Notice} from "../ui/notice";
import {Label} from "../components/ui/label";
import {Textarea} from "../components/ui/textarea";
import {CaptureOptionSelect} from "../ui/capture-option-select";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";
import {AIModelSelect} from "./ai-model-select";
import type {AIConfiguration,AISelection} from "./ai-configuration";

// These controls edit the existing shared bindings. Provider credentials and
// model collections are still owned exclusively by the AI services page.
export function CaptureAISettings({onActivity}:{onActivity?:PageActivityChanged}) {
 const {t}=useTranslation();
 const [draft,setDraft]=useState<AIConfiguration>(),[saved,setSaved]=useState<AIConfiguration>();
 const [working,setBusy]=useState(false),[loading,setLoading]=useState(false),[error,setError]=useState(""),[notice,setNotice]=useState("");
 const busy=working||loading;
 const dirty=!!draft&&JSON.stringify(draft)!==JSON.stringify(saved);
 // Protect saves and edits; a slow read can safely finish after leaving.
 usePageActivity(onActivity,dirty,working,"capture-ai");
 async function load(){setLoading(true);setError("");setNotice("");try{const value=await captureCall<AIConfiguration>("ai.configuration.get");setDraft(value);setSaved(value);}catch(error){setError(String(error));}finally{setLoading(false);}}
 useEffect(()=>{void load();},[]);
 async function save(){setBusy(true);setError("");setNotice("");try{const value=await captureCall<AIConfiguration>("ai.configuration.save",{configuration:draft});setDraft(value);setSaved(value);setNotice(t("common.saved"));}catch(error){setError(String(error));}finally{setBusy(false);}}
 function change(task:string,value?:AISelection){if(!draft)return;const tasks={...draft.tasks};if(value)tasks[task]=value;else delete tasks[task];setDraft({...draft,tasks});}
 const select=(task:string)=><AIModelSelect label={t(`ai.${task}Task`)} providers={draft!.providers} value={draft!.tasks[task]} inheritLabel={t("ai.followDefaultService")} disabled={busy} onChange={value=>change(task,value)}/>;
 return <div className="space-y-5">
  {error&&<Notice actions={!dirty&&<Button disabled={busy} onClick={()=>void load()}>{t("common.retry")}</Button>}>{error}</Notice>}
  {!draft&&!error&&<p role="status">{t("capture.loading")}</p>}
  {draft&&<fieldset className="plain-fieldset space-y-5" disabled={busy}>
   <section className="settings-group" aria-label={t("ai.recognitionGroup")}><h3 className="settings-group-title">{t("ai.recognitionGroup")}</h3>
   <CaptureOptionSelect stacked label={t("ai.ocrEngine")} value={draft.ocrEngine} disabled={busy} items={[{value:"local",label:t("ai.localOCR")},{value:"ai",label:t("ai.sharedModel")}]} onChange={ocrEngine=>setDraft({...draft,ocrEngine})}/>
   {draft.ocrEngine==="ai"&&<>{select("ocr")}<p className="muted">{t("ai.visionHint")}</p></>}
   </section>
   <section className="settings-group" aria-label={t("ai.translationGroup")}><h3 className="settings-group-title">{t("ai.translationGroup")}</h3>
   <CaptureOptionSelect stacked label={t("ai.engine")} value={draft.engine} disabled={busy} items={[{value:"google",label:t("ai.builtin")},{value:"llm",label:t("ai.sharedModel")}]} onChange={engine=>setDraft({...draft,engine})}/>
   {draft.engine==="llm"?select("translate"):<p className="muted">{t("ai.online")}</p>}
   <CaptureOptionSelect stacked label={t("ai.target")} value={draft.target} disabled={busy} items={[['auto','auto'],['zh-CN','chinese'],['zh-TW','traditional'],['en','english'],['ja','japanese'],['ko','korean']].map(([value,key])=>({value,label:t(`ai.${key}`)}))} onChange={target=>setDraft({...draft,target})}/>
   {draft.engine==="llm"&&<div className="space-y-2"><Label htmlFor="capture-ai-prompt">{t("ai.prompt")}</Label><Textarea id="capture-ai-prompt" value={draft.prompt} placeholder={t("ai.promptDefault")} onChange={event=>setDraft({...draft,prompt:event.target.value})}/><p className="muted">{t("ai.promptHint")}</p></div>}
   </section>
   <section className="settings-group" aria-label={t("ai.conversationGroup")}><h3 className="settings-group-title">{t("ai.conversationGroup")}</h3>{select("chat")}</section>
   <div className="detail-actions"><Button icon={Save} variant="primary" disabled={!dirty||busy} onClick={()=>void save()}>{t("capture.save")}</Button>{dirty&&<Button icon={RotateCcw} disabled={busy} onClick={()=>void load()}>{t("shortcuts.undo")}</Button>}</div>
   {notice&&!dirty&&<p role="status">{notice}</p>}
  </fieldset>}
 </div>;
}
