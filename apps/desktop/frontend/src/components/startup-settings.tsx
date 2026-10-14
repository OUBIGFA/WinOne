import {Switch} from "./ui/switch";
import {Label} from "./ui/label";
import {useTranslation} from "../i18n";
import {Power} from "lucide-react";
import {IconLabel} from "../ui/icon-label";

export interface StartupPreferences {runAtLogon:boolean;startMinimized:boolean;preferAdministrator?:boolean;keepUIRunning?:boolean}
export function StartupSettings({value,disabled,onChange}:{value:StartupPreferences;disabled:boolean;onChange:(next:StartupPreferences)=>void}){
 const {t}=useTranslation();
 return <section className="settings-section">
  <h2><IconLabel icon={Power}>{t("startup.title")}</IconLabel></h2>
  <div className="setting-row"><div><Label htmlFor="winone-start-at-login">{t("startup.logon")}</Label><p>{t("startup.logonHint")}</p></div><Switch id="winone-start-at-login" disabled={disabled} checked={value.runAtLogon} onCheckedChange={checked=>onChange({...value,runAtLogon:checked})}/></div>
  <div className="setting-row"><div><Label htmlFor="winone-start-minimized">{t("startup.silent")}</Label><p>{t("startup.silentHint")}</p></div><Switch id="winone-start-minimized" disabled={disabled} checked={value.startMinimized} onCheckedChange={checked=>onChange({...value,startMinimized:checked})}/></div>
  <div className="setting-row"><div><Label htmlFor="winone-keep-ui">{t("startup.keepUI")}</Label><p>{t("startup.keepUIHint")}</p></div><Switch id="winone-keep-ui" disabled={disabled} checked={value.keepUIRunning??true} onCheckedChange={checked=>onChange({...value,keepUIRunning:checked})}/></div>
 </section>;
}
