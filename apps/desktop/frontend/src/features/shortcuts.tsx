import {SettingsSaveBar} from "../components/settings-save-bar";
import {Save,RotateCcw,RefreshCw} from "lucide-react";
import {Notice} from "../ui/notice";
import { useEffect, useState, type ReactNode } from "react";
import { getSnapshot, getShortcuts, saveSettings, shortcutRecordingPort, type Settings, type ShortcutStatus } from "../bridge";
import { useTranslation } from "../i18n";
import { Button } from "../ui/button";
import {ShortcutRecorder} from "../ui/shortcut-recorder";
import {Label} from "../components/ui/label";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";

export function Shortcuts({ onSaved,moduleId,onActivity,renderLayout }: { onSaved: () => void;moduleId:"application"|"capture"|"selection"|"quick-assistant"|"always-on-top";onActivity?:PageActivityChanged;renderLayout?:(fields:ReactNode,actions:ReactNode)=>ReactNode }) {
  const {t}=useTranslation();
  const [draft,setDraft]=useState<Settings>();
  const [savedSettings,setSavedSettings]=useState<Settings>();
  const [status,setStatus]=useState<ShortcutStatus>();
  const [error,setError]=useState("");
  const [working,setBusy]=useState(false),[loading,setLoading]=useState(false);
  const busy=working||loading;
  const [notice,setNotice]=useState("");
  const fields=moduleId==="application"?([['openShortcut','shortcuts.open','open-winone']] as const):moduleId==="capture"?([['captureShortcut','shortcuts.capture','capture'],['ocrShortcut','shortcuts.ocr','ocr'],['translateShortcut','shortcuts.translate','translate']] as const):moduleId==="quick-assistant"?([['quickAssistantShortcut','shortcuts.quickAssistant','quick-assistant']] as const):moduleId==="selection"?([['selectionShortcut','shortcuts.selection','selection']] as const):([['pinShortcut','shortcuts.pin','pin']] as const);
  const dirty=!!draft&&fields.some(([key])=>(draft[key]??"")!==(savedSettings?.[key]??""));
  const [recording,setRecording]=useState(false);
  // Reading bindings is safe to leave; recording and saving still need the
  // existing guard because they change registrations or persisted settings.
  usePageActivity(onActivity,dirty,working||recording,"shortcuts");
  const registrationError=status?.bindings.find(binding=>fields.some(([, ,id])=>id===binding.id)&&binding.error)?.error;
  async function refresh() {
    try {setStatus(await getShortcuts());}
    catch(error){setStatus(undefined);throw error;}
  }
  async function reload() {
    setLoading(true);setError("");setNotice("");
    try {const [snapshot,next]=await Promise.all([getSnapshot(),getShortcuts()]);const settings={...snapshot.settings,openShortcut:snapshot.settings.openShortcut??"Alt+A",ocrShortcut:snapshot.settings.ocrShortcut??"Shift+C",translateShortcut:snapshot.settings.translateShortcut??"Alt+Shift+A",quickAssistantShortcut:snapshot.settings.quickAssistantShortcut??""};setDraft(settings);setSavedSettings(settings);setStatus(next);}
    catch(error){setError(String(error));}finally{setLoading(false);}
  }
  useEffect(()=>{void reload();},[moduleId]);
  async function save() {
    if(!draft)return;
    setBusy(true);setError("");setNotice("");
    // A later status read can fail independently of persistence. Publish the
    // successful save before refreshing registration, without resubmitting it.
    try {const saved=await saveSettings(draft);setDraft(saved);setSavedSettings(saved);setNotice("common.saved");onSaved();await refresh();}
    catch(e){setError(String(e));}finally{setBusy(false);}
  }
  const form=<>
    {draft && <fieldset className="plain-fieldset" disabled={busy}>
      <p className="shortcut-settings-hint">{t("shortcuts.recordHint")}</p>
      <div className="shortcut-settings-list">
      {fields.map(([key,label,id])=>{
        const state=status?.bindings.find(b=>b.id===id);
        return <div key={key} className="shortcut-setting">
          <div className="shortcut-setting-copy"><Label htmlFor={`shortcut-${id}`}>{t(label)}</Label>
          <p className="shortcut-setting-status">{recording?t("shortcuts.pausedForRecording"):!status?t("shortcuts.unknown"):state?.error || (state?.registered ? t("settingsDesign.shortcutActive") : t("shortcuts.disabled"))}</p></div>
          <div className="min-w-0"><ShortcutRecorder id={`shortcut-${id}`} showHint={false} value={draft[key]??""} disabled={busy} port={shortcutRecordingPort} onRecordingChange={active=>{setRecording(active);if(active)setNotice("");else void refresh().catch(error=>setError(String(error)));}} onChange={value=>setDraft({...draft,[key]:value})} /></div>
        </div>;
      })}
      </div>
    </fieldset>}
  </>;
  // Keep the save transaction alongside this editor; the shared component
  // renders it in the shell footer even when the composed tab is very short.
  const actions=draft&&<SettingsSaveBar disabled={busy}>
    <Button icon={Save} variant="primary" disabled={!dirty||recording} onClick={()=>void save()}>{t("shortcuts.save")}</Button>
    {dirty&&<Button icon={RotateCcw} onClick={()=>void reload()}>{t("shortcuts.undo")}</Button>}
    {!dirty&&registrationError&&<Button icon={RefreshCw} onClick={()=>void save()}>{t("shortcuts.retry")}</Button>}
  </SettingsSaveBar>;
  // The editor stays mounted for the whole page, so its draft survives a tab
  // change and every binding keeps its own registration status and error.
  return <section>
    {error&&<Notice actions={!dirty&&<Button disabled={busy} onClick={()=>void reload()}>{t("shortcuts.readAgain")}</Button>}>{error}</Notice>}
    {renderLayout?renderLayout(form,actions):<>{form}{actions}</>}
    {notice&&!dirty&&<p role="status">{t(notice)}</p>}
    {fields.map(([, ,id])=>status?.actionErrors?.[id]&&<Notice key={id}>{t("shortcuts.failed",{error:status.actionErrors[id]})}</Notice>)}
  </section>;
}
