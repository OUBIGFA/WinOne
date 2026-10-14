import {useState} from "react";
import {FolderOpen,Search} from "lucide-react";
import {directoryLinksCall,pickDirectory} from "../bridge";
import {useTranslation} from "../i18n";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";
import {Button} from "../ui/button";
import {Notice} from "../ui/notice";
import {Input} from "../components/ui/input";
import {Label} from "../components/ui/label";
import {Checkbox} from "../components/ui/checkbox";
import type {DirectoryLinkRecord} from "./directory-links";

export function DirectoryLinksScan({disabled=false,onActivity,onChanged}:{disabled?:boolean;onActivity?:PageActivityChanged;onChanged:(rows:DirectoryLinkRecord[])=>void}){
 const {t}=useTranslation();const [directory,setDirectory]=useState("");const [recursive,setRecursive]=useState(true);const [busy,setBusy]=useState(false);const [error,setError]=useState("");const [result,setResult]=useState<{records:DirectoryLinkRecord[];issues:{path:string;error:string}[];scanned:number;truncated:boolean;cancelled:boolean}>();
 usePageActivity(onActivity,false,busy,"directory-links-scan");
 const locked=busy||disabled;
 async function scan(){setBusy(true);setError("");setResult(undefined);try{const value=await directoryLinksCall<NonNullable<typeof result>>("scan",{directory,recursive});setResult(value);onChanged(value.records);}catch(e){setError(String(e));}finally{setBusy(false);}}
 return <section className="settings-stack" aria-label={t("links.findExisting")}>
  <p className="muted">{t("links.scanHint")}</p>{error&&<Notice>{error}</Notice>}
  <div className="directory-links-field"><Label htmlFor="links-scan-directory">{t("links.scanDirectory")}</Label><div className="directory-links-input-action"><Input id="links-scan-directory" disabled={locked} value={directory} onChange={e=>setDirectory(e.target.value)}/><Button disabled={locked} icon={FolderOpen} onClick={()=>{void pickDirectory().then(value=>{if(value)setDirectory(value);}).catch(e=>setError(String(e)));}}>{t("links.folder")}</Button></div></div>
  <Label className="directory-links-check"><Checkbox checked={recursive} disabled={locked} onCheckedChange={setRecursive}/>{t("links.recursive")}</Label>
  <div className="directory-links-actions"><Button busy={busy} disabled={locked||!directory} icon={Search} onClick={()=>void scan()}>{t("links.scan")}</Button>{busy&&<Button onClick={()=>{void directoryLinksCall("scan.cancel").catch(e=>setError(String(e)));}}>{t("links.cancelScan")}</Button>}</div>
  {result&&<><Notice tone="info">{t("links.scanResult",{count:result.records.length,scanned:result.scanned})}{result.truncated&&<p>{t("links.scanLimit")}</p>}{result.cancelled&&<p>{t("links.scanCancelled")}</p>}</Notice>{result.issues.length>0&&<details><summary>{t("links.scanIssues",{count:result.issues.length})}</summary>{result.issues.map((issue,index)=><p key={index} className="directory-links-error">{issue.path}：{issue.error}</p>)}</details>}</>}
 </section>;
}
