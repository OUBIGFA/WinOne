import {useEffect,useRef} from 'react';
import {directoryLinksCall,settingsNavigationPort} from '../bridge';

export interface DirectoryLinkIntent{requested:boolean;paths:string[];options?:{kind?:'auto'|'hardlink'|'junction'|'symlink';relative?:boolean;retainScript?:boolean;hideSuccess?:boolean}}

/** Explorer only queues a prefill. The ordinary navigation guard still owns
 * leaving a dirty page, and a later explicit Create owns filesystem changes. */
export function useDirectoryLinkIntents(ready:boolean,onIntent:(value:DirectoryLinkIntent)=>void,onError:(error:string)=>void){
 const actions=useRef({onIntent,onError});actions.current={onIntent,onError};
 useEffect(()=>{
  if(!ready)return;let alive=true;
  const read=()=>{void directoryLinksCall<DirectoryLinkIntent>('pending').then(value=>{if(alive&&value.requested)actions.current.onIntent(value);}).catch(error=>{if(alive)actions.current.onError(String(error));});};
  const unsubscribe=settingsNavigationPort.subscribe(read);window.addEventListener('focus',read);read();
  return()=>{alive=false;unsubscribe();window.removeEventListener('focus',read);};
 },[ready]);
}
