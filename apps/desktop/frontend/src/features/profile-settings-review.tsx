import type { ProfilePath } from "./profile-import";
import { useTranslation } from "../i18n";

// Only validated built-in settings are passed here; unknown module payloads stay
// opaque. React renders file-provided names and arguments as text, never markup.
export function ProfileSettingsReview({values,paths}:{values:Record<string,unknown>;paths:ProfilePath[]}) {
  const {t}=useTranslation();
  function show(key:string,value:unknown):React.ReactNode {
    if(key==="openEditor"&&typeof value==="boolean")return t(value?"capture.editFirst":"capture.quickSave");
    if(key==="fileNameTemplate"&&value==="")return t("profiles.defaultFilename");
    if(key==="toolbarTools")return value===null?t("capture.defaultToolbar"):Array.isArray(value)&&value.length?value.map(tool=>t(`capture.${String(tool)}`)).join("、"):t("profiles.none");
    if(["outputDirectoryRef","programRef","pathRef","processPathRef"].includes(key))return paths.find(path=>path.id===value)?.value||t("profiles.pendingLocation");
    if(key==="code")return <pre className="profile-action-source">{String(value??"")}</pre>;
    if(typeof value==="boolean")return t(value?"profiles.on":"profiles.off");
    if(Array.isArray(value))return value.length?<ul>{value.map((item,index)=><li key={index}>{show("",item)}</li>)}</ul>:t("profiles.none");
    if(value&&typeof value==="object")return <ProfileSettingsReview values={value as Record<string,unknown>} paths={paths}/>;
    return String(value??"")||t("profiles.unset");
  }
  return <dl className="profile-settings-review">{Object.entries(values).filter(([key])=>!["id","version","revision","schemaVersion"].includes(key)).map(([key,value])=><div key={key}><dt>{t(`profileFields.${key}`,{defaultValue:key})}</dt><dd>{show(key,value)}</dd></div>)}</dl>;
}
