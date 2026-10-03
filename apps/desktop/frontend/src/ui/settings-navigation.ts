import {useEffect,useRef} from "react";

// Native requests use the ordinary page guard; receiving one cannot discard a draft.
export function useSettingsNavigation(port:{read:()=>Promise<number>;subscribe:(listener:()=>void)=>()=>void}, requestPage:(page:string)=>void, onError:(error:string)=>void) {
  const actions=useRef({requestPage,onError});
  actions.current={requestPage,onError};
  const received=useRef(0);
  useEffect(()=>{
    let active=true;
    const read=()=>{void port.read().then(sequence=>{
      if(!active||sequence<=received.current)return;
      received.current=sequence;
      actions.current.requestPage("settings");
    }).catch(error=>{if(active)actions.current.onError(String(error));});};
    const unsubscribe=port.subscribe(read);
    read();
    return()=>{active=false;unsubscribe();};
  },[port]);
}
