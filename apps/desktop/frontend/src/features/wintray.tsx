import {Notice,InlineError} from "../ui/notice";
import { useEffect, useState } from "react";
import { pickProgram, pickPrograms, programInfo, wintrayCall } from "../bridge";
import {Diagnostics} from "./diagnostics";
import { ResidencyStatus } from "./residency";
import { Button } from "../ui/button";
import { useTranslation } from "../i18n";
import {ArrowUp,ArrowDown,FolderOpen,AppWindow} from "lucide-react";
import "./wintray.css";
import {Switch} from "../components/ui/switch";
import {Label} from "../components/ui/label";
import {Item,ItemGroup} from "../components/ui/item";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "../components/ui/tabs";
import {ResizablePanelGroup,ResizablePanel,ResizableHandle} from "../components/ui/resizable";
import {WinTrayProgramSettings,validProgramTiming} from "./wintray-program-settings";
import {NumberField,validNumber} from "../ui/number-field";
import {WinTrayIcons,useCollectedIcons,iconsForProgram} from "./wintray-icons";
import {WinTrayMaintenance} from "./wintray-maintenance";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";

export interface Program {
  id: string;
  name: string;
  exePath: string;
  args: string;
  runOnStartup: boolean;
  collectTrayIcon: boolean;
  launchViaLogonTask: boolean;
  launchHiddenInBackground: boolean;
  trayBehavior: {
    autoMinimizeAndHideOnLaunch: boolean;
    closeDelaySeconds: number;
  };
  schedule: {
    enabled: boolean;
    startDelayMinutes: number;
    autoExitMinutes: number;
    frequencyEnabled: boolean;
    frequencyDays: number;
    frequencyRuns: number;
  };
}
interface TraySettings {
  schemaVersion: number;
  language: string;
  runAtLogon: boolean;
  startMinimizedToTray: boolean;
  exitAfterManagedAppsCompleted: boolean;
  closeWindowRetrySeconds: number;
  startupIntervalSeconds: number;
  managedApps: Program[];
}
interface TraySnapshot {
  revision: string;
  settings: TraySettings;
  recoveryError?: string;
  backgroundError?: string;
  startupError?: string;
}
interface Job {
  id: string;
  state: string;
  error?: string;
  result?: unknown;
}

export function WinTrayPage({ disabled = false,onChanged=()=>{},onActivity }: { disabled?: boolean;onChanged?:()=>void;onActivity?:PageActivityChanged }) {
  const {t}=useTranslation();
  const [view,setView]=useState("programs");
  const [saved, setSaved] = useState<TraySnapshot>();
  const [draft, setDraft] = useState<TraySettings>();
  const [selected, setSelected] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [dragged, setDragged] = useState("");
  async function reload(preserveDraft = false) {
    if (!preserveDraft) setError("");
    try {
      const next = await wintrayCall<TraySnapshot>(disabled ? "settings.get" : "snapshot");
      setSaved((current) =>
        preserveDraft && current
          ? {
              ...current,
              recoveryError: next.recoveryError,
              backgroundError: next.backgroundError,
              startupError: next.startupError,
            }
          : next,
      );
      if (!preserveDraft) { setSelected(current=>next.settings.managedApps.some(p=>p.id===current)?current:next.settings.managedApps[0]?.id??""); setDraft(next.settings); }
      if (next.recoveryError || next.backgroundError)
        setError(next.recoveryError || next.backgroundError || "");
    } catch (e) {
      setError(String(e));
    }
  }
  useEffect(() => {
    void reload();
  }, [disabled]);
  const apps = draft?.managedApps ?? [];
  const [metadata,setMetadata]=useState<Record<string,{name:string;icon?:string}>>({});
  const programPaths=JSON.stringify([...new Set(apps.map(program=>program.exePath))]);
  useEffect(()=>{
    let alive=true;
    const pending=(JSON.parse(programPaths) as string[]).filter(path=>!metadata[path]);let cursor=0;
    // Bound Shell reads so a long program list does not create a thread per row.
    async function load(){while(alive&&cursor<pending.length){const path=pending[cursor++];
      try{const info=await programInfo(path);if(alive)setMetadata(current=>({...current,[path]:info}));}
      catch{if(alive)setMetadata(current=>({...current,[path]:{name:""}}));}
    }}
    for(let worker=0;worker<Math.min(4,pending.length);worker++)void load();
    return()=>{alive=false;};
  },[programPaths]);
  // Saved/edited names are authoritative. Metadata seeds Add/Replace only;
  // a fallback keeps a temporarily empty draft identifiable in the list.
  const displayName=(program:Program)=>program.name.trim()?program.name:(metadata[program.exePath]?.name||program.exePath.split(/[\\/]/).pop()||t("wintray.name"));
  const collected=useCollectedIcons(!disabled&&view==="programs"&&!!saved?.settings.managedApps.some(program=>program.collectTrayIcon||program.launchHiddenInBackground||program.trayBehavior.autoMinimizeAndHideOnLaunch));
  const active = apps.find((a) => a.id === selected);
  const invalidTimingSections=apps.filter(program=>!validProgramTiming(program)).map(displayName);
  if(draft&&(!validNumber(draft.startupIntervalSeconds,0,120)||!validNumber(draft.closeWindowRetrySeconds,0,120)))invalidTimingSections.push(t("wintray.advancedStartup"));
  const validNumbers=invalidTimingSections.length===0;
  const invalidNames=apps.filter(program=>!program.name.trim()).map(displayName);
  const validNames=invalidNames.length===0;
  const dirty =
    !!saved && JSON.stringify(saved.settings) !== JSON.stringify(draft);
  usePageActivity(onActivity,dirty,busy);
  function update(patch: Partial<Program>, id=selected) {
    if(busy||saved?.recoveryError)return;
    setDraft((current) =>
      current
        ? {
            ...current,
            managedApps: current.managedApps.map((a) =>
              a.id === id ? { ...a, ...patch } : a,
            ),
          }
        : current,
    );
  }
  function reorder(from: string, to: string) {
    if (busy || saved?.recoveryError || from === to) return;
    setDraft((current) => {
      if (!current) return current;
      const next = [...current.managedApps];
      const source = next.findIndex((a) => a.id === from),
        target = next.findIndex((a) => a.id === to);
      if (source < 0 || target < 0) return current;
      next.splice(target, 0, ...next.splice(source, 1));
      return { ...current, managedApps: next };
    });
  }
  async function add() {
    if(busy)return;
    setBusy(true);setError("");setMessage("");
    try {
      const paths = await pickPrograms();
      if (!paths.length) return;
      const infos=await Promise.all(paths.map(path=>programInfo(path)));
      setMetadata(current=>({...current,...Object.fromEntries(paths.map((path,index)=>[path,infos[index]]))}));
      const entries = paths.map((path,index):Program=>{
      const exe = /\.exe$/i.test(path);
      return {
        id: crypto.randomUUID(),
        name: infos[index].name || path
          .split(/[\\/]/)
          .pop()!
          .replace(/\.[^.]+$/, ""),
        exePath: path,
        args: "",
        runOnStartup: true,
        collectTrayIcon: false,
        launchViaLogonTask: false,
        launchHiddenInBackground: !exe,
        trayBehavior: {
          autoMinimizeAndHideOnLaunch: exe,
          closeDelaySeconds: 0,
        },
        schedule: {
          enabled: false,
          startDelayMinutes: 0,
          autoExitMinutes: 30,
          frequencyEnabled: false,
          frequencyDays: 1,
          frequencyRuns: 1,
        },
      };
      });
      setDraft((s) =>
        s ? { ...s, managedApps: [...s.managedApps, ...entries] } : s,
      );
      setSelected(entries[entries.length-1].id);
    } catch (e) {
      setError(String(e));
    } finally {setBusy(false);}
  }
  async function replaceProgram(id:string) {
    if(busy)return;
    setBusy(true);setError("");setMessage("");
    try {
      const path=await pickProgram();if(!path)return;
      const info=await programInfo(path);setMetadata(current=>({...current,[path]:info}));
      // Preserve identity, arguments and scheduling. A different target cannot
      // inherit collected icons; scripts also cannot use executable-only modes.
      setDraft(current=>current?{...current,managedApps:current.managedApps.map(program=>{
        if(program.id!==id||program.exePath.replaceAll("/","\\").toLowerCase()===path.replaceAll("/","\\").toLowerCase())return program;
        const script=!/\.exe$/i.test(path);
        return {...program,exePath:path,name:info.name||path.split(/[\\/]/).pop()!.replace(/\.[^.]+$/,""),collectTrayIcon:false,
          launchViaLogonTask:script?false:program.launchViaLogonTask,
          launchHiddenInBackground:script?true:program.launchHiddenInBackground,
          trayBehavior:{...program.trayBehavior,autoMinimizeAndHideOnLaunch:script?false:program.trayBehavior.autoMinimizeAndHideOnLaunch}};
      })}:current);
    } catch(e){setError(String(e));}finally{setBusy(false);}
  }
  async function runJob(method: string, params: unknown) {
    if(method==="settings.save"&&(!validNumbers||!validNames))return;
    setBusy(true);
    setError("");
    setMessage("wintray.working");
    try {
      let job = await wintrayCall<Job>(method, params);
      // Completed configuration commits need no polling ID. Running worker
      // operations must retain the original ID until their result arrives.
      const id=job?.id;
      if(!job||(job.state==="running"&&(typeof id!=="string"||!id)))throw new Error(t("wintray.unknownResult"));
      const deadline = Date.now() + 5 * 60_000;
      while (job.state === "running") {
        if (Date.now() > deadline)
          throw new Error(t("wintray.pending"));
        await new Promise((resolve) => setTimeout(resolve, 250));
        job = await wintrayCall<Job>("jobs.get", { id });
        if(job?.id!==id)throw new Error(t("wintray.unknownResult"));
      }
      if (job.state !== "completed") throw new Error(job.error ?? t("wintray.failed"));
      setMessage(
        method === "launch"
          ? "wintray.launched"
          : method === "recover"
            ? "wintray.recovered"
            : "common.saved",
      );
      await reload();
    } catch (e) {
      setError(String(e));
      setMessage("");
      await reload(true);
    } finally {
      setBusy(false);
    }
  }
  const navigationTabs=(<TabsList className="max-w-full flex-wrap justify-start gap-1 group-data-horizontal/tabs:h-auto" aria-label={t("nav.wintray")}><TabsTrigger value="programs">{t("wintray.list")}</TabsTrigger><TabsTrigger value="startup">{t("wintray.advancedStartup")}</TabsTrigger><TabsTrigger value="logs">{t("diagnostics.title")}</TabsTrigger></TabsList>);
  const unavailable=error
    ? <Notice actions={<Button disabled={busy} onClick={()=>void reload()}>{t("common.retry")}</Button>}>{error}</Notice>
    : <p role="status">{t("wintray.loading")}</p>;



  return (
    <section>
      {error && draft && (
        <Notice actions={<Button disabled={busy} onClick={() => void reload()}>{t("wintray.discard")}</Button>}>{error}</Notice>
      )}
      {disabled && <p className="muted">{t("wintray.disabled")}</p>}
      {!disabled && saved?.recoveryError && (
        <Button disabled={busy} onClick={() => void runJob("recover", null)}>{t("wintray.recover")}</Button>
      )}
      {!disabled && saved?.startupError && <Notice actions={<Button disabled={busy || dirty || !!saved.recoveryError} onClick={()=>void runJob("startup.retry",null)}>{t("wintray.retryStartup")}</Button>}>{saved.startupError}</Notice>}
      <Tabs value={view} onValueChange={value=>setView(String(value))}>
        {navigationTabs}
        {draft&&view!=="logs"&&<>
      <div className="module-toolbar">
        <Button
          variant="primary"
          disabled={busy || !!saved?.recoveryError}
          onClick={() => void add()}
        >
          {t("wintray.add")}
        </Button>
        <span role="status">
          {busy ? (message?t(message):"") : dirty ? t("wintray.dirty") : message?t(message):""}
        </span>
        <Button
          disabled={busy || !dirty || !validNumbers || !validNames || !!saved?.recoveryError}
          onClick={() =>
            void runJob("settings.save", {
              revision: saved!.revision,
              settings: draft,
            })
          }
        >
          {t("wintray.save")}
        </Button>
      </div>
      {!validNumbers&&<InlineError>{t("wintray.invalidNumbers",{sections:invalidTimingSections.join("、")})}</InlineError>}
      {!validNames&&<InlineError id="wintray-name-error">{t("wintray.nameRequired",{programs:invalidNames.join(", ")})}</InlineError>}
        </>}
        <TabsContent value="programs">{draft?<>
          {collected.error&&<InlineError>{collected.error}</InlineError>}
          <fieldset disabled={busy||!!saved?.recoveryError} className="plain-fieldset">
            <div className="wintray-workspace"><ResizablePanelGroup orientation="horizontal" className="wintray-panels min-h-[440px] border-t" disabled={busy}>
              <ResizablePanel defaultSize="44%" minSize="34%" style={{overflowX:"clip"}}>
                <ItemGroup aria-label={t("wintray.list")} className="gap-1 py-3 pr-4">
                  {!apps.length&&<p className="py-4 text-muted-foreground">{t("wintray.empty")}</p>}
                  {apps.map((a,index)=><Item key={a.id} role="listitem" aria-label={displayName(a)} aria-current={selected===a.id?"true":undefined} tabIndex={busy?-1:0} onClick={()=>{if(!busy)setSelected(a.id);}} onKeyDown={event=>{if(event.target===event.currentTarget&&!busy&&(event.key==="Enter"||event.key===" ")){event.preventDefault();setSelected(a.id);}}} variant={selected===a.id?"muted":"default"} className="min-w-0 cursor-pointer gap-3" draggable={!busy} onDragStart={()=>setDragged(a.id)} onDragOver={event=>event.preventDefault()} onDrop={()=>{if(!busy)reorder(dragged,a.id);}}>
                    {metadata[a.exePath]?.icon?<img src={`data:image/png;base64,${metadata[a.exePath].icon}`} alt="" className="size-8 shrink-0"/>:<AppWindow aria-hidden="true" className="size-8 shrink-0 text-muted-foreground"/>}
                    <span className="min-w-0 flex-1 break-words">{displayName(a)}</span>
                    <span className="text-muted-foreground">{t(a.runOnStartup?"wintray.enabled":"wintray.paused")}</span>
                    <div className="flex w-full flex-wrap items-center justify-between gap-2">
                      <div className="flex items-center gap-2"><Switch id={"collect-"+a.id} checked={a.collectTrayIcon} disabled={busy||!!saved?.recoveryError||!/\.exe$/i.test(a.exePath)} onCheckedChange={value=>update({collectTrayIcon:value},a.id)}/><Label htmlFor={"collect-"+a.id}>{t("wintray.collect")}</Label></div>
                      <div className="flex items-center gap-1">
                        <Button variant="ghost" size="sm" aria-label={t("wintray.replaceNamed",{name:displayName(a)})} onClick={()=>void replaceProgram(a.id)}><FolderOpen size={16}/>{t("wintray.replaceProgram")}</Button>
                        <Button variant="ghost" size="icon" aria-label={t("wintray.moveUp",{name:displayName(a)})} disabled={index===0} onClick={()=>reorder(a.id,apps[index-1].id)}><ArrowUp size={16}/></Button>
                        <Button variant="ghost" size="icon" aria-label={t("wintray.moveDown",{name:displayName(a)})} disabled={index===apps.length-1} onClick={()=>reorder(a.id,apps[index+1].id)}><ArrowDown size={16}/></Button>
                      </div>
                    </div>
                    <WinTrayIcons icons={iconsForProgram(collected.icons,a.exePath)} disabled={busy}/>
                  </Item>)}
                </ItemGroup>
              </ResizablePanel>
              <ResizableHandle withHandle aria-label={t("wintray.resizePanels")}/>
              <ResizablePanel minSize="40%" style={{overflowX:"clip"}}>
                <div className="wintray-program-detail min-w-0 space-y-4 py-4 pl-5">
                  {active?<>
                    <div><h2 className="break-words">{displayName(active)}</h2><p className="break-all text-sm text-muted-foreground">{active.exePath}</p></div>
                    <div className="flex flex-wrap gap-2" role="group" aria-label={t("wintray.programActions")}>
                      <Button disabled={disabled||dirty} onClick={()=>void runJob("launch",{id:active.id})}>{t("wintray.launch")}</Button>
                      <Button onClick={()=>{const next=apps.filter(a=>a.id!==active.id);setDraft({...draft,managedApps:next});setSelected(next[0]?.id??"");}}>{t("wintray.remove")}</Button>
                    </div>
                    <WinTrayProgramSettings key={active.id} program={active} blocked={busy||!!saved?.recoveryError} update={update}/>
                  </>:<p className="text-muted-foreground">{t("wintray.select")}</p>}
                </div>
              </ResizablePanel>
            </ResizablePanelGroup></div>
          </fieldset>
        </>:unavailable}</TabsContent>
        <TabsContent value="startup">{draft?<>
          <fieldset disabled={busy||!!saved?.recoveryError} className="plain-fieldset space-y-5 py-4">
            <div className="flex items-center justify-between gap-4"><Label htmlFor="wintray-exit">{t("wintray.exitWhenDone")}</Label><Switch id="wintray-exit" checked={draft.exitAfterManagedAppsCompleted} disabled={busy||!!saved?.recoveryError} onCheckedChange={value=>setDraft({...draft,exitAfterManagedAppsCompleted:value})}/></div>
            {!disabled&&view==="startup"&&<ResidencyStatus/>}
            <div className="grid gap-5 sm:grid-cols-2">
              <NumberField label={t("wintray.interval")} value={draft.startupIntervalSeconds} max={120} onChange={value=>setDraft({...draft,startupIntervalSeconds:value})}/>
              <NumberField label={t("wintray.windowWait")} value={draft.closeWindowRetrySeconds} max={120} onChange={value=>setDraft({...draft,closeWindowRetrySeconds:value})}/>
            </div>
            <WinTrayMaintenance disabled={busy||dirty||!!saved?.recoveryError} onBusyChange={setBusy} onChanged={async()=>{await reload();onChanged();}}/>
          </fieldset>
        </>:unavailable}</TabsContent>
        <TabsContent value="logs">{view==="logs"&&<Diagnostics moduleId="wintray"/>}</TabsContent>
      </Tabs>
    </section>
  );
}
