import {RadioGroup, RadioGroupItem} from "./ui/radio-group";
import {Label} from "./ui/label";
import {useTranslation} from "../i18n";
import type {Settings} from "../bridge";

// Global preferences share the existing save path; modules inherit these values.
export function AppearanceSettings({theme, language, disabled, onThemeChange, onLanguageChange}: {
  theme: Settings["theme"];
  language: NonNullable<Settings["language"]>;
  disabled: boolean;
  onThemeChange: (value: Settings["theme"]) => void;
  onLanguageChange: (value: NonNullable<Settings["language"]>) => void;
}) {
  const {t} = useTranslation();
  return <div className="space-y-6 py-4">
    <div className="space-y-3">
      <Label id="theme-label">{t("shell.theme")}</Label>
      <RadioGroup aria-labelledby="theme-label" value={theme} disabled={disabled}
        onValueChange={value => onThemeChange(value as Settings["theme"])} className="flex flex-wrap gap-x-6 gap-y-3">
        {(["system", "light", "dark"] as const).map(value => <div key={value} className="flex items-center gap-2">
          <RadioGroupItem id={`theme-${value}`} value={value}/><Label htmlFor={`theme-${value}`}>{t(`shell.${value}`)}</Label>
        </div>)}
      </RadioGroup>
    </div>
    <div className="space-y-3">
      <Label id="language-label">{t("shell.language")}</Label>
      <RadioGroup aria-labelledby="language-label" value={language} disabled={disabled}
        onValueChange={value => onLanguageChange(value as NonNullable<Settings["language"]>)} className="flex flex-wrap gap-x-6 gap-y-3">
        {([['system', t('shell.system')], ['zh-CN', '简体中文'], ['en-US', 'English']] as const).map(([value, label]) => <div key={value} className="flex items-center gap-2">
          <RadioGroupItem id={`language-${value}`} value={value}/><Label htmlFor={`language-${value}`}>{label}</Label>
        </div>)}
      </RadioGroup>
    </div>
  </div>;
}
