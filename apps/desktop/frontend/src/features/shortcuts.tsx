import {Notice} from "../ui/notice";
import { useEffect, useState } from "react";
import { getSnapshot, getShortcuts, saveSettings, shortcutRecordingPort, type Settings, type ShortcutStatus } from "../bridge";
import { useTranslation } from "../i18n";
import { Button } from "../ui/button";
import {ShortcutRecorder} from "../ui/shortcut-recorder";
import {Field,FieldLabel} from "../components/ui/field";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";

export function Shortcuts({ onSaved,moduleId,onActivity }: { onSaved: () => void;moduleId:"capture"|"always-on-top";onActivity?:PageActivityChanged }) {
  const {t}=useTranslation();
  const [draft,setDraft]=useState<Settings>();
  const [savedSettings,setSavedSettings]=useState<Settings>();
  const [status,setStatus]=useState<ShortcutStatus>();
  const [error,setError]=useState("");
  const [busy,setBusy]=useState(false);
  const [notice,setNotice]=useState("");
  const settingKey=moduleId==="capture"?"captureShortcut":"pinShortcut";
  const dirty=!!draft&&(draft[settingKey]??"")!==(savedSettings?.[settingKey]??"");
  const [recording,setRecording]=useState(false);
  usePageActivity(onActivity,dirty,busy||recording,"shortcuts");
  const action=moduleId==="capture"?"capture":"pin";
  const registrationError=status?.bindings.find(binding=>binding.id===action)?.error;
  async function refresh() {
    try {setStatus(await getShortcuts());}
    catch(error){setStatus(undefined);throw error;}
  }
  async function reload() {
    setBusy(true);setError("");setNotice("");
    try {const [snapshot,next]=await Promise.all([getSnapshot(),getShortcuts()]);setDraft(snapshot.settings);setSavedSettings(snapshot.settings);setStatus(next);}
    catch(error){setError(String(error));}finally{setBusy(false);}
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
  return <section>
    {error && <Notice actions={!dirty&&<Button disabled={busy} onClick={()=>void reload()}>{t("shortcuts.readAgain")}</Button>}>{error}</Notice>}
    {draft && <fieldset className="plain-fieldset" disabled={busy}>
      {([['captureShortcut','shortcuts.capture','capture'],['pinShortcut','shortcuts.pin','pin']] as const).filter(([, ,id])=>id===action).map(([key,label,id])=>{
        const state=status?.bindings.find(b=>b.id===id);
        return <div key={key}>
          <Field><FieldLabel htmlFor={`shortcut-${id}`}>{t(label)}</FieldLabel><ShortcutRecorder id={`shortcut-${id}`} value={draft[key]??""} disabled={busy} port={shortcutRecordingPort} onRecordingChange={active=>{setRecording(active);if(active)setNotice("");else void refresh().catch(error=>setError(String(error)));}} onChange={value=>setDraft({...draft,[key]:value})} /></Field>
          <p className="muted">{recording?t("shortcuts.pausedForRecording"):!status?t("shortcuts.unknown"):state?.error || (state?.registered ? t("shortcuts.enabled",{key:state.shortcut}) : t("shortcuts.disabled"))}</p>
        </div>;
      })}
      <div className="detail-actions">
        <Button variant="primary" disabled={!dirty||recording} onClick={()=>void save()}>{t("shortcuts.save")}</Button>
        {dirty&&<Button onClick={()=>void reload()}>{t("shortcuts.undo")}</Button>}
        {!dirty&&registrationError&&<Button onClick={()=>void save()}>{t("shortcuts.retry")}</Button>}
      </div>
    </fieldset>}
    {notice&&!dirty&&<p role="status">{t(notice)}</p>}
    {status?.actionErrors?.[action] && <Notice>{t("shortcuts.failed",{error:status.actionErrors[action]})}</Notice>}
  </section>;
}
