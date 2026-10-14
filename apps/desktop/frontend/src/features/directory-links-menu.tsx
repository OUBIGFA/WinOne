import {useEffect,useState} from "react";
import {directoryLinksCall} from "../bridge";
import {useTranslation} from "../i18n";
import {Switch} from "../components/ui/switch";
import {Label} from "../components/ui/label";
import {InlineError} from "../ui/notice";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";

export function DirectoryLinksMenu({disabled,onActivity}:{disabled:boolean;onActivity?:PageActivityChanged}){
 const {t}=useTranslation();const [state,setState]=useState<{enabled:boolean;partial:boolean;conflict?:string}>();const [busy,setBusy]=useState(false);const [error,setError]=useState("");
 usePageActivity(onActivity,false,busy,"directory-links-menu");
 useEffect(()=>{let live=true;void directoryLinksCall<NonNullable<typeof state>>("menu.get").then(value=>{if(live)setState(value);}).catch(e=>{if(live)setError(String(e));});return()=>{live=false;};},[]);
 async function change(enabled:boolean){setBusy(true);setError("");try{setState(await directoryLinksCall("menu.set",{enabled}));}catch(e){setError(String(e));}finally{setBusy(false);}}
 return <div className="directory-links-menu"><div className="directory-links-menu-setting"><div><Label htmlFor="directory-links-explorer-menu">{t("links.explorerIntegration")}</Label><p id="directory-links-explorer-hint" className="muted">{t("links.explorerMenuHint")}</p></div><Switch id="directory-links-explorer-menu" aria-describedby="directory-links-explorer-hint" checked={state?.enabled??false} disabled={disabled||busy||!state||!!state.conflict} onCheckedChange={value=>void change(value)}/></div>{state?.partial&&<InlineError>{t("links.explorerMenuPartial")}</InlineError>}{state?.conflict&&<InlineError>{state.conflict}</InlineError>}{error&&<InlineError>{error}</InlineError>}</div>;
}
