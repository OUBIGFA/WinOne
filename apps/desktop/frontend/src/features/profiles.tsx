import {Notice} from "../ui/notice";
import { useState } from "react";
import { exportProfile } from "../bridge";
import { Button } from "../ui/button";
import { ProfileImport } from "./profile-import";
import { useTranslation } from "../i18n";
import { usePageActivity, type PageActivityChanged } from "../ui/page-navigation";
import {Download} from "lucide-react";
import {SettingsSection} from "../components/settings-layout";

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
  return <div className="profile-settings settings-section-stack">
    <SettingsSection icon={Download} title={t("profiles.export")} description={t("profiles.exportHint")}>
      <div className="settings-action-row">
        <Button icon={Download} busy={busy} onClick={()=>void save()}>{t("profiles.exportFile")}</Button>
        {message&&<p role="status" className="program-path">{t("profiles.exported",{path:message})}</p>}
      </div>
      {error && <Notice>{error}</Notice>}
    </SettingsSection>
    <ProfileImport onApplied={onApplied} onActivity={onActivity}/>
  </div>;
}
