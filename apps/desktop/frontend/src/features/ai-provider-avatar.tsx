import {usePathDrop} from "../ui/path-drop";
import {useEffect,useRef,useState} from "react";
import {Globe,ImagePlus,Link,RotateCcw,Shapes} from "lucide-react";
import {captureCall,readDroppedImage} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {Input} from "../components/ui/input";
import {Notice} from "../ui/notice";
import {Collapse} from "./cindy/collapse";
import {AIProviderIcon,builtInProviderIcons} from "./ai-provider-icon";
import {maximumAvatarBytes,storeProviderAvatar,refreshProviderAvatars} from "./provider-avatar-cache";
import type {AIProvider} from "./ai-configuration";

// Adapted from Magpie's iconPicker: current picture, file/site/built-in choices,
// searchable marks and reset. WinOne supplies controls; Cindy supplies collapse.
export function AIProviderAvatar({provider,disabled,onChange,onBusy}:{provider:AIProvider;disabled:boolean;onChange:(avatar:string)=>void;onBusy:(busy:boolean)=>void}){
 const {t}=useTranslation();const file=useRef<HTMLInputElement>(null);
 const [mode,setMode]=useState<""|"builtin"|"url">(""),[seek,setSeek]=useState(""),[url,setURL]=useState(""),[busy,setBusy]=useState(false),[error,setError]=useState("");
 const lifetime=useRef<AbortController|undefined>(undefined);
 useEffect(()=>{const controller=new AbortController();lifetime.current=controller;return()=>controller.abort();},[]);
 async function pick(action:()=>Promise<string>){
  setBusy(true);onBusy(true);setError("");
  try{const icon=await action();if(!lifetime.current?.signal.aborted){refreshProviderAvatars();onChange(icon);setMode("");}}
  catch(error){if(!lifetime.current?.signal.aborted)setError(String(error));}
  finally{setBusy(false);onBusy(false);}
 }
 const locked=disabled||busy;
 const imageDrop=usePathDrop({kind:"file",disabled:locked,onError:setError,onPaths:paths=>pick(async()=>{const data=await readDroppedImage(paths[0]);return storeProviderAvatar(Uint8Array.from(atob(data),character=>character.charCodeAt(0)),lifetime.current?.signal);})});
 return <section {...imageDrop} className="settings-group" aria-label={t("ai.avatar")}>
  <h3 className="settings-group-title">{t("ai.avatar")}</h3>
  <div className="flex flex-wrap items-center gap-2">
   <span className="mr-2 inline-flex size-10 items-center justify-center rounded-lg bg-muted/60 [&>span]:size-8"><AIProviderIcon icon={provider.avatar||provider.icon}/></span>
   <input ref={file} type="file" hidden aria-label={t("ai.avatarChoose")} accept="image/png,image/jpeg,image/gif,image/webp,image/x-icon,image/svg+xml,.ico,.svg" disabled={locked} onChange={event=>{const selected=event.target.files?.[0];event.target.value="";if(!selected)return;if(!selected.size||selected.size>maximumAvatarBytes){setError(t("ai.avatarSize"));return;}void pick(async()=>storeProviderAvatar(new Uint8Array(await selected.arrayBuffer()),lifetime.current?.signal));}}/>
   <Button variant="ghost" icon={ImagePlus} disabled={locked} onClick={()=>file.current?.click()}>{t("ai.avatarChoose")}</Button>
   <Button variant="ghost" icon={Globe} disabled={locked||!provider.baseUrl?.trim()} onClick={()=>void pick(async()=>{const result=await captureCall<{icon:string}>("ai.icons.website",{url:provider.baseUrl,name:provider.name});return result.icon;})}>{t("ai.avatarWebsite")}</Button>
   <Button variant="ghost" icon={Shapes} disabled={locked} aria-expanded={mode==="builtin"} onClick={()=>setMode(mode==="builtin"?"":"builtin")}>{t("ai.avatarBuiltin")}</Button>
   <Button variant="ghost" icon={Link} disabled={locked} aria-expanded={mode==="url"} onClick={()=>setMode(mode==="url"?"":"url")}>{t("ai.avatarURL")}</Button>
   {!!provider.avatar&&<Button variant="ghost" icon={RotateCcw} disabled={locked} onClick={()=>{onChange("");setError("");}}>{t("ai.avatarReset")}</Button>}
  </div>
  <p className="settings-help">{busy?t("ai.avatarLoading"):t("ai.avatarHint")}</p>
  <Collapse open={mode==="url"}><div className="flex items-center gap-2 pt-2"><Input type="url" aria-label={t("ai.avatarURL")} value={url} placeholder="https://example.com/avatar.png" disabled={locked} onChange={event=>setURL(event.target.value)}/><Button disabled={locked||!url.trim()} onClick={()=>void pick(async()=>{const result=await captureCall<{icon:string}>("ai.icons.fetch",{url:url.trim()});return result.icon;})}>{t("ai.avatarUseURL")}</Button></div></Collapse>
  <Collapse open={mode==="builtin"}><div className="space-y-3 pt-2"><Input aria-label={t("ai.avatarSearch")} placeholder={t("ai.avatarSearch")} value={seek} onChange={event=>setSeek(event.target.value)}/><div className="flex max-h-48 flex-wrap gap-1 overflow-auto" role="group" aria-label={t("ai.avatarBuiltin")}>{builtInProviderIcons.filter(icon=>icon.toLowerCase().includes(seek.trim().toLowerCase())).map(icon=><Button key={icon} variant="ghost" size="icon" disabled={locked} title={icon.replace(/-color$/,"")} aria-label={icon.replace(/-color$/,"")} aria-pressed={provider.avatar===icon} className="data-[selected=true]:bg-accent" data-selected={provider.avatar===icon} onClick={()=>{onChange(icon);setMode("");setError("");}}><AIProviderIcon icon={icon}/></Button>)}</div></div></Collapse>
  {error&&<Notice>{error}</Notice>}
 </section>;
}
