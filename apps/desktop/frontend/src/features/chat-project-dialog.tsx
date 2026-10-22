import {Input} from "../components/ui/input";
import {Button} from "../ui/button";
import {useTranslation} from "../i18n";
import type {ChatPermission} from "./chat-state";
import {PERMISSION_ICONS} from './cindy/PermissionSelector';

const permissionOptions:[ChatPermission,string][]=[["ask","chat.projectPermissionAsk"],["auto","chat.projectPermissionAuto"],["bypassPermissions","chat.projectPermissionFull"]];

export function ChatProjectFields({mode,onMode,name,onName,directory,onChooseDirectory,busy,permission,onPermission}:{mode:"open"|"create";onMode:(mode:"open"|"create")=>void;name:string;onName:(name:string)=>void;directory:string;onChooseDirectory:()=>void;busy:boolean;permission:ChatPermission;onPermission:(value:ChatPermission)=>void}){
 const {t}=useTranslation();
 function fields(value:"open"|"create"){
  return <>
  <label className="chat-dialog-field" htmlFor={"chat-project-name-"+value}>{t("chat.projectName")}<Input id={"chat-project-name-"+value} autoFocus value={name} disabled={busy} onChange={event=>onName(event.target.value)}/></label>
  <div className="chat-dialog-field"><span>{t(value==="open"?"chat.location":"chat.projectParent")}</span><div><span>{directory||t("chat.chooseLocation")}</span><Button disabled={busy} onClick={onChooseDirectory}>{t("chat.chooseLocation")}</Button></div></div>
  {directory&&<p className="text-sm text-muted-foreground break-all">{t("chat.projectDestination")} · {value==="create"?directory.replace(/[\\/]+$/,"")+(directory.includes("\\")?"\\":"/")+name:directory}</p>}
  <div className="chat-dialog-field"><span>{t("chat.projectPermission")}</span><div className="chat-project-permissions" role="group" aria-label={t("chat.projectPermission")}>{permissionOptions.map(([value,label])=><Button key={value} icon={PERMISSION_ICONS[value]} variant={permission===value?"primary":"secondary"} disabled={busy} aria-pressed={permission===value} onClick={()=>onPermission(value)}>{t(label)}</Button>)}</div></div>
  </>;
 }
 // One form owns both cases. Creating a child folder is an explicit option,
 // preserving the distinct filesystem behavior without duplicating the page.
 return <div className="chat-project-form"><label className="chat-project-create-option"><input type="checkbox" checked={mode==='create'} disabled={busy} onChange={event=>onMode(event.target.checked?'create':'open')}/><span>{t('chat.createProjectFolder')}</span></label>{fields(mode)}</div>;
}
