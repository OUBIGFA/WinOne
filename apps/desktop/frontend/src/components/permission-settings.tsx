import {InlineError} from "../ui/notice";
import {SettingsSection, SettingsToggle} from "./settings-layout";
import {Button} from "../ui/button";
import {useTranslation} from "../i18n";
import {ShieldCheck} from "lucide-react";
import {IconLabel} from "../ui/icon-label";

export function PermissionSettings({administrator,remembered,disabled,restarting,onRemember,onRestart,error}:{administrator?:boolean;remembered:boolean;disabled:boolean;restarting:boolean;onRemember:(value:boolean)=>void;onRestart:()=>void;error?:string}){
 const {t}=useTranslation();
 return <SettingsSection level={2} title={<IconLabel icon={ShieldCheck}>{t("permission.title")}</IconLabel>}>
  <p className="text-muted-foreground">{t(administrator===undefined?"permission.loading":administrator?"permission.administrator":"permission.standard")}</p>
  <SettingsToggle id="winone-always-administrator" label={t("permission.remember")} description={t("permission.consent")} checked={remembered} disabled={disabled} onChange={onRemember}/>
  {remembered&&!administrator&&<p className="text-sm text-muted-foreground">{t("permission.nextStart")}</p>}
  {!remembered&&administrator&&<p className="text-sm text-muted-foreground">{t("permission.untilExit")}</p>}
  <Button icon={ShieldCheck} disabled={disabled||administrator!==false} busy={restarting} onClick={onRestart}>{t(restarting?"permission.restarting":"permission.restart")}</Button>
  {error&&<InlineError>{error}</InlineError>}
 </SettingsSection>;
}
