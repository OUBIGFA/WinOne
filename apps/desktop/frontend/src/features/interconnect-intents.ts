import {useEffect,useRef} from 'react';
import {interconnectCall,settingsNavigationPort} from '../bridge';
export type InterconnectIntent={requested:boolean;paths:string[];receive?:boolean;code?:string};
// Reuse the Explorer prefill flow and ordinary page guard. No polling or sending.
export function useInterconnectIntents(ready:boolean,onIntent:(intent:InterconnectIntent)=>void,onError:(error:string)=>void){
 const current=useRef({onIntent,onError});current.current={onIntent,onError};
 useEffect(()=>{if(!ready)return;let alive=true;const read=()=>{void interconnectCall<InterconnectIntent>('pending').then(value=>{if(alive&&value?.requested)current.current.onIntent(value);}).catch(e=>{if(alive)current.current.onError(String(e));});};const unsubscribe=settingsNavigationPort.subscribe(read);window.addEventListener('focus',read);read();return()=>{alive=false;unsubscribe();window.removeEventListener('focus',read);};},[ready]);
}
