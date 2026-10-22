import {useEffect,useLayoutEffect,useRef,useState} from 'react';
import {RefreshCw} from 'lucide-react';
import {captureCall,pickDirectory} from '../bridge';
import {useTranslation} from '../i18n';
import {Button} from '../ui/button';
import {Notice} from '../ui/notice';
import type {AIAgent} from './ai-configuration';
import {pluginPageURL,type PluginPageRequest} from './ai-plugin-extensions';

/** The upstream manager owns installation and immediate plugin operations.
 * Only the authenticated child may ask WinOne for picking or navigation. */
export function AIThirdPartyPlugins({active,disabled,revision,onProvider,onChanged,pageRequest}:{pageRequest?:PluginPageRequest;active:boolean;disabled:boolean;revision?:number;onProvider:(agent:AIAgent,signIn?:boolean)=>void|Promise<void>;onChanged?:()=>void}){
 const {t,i18n}=useTranslation(),frame=useRef<HTMLIFrameElement>(null),[url,setURL]=useState(''),[error,setError]=useState(''),[busy,setBusy]=useState(false),[retry,setRetry]=useState(0),[height,setHeight]=useState(600);
 const callback=useRef(onProvider);callback.current=onProvider;
 const changed=useRef(onChanged);changed.current=onChanged;
 useEffect(()=>{
  if(!active)return;
  let alive=true;setBusy(true);setError('');setURL('');
  void captureCall<{url:string}>('ai.routing.plugins.open').then(value=>{if(alive)setURL(value.url);else void captureCall('ai.routing.plugins.close').catch(console.error);}).catch(error=>{if(alive)setError(String(error));}).finally(()=>{if(alive)setBusy(false);});
  return()=>{alive=false;};
 },[active,retry,revision]);
 useEffect(()=>{if(!active)return;return()=>{void captureCall('ai.routing.plugins.close').catch(console.error);};},[active]);
 useLayoutEffect(()=>{
  if(!active||!url)return;
  const origin=new URL(url).origin;
  const receive=async(event:MessageEvent)=>{
   if(event.source!==frame.current?.contentWindow||event.origin!==origin)return;
   const message=event.data;
   if(message?.type==='winone-plugin-changed'){changed.current?.();return;}
   if(message?.type==='winone-plugin-height'){
    if(typeof message.height==='number'&&Number.isFinite(message.height)&&message.height>0)setHeight(Math.ceil(message.height));
    return;
   }
   if(message?.type!=='winone-plugin-native'||typeof message.id!=='string')return;
   const child=frame.current?.contentWindow;
   try{
    // An operation already posted before inert was applied must receive an
    // error, not leave the authenticated child's promise pending forever.
    if(disabled)throw Error(t('aiPlugins.busyAction'));
    let result:unknown;
    if(message.method==='choose')result=await pickDirectory();
    else if(message.method==='provider'&&typeof message.provider==='string'){
     const agent:AIAgent={id:message.provider,name:typeof message.name==='string'?message.name:message.provider,backend:'plugin',plugin:typeof message.plugin==='string'?message.plugin:undefined};
     if(message.signIn===true)await callback.current(agent,true);else await callback.current(agent);
    }else throw Error(t('aiPlugins.unsupportedAction'));
    child?.postMessage({type:'winone-plugin-native-result',id:message.id,result},origin);
   }catch(error){child?.postMessage({type:'winone-plugin-native-result',id:message.id,error:error instanceof Error?error.message:String(error)},origin);}
  };
  window.addEventListener('message',receive);return()=>window.removeEventListener('message',receive);
 },[active,url,disabled,t]);
 function sync(){if(!url)return;frame.current?.contentWindow?.postMessage({type:'winone-plugin-theme',language:i18n.language,theme:document.documentElement.dataset.theme??(document.documentElement.classList.contains('dark')?'dark':'light')},new URL(url).origin);}
 useEffect(()=>{if(!active)return;const observer=new MutationObserver(sync);observer.observe(document.documentElement,{attributes:true,attributeFilter:['class','data-theme']});sync();return()=>observer.disconnect();},[active,url,i18n.language]);
 return <section className="settings-stack" aria-label={t('aiPlugins.title')}>
  {error&&<Notice actions={<Button icon={RefreshCw} onClick={()=>setRetry(value=>value+1)}>{t('common.retry')}</Button>}>{error}</Notice>}
  {busy&&!url&&<p role="status">{t('aiPlugins.loading')}</p>}
  {active&&url&&<iframe key={pageRequest?.nonce??'manager'} ref={frame} title={t('aiPlugins.title')} src={pluginPageURL(url,pageRequest)} onLoad={sync} inert={disabled} style={{width:'100%',height,border:0,display:'block'}}/>}
 </section>;
}
