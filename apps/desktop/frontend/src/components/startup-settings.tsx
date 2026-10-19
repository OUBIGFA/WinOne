import {useTranslation} from "../i18n";
import {Power} from "lucide-react";
import {IconLabel} from "../ui/icon-label";
import {SettingsSection, SettingsToggle} from "./settings-layout";

export interface StartupPreferences {runAtLogon:boolean;startMinimized:boolean;preferAdministrator?:boolean;keepUIRunning?:boolean}
export function StartupSettings({value,disabled,onChange}:{value:StartupPreferences;disabled:boolean;onChange:(next:StartupPreferences)=>void}){
 const {t}=useTranslation();
 return <SettingsSection level={2} title={<IconLabel icon={Power}>{t("startup.title")}</IconLabel>}>
  <SettingsToggle id="winone-start-at-login" label={t("startup.logon")} description={t("startup.logonHint")} disabled={disabled} checked={value.runAtLogon} onChange={runAtLogon=>onChange({...value,runAtLogon})}/>
  <SettingsToggle id="winone-start-minimized" label={t("startup.silent")} description={t("startup.silentHint")} disabled={disabled} checked={value.startMinimized} onChange={startMinimized=>onChange({...value,startMinimized})}/>
  <SettingsToggle id="winone-keep-ui" label={t("startup.keepUI")} description={t("startup.keepUIHint")} disabled={disabled} checked={value.keepUIRunning??true} onChange={keepUIRunning=>onChange({...value,keepUIRunning})}/>
 </SettingsSection>;
}
