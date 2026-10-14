import {ScanLine,FileType,ScrollText,Save,FolderOpen,Languages} from "lucide-react";
import {IconLabel} from "../ui/icon-label";
import {Notice} from "../ui/notice";
import {RadioGroup,RadioGroupItem} from "../components/ui/radio-group";
import {Label} from "../components/ui/label";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "../components/ui/tabs";
import {Checkbox} from "../components/ui/checkbox";
import {NumberField,validNumber} from "../ui/number-field";

import {CaptureAISettings} from "./capture-ai-settings";
import {CaptureFileName} from "./capture-filename";

import { useEffect, useState } from "react";
import { captureCall, pickDirectory } from "../bridge";
import { useTranslation } from "../i18n";
import { Button } from "../ui/button";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";


interface CaptureSettings {
  schemaVersion: number;
  revision: number;
  outputDirectory: string;
  copyToClipboard: boolean;
  askWhereToSave: boolean;
  openEditor: boolean;
  pinAfterCapture: boolean;
  theme: string;
  scrollDelay: number;
  scrollFromTop: boolean;
  toolbarTools?: string[] | null;
  toolbarVersion?: number;
  fileNameTemplate?: string | null;
}
interface Snapshot { settings: CaptureSettings }
export function CapturePage({ disabled = false,onEdit,onOpenDraft,onEditorActivity,refreshKey=0,onActivity }: { disabled?: boolean;onEdit?:(id:string)=>Promise<void>;onOpenDraft?:(id:string)=>Promise<void>;onEditorActivity?:(active:boolean)=>void;refreshKey?:number;onActivity?:PageActivityChanged }) {
  const {t}=useTranslation();
  const [settingsTab,setSettingsTab]=useState("general");
  const [aiOpened,setAIOpened]=useState(false);

  const [state, setState] = useState<Snapshot>();
  const [draft, setDraft] = useState<CaptureSettings>();
  const validScrollDelay=!!draft&&validNumber(draft.scrollDelay,100,2000);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const pendingSettings=!!draft&&!!state&&JSON.stringify(draft)!==JSON.stringify(state.settings);
  usePageActivity(onActivity,pendingSettings,busy);
  useEffect(()=>{onEditorActivity?.(busy||pendingSettings);return()=>onEditorActivity?.(false);},[busy,pendingSettings]);
  async function reload(preserveDraft = false) {
    if (!preserveDraft) setError("");
    try {
      const next: Snapshot = disabled
        ? { settings: await captureCall<CaptureSettings>("settings.get") }
        : await captureCall<Snapshot>("snapshot");
      setState((current) =>
        preserveDraft && current
          ? { ...next, settings: current.settings }
          : next,
      );
      if (!preserveDraft) setDraft(next.settings);
    } catch (e) {
      setError(String(e));
    }
  }
  useEffect(() => {
    void reload();
  }, [disabled,refreshKey]);
  async function save() {
    if (!draft||!validScrollDelay) return;
    setBusy(true);
    setError("");
    try {
      const next = await captureCall<CaptureSettings>("settings.save", draft);
      setDraft(next);
      setState((s) => (s ? { ...s, settings: next } : s));
      setMessage("common.saved");
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  async function browse() {
    if(busy)return;
    setBusy(true);setError("");setMessage("");
    try {
      const path = await pickDirectory();
      if (path) setDraft((s) => (s ? { ...s, outputDirectory: path } : s));
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  const navigationTabs=(<TabsList variant="default" className="max-w-full flex-wrap justify-start" aria-label={t("capture.save")}><TabsTrigger value="general"><IconLabel icon={ScanLine}>{t("capture.after")}</IconLabel></TabsTrigger><TabsTrigger value="filename"><IconLabel icon={FileType}>{t("capture.fileNameTemplate")}</IconLabel></TabsTrigger><TabsTrigger value="scroll"><IconLabel icon={ScrollText}>{t("capture.scrollSettings")}</IconLabel></TabsTrigger><TabsTrigger value="ai"><IconLabel icon={Languages}>{t("ai.title")}</IconLabel></TabsTrigger></TabsList>);
  const unavailable=error
    ? <Notice actions={<Button disabled={busy} onClick={()=>void reload()}>{t("common.retry")}</Button>}>{error}</Notice>
    : <p role="status">{t("capture.loading")}</p>;



  const dirty = !!draft && JSON.stringify(draft) !== JSON.stringify(state?.settings);
  return (
    <div className="module-sections capture-settings-page">
      <section className="module-section">
      <Tabs value={settingsTab} onValueChange={value=>{setSettingsTab(String(value));if(value==="ai")setAIOpened(true);}}>{navigationTabs}
      {error && draft && (
        <Notice actions={<Button disabled={busy} onClick={() => void reload()}>{t("capture.discard")}</Button>}>{error}</Notice>
      )}
<TabsContent value="general" className="">{draft?<fieldset className="plain-fieldset settings-stack" disabled={busy}>
<section className="settings-group"><h3 className="settings-group-title">{t("capture.workflow")}</h3>
<RadioGroup aria-label={t("capture.workflow")} value={draft.openEditor?"edit":"save"} disabled={busy} onValueChange={value=>setDraft({...draft,openEditor:value==="edit",askWhereToSave:value==="edit"?draft.askWhereToSave:false})} className="gap-4">
  <div className="flex items-center gap-2"><RadioGroupItem id="capture-edit-first" value="edit"/><Label htmlFor="capture-edit-first">{t("capture.editFirst")}</Label></div>
  <div className="flex items-center gap-2"><RadioGroupItem id="capture-quick-save" value="save"/><Label htmlFor="capture-quick-save">{t("capture.quickSave")}</Label></div>
</RadioGroup></section>
<section className="settings-group"><h3 className="settings-group-title">{t("capture.outputOptions")}</h3>
        <label className="check-row">
          <Checkbox
            checked={draft.copyToClipboard}
            onCheckedChange={(checked) =>
              setDraft({ ...draft, copyToClipboard: checked })
            }
          />
          {t("capture.clipboard")}
        </label>

        <label className="check-row">
          <Checkbox
            checked={draft.pinAfterCapture}
            onCheckedChange={(checked) =>
              setDraft({ ...draft, pinAfterCapture: checked })
            }
          />
          {t("capture.pin")}
        </label>
</section>
<section className="settings-group"><h3 className="settings-group-title">{t("capture.saveLocation")}</h3>
{draft.openEditor&&        <label className="check-row">
          <Checkbox
            checked={draft.askWhereToSave}
            onCheckedChange={(checked) =>
              setDraft({ ...draft, askWhereToSave: checked })
            }
          />
          {t("capture.chooseEveryTime")}
        </label>}
        <div className="setting-row">
          <div>
            <span>{t("capture.directory")}</span>
            <p className="program-path" id="capture-path">
              {draft.outputDirectory}
            </p>
          </div>
          <Button icon={FolderOpen} aria-describedby="capture-path" onClick={() => void browse()}>{t("capture.chooseDirectory")}</Button>
        </div>
        </section></fieldset>:unavailable}</TabsContent><TabsContent value="filename" className="">{draft?<fieldset className="plain-fieldset" disabled={busy}>
        <CaptureFileName value={draft.fileNameTemplate??""} onChange={value=>setDraft({...draft,fileNameTemplate:value})}/>
        </fieldset>:unavailable}</TabsContent><TabsContent value="scroll" className="space-y-5">{draft?<fieldset className="plain-fieldset settings-stack" disabled={busy}>
        <label className="check-row">
          <Checkbox
            checked={draft.scrollFromTop}
            onCheckedChange={(checked) =>
              setDraft({ ...draft, scrollFromTop: checked })
            }
          />
          {t("capture.fromTop")}
        </label>
        <NumberField id="capture-scroll-delay" label={t("capture.delay")} min={100} max={2000} value={draft.scrollDelay} onChange={value=>setDraft({...draft,scrollDelay:value})}/>
        </fieldset>:unavailable}</TabsContent><TabsContent value="ai" keepMounted className="">{aiOpened&&<CaptureAISettings onActivity={onActivity}/>}</TabsContent></Tabs>
        {draft&&settingsTab!=="ai"&&<div className="settings-save-bar">
          {!validScrollDelay&&settingsTab!=="scroll"&&<Notice actions={<Button onClick={()=>{setSettingsTab("scroll");requestAnimationFrame(()=>document.getElementById("capture-scroll-delay")?.focus());}}>{t("capture.checkScrollDelay")}</Button>}>{t("capture.invalidScrollDelay")}</Notice>}
          <Button icon={Save} variant="primary" disabled={busy||!dirty||!validScrollDelay} onClick={() => void save()}>{t("capture.save")}</Button></div>}
      </section>
      {message&&<p className="muted" role="status">{t(message)}</p>}
    </div>
  );
}
