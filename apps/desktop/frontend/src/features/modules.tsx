import {Notice,InlineError} from "../ui/notice";
import { useEffect, useState } from "react";
import { Pin, ScanLine, PanelsTopLeft } from "lucide-react";
import { getShortcuts, moduleCall, type Module, type ShortcutStatus } from "../bridge";
import { Button } from "../ui/button";
import { Switch } from "../ui/switch";
import {ResizablePanelGroup,ResizablePanel,ResizableHandle} from "../components/ui/resizable";
import "./modules.css";
import {Item,ItemGroup,ItemContent,ItemTitle,ItemDescription,ItemActions,ItemMedia} from "../components/ui/item";
import { useTranslation } from "../i18n";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";

interface ModuleEntry {
  manifest: { id: string; name: { "zh-CN": string; "en-US": string }; distribution: string };
  enabled: boolean;
  pendingStop: boolean;
  runtime: Module;
}
interface ModuleSnapshot { revision: number; modules: ModuleEntry[]; configurationError?: string }
interface ModuleJob { id: string; state: string; error?: string }
const icons: Record<string, typeof Pin> = { capture: ScanLine, "always-on-top": Pin, wintray: PanelsTopLeft };

function status(entry: ModuleEntry): string {
  if (entry.pendingStop) return "moduleStatus.incomplete";
  if (entry.runtime.state === "faulted") return "moduleStatus.faulted";
  if (entry.runtime.state === "unavailable") return "moduleStatus.unavailable";
  if (entry.runtime.state === "starting" || entry.runtime.state === "stopping") return "moduleStatus.working";
  return entry.enabled ? "moduleStatus.enabled" : "moduleStatus.disabled";
}

export function ModulesPage({ onChanged,onActivity,onReady,startup,administrator }: { onChanged: () => void;onActivity?:PageActivityChanged;onReady?:()=>void;startup?:{runAtLogon:boolean;startMinimized:boolean};administrator?:boolean }) {
  const {t,i18n}=useTranslation();
  const language=i18n.language==="en-US"?"en-US":"zh-CN";
  const [snapshot, setSnapshot] = useState<ModuleSnapshot>();
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [loading, setLoading] = useState(true);
  const [shortcuts,setShortcuts]=useState<ShortcutStatus>();
  const [shortcutError,setShortcutError]=useState("");
  usePageActivity(onActivity,false,!!busy);
  async function reload() {
    setLoading(true);
    try {
      setSnapshot(await moduleCall<ModuleSnapshot>("snapshot"));onReady?.();
      try{setShortcuts(await getShortcuts());setShortcutError("");}
      catch(error){setShortcuts(undefined);setShortcutError(error instanceof Error?error.message:String(error));}
    }
    finally { setLoading(false); }
  }
  useEffect(() => { void reload().catch(e => setError(String(e))); }, []);
  async function setEnabled(entry: ModuleEntry, enabled: boolean) {
    if (!snapshot || busy) return;
    setBusy(entry.manifest.id); setError(""); setNotice("");
    try {
      let job = await moduleCall<ModuleJob>("setEnabled", { id: entry.manifest.id, enabled, revision: snapshot.revision });
      const id=job?.id;
      // The desktop bridge already waits for configuration commits. Only a
      // still-running legacy module job needs an ID for further polling.
      if(!job||(job.state==="running"&&(typeof id!=="string"||!id)))throw new Error(t("module.unknownResult"));
      while (job.state === "running") {
        await new Promise(resolve => setTimeout(resolve, 200));
        job = await moduleCall<ModuleJob>("jobs.get", { id });
        if(job?.id!==id)throw new Error(t("module.unknownResult"));
      }
      if (job.state !== "completed") throw new Error(job.error || t("module.failed"));
      setNotice(t(enabled?"module.enabled":"module.disabled",{name:entry.manifest.name[language]}));
    } catch (e) { setError(e instanceof Error?e.message:String(e)); }
    finally {
      try { await reload(); } catch (e) { setError(current => current || String(e)); }
      onChanged(); setBusy("");
    }
  }
  async function recoverConfiguration() {
    if(busy)return;
    setBusy("recovery");setError("");setNotice("");
    try {
      const job=await moduleCall<ModuleJob>("recoverConfiguration");
      if(job.state!=="completed")throw new Error(job.error||t("module.recoveryFailed"));
      setNotice(t("module.recovered"));
    } catch(error){setError(error instanceof Error?error.message:String(error));}
    finally{try{await reload();}catch(error){setError(current=>current||(error instanceof Error?error.message:String(error)));}onChanged();setBusy("");}
  }
  const active=snapshot?.modules.filter(entry=>entry.enabled&&!entry.pendingStop)??[];
  const attention=snapshot?.modules.filter(entry=>entry.pendingStop||["faulted","unavailable"].includes(entry.runtime.state)).length??0;
  const activeShortcuts=shortcuts?.bindings.filter(binding=>active.some(entry=>entry.manifest.id===(binding.id==="pin"?"always-on-top":binding.id)))??[];
  return <section className="space-y-8">
    {error && !snapshot?.configurationError && <Notice>{error}</Notice>}
    {snapshot?.configurationError && <Notice actions={<Button busy={busy==="recovery"} disabled={!!busy} onClick={()=>void recoverConfiguration()}>{t("module.retryRecovery")}</Button>}>{error||snapshot.configurationError}</Notice>}
    {!snapshot && (loading ? <p role="status">{t("module.loading")}</p> : <Button busy={!!busy} onClick={() => {setError("");void reload().catch(e => setError(e instanceof Error?e.message:String(e)));}}>{t("common.retry")}</Button>)}
    {snapshot&&<div className="dashboard-workspace"><ResizablePanelGroup orientation="horizontal" className="dashboard-panels items-stretch">
     <ResizablePanel defaultSize="60%" minSize="40%"><div className="dashboard-tools pr-8">    {snapshot&&<div className="space-y-2 mb-6"><h2 className="text-2xl font-semibold">{t("dashboard.manageTools")}</h2><p className="text-base text-muted-foreground">{t("module.intro")}</p></div>}
    <ItemGroup className="divide-y divide-border">
      {snapshot?.modules.map(entry => {
        const id = entry.manifest.id;
        const Icon = icons[id] || PanelsTopLeft;
        return <Item key={id} role="listitem" variant="default" className="min-w-0 flex-nowrap items-center gap-4 rounded-none px-0 py-6">
          <ItemMedia variant="icon" className="size-12"><Icon aria-hidden="true" className="size-7" /></ItemMedia>
          <ItemContent className="min-w-0">
            <ItemTitle role="heading" aria-level={3} className="max-w-full text-lg"><span id={`module-${id}-name`} className="min-w-0 [overflow-wrap:anywhere]">{entry.manifest.name[language]}</span></ItemTitle>
            {i18n.exists(`moduleDescription.${id}`)&&<ItemDescription className="line-clamp-none text-sm leading-relaxed">{t(`moduleDescription.${id}`)}</ItemDescription>}
            {entry.pendingStop && !snapshot.configurationError && <div className="module-entry__recovery"><Button disabled={!!busy} onClick={() => void setEnabled(entry, false)}>{t("module.retryClose")}</Button></div>}
          </ItemContent>
          <ItemActions className="shrink-0 flex-col items-end gap-3">
            <span className="text-sm text-muted-foreground">{busy === id ? t("module.updated") : t(status(entry))}</span>
            <Switch aria-labelledby={`module-${id}-name`} checked={entry.enabled && !entry.pendingStop} disabled={!!busy || entry.pendingStop || !!snapshot.configurationError} onCheckedChange={value => void setEnabled(entry, value)} />
          </ItemActions>
        </Item>;
      })}
    </ItemGroup></div></ResizablePanel>
     <ResizableHandle withHandle aria-label={t("dashboard.resizePanels")}/>
     <ResizablePanel defaultSize="40%" minSize="30%"><div className="dashboard-details pl-8">    <div className="dashboard-summary space-y-8">
      <div className="space-y-3"><h2 className="text-base font-medium text-muted-foreground">{t("dashboard.toolsEnabled")}</h2><p className="text-5xl font-semibold tracking-tight">{active.length}<span className="ml-3 text-xl font-normal text-muted-foreground">/ {snapshot.modules.length}</span></p>{attention>0&&<p className="text-base text-muted-foreground">{t("dashboard.attention",{count:attention})}</p>}{startup&&<dl className="grid grid-cols-[1fr_auto] gap-x-4 gap-y-2 pt-3 text-sm"><dt>{t("startup.logon")}</dt><dd>{t(startup.runAtLogon?"profiles.on":"profiles.off")}</dd><dt>{t("startup.silent")}</dt><dd>{t(startup.startMinimized?"profiles.on":"profiles.off")}</dd></dl>}{typeof administrator==="boolean"&&<div className="flex justify-between gap-4 text-sm"><span>{t("dashboard.permission")}</span><span>{t(administrator?"dashboard.administrator":"dashboard.standard")}</span></div>}</div>
      <section className="min-w-0 space-y-4" aria-label={t("dashboard.shortcuts")}><h2 className="text-xl font-semibold">{t("dashboard.shortcuts")}</h2>
        {activeShortcuts.map(binding=><div key={binding.id} className="flex flex-wrap items-center justify-between gap-3 border-b pb-3"><span className="text-base">{t(binding.id==="pin"?"shortcuts.pin":"shortcuts.capture")}</span><span className="break-all font-mono text-lg">{binding.registered?binding.shortcut:t("shortcuts.disabled")}</span>{binding.error&&<InlineError className="w-full text-sm">{binding.error}</InlineError>}</div>)}
        {!shortcutError&&!activeShortcuts.length&&<p className="text-muted-foreground">{t("dashboard.noShortcuts")}</p>}
        {shortcutError&&<InlineError>{shortcutError}</InlineError>}
      </section>
    </div>
</div></ResizablePanel>
    </ResizablePanelGroup></div>}

    <p role="status" className="muted">{notice}</p>
  </section>;
}

export function DisabledModule({ name, onManage }: { name: string; onManage: () => void }) {
  const {t}=useTranslation();
  return <section className="empty"><h2>{t("module.disabledTitle",{name})}</h2><p>{t("module.retained")}</p><Button onClick={onManage}>{t("module.manage")}</Button></section>;
}
