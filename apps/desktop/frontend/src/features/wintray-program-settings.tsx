import {NumberField,validNumber} from "../ui/number-field";
import {useState} from "react";
import type {Program} from "./wintray";
import {useTranslation} from "../i18n";
import {Input} from "../components/ui/input";
import {Label} from "../components/ui/label";
import {Switch} from "../components/ui/switch";
import {Checkbox} from "../components/ui/checkbox";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "../components/ui/tabs";
import {Select,SelectTrigger,SelectValue,SelectContent,SelectItem} from "../components/ui/select";

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
  const [initialTab]=useState(()=>p.name.trim()?"common":"advanced");
  const mode=p.launchViaLogonTask?"task":p.launchHiddenInBackground?"hidden":p.trayBehavior.autoMinimizeAndHideOnLaunch?"tray":"normal";
  const modes=(/\.exe$/i.test(p.exePath)?["tray","hidden","task","normal"]:["hidden"]).map(value=>({value,label:t(`wintray.${value}`)}));
  return <Tabs defaultValue={initialTab}>
    <TabsList variant="line" aria-label={p.name||t("wintray.advancedProgram")}><TabsTrigger value="common">{t("wintray.commonSettings")}</TabsTrigger><TabsTrigger value="advanced">{t("wintray.moreSettings")}</TabsTrigger></TabsList>
    <TabsContent value="common" className="space-y-5 pt-3">
      <div className="flex items-center justify-between gap-4"><Label htmlFor={`startup-${p.id}`}>{t("wintray.startup")}</Label><Switch id={`startup-${p.id}`} checked={p.runOnStartup} disabled={blocked} onCheckedChange={value=>update({runOnStartup:value})}/></div>
      <div className="space-y-2"><Label htmlFor="program-mode">{t("wintray.mode")}</Label>
        <Select value={mode} items={modes} disabled={blocked} onValueChange={value=>{if(value)update({launchViaLogonTask:value==="task",launchHiddenInBackground:value==="hidden",trayBehavior:{...p.trayBehavior,autoMinimizeAndHideOnLaunch:value==="tray"}});}}>
          <SelectTrigger id="program-mode" className="w-full"><SelectValue/></SelectTrigger><SelectContent>{modes.map(item=><SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent>
        </Select>
      </div>
      {mode==="tray"&&<NumberField label={t("wintray.closeDelay")} value={p.trayBehavior.closeDelaySeconds} max={600} onChange={value=>update({trayBehavior:{...p.trayBehavior,closeDelaySeconds:value}})}/>}
      <div className="flex items-center justify-between gap-4 border-t pt-5"><Label htmlFor={`schedule-${p.id}`}>{t("wintray.schedule")}</Label><Switch id={`schedule-${p.id}`} checked={p.schedule.enabled} disabled={blocked} onCheckedChange={value=>update({schedule:{...p.schedule,enabled:value}})}/></div>
      {p.schedule.enabled&&<div className="grid gap-4">
        <NumberField label={t("wintray.startDelay")} value={p.schedule.startDelayMinutes} max={1440} onChange={value=>update({schedule:{...p.schedule,startDelayMinutes:value}})}/>
        <NumberField label={t("wintray.autoExit")} value={p.schedule.autoExitMinutes} max={1440} onChange={value=>update({schedule:{...p.schedule,autoExitMinutes:value}})}/>
      </div>}
    </TabsContent>
    <TabsContent value="advanced" className="space-y-5 pt-3">
      <div className="space-y-2"><Label htmlFor="program-name">{t("wintray.name")}</Label><Input id="program-name" required aria-invalid={!p.name.trim()} aria-describedby={p.name.trim()?undefined:"wintray-name-error"} value={p.name} onChange={event=>update({name:event.target.value})}/></div>
      <div className="space-y-2"><Label htmlFor="program-args">{t("wintray.args")}</Label><Input id="program-args" value={p.args} onChange={event=>update({args:event.target.value})}/></div>
      <div className="flex items-center gap-2"><Checkbox id={`frequency-${p.id}`} disabled={blocked||!p.schedule.enabled} checked={p.schedule.frequencyEnabled} onCheckedChange={value=>update({schedule:{...p.schedule,frequencyEnabled:value}})}/><Label htmlFor={`frequency-${p.id}`}>{t("wintray.limitRuns")}</Label></div>
      {p.schedule.enabled&&p.schedule.frequencyEnabled&&<div className="grid gap-4">
        <NumberField label={t("wintray.days")} value={p.schedule.frequencyDays} min={1} max={365} onChange={value=>update({schedule:{...p.schedule,frequencyDays:value}})}/>
        <NumberField label={t("wintray.runs")} value={p.schedule.frequencyRuns} min={1} max={1000} onChange={value=>update({schedule:{...p.schedule,frequencyRuns:value}})}/>
      </div>}
    </TabsContent>
  </Tabs>;
}
