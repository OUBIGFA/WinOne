import {Notice} from "../ui/notice";
import { useState } from "react";
import { exportProfile } from "../bridge";
import { Button } from "../ui/button";
import { ProfileImport } from "./profile-import";
import { useTranslation } from "../i18n";
import { usePageActivity, type PageActivityChanged } from "../ui/page-navigation";

export function Profiles({onApplied=()=>{},onActivity}:{onApplied?:()=>void;onActivity?:PageActivityChanged}) {
  const {t}=useTranslation();
  const [busy,setBusy]=useState(false);
  const [message,setMessage]=useState("");
  const [error,setError]=useState("");
  usePageActivity(onActivity,false,busy,"profile-export");
  async function save() {
    setBusy(true);setMessage("");setError("");
    try {
      const path=await exportProfile();
      if(path)setMessage(path);
    } catch(error) {setError(String(error));}
    finally {setBusy(false);}
  }
  return <>
    <h2>{t("profiles.title")}</h2>
    <div className="setting-row">
      <div><span>{t("profiles.export")}</span><p>{t("profiles.exportHint")}</p></div>
      <Button busy={busy} onClick={()=>void save()}>{t("profiles.export")}</Button>
    </div>
    {error && <Notice>{error}</Notice>}
    <p role="status" className="program-path">{message?t("profiles.exported",{path:message}):""}</p>
    <ProfileImport onApplied={onApplied} onActivity={onActivity}/>
  </>;
}
