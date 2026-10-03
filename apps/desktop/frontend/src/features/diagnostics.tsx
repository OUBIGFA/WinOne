import {Notice} from "../ui/notice";
import {useEffect,useState} from "react";
import {readModuleLogs,openModuleLogFolder,type ModuleLogs} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import "./diagnostics.css";

// The owning panel mounts this viewer only while its Logs tab is selected.
export function Diagnostics({moduleId}:{moduleId:"host"|"capture"|"always-on-top"|"wintray"}) {
  return <div className="diagnostics">
    <LogViewer key={moduleId} module={moduleId}/>
  </div>;
}
function LogViewer({module}:{module:string}) {
  const {t}=useTranslation();
  const [data,setData]=useState<ModuleLogs>();
  const [error,setError]=useState("");
  const [openError,setOpenError]=useState("");
  const [busy,setBusy]=useState(true);
  const [opening,setOpening]=useState(false);
  const [revision,setRevision]=useState(0);
  useEffect(()=>{
    let active=true;
    setBusy(true);setData(undefined);setError("");
    void readModuleLogs(module).then(result=>{if(active)setData(result);}).catch(error=>{if(active)setError(String(error));}).finally(()=>{if(active)setBusy(false);});
    return ()=>{active=false;};
  },[module,revision]);
  async function openFolder() {
    setOpening(true);setOpenError("");
    try {await openModuleLogFolder(module);}catch(error){setOpenError(String(error));}finally{setOpening(false);}
  }
  return <>
    <div className="detail-actions">
      <Button disabled={busy} onClick={()=>setRevision(value=>value+1)}>{t("diagnostics.refresh")}</Button>
      <Button disabled={opening||!data?.folderAvailable} onClick={()=>void openFolder()}>{t("diagnostics.openFolder")}</Button>
    </div>
    {busy&&<p role="status">{t("diagnostics.loading")}</p>}
    {error&&<Notice>{error}</Notice>}
    {openError&&<Notice>{openError}</Notice>}
    {data&&!busy&&data.entries.length===0&&<p className="muted">{t("diagnostics.empty")}</p>}
    {data?.entries.map(entry=><div key={entry.name}>
      <h3>{t(`diagnostics.${entry.name}`)}</h3>
      {entry.truncated&&<p className="muted">{t("diagnostics.truncated")}</p>}
      {entry.available&&entry.text?<pre tabIndex={0} className="diagnostics-log" aria-label={t(`diagnostics.${entry.name}`)}>{entry.text}</pre>:<p className="muted">{t("diagnostics.empty")}</p>}
    </div>)}
  </>;
}
