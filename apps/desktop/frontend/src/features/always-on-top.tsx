import {Settings,SlidersHorizontal,Save,RotateCcw} from "lucide-react";
import {IconLabel} from "../ui/icon-label";
import {Notice,InlineError} from "../ui/notice";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "../components/ui/tabs";

import {Label} from "../components/ui/label";
import { useEffect, useState } from "react";
import { alwaysOnTopCall } from "../bridge";
import { Switch } from "../ui/switch";
import { Button } from "../ui/button";
import { useTranslation } from "../i18n";
import {Input} from "../components/ui/input";
import {NumberField,validNumber} from "../ui/number-field";
import {Textarea} from "../components/ui/textarea";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";

interface Preferences {
  schemaVersion: number; revision: number;
  frame: boolean; frameThickness: number; frameColor: string; frameOpacity: number;
  useAccent: boolean; roundCorners: boolean; blockInGameMode: boolean;
  sound: boolean; excludedApps: string[];
}
interface Snapshot { settings: Preferences; administrator?: boolean }


export function AlwaysOnTopPage({ disabled = false,onActivity }: { disabled?: boolean;onActivity?:PageActivityChanged }) {
  const {t}=useTranslation();
  const [tab,setTab]=useState("general");
  const [settings, setSettings] = useState<Preferences>();
  const [savedSettings,setSavedSettings]=useState<Preferences>();
  const [exclusions, setExclusions] = useState("");
  const [working, setBusy] = useState(false);
  const [loading,setLoading]=useState(false);
  const busy=working||loading;
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  // Compare the values that will be saved, including trimmed exclusion lines,
  // so undoing an edit also clears the navigation warning.
  const excludedApps=exclusions.split(/\r?\n/).map(value=>value.trim()).filter(Boolean);
  const dirty=!!settings&&JSON.stringify({...settings,excludedApps})!==JSON.stringify(savedSettings);

  usePageActivity(onActivity,dirty,working);

  async function refresh(includeSettings: boolean) {
    if (disabled && !includeSettings) return;
    const next: Snapshot = disabled
      ? { settings: await alwaysOnTopCall<Preferences>("settings.get"), administrator: false }
      : await alwaysOnTopCall<Snapshot>("snapshot");


    if (includeSettings) {
      setSettings(next.settings);
      setSavedSettings(next.settings);
      setExclusions(next.settings.excludedApps.join("\n"));
    }
  }
  async function action(run: () => Promise<void>,readOnly=false) {
    // Keep form reads local; only a save must hold the navigation guard.
    const setPending=readOnly?setLoading:setBusy;
    setPending(true); setError(""); setNotice("");
    try { await run(); } catch (e) { setError(String(e)); }
    finally { setPending(false); }
  }
  useEffect(() => { void action(() => refresh(true),true); }, [disabled]);
  function change<K extends keyof Preferences>(key: K, value: Preferences[K]) {
    if (settings) setSettings({ ...settings, [key]: value });
  }
  async function save() {
    if (!settings) return;
    const saved = await alwaysOnTopCall<Preferences>("settings.save", {
      ...settings, excludedApps,
    });
    setSettings(saved); setSavedSettings(saved); setExclusions(saved.excludedApps.join("\n"));
    setNotice("common.saved");
  }
  const navigationTabs=(<TabsList variant="line" className="max-w-full flex-wrap justify-start gap-1 group-data-horizontal/tabs:h-auto" aria-label={t("aot.settings")}><TabsTrigger value="general"><IconLabel icon={Settings}>{t("aot.general")}</IconLabel></TabsTrigger><TabsTrigger value="advanced"><IconLabel icon={SlidersHorizontal}>{t("aot.options")}</IconLabel></TabsTrigger></TabsList>);
  const unavailable=error
    ? <Notice actions={<Button disabled={busy} onClick={()=>void action(()=>refresh(true),true)}>{t("common.retry")}</Button>}>{error}</Notice>
    : <p role="status">{t("aot.loading")}</p>;


  const validNumbers=!!settings&&validNumber(settings.frameThickness,1,15)&&validNumber(settings.frameOpacity,10,100);
  const toggle=(key:"frame"|"useAccent"|"roundCorners"|"blockInGameMode"|"sound",label:string)=><div className="settings-switch-row py-3"><Label htmlFor={`aot-${key}`}>{t(label)}</Label><Switch id={`aot-${key}`} checked={!!settings?.[key]} disabled={busy} onCheckedChange={value=>change(key,value)}/></div>;
  return <section>
      <Tabs value={tab} onValueChange={value=>setTab(String(value))}>
        {navigationTabs}
    {error && settings && <Notice>{error}</Notice>}
    {disabled && <p className="muted">{t("aot.disabled")}</p>}
        <TabsContent value="general" className="space-y-3">{settings?<fieldset className="plain-fieldset settings-stack" disabled={busy}>
          {toggle("frame","aot.frame")}
          {(settings.frame||!validNumbers)&&<div className="settings-group border-t pt-5">
            {toggle("useAccent","aot.accent")}
            <div className="settings-switch-row"><Label htmlFor="aot-color">{t("aot.color")}</Label><Input id="aot-color" className="w-16 shrink-0 px-1" type="color" disabled={busy} value={settings.frameColor} onChange={event=>{
              // Choosing a custom color is an explicit alternative to the
              // accent color. Keep unrelated edits and commit both together.
              const frameColor=event.target.value;
              setSettings(current=>current?{...current,frameColor,useAccent:false}:current);
            }}/></div>
            <div className="settings-field-grid">
              <NumberField id="aot-thickness" label={t("aot.thickness")} min={1} max={15} value={settings.frameThickness} onChange={value=>change("frameThickness",value)}/>
              <NumberField id="aot-opacity" label={t("aot.opacity")} min={10} max={100} value={settings.frameOpacity} onChange={value=>change("frameOpacity",value)}/>
            </div>
            {toggle("roundCorners","aot.rounded")}
          </div>}
          {toggle("sound","aot.sound")}
        </fieldset>:unavailable}</TabsContent>
        <TabsContent value="advanced" className="space-y-4">{settings?<fieldset className="plain-fieldset settings-stack" disabled={busy}>
          {toggle("blockInGameMode","aot.game")}
          <div className="settings-stack-compact"><Label htmlFor="aot-exclusions">{t("aot.exclusions")}</Label><Textarea id="aot-exclusions" rows={4} value={exclusions} onChange={event=>setExclusions(event.target.value)}/></div>

        </fieldset>:unavailable}</TabsContent>

      </Tabs>
      {settings&&<fieldset className="plain-fieldset" disabled={busy}>
      {!validNumbers&&<InlineError className="text-sm">{t("aot.numberRange")}</InlineError>}
      <div className="settings-save-bar">
        <Button icon={Save} variant="primary" disabled={!dirty||!validNumbers} onClick={() => void action(save)}>{t("aot.save")}</Button>
        <Button icon={RotateCcw} disabled={!dirty} onClick={() => void action(() => refresh(true),true)}>{t("aot.undo")}</Button>
      </div>
      <p role="status">{dirty ? t("aot.dirty") : notice?t(notice):""}</p>
      </fieldset>}
  </section>;
}
