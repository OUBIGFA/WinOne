import {Layers,Sparkle} from "lucide-react";
import {useEffect,useState,useSyncExternalStore} from "react";
import {isStoredAvatar,loadProviderAvatar,subscribeProviderAvatars,providerAvatarRevision} from "./provider-avatar-cache";
import {useTranslation} from "../i18n";

// The unchanged SVGs remain the canonical artwork. Both UIs display the same
// resvg samples so small gradient details have smooth, consistent coverage.
const sources=import.meta.glob("../../../../result-ui/assets/providers/*.{svg,png}",{eager:true,query:"?url",import:"default"}) as Record<string,string>;
const sampled=import.meta.glob("../../../../result-ui/assets/provider-bitmaps/*.png",{eager:true,query:"?url",import:"default"}) as Record<string,string>;
export const builtInProviderIcons=[...new Set(Object.keys(sources).map(path=>path.slice(path.lastIndexOf("/")+1).replace(/\.(svg|png)$/,"")))].sort((a,b)=>a.replace(/-color$/,"").localeCompare(b.replace(/-color$/,"")));
export function AIProviderIcon({icon}:{icon?:string}){
 const {t}=useTranslation();
 const revision=useSyncExternalStore(subscribeProviderAvatars,providerAvatarRevision,providerAvatarRevision);
 const [avatar,setAvatar]=useState<{icon:string;source?:string;error?:boolean}>();
 useEffect(()=>{let live=true;if(isStoredAvatar(icon))void loadProviderAvatar(icon!).then(source=>{if(live)setAvatar({icon:icon!,source});},()=>{if(live)setAvatar({icon:icon!,error:true});});return()=>{live=false;};},[icon,revision]);
 const custom=avatar?.icon===icon?avatar:undefined;
 const svg=icon&&sources[`../../../../result-ui/assets/providers/${icon}.svg`];
 const raster=icon&&sources[`../../../../result-ui/assets/providers/${icon}.png`];
 const source=svg?sampled[`../../../../result-ui/assets/provider-bitmaps/${icon}.png`]:raster;
 if(svg&&!source)throw Error("Provider images must be generated before building the UI");
 const mask={maskImage:`url("${source}")`,maskSize:"contain",maskRepeat:"no-repeat",maskPosition:"center"};
 const className="inline-flex size-5 shrink-0 items-center justify-center";
 if(isStoredAvatar(icon))return <span data-icon={icon} aria-hidden="true" className={className} title={custom?.error?t("ai.avatarReadError"):undefined}>{custom?.source&&!custom.error?<img src={custom.source} alt="" draggable={false} className="block size-full object-contain" onError={()=>setAvatar({icon:icon!,error:true})}/>:<Layers className="size-5 text-muted-foreground"/>}</span>;
 if(icon==="model")return <span data-icon="model" aria-hidden="true" className={className}><Sparkle className="size-5 text-muted-foreground"/></span>;
 if(!source)return <span data-icon="service" aria-hidden="true" className={className}><Layers className="size-5 text-muted-foreground"/></span>;
 return <span data-icon={icon} aria-hidden="true" className={className}>{raster?<img src={source} alt="" draggable={false} className="block size-full object-contain dark:grayscale"/>:icon?.endsWith("-color")?<><img src={source} alt="" draggable={false} className="block size-full object-contain dark:hidden"/><span className="hidden size-full bg-current dark:block" style={mask}/></>:<span className="size-full bg-current" style={mask}/>}</span>;
}
