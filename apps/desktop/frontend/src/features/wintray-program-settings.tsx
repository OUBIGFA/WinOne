import {NumberField,validNumber} from "../ui/number-field";
import {Rocket,CalendarClock} from "lucide-react";
import {useState} from "react";
import type {Program} from "./wintray";
import {useTranslation} from "../i18n";
import {Input} from "../components/ui/input";
import {Label} from "../components/ui/label";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "../components/ui/tabs";
import {SettingsSection,SettingsOptions,SettingsToggle} from "../components/settings-layout";

export function validProgramTiming(program:Program){
  const {schedule,trayBehavior}=program;
  return validNumber(trayBehavior.closeDelaySeconds,0,600)&&validNumber(schedule.startDelayMinutes,0,1440)&&validNumber(schedule.autoExitMinutes,0,1440)
    // Even hidden unfinished edits must not become JSON null during saving.
    &&Number.isInteger(schedule.frequencyDays)&&Number.isInteger(schedule.frequencyRuns)
    &&(!schedule.frequencyEnabled||validNumber(schedule.frequencyDays,1,365)&&validNumber(schedule.frequencyRuns,1,1000));
}

export function WinTrayProgramSettings({program:p,blocked,update}:{program:Program;blocked:boolean;update:(patch:Partial<Program>)=>void}) {
  const {t}=useTranslation();
  // Pick the initial page once; editing the name must not reset tab state.
  const [tab,setTab]=useState(()=>p.name.trim()?"common":"advanced");
  const mode=p.launchViaLogonTask?"task":p.launchHiddenInBackground?"hidden":p.trayBehavior.autoMinimizeAndHideOnLaunch?"tray":"normal";
  const modes=(/\.exe$/i.test(p.exePath)?["tray","hidden","task","normal"]:["hidden"]).map(value=>({value,label:t(`wintray.${value}`)}));
  return <Tabs value={tab} onValueChange={value=>setTab(String(value))}>
    <TabsList variant="line" aria-label={p.name||t("wintray.advancedProgram")}><TabsTrigger value="common">{t("wintray.commonSettings")}</TabsTrigger><TabsTrigger value="advanced">{t("wintray.moreSettings")}</TabsTrigger></TabsList>
    <TabsContent value="common" className="settings-section-stack">
      <SettingsSection icon={Rocket} title={t("settingsDesign.launchBehavior")}>
      <SettingsToggle id={`startup-${p.id}`} label={t("wintray.startup")} checked={p.runOnStartup} disabled={blocked} onChange={runOnStartup=>update({runOnStartup})}/>
      <SettingsOptions id="program-mode" label={t("wintray.mode")} value={mode} items={modes} disabled={blocked}
        onChange={value=>update({launchViaLogonTask:value==="task",launchHiddenInBackground:value==="hidden",trayBehavior:{...p.trayBehavior,autoMinimizeAndHideOnLaunch:value==="tray"}})}/>
      {mode==="tray"&&<div className="settings-dependent"><NumberField label={t("wintray.closeDelay")} value={p.trayBehavior.closeDelaySeconds} max={600} onChange={value=>update({trayBehavior:{...p.trayBehavior,closeDelaySeconds:value}})}/></div>}
      </SettingsSection>
      <SettingsSection icon={CalendarClock} title={t("settingsDesign.schedule")}>
      <SettingsToggle id={`schedule-${p.id}`} label={t("wintray.schedule")} checked={p.schedule.enabled} disabled={blocked} onChange={enabled=>update({schedule:{...p.schedule,enabled}})}/>
      {p.schedule.enabled&&<div className="settings-dependent settings-field-grid">
        <NumberField label={t("wintray.startDelay")} value={p.schedule.startDelayMinutes} max={1440} onChange={value=>update({schedule:{...p.schedule,startDelayMinutes:value}})}/>
        <NumberField label={t("wintray.autoExit")} value={p.schedule.autoExitMinutes} max={1440} onChange={value=>update({schedule:{...p.schedule,autoExitMinutes:value}})}/>
      </div>}
      {/* Timing and run limits share one plan and the same saved schedule. */}
      <SettingsToggle id={`frequency-${p.id}`} label={t("wintray.limitRuns")} disabled={blocked||!p.schedule.enabled} checked={p.schedule.frequencyEnabled} onChange={frequencyEnabled=>update({schedule:{...p.schedule,frequencyEnabled}})}/>
      {!p.schedule.enabled&&<p className="settings-help">{t("settingsDesign.scheduleRequired")}</p>}
      {p.schedule.enabled&&p.schedule.frequencyEnabled&&<div className="settings-dependent settings-field-grid">
        <NumberField label={t("wintray.days")} value={p.schedule.frequencyDays} min={1} max={365} onChange={value=>update({schedule:{...p.schedule,frequencyDays:value}})}/>
        <NumberField label={t("wintray.runs")} value={p.schedule.frequencyRuns} min={1} max={1000} onChange={value=>update({schedule:{...p.schedule,frequencyRuns:value}})}/>
      </div>}
      </SettingsSection>
    </TabsContent>
    <TabsContent value="advanced" className="settings-stack">
      <div className="space-y-2"><Label htmlFor="program-name">{t("wintray.name")}</Label><Input id="program-name" required aria-invalid={!p.name.trim()} aria-describedby={p.name.trim()?undefined:"wintray-name-error"} value={p.name} onChange={event=>update({name:event.target.value})}/></div>
      <div className="space-y-2"><Label htmlFor="program-args">{t("wintray.args")}</Label><Input id="program-args" value={p.args} onChange={event=>update({args:event.target.value})}/></div>
    </TabsContent>
  </Tabs>;
}
