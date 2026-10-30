import {useState} from "react";
import {Download,Upload} from "lucide-react";
import {directoryLinksCall,pickDirectory,pickFile} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {Notice} from "../ui/notice";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";
import {Dialog,DialogContent,DialogDescription,DialogFooter,DialogTitle} from "../components/ui/dialog";
import type {DirectoryLinkRecord} from "./directory-links";

type ImportPreview={id?:string;expiresAt?:string;records:DirectoryLinkRecord[];issues:{path:string;error:string}[]};

// Reuse the page's native pickers and confirmation primitives. A backup is a
// list of relationships only; preview and commit both verify the local disk.
export function DirectoryLinksArchive({ids,disabled=false,onChanged,onActivity}:{ids:string[];disabled?:boolean;onChanged:(rows:DirectoryLinkRecord[])=>void;onActivity?:PageActivityChanged}){
 const {t}=useTranslation();
 const [busy,setBusy]=useState(false),[preview,setPreview]=useState<ImportPreview>(),[error,setError]=useState(""),[notice,setNotice]=useState("");
 usePageActivity(onActivity,!!preview,busy,"directory-links-archive",()=>setPreview(undefined));
 const locked=busy||disabled;
 async function exportHistory(){
  setBusy(true);setError("");setNotice("");
  try{
   const directory=await pickDirectory();if(!directory)return;
   const result=await directoryLinksCall<{path:string}>("history.export",{ids,directory});
   setNotice(t("links.archiveSaved",{path:result.path}));
  }catch(e){setError(String(e));}finally{setBusy(false);}
 }
 async function previewHistory(){
  setBusy(true);setError("");setNotice("");setPreview(undefined);
  try{
   const path=await pickFile();if(!path)return;
   setPreview(await directoryLinksCall<ImportPreview>("history.preview",{path}));
  }catch(e){setError(String(e));}finally{setBusy(false);}
 }
 async function importHistory(){
  if(!preview?.id||busy)return;
  setBusy(true);setError("");
  try{
   const rows=await directoryLinksCall<DirectoryLinkRecord[]>("history.import",{token:preview.id,confirmed:true});
   onChanged(rows);setNotice(t("links.archiveImported",{count:rows.length}));
  }catch(e){setError(String(e));}finally{
   // Commit tokens are single-use even on failure. Reopen the backup to get
   // a fresh inspection instead of offering a misleading blind retry.
   setPreview(undefined);setBusy(false);
  }
 }
 return <section className="settings-stack-compact" aria-label={t("links.archiveTitle")}>
  <div className="directory-links-actions"><Button icon={Download} disabled={locked||!ids.length} onClick={()=>void exportHistory()}>{t("links.archiveExport")}</Button><Button icon={Upload} disabled={locked} onClick={()=>void previewHistory()}>{t("links.archiveImport")}</Button></div>
  {error&&<Notice>{error}</Notice>}{notice&&<Notice tone="info">{notice}</Notice>}
  <Dialog open={!!preview} onOpenChange={open=>{if(!open&&!busy)setPreview(undefined);}}><DialogContent>
   <DialogTitle>{t("links.archivePreview")}</DialogTitle><DialogDescription>{t("links.archiveHint")}</DialogDescription>
   <p>{t("links.archiveCounts",{count:preview?.records.length??0,issues:preview?.issues.length??0})}</p>
   <div className="max-h-80 min-w-0 overflow-y-auto break-all space-y-3">
    {preview?.records.map(row=><div key={row.id}><p>{row.name} · {t(`links.status.${row.status}`)}</p><p className="settings-help">{row.linkPath} → {row.targetPath}</p></div>)}
    {!!preview?.issues.length&&<details><summary>{t("links.archiveSkipped")}</summary>{preview.issues.map((issue,index)=><p key={index} className="settings-help">{issue.path}：{issue.error}</p>)}</details>}
   </div>
   <DialogFooter><Button disabled={busy} onClick={()=>setPreview(undefined)}>{t("links.cancel")}</Button><Button variant="primary" busy={busy} disabled={busy||!preview?.id||!preview.records.length} onClick={()=>void importHistory()}>{t("links.archiveConfirm")}</Button></DialogFooter>
  </DialogContent></Dialog>
 </section>;
}
