import {useState, type ReactNode} from "react";
import {FileText, FolderSync, Info, Palette, Power, ShieldCheck} from "lucide-react";
import {Tabs, TabsList, TabsTrigger, TabsContent} from "./ui/tabs";
import {SettingsSection} from "./settings-layout";
import {Diagnostics} from "../features/diagnostics";
import {Updates} from "../features/updates";
import {useTranslation} from "../i18n";
import {type PageActivityChanged} from "../ui/page-navigation";
import {IconLabel} from "../ui/icon-label";

export function SettingsPanel({children, startup, permissions, profiles, connection, feedback, error}: {
  children: ReactNode;
  startup?: ReactNode;
  permissions?: ReactNode;
  profiles?: ReactNode;
  connection?: ReactNode;
  feedback?: ReactNode;
  error?: ReactNode;
  onActivity?: PageActivityChanged;
}) {
  const {t} = useTranslation();
  const [tab, setTab] = useState("general");
  const [aboutOpened, setAboutOpened] = useState(false);
  // Tabs organize tasks, not transactions. Keep forms mounted so recording,
  // import review and unsaved drafts survive an in-page navigation change.
  // Diagnostics is independent of reads/disabled fieldsets on every other tab.
  return <div className="settings-panel settings-stack">
    <Tabs value={tab} onValueChange={value => {
      setTab(String(value));
      if (value === "about") setAboutOpened(true);
    }}>
      <TabsList variant="default" aria-label={t("nav.settings")}>
        <TabsTrigger title={t("shell.general")} value="general"><IconLabel icon={Palette}>{t("shell.general")}</IconLabel></TabsTrigger>
        {startup && <TabsTrigger title={t("settingsDesign.startupAndShortcuts")} value="startup"><IconLabel icon={Power}>{t("settingsDesign.startupAndShortcuts")}</IconLabel></TabsTrigger>}
        {permissions && <TabsTrigger title={t("permission.title")} value="permissions"><IconLabel icon={ShieldCheck}>{t("permission.title")}</IconLabel></TabsTrigger>}
        {profiles && <TabsTrigger title={t("settingsDesign.profileTransfer")} value="profiles"><IconLabel icon={FolderSync}>{t("settingsDesign.profileTransfer")}</IconLabel></TabsTrigger>}
        <TabsTrigger title={t("settingsDesign.aboutAndDiagnostics")} value="about"><IconLabel icon={Info}>{t("settingsDesign.aboutAndDiagnostics")}</IconLabel></TabsTrigger>
      </TabsList>
      {/* A failed preference stays visible without moving the task navigation. */}
      {error}
      <TabsContent value="general" keepMounted>{children}</TabsContent>
      {startup && <TabsContent value="startup" keepMounted>{startup}</TabsContent>}
      {permissions && <TabsContent value="permissions" keepMounted>{permissions}</TabsContent>}
      {profiles && <TabsContent value="profiles" keepMounted>{profiles}</TabsContent>}
      <TabsContent value="about" keepMounted>
        {aboutOpened && <div className="settings-about">
          <Updates/>
          {connection}
          <SettingsSection level={2} title={<IconLabel icon={FileText}>{t("diagnostics.title")}</IconLabel>}>
            <Diagnostics moduleId="host"/>
          </SettingsSection>
        </div>}
      </TabsContent>
    </Tabs>
    {feedback}
  </div>;
}
