import {Notice} from "../ui/notice";
import { useEffect, useRef, useState } from "react";
import { Pin, ScanLine, LaptopMinimal,MousePointer2,RefreshCw,Package,Rocket } from "lucide-react";
import {QuickAssistantIcon} from "../components/quick-assistant-icon";
import {moduleRank} from "../module-order";
import { moduleCall, type Module } from "../bridge";
import { Button } from "../ui/button";
import { Switch } from "../ui/switch";
import "./modules.css";
import {Item,ItemGroup,ItemContent,ItemTitle,ItemDescription,ItemActions,ItemMedia} from "../components/ui/item";
import { useTranslation } from "../i18n";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";
import {ModuleInstallation} from './module-installation';
import {ModuleRemoval} from './module-removal';
import type {PackageReference,SharedRuntime} from './package-types';

interface ModuleEntry {
  manifest: { id: string; name: { "zh-CN": string; "en-US": string }; distribution: string };
  enabled: boolean;
  pendingStop: boolean;
  runtime: Module;
  blocked?: boolean;
  recoveryId?: string;
}
interface ModuleSnapshot { revision: number; modules: ModuleEntry[]; configurationError?: string; cleanup?:{pending:boolean;error?:string};packages?:{runtimes?:SharedRuntime[]|null;dependencies?:Record<string,PackageReference[]>|null;error?:string} }
interface ModuleJob { id: string; state: string; error?: string }
const icons: Record<string, typeof Pin> = { capture: ScanLine,selection:MousePointer2,"quick-assistant":QuickAssistantIcon, "always-on-top": Pin, wintray: LaptopMinimal };

function status(entry: ModuleEntry): string {
  if (entry.pendingStop) return "moduleStatus.incomplete";
  if (entry.runtime.state === "faulted") return "moduleStatus.faulted";
  if (entry.runtime.state === "unavailable") return "moduleStatus.unavailable";
  if (entry.runtime.state === "starting" || entry.runtime.state === "stopping") return "moduleStatus.working";
  return entry.enabled ? "moduleStatus.enabled" : "moduleStatus.disabled";
}

export function ModulesPage({ onChanged,onActivity,onReady,only,controlsDisabled=false }: {only?:string;controlsDisabled?:boolean; onChanged: () => void;onActivity?:PageActivityChanged;onReady?:()=>void }) {
  const {t,i18n}=useTranslation();
  const language=i18n.language==="en-US"?"en-US":"zh-CN";
  const [snapshot, setSnapshot] = useState<ModuleSnapshot>();
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [loading, setLoading] = useState(true);
  const readyNotified=useRef(false);
  // Home owns tool availability. Unrelated preference reads must not delay
  // readiness; notify only after React commits the available controls.
  useEffect(()=>{
    if(snapshot&&!loading&&!readyNotified.current){readyNotified.current=true;onReady?.();}
  },[snapshot,loading,onReady]);
  usePageActivity(onActivity,false,!!busy,"module-availability");
  async function reload() {
    setLoading(true);
    try {
      setSnapshot(await moduleCall<ModuleSnapshot>("snapshot"));
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
  async function recoverPackage(entry:ModuleEntry) {
    if(busy||!snapshot)return;
    setBusy(entry.manifest.id);setError("");setNotice("");
    try {
      const job=await moduleCall<ModuleJob>("recoverPackage",{id:entry.manifest.id,revision:snapshot.revision,recoveryId:entry.recoveryId??""});
      if(job.state!=="completed")throw new Error(job.error||t("module.recoveryFailed"));
      setNotice(t("module.packageRecovered",{name:entry.manifest.name[language]}));
    }catch(error){setError(error instanceof Error?error.message:String(error));}
    finally{try{await reload();}catch(error){setError(current=>current||(error instanceof Error?error.message:String(error)));}onChanged();setBusy("");}
  }
  async function cleanupPackages(){
    if(busy)return;
    setBusy("cleanup");setError("");setNotice("");
    try{
      const result=await moduleCall<ModuleJob>("cleanupPackages",{});
      if(result.state!=="completed")throw Error(result.error||t("module.cleanupFailed"));
      setNotice(t("module.cleanupDone"));
    }catch(error){setError(error instanceof Error?error.message:String(error));}
    finally{try{await reload();}catch(error){setError(current=>current||(error instanceof Error?error.message:String(error)));}onChanged();setBusy("");}
  }
  // Stroke keeps its independent lifecycle inside Convenience. Home lists
  // navigation modules, so it must not offer a second mouse-gesture module card.
  const visibleModules=snapshot?.modules.filter(entry=>only?entry.manifest.id===only:entry.manifest.id!=="stroke")??[];
  const active=visibleModules.filter(entry=>entry.enabled&&!entry.pendingStop);
  const attention=snapshot?.modules.filter(entry=>entry.pendingStop||["faulted","unavailable"].includes(entry.runtime.state)).length??0;
  return <section className={only?"settings-group border-b border-border pb-4":"dashboard-workspace"}>
    {error && !snapshot?.configurationError && <Notice>{error}</Notice>}
    {snapshot?.configurationError && <Notice actions={<Button busy={busy==="recovery"} disabled={!!busy} onClick={()=>void recoverConfiguration()}>{t("module.retryRecovery")}</Button>}>{error||snapshot.configurationError}</Notice>}
    {!snapshot && (loading ? <p role="status">{t("module.loading")}</p> : <Button busy={!!busy} onClick={() => {setError("");void reload().catch(e => setError(e instanceof Error?e.message:String(e)));}}>{t("common.retry")}</Button>)}
    {snapshot&&<>
    {!only&&<div className="dashboard-heading">
      <div className="dashboard-heading-copy"><p>{t("module.intro")}</p></div>
      <div className="flex flex-wrap items-center gap-3"><span className="dashboard-count">{t("dashboard.enabledCount",{count:active.length,total:visibleModules.length})}</span><ModuleInstallation disabled={!!busy||!!snapshot.configurationError} onActivity={onActivity} onChanged={async result=>{await reload();onChanged();setNotice(result.state==='completed'?(result.warning||t(result.kind==='runtime'?'sharedPackages.installed':result.operation==='update'?'module.packageUpdated':'module.packageInstalled')):'');}}/></div>
    </div>}
    {!only&&attention>0&&<Notice tone="info">{t("dashboard.attention",{count:attention})}</Notice>}
    {!only&&snapshot.cleanup?.pending&&<Notice tone="info" actions={<Button icon={RefreshCw} busy={busy==="cleanup"} disabled={!!busy} onClick={()=>void cleanupPackages()}>{t("module.retryCleanup")}</Button>}>{snapshot.cleanup.error||t("module.cleanupPending")}</Notice>}
    <ItemGroup className={only?"gap-0":"dashboard-tools grid gap-5 @max-[580px]:gap-4"}>
      {visibleModules.sort((a,b)=>moduleRank(a.manifest.id)-moduleRank(b.manifest.id)).map(entry => {
        const id = entry.manifest.id;
        // A feature tab needs one setting row. Keep the dashboard's card
        // presentation on Home while sharing the same guarded lifecycle calls.
        if(only)return <div key={id} className="space-y-3">
          <div className="flex items-center justify-between gap-6 py-1">
            <div className="min-w-0 space-y-1"><label id={`module-${id}-name`} className="text-sm font-medium">{t("module.enableTool",{name:entry.manifest.name[language]})}</label><p className="text-sm text-muted-foreground">{busy===id?t("module.updated"):t(status(entry))}</p></div>
            <Switch className="shrink-0" aria-labelledby={`module-${id}-name`} checked={entry.enabled&&!entry.pendingStop} disabled={controlsDisabled||!!busy||entry.pendingStop||!!entry.blocked||!!snapshot.configurationError} onCheckedChange={value=>void setEnabled(entry,value)}/>
          </div>
          {(entry.runtime.state==="faulted"||entry.blocked)&&entry.runtime.detail&&<Notice>{entry.runtime.detail}</Notice>}
          {entry.pendingStop&&!snapshot.configurationError&&<Button disabled={controlsDisabled||!!busy} onClick={()=>void setEnabled(entry,false)}>{t("module.retryClose")}</Button>}
          {entry.blocked&&<Button disabled={controlsDisabled||!!busy} onClick={()=>void recoverPackage(entry)}>{t(entry.recoveryId?"module.restorePackage":"module.retryPackage")}</Button>}
        </div>;
        const dependencies=snapshot.packages?.dependencies?.[id]??[];
        const grouped=!only&&id==="always-on-top";
        const Icon = grouped?Rocket:icons[id] || LaptopMinimal;
        return <Item key={id} role="listitem" variant="outline" className="dashboard-tool grid gap-4 p-6 @max-[580px]:gap-3 @max-[580px]:p-5">
          <ItemMedia variant="icon" className="dashboard-tool-icon"><Icon aria-hidden="true" className="size-6" /></ItemMedia>
          <ItemContent className="min-w-0">
            <ItemTitle role="heading" aria-level={3} className="max-w-full text-base"><span id={`module-${id}-name`} className="min-w-0 [overflow-wrap:anywhere]">{grouped?t("nav.convenience"):entry.manifest.name[language]}</span></ItemTitle>
            {grouped&&<p className="text-sm">{t("nav.always-on-top")}</p>}
            {i18n.exists(`moduleDescription.${id}`)&&<ItemDescription className="line-clamp-none text-sm leading-relaxed">{t(`moduleDescription.${id}`)}</ItemDescription>}
            {entry.blocked&&<><ItemDescription className="line-clamp-none text-sm leading-relaxed">{t("module.packageBlocked")}</ItemDescription><details className="text-sm text-muted-foreground"><summary className="cursor-pointer">{t("module.failureDetails")}</summary><p className="mt-2 break-words">{entry.runtime.detail}</p></details><div className="module-entry__recovery"><Button icon={RefreshCw} disabled={!!busy} busy={busy===id} onClick={()=>void recoverPackage(entry)}>{t(entry.recoveryId?"module.restorePackage":"module.retryPackage")}</Button></div></>}
            {entry.pendingStop && !entry.blocked && !snapshot.configurationError && <div className="module-entry__recovery"><Button disabled={!!busy} onClick={() => void setEnabled(entry, false)}>{t("module.retryClose")}</Button></div>}
            {!!dependencies.length&&<details className="text-sm text-muted-foreground"><summary className="cursor-pointer">{t('sharedPackages.dependencies')} · {dependencies.length}</summary><ul className="mt-2 space-y-1">{dependencies.map(dependency=><li className="break-words" key={dependency.id}>{dependency.id} · {dependency.version}</li>)}</ul></details>}
            {entry.manifest.distribution==='optional'&&!entry.blocked&&<div><ModuleRemoval id={id} disabled={!!busy||entry.enabled||entry.pendingStop||!!snapshot.configurationError} onActivity={onActivity} onChanged={async warning=>{await reload();onChanged();setNotice(warning||t('module.packageRemoved'));}}/></div>}
          </ItemContent>
          <ItemActions className="dashboard-tool-actions">
            <span className="text-sm text-muted-foreground">{busy === id ? t("module.updated") : t(status(entry))}</span>
            <Switch aria-labelledby={`module-${id}-name`} checked={entry.enabled && !entry.pendingStop} disabled={controlsDisabled || !!busy || entry.pendingStop || !!entry.blocked || !!snapshot.configurationError} onCheckedChange={value => void setEnabled(entry, value)} />
          </ItemActions>
        </Item>;
      })}
    </ItemGroup>
    {!only&&(!!snapshot.packages?.runtimes?.length||snapshot.packages?.error)&&<section className="mt-6 border-t border-border pt-6" aria-label={t('sharedPackages.title')}>
      <h3 className="flex items-center gap-2 text-base font-medium"><Package aria-hidden="true" className="size-4"/>{t('sharedPackages.title')}</h3>
      <p className="mt-2 text-sm leading-relaxed text-muted-foreground">{t('sharedPackages.description')}</p>
      {snapshot.packages.error&&<Notice>{snapshot.packages.error}</Notice>}
      <div role="list" className="mt-3 divide-y divide-border">{(snapshot.packages.runtimes??[]).map(runtime=><div role="listitem" className="flex flex-wrap items-center justify-between gap-x-6 gap-y-3 py-4" key={runtime.manifestSha256}>
        <div className="min-w-0 flex-1 basis-64">
          <p className="flex flex-wrap items-baseline gap-x-3 gap-y-1 text-sm"><span className="font-medium [overflow-wrap:anywhere]">{runtime.name[language]}</span><span className="text-muted-foreground">{runtime.version}</span></p>
          {!runtime.error&&<p className="mt-1 text-sm leading-relaxed text-muted-foreground [overflow-wrap:anywhere]">{runtime.requiredBy?.length?t('sharedPackages.requiredBy',{names:runtime.requiredBy.map(reference=>snapshot.modules.find(module=>module.manifest.id===reference.id)?.manifest.name[language]??snapshot.packages?.runtimes?.find(candidate=>candidate.id===reference.id)?.name[language]??reference.id).join('、')}):t('sharedPackages.unused')}</p>}
          {runtime.error&&<Notice>{runtime.error}</Notice>}
        </div>
        <ModuleRemoval id={runtime.id} kind="runtime" manifestSha256={runtime.manifestSha256} disabled={!!busy||!runtime.canRemove||!!snapshot.configurationError} onActivity={onActivity} onChanged={async warning=>{await reload();onChanged();setNotice(warning||t('sharedPackages.removed'));}}/>
      </div>)}</div>
    </section>}
    </>}
    {notice&&<p role="status" className="muted">{notice}</p>}
  </section>;
}

export function DisabledModule({ name, onManage }: { name: string; onManage: () => void }) {
  const {t}=useTranslation();
  return <section className="empty"><h2>{t("module.disabledTitle",{name})}</h2><p>{t("module.retained")}</p><Button onClick={onManage}>{t("module.manage")}</Button></section>;
}
