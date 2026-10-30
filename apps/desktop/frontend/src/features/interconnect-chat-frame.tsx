import {useEffect,useRef,useState} from 'react';
import {interconnectCall,pickFiles,pickDirectory} from '../bridge';
import {useTranslation} from '../i18n';
import {usePageActivity,type PageActivityChanged} from '../ui/page-navigation';
import type {PeerRecord} from './interconnect-peer';
import {PathDropArea} from '../ui/path-drop-zone';
import {Notice} from '../ui/notice';
import './interconnect.css';

// Uses WinOne's existing iframe identity/theme pattern. The child alone needs
// WebSocket signaling CSP; the settings document keeps its restricted policy.
export function PeerChatFrame({active,onActivity}:{active:boolean;onActivity:PageActivityChanged}){
 const {t,i18n}=useTranslation(),frame=useRef<HTMLIFrameElement>(null);
 const [loaded,setLoaded]=useState(false),[dirty,setDirty]=useState(false),[busy,setBusy]=useState(false);
 const owned=useRef(new Set<string>());
 const [acceptsFiles,setAcceptsFiles]=useState(false),[error,setError]=useState('');
 useEffect(()=>{if(active)setLoaded(true);},[active]);
 usePageActivity(onActivity,dirty,busy,'interconnect-chat');
 const sync=()=>frame.current?.contentWindow?.postMessage({type:'interconnect-theme',language:i18n.language,theme:document.documentElement.dataset.theme??(document.documentElement.classList.contains('dark')?'dark':'light')},location.origin);
 useEffect(()=>{if(!loaded)return;const observer=new MutationObserver(sync);observer.observe(document.documentElement,{attributes:true,attributeFilter:['class','data-theme']});sync();return()=>observer.disconnect();},[loaded,i18n.language]);
 useEffect(()=>{
  let alive=true;
  const receive=async(event:MessageEvent)=>{
   if(event.source!==frame.current?.contentWindow||event.origin!==location.origin)return;
   const data=event.data,child=frame.current?.contentWindow;
   if(data?.type==='interconnect-state'){setDirty(data.dirty===true);setBusy(data.busy===true);setAcceptsFiles(data.acceptsFiles===true);return;}
   if(data?.type!=='interconnect-native'||typeof data.id!=='string')return;
   try{
    let result:unknown;
    if(data.method==='preferences.get')result=await interconnectCall('chat.preferences.get');
    else if(data.method==='preferences.save')result=await interconnectCall('chat.preferences.save',{nickname:data.params?.nickname});
    else if(data.method==='files'){const paths=await pickFiles();result=await interconnectCall('transfer_files',{paths});}
    else if(data.method==='folder'){const path=await pickDirectory();result=path?await interconnectCall('transfer_files',{paths:[path]}):[];}
    else if(data.method==='share'){
     const started=await interconnectCall<{id:string}>('transfer_start',{mode:'share_server',paths:data.params});owned.current.add(started.id);result=started;
    }else if(data.method==='connect'){
     const started=await interconnectCall<{id:string}>('transfer_start',{mode:'share_client',code:data.params,listen:'127.0.0.1:0'});owned.current.add(started.id);result=started;
    }else if(data.method==='list')result=(await interconnectCall<PeerRecord[]>('transfer_list')).filter(r=>owned.current.has(r.id));
    else if(data.method==='stop'&&owned.current.has(data.params)){result=await interconnectCall('transfer_stop',{id:data.params});owned.current.delete(data.params);}
    else throw Error('不支持的聊天文件操作');
    if(alive)child?.postMessage({type:'interconnect-result',id:data.id,result},location.origin);
    else for(const id of owned.current)void interconnectCall('transfer_stop',{id}).catch(console.error);
   }catch(error){if(alive)child?.postMessage({type:'interconnect-result',id:data.id,error:String(error)},location.origin);}
  };
  window.addEventListener('message',receive);
  return()=>{alive=false;window.removeEventListener('message',receive);for(const id of owned.current)void interconnectCall('transfer_stop',{id}).catch(console.error);owned.current.clear();};
 },[]);
 return loaded?<PathDropArea className="flash-chat-frame" kind="file" multiple disabled={!active||!acceptsFiles||busy} onError={setError} onPaths={paths=>{void interconnectCall('transfer_files',{paths}).then(files=>frame.current?.contentWindow?.postMessage({type:'interconnect-files',files},location.origin)).catch(e=>setError(String(e)));}}>{error&&<Notice>{error}</Notice>}<iframe ref={frame} title={t('interconnect.peerChat')} src="/interconnect-chat.html" onLoad={sync}/></PathDropArea>:null;
}
