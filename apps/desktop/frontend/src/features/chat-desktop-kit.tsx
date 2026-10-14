// Shared UI from ChatGPT-Desktop-UI 0.6.0 (MIT). WinOne owns theme and locale;
// this scoped surface never writes preferences or changes the document theme.
import {useEffect,useState,type ReactNode,type ComponentProps} from 'react';
import {DesktopProvider,UIStringsProvider,AIResponseProvider,Button as KitButton} from 'chatgpt-desktop-kit';
import type {LucideIcon} from 'lucide-react';
import {useTranslation} from '../i18n';
import 'chatgpt-desktop-kit/components.css';
import './chat-desktop-kit.css';

export function useChatTheme(){
 const read=()=>document.documentElement.classList.contains('dark')?'dark' as const:'light' as const;
 const [theme,setTheme]=useState(read);
 useEffect(()=>{const observer=new MutationObserver(()=>setTheme(read()));observer.observe(document.documentElement,{attributes:true,attributeFilter:['class','data-theme']});return()=>observer.disconnect();},[]);
 return theme;
}

export function ChatDesktopSurface({children}:{children:ReactNode}){
 const theme=useChatTheme(),{t,i18n}=useTranslation();
 return <DesktopProvider theme={theme} className="chat-desktop-surface"><UIStringsProvider locale={i18n.language} labels={{'Code':t('chat.code'),'Copy code':t('chat.copyCode'),'Copied':t('chat.copied'),'Copy unavailable':t('chat.copyFailed')}}><AIResponseProvider>{children}</AIResponseProvider></UIStringsProvider></DesktopProvider>;
}

// Keep existing host callback/trigger contracts, with the upstream renderer.
type Props=Omit<ComponentProps<typeof KitButton>,'icon'|'size'|'variant'> & {icon?:LucideIcon;size?:'icon'|'xs';variant?:'ghost'|'primary'|'secondary'|'danger';busy?:boolean};
export function ChatPanelButton({icon:Icon,size,variant='ghost',busy,disabled,...props}:Props){
 return <KitButton {...props} disabled={disabled||busy} aria-busy={busy||undefined} data-slot="button" variant={variant} size={size==='xs'?'sm':'md'} icon={Icon?<Icon size={16}/>:undefined} iconOnly={size==='icon'}/>;
}
