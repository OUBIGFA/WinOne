import {useEffect,useLayoutEffect,useRef,useState} from 'react';
import {useTranslation} from '../i18n';
import type {PluginExtensions} from './ai-plugin-extensions';

// Privacy reuses Magpie's full summary/records surface in its own AI tab. Its
// URL inherits only the Host-owned capability, never a provider or credential.
export function AIPrivacyRecords({extensions,active,record}:{extensions?:PluginExtensions;active:boolean;record?:string}){
 const {i18n}=useTranslation(),frame=useRef<HTMLIFrameElement>(null),[height,setHeight]=useState(320);
 const url=extensions?new URL('masking.html',extensions.url):undefined;
 // The record page belongs to the plugin host, not an arbitrary provider URL.
 if(url)url.pathname=url.pathname.replace(/[^/]+$/,'plugins.html');
 if(record)url?.searchParams.set('record',record);
 const source=url?.href,origin=url?.origin;
 const sync=()=>{if(origin)frame.current?.contentWindow?.postMessage({type:'winone-plugin-theme',language:i18n.language,theme:document.documentElement.dataset.theme??(document.documentElement.classList.contains('dark')?'dark':'light')},origin);};
 useLayoutEffect(()=>{
  if(!active||!origin)return;
  const receive=(event:MessageEvent)=>{if(event.source!==frame.current?.contentWindow||event.origin!==origin||event.data?.type!=='winone-plugin-height')return;const next=event.data.height;if(typeof next==='number'&&Number.isFinite(next)&&next>0)setHeight(Math.ceil(next));};
  window.addEventListener('message',receive);return()=>window.removeEventListener('message',receive);
 },[active,origin]);
 useEffect(()=>{if(!active)return;const observer=new MutationObserver(sync);observer.observe(document.documentElement,{attributes:true,attributeFilter:['class','data-theme']});sync();return()=>observer.disconnect();},[active,source,i18n.language]);
 if(!active||!source)return null;
 return <iframe ref={frame} src={source} title={i18n.language.startsWith('zh')?'脱敏记录':'Masking records'} onLoad={sync} style={{width:'100%',height,border:0,display:'block'}}/>;
}
