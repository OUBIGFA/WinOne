import {useTranslation} from "../i18n";
import type {Settings} from "../bridge";
import {useState, type ReactNode} from "react";
import {Palette, Languages, PanelLeft} from "lucide-react";
import {Notice} from "../ui/notice";
import {IconLabel} from "../ui/icon-label";
import {SettingsOptions, SettingsSection} from "./settings-layout";
import {saveSidebarStartup,useSidebarStartup,type SidebarStartup} from "./sidebar-preference";

// Compact choices share the global decision language and original save action.
export function AppearanceSettings({theme, language, disabled, onThemeChange, onLanguageChange, children}: {
  theme: Settings["theme"];
  language: NonNullable<Settings["language"]>;
  disabled: boolean;
  onThemeChange: (value: Settings["theme"]) => void;
  onLanguageChange: (value: NonNullable<Settings["language"]>) => void;
  children?: ReactNode;
}) {
  const {t} = useTranslation();
  const startup=useSidebarStartup(),[error,setError]=useState("");
  return <>
    <SettingsSection level={2} headingId="theme-label" title={<IconLabel icon={Palette}>{t("shell.theme")}</IconLabel>}>
      <SettingsOptions label={t("shell.theme")} labelledBy="theme-label" value={theme} disabled={disabled}
        onChange={onThemeChange} items={(["system", "light", "dark"] as const).map(value =>
          ({value, id: "theme-"+value, label: t("shell."+value)}))}/>
    </SettingsSection>
    <SettingsSection level={2} headingId="language-label" title={<IconLabel icon={Languages}>{t("shell.language")}</IconLabel>}>
      <SettingsOptions label={t("shell.language")} labelledBy="language-label" value={language} disabled={disabled}
        onChange={onLanguageChange} items={([['system', t('shell.system')], ['zh-CN', '简体中文'], ['en-US', 'English']] as const).map(([value, label]) =>
          ({value, id: "language-"+value, label}))}/>
    </SettingsSection>
    <SettingsSection level={2} title={<IconLabel icon={PanelLeft}>{t("settingsDesign.defaultView")}</IconLabel>}>
      <SettingsOptions label={t("settingsDesign.sidebar")} value={startup} disabled={disabled}
        onChange={value=>{try{saveSidebarStartup(value as SidebarStartup);setError("");}catch{setError(t("shell.sidebarSaveFailed"));}}}
        items={(["collapsed","expanded"] as const).map(value=>({value, id: "sidebar-"+value,
          label: t(value==="collapsed"?"settingsDesign.sidebarIconsOnly":"settingsDesign.sidebarIconsAndNames")}))}/>
      {error&&<Notice>{error}</Notice>}
      {children}
    </SettingsSection>
  </>;
}
