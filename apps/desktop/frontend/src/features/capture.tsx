import {Notice} from "../ui/notice";
import {runCaptureJob as runJob} from "./capture-draft-api";
import {RadioGroup,RadioGroupItem} from "../components/ui/radio-group";
import {Label} from "../components/ui/label";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "../components/ui/tabs";
import {Checkbox} from "../components/ui/checkbox";
import {NumberField,validNumber} from "../ui/number-field";
import {Diagnostics} from "./diagnostics";
import {CaptureFileName} from "./capture-filename";
import {captureToolbarPreferences} from "./capture-toolbar-preferences";
import { lazy, Suspense, useEffect, useState } from "react";
import { captureCall, pickDirectory, reveal } from "../bridge";
import { useTranslation } from "../i18n";
import { Button } from "../ui/button";
import { CaptureToolbar, type CaptureAction } from "../ui/capture-toolbar";
import { CapturePreview } from "../ui/capture-preview";
import type { EditPreview } from "./capture-editor-model";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";

// Canvas code is loaded only when the user opens the annotation workspace.
const CaptureEditor=lazy(()=>import("./capture-editor"));

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
  fileNameTemplate?: string | null;
}
interface CaptureOutput {
  id?: string;
  previewReady?: boolean;
  previewWarning?: string;
  path: string;
  width: number;
  height: number;
  copied: boolean;
  pinned: boolean;
}
interface Snapshot {
  settings: CaptureSettings;
  lastCapture?: CaptureOutput;
  busy: boolean;
  pins: number;
}
export function CapturePage({ disabled = false,onEdit,onOpenDraft,onEditorActivity,refreshKey=0,onActivity }: { disabled?: boolean;onEdit?:(id:string)=>Promise<void>;onOpenDraft?:(id:string)=>Promise<void>;onEditorActivity?:(active:boolean)=>void;refreshKey?:number;onActivity?:PageActivityChanged }) {
  const {t}=useTranslation();
  const [settingsTab,setSettingsTab]=useState("general");
  const [state, setState] = useState<Snapshot>();
  const [draft, setDraft] = useState<CaptureSettings>();
  const validScrollDelay=!!draft&&validNumber(draft.scrollDelay,100,2000);
  const [busy, setBusy] = useState(false);
  const [scrollJob,setScrollJob]=useState<string>();
  const [scrollStopping,setScrollStopping]=useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [editing,setEditing]=useState<CaptureOutput>();
  const pendingSettings=!!draft&&!!state&&JSON.stringify(draft)!==JSON.stringify(state.settings);
  usePageActivity(onActivity,pendingSettings,busy||!!editing);
  useEffect(()=>{onEditorActivity?.(!!editing||busy||pendingSettings);return()=>onEditorActivity?.(false);},[!!editing,busy,pendingSettings]);
  async function editImage() {
    const image=state?.lastCapture;if(!image?.id)return;
    if(!onEdit){setEditing(image);return;}
    setBusy(true);setError("");
    try{await onEdit(image.id);}catch(error){setError(String(error));}finally{setBusy(false);}
  }
  const imageCommands:Record<CaptureAction,string>={copy:"copy.last",save:"save.last",edit:"edit.last",pin:"pin.last",closePins:"pins.close"};
  async function reload(preserveDraft = false) {
    if (!preserveDraft) setError("");
    try {
      const next: Snapshot = disabled
        ? { settings: await captureCall<CaptureSettings>("settings.get"), busy: false, pins: 0 }
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
  async function act(method: string, params: unknown = null) {
    const hidden = method === "capture.start" || method === "capture.unified" || method === "edit.last";
    setBusy(true);
    setError("");
    setMessage("capture.working");
    try {
      const scrolling=method==="capture.unified"&&(params as {mode?:string}|null)?.mode==="scroll";
      const job = await runJob(method, params,scrolling?setScrollJob:undefined);
      if(method==="capture.unified"&&job.state==="completed") {
        const result=job.result as {draftId?:string}|undefined;
        if(result?.draftId) {
          if(!onOpenDraft||!/^[a-f0-9]{32}$/.test(result.draftId))throw Error(t("capture.draftUnavailable"));
          await onOpenDraft(result.draftId);
        }
      }
      setMessage(job.state === "cancelled" ? "capture.cancelled" : "capture.completed");
      if((method==="save.last"||method==="edit.apply")&&job.state==="cancelled")return false;
      await reload(JSON.stringify(draft) !== JSON.stringify(state?.settings));
      return job.state==="completed";
    } catch (e) {
      setError(String(e));
      setMessage("");
      await reload(true);
      if(method==="edit.apply")throw e;
      return false;
    } finally {
      setBusy(false);
      setScrollJob(undefined);
      setScrollStopping(false);
      if (hidden) {
        try { await reveal(); }
        catch(error) { setError(String(error));setMessage(""); }
      }
    }
  }
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
  const navigationTabs=(<TabsList className="grid max-w-full grid-cols-2 justify-start gap-1 min-[700px]:flex group-data-horizontal/tabs:h-auto" aria-label={t("capture.save")}><TabsTrigger value="general">{t("capture.after")}</TabsTrigger><TabsTrigger value="filename">{t("capture.fileNameTemplate")}</TabsTrigger><TabsTrigger value="scroll">{t("capture.scrollSettings")}</TabsTrigger><TabsTrigger value="logs">{t("diagnostics.title")}</TabsTrigger></TabsList>);
  const unavailable=error
    ? <Notice actions={<Button disabled={busy} onClick={()=>void reload()}>{t("common.retry")}</Button>}>{error}</Notice>
    : <p role="status">{t("capture.loading")}</p>;



  const dirty = !!draft && JSON.stringify(draft) !== JSON.stringify(state?.settings);
  return (
    <div className="module-sections capture-settings-page">
      {error && draft && (
        <Notice actions={<Button disabled={busy} onClick={() => void reload()}>{t("capture.discard")}</Button>}>{error}</Notice>
      )}
      <section className="module-section">
      <Tabs value={settingsTab} onValueChange={value=>setSettingsTab(String(value))}>{navigationTabs}<TabsContent value="general" className="pt-5">{draft?<fieldset className="plain-fieldset" disabled={busy}>
<RadioGroup aria-label={t("capture.workflow")} value={draft.openEditor?"edit":"save"} disabled={busy} onValueChange={value=>setDraft({...draft,openEditor:value==="edit",askWhereToSave:value==="edit"?draft.askWhereToSave:false})} className="gap-3 py-4">
  <div className="flex items-center gap-2"><RadioGroupItem id="capture-edit-first" value="edit"/><Label htmlFor="capture-edit-first">{t("capture.editFirst")}</Label></div>
  <div className="flex items-center gap-2"><RadioGroupItem id="capture-quick-save" value="save"/><Label htmlFor="capture-quick-save">{t("capture.quickSave")}</Label></div>
</RadioGroup>
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
          <Button aria-describedby="capture-path" onClick={() => void browse()}>{t("capture.chooseDirectory")}</Button>
        </div>
        </fieldset>:unavailable}</TabsContent><TabsContent value="filename" className="pt-5">{draft?<fieldset className="plain-fieldset" disabled={busy}>
        <CaptureFileName value={draft.fileNameTemplate??""} onChange={value=>setDraft({...draft,fileNameTemplate:value})}/>
        </fieldset>:unavailable}</TabsContent><TabsContent value="scroll" className="space-y-5 pt-5">{draft?<fieldset className="plain-fieldset space-y-5" disabled={busy}>
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
        </fieldset>:unavailable}</TabsContent><TabsContent value="logs" className="pt-5">{settingsTab==="logs"&&<Diagnostics moduleId="capture"/>}</TabsContent></Tabs>
        {draft&&settingsTab!=="logs"&&<div className="pt-6">
          {!validScrollDelay&&settingsTab!=="scroll"&&<Notice actions={<Button onClick={()=>{setSettingsTab("scroll");requestAnimationFrame(()=>document.getElementById("capture-scroll-delay")?.focus());}}>{t("capture.checkScrollDelay")}</Button>}>{t("capture.invalidScrollDelay")}</Notice>}
          <Button variant="primary" disabled={busy||!dirty||!validScrollDelay} onClick={() => void save()}>{t("capture.save")}</Button></div>}
      </section>
      {draft&&<section className="module-section" aria-label={t("nav.capture")}>
      <h2 className="section-heading">{t("nav.capture")}</h2>
      {disabled && <p className="muted">{t("capture.disabled")}</p>}
      {busy&&scrollJob&&<div role="status" className="flex items-center gap-3"><span>{t("capture.scrolling")}</span><Button busy={scrollStopping} onClick={()=>{
        setScrollStopping(true);
        void captureCall<{stopping:boolean}>("scroll.stop",{id:scrollJob}).then(result=>{if(!result.stopping)setScrollStopping(false);}).catch(error=>{setScrollStopping(false);setError(String(error));});
      }}>{t("capture.stopScroll")}</Button></div>}
      {!disabled && <div className="capture-actions">
        {[
          ["window", "capture.window"],
          ["region", "capture.region"],
          ["fullscreen", "capture.fullscreen"],
          ["scroll", "capture.scroll"],
        ].map(([mode, title]) => (
          <Button
            key={mode}
            variant={mode==="window"?"primary":"secondary"}
            disabled={busy || dirty}
            onClick={() => void act(onOpenDraft?"capture.unified":"capture.start", { mode })}
          >
            {t(title)}
          </Button>
        ))}
      </div>}
      <p className="muted" role="status">
        {dirty ? t("capture.saveFirst") : message ? t(message) : (disabled ? "" : t("capture.hint"))}
      </p>
      {!disabled && state?.lastCapture && (
        <section className="last-capture">
          <h2>{t("capture.recent")}</h2>
          <CapturePreview key={state.lastCapture.id} id={state.lastCapture.id} ready={state.lastCapture.previewReady} width={state.lastCapture.width} height={state.lastCapture.height} onRefresh={()=>void reload(true)}/>
          {state.lastCapture.previewWarning&&<Notice  tone="info">{state.lastCapture.previewWarning}</Notice>}
          <p className="program-path">{state.lastCapture.path}</p>
          <p className="muted">
            {state.lastCapture.width} × {state.lastCapture.height}
          </p>
          <CaptureToolbar busy={busy} canEdit={!dirty} hasPins={(state?.pins??0)>0} onAction={action=>action==="edit"&&onEdit?void editImage():void act(imageCommands[action],action==="closePins"?null:{expectedId:state.lastCapture?.id})}/>
          {!onEdit&&state.lastCapture.id&&state.lastCapture.previewReady&&<Button disabled={busy||dirty} onClick={()=>void editImage()}>{t("capture.annotate")}</Button>}
        </section>
      )}
      {editing?.id&&<Suspense fallback={<p role="status">{t("capture.working")}</p>}><CaptureEditor toolbarPort={captureToolbarPreferences} key={editing.id} image={{id:editing.id,width:editing.width,height:editing.height}} onClose={()=>setEditing(undefined)} onPreview={async document=>{
        const job=await runJob("edit.preview",{expectedId:editing.id,document});
        const result=job.result as EditPreview|undefined;
        if(job.state!=="completed"||!result||!/^[a-f0-9]{32}$/.test(result.id)||result.sourceId!==editing.id||result.width!==(document.crop?.width??document.width)||result.height!==(document.crop?.height??document.height))throw new Error(t("capture.previewFailed"));
        return result;
      }} onSave={async document=>{
        // Bind the whole edit session to its source, even if another capture
        // becomes current while the save dialog is open.
        return await act("edit.apply",{expectedId:editing.id,document})===true;
      }}/></Suspense>}


      </section>}
    </div>
  );
}
