import {usePathDrop} from "../ui/path-drop";
import {SettingsSaveBar} from "../components/settings-save-bar";
import {ScanLine,FileType,ScrollText,Save,FolderOpen,Languages,SendHorizontal,Workflow,ListChecks} from "lucide-react";
import {IconLabel} from "../ui/icon-label";
import {Notice} from "../ui/notice";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "../components/ui/tabs";
import {Checkbox} from "../components/ui/checkbox";
import {NumberField,validNumber} from "../ui/number-field";
import {SettingsSection,SettingsOptions,SettingsToggle} from "../components/settings-layout";

import {Shortcuts} from "./shortcuts";
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
export function CapturePage({ disabled = false,onEdit,onOpenDraft,onEditorActivity,refreshKey=0,onActivity,onSaved }: { disabled?: boolean;onEdit?:(id:string)=>Promise<void>;onOpenDraft?:(id:string)=>Promise<void>;onEditorActivity?:(active:boolean)=>void;refreshKey?:number;onActivity?:PageActivityChanged;onSaved?:()=>void }) {
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
  // Reverting is local: the last Host snapshot is already the saved baseline.
  // Edits create new objects, so sharing that snapshot cannot mutate it.
  function discardDraft(){if(state){setDraft(state.settings);setError("");setMessage("");}}
  usePageActivity(onActivity,pendingSettings,busy,"capture",discardDraft);
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
  const outputDrop=usePathDrop({kind:"directory",disabled:busy||!draft,onPaths:paths=>{setError("");setDraft(value=>value?{...value,outputDirectory:paths[0]}:value);},onError:setError});
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
  const navigationTabs=(<TabsList variant="default" className="max-w-full flex-wrap justify-start" aria-label={t("capture.save")}><TabsTrigger title={t("capture.after")} value="general"><IconLabel icon={ScanLine}>{t("capture.after")}</IconLabel></TabsTrigger><TabsTrigger title={t("settingsDesign.captureNaming")} value="filename"><IconLabel icon={FileType}>{t("settingsDesign.captureNaming")}</IconLabel></TabsTrigger><TabsTrigger title={t("settingsDesign.captureScrolling")} value="scroll"><IconLabel icon={ScrollText}>{t("settingsDesign.captureScrolling")}</IconLabel></TabsTrigger><TabsTrigger title={t("settingsDesign.captureAI")} value="ai"><IconLabel icon={Languages}>{t("settingsDesign.captureAI")}</IconLabel></TabsTrigger><TabsTrigger title={t("shortcuts.title")} value="shortcuts"><IconLabel icon={SendHorizontal}>{t("shortcuts.title")}</IconLabel></TabsTrigger></TabsList>);
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
<TabsContent value="general" className="">{draft?<fieldset className="plain-fieldset settings-section-stack" disabled={busy}>
<SettingsSection className="capture-workflow" headingId="capture-workflow-heading" icon={Workflow} title={t("settingsDesign.captureWorkflow")}>
<SettingsOptions label={t("capture.workflow")} labelledBy="capture-workflow-heading" presentation="list" value={draft.openEditor?"edit":"save"} disabled={busy}
  onChange={value=>setDraft({...draft,openEditor:value==="edit",askWhereToSave:value==="edit"?draft.askWhereToSave:false})}
  items={[
    {value:"edit",id:"capture-edit-first",label:t("capture.editFirst"),description:t("settingsDesign.captureEditHint")},
    {value:"save",id:"capture-quick-save",label:t("capture.quickSave"),description:t("settingsDesign.captureSaveHint")},
  ]}/></SettingsSection>
<SettingsSection className="capture-output-options" icon={ListChecks} title={t("capture.outputOptions")}>
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
</SettingsSection>
<SettingsSection icon={FolderOpen} title={t("capture.saveLocation")}>
{draft.openEditor&&<SettingsToggle label={t("capture.chooseEveryTime")} checked={draft.askWhereToSave} onChange={askWhereToSave=>setDraft({...draft,askWhereToSave})}/>}
        <div className="capture-directory-setting" {...outputDrop}>
            <span>{t("capture.directory")}</span>
          <div className="capture-directory-control">
            <p className="program-path" id="capture-path">
              {draft.outputDirectory}
            </p>
          <Button icon={FolderOpen} aria-describedby="capture-path" onClick={() => void browse()}>{t("capture.chooseDirectory")}</Button>
          </div>
        </div>
        </SettingsSection></fieldset>:unavailable}</TabsContent><TabsContent value="filename" className="">{draft?<fieldset className="plain-fieldset" disabled={busy}>
        <CaptureFileName value={draft.fileNameTemplate??""} onChange={value=>setDraft({...draft,fileNameTemplate:value})}/>
        </fieldset>:unavailable}</TabsContent><TabsContent value="scroll" className="space-y-5">{draft?<fieldset className="plain-fieldset settings-stack" disabled={busy}>
        <SettingsSection icon={ScrollText} title={t("settingsDesign.captureScrolling")}>
          <SettingsToggle label={t("capture.fromTop")} checked={draft.scrollFromTop} onChange={scrollFromTop=>setDraft({...draft,scrollFromTop})}/>
          <NumberField id="capture-scroll-delay" label={t("capture.delay")} min={100} max={2000} value={draft.scrollDelay} onChange={value=>setDraft({...draft,scrollDelay:value})}/>
        </SettingsSection>
        </fieldset>:unavailable}</TabsContent><TabsContent value="ai" keepMounted className="">{aiOpened&&<CaptureAISettings onActivity={onActivity}/>}</TabsContent><TabsContent value="shortcuts" className="settings-section-stack"><Shortcuts moduleId="capture" onSaved={()=>onSaved?.()} onActivity={onActivity}/></TabsContent></Tabs>
        {draft&&settingsTab!=="ai"&&settingsTab!=="shortcuts"&&<SettingsSaveBar undo={{disabled:busy||!dirty,onClick:discardDraft}}>
          {!validScrollDelay&&settingsTab!=="scroll"&&<Notice actions={<Button onClick={()=>{setSettingsTab("scroll");requestAnimationFrame(()=>document.getElementById("capture-scroll-delay")?.focus());}}>{t("capture.checkScrollDelay")}</Button>}>{t("capture.invalidScrollDelay")}</Notice>}
          <Button icon={Save} variant="primary" disabled={busy||!dirty||!validScrollDelay} onClick={() => void save()}>{t("capture.save")}</Button></SettingsSaveBar>}
      </section>
      {message&&<p className="muted" role="status">{t(message)}</p>}
    </div>
  );
}
