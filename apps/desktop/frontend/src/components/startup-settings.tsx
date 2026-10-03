import {Switch} from "./ui/switch";
import {Label} from "./ui/label";
import {useTranslation} from "../i18n";

export interface StartupPreferences {runAtLogon:boolean;startMinimized:boolean;preferAdministrator?:boolean}
export function StartupSettings({value,disabled,onChange}:{value:StartupPreferences;disabled:boolean;onChange:(next:StartupPreferences)=>void}){
 const {t}=useTranslation();
 return <section className="mt-8 space-y-2">
  <h2>{t("startup.title")}</h2>
  <div className="setting-row"><div><Label htmlFor="winone-start-at-login">{t("startup.logon")}</Label><p>{t("startup.logonHint")}</p></div><Switch id="winone-start-at-login" disabled={disabled} checked={value.runAtLogon} onCheckedChange={checked=>onChange({...value,runAtLogon:checked})}/></div>
  <div className="setting-row"><div><Label htmlFor="winone-start-minimized">{t("startup.silent")}</Label><p>{t("startup.silentHint")}</p></div><Switch id="winone-start-minimized" disabled={disabled} checked={value.startMinimized} onCheckedChange={checked=>onChange({...value,startMinimized:checked})}/></div>
 </section>;
}
