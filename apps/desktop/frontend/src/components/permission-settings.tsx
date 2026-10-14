import {InlineError} from "../ui/notice";
import {Switch} from "./ui/switch";
import {Label} from "./ui/label";
import {Button} from "../ui/button";
import {useTranslation} from "../i18n";
import {ShieldCheck} from "lucide-react";
import {IconLabel} from "../ui/icon-label";

export function PermissionSettings({administrator,remembered,disabled,restarting,onRemember,onRestart,error}:{administrator?:boolean;remembered:boolean;disabled:boolean;restarting:boolean;onRemember:(value:boolean)=>void;onRestart:()=>void;error?:string}){
 const {t}=useTranslation();
 return <section className="settings-section">
  <h2><IconLabel icon={ShieldCheck}>{t("permission.title")}</IconLabel></h2>
  <p className="text-muted-foreground">{t(administrator===undefined?"permission.loading":administrator?"permission.administrator":"permission.standard")}</p>
  <div className="setting-row"><div><Label htmlFor="winone-always-administrator">{t("permission.remember")}</Label><p>{t("permission.consent")}</p></div><Switch id="winone-always-administrator" checked={remembered} disabled={disabled} onCheckedChange={onRemember}/></div>
  {remembered&&!administrator&&<p className="text-sm text-muted-foreground">{t("permission.nextStart")}</p>}
  {!remembered&&administrator&&<p className="text-sm text-muted-foreground">{t("permission.untilExit")}</p>}
  <Button icon={ShieldCheck} disabled={disabled||administrator!==false} busy={restarting} onClick={onRestart}>{t(restarting?"permission.restarting":"permission.restart")}</Button>
  {error&&<InlineError>{error}</InlineError>}
 </section>;
}
