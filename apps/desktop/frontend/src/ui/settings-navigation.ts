import {useEffect,useRef} from "react";

// Native requests use the ordinary page guard; receiving one cannot discard a draft.
export function useSettingsNavigation(port:{read:()=>Promise<number>;subscribe:(listener:()=>void)=>()=>void}, requestPage:(page:string)=>void, onError:(error:string)=>void,defaultPage="overview",enabled=true) {
  const actions=useRef({requestPage,onError,defaultPage});
  actions.current={requestPage,onError,defaultPage};
  const received=useRef(0);
  useEffect(()=>{
    if(!enabled)return;
    let active=true;
    const read=()=>{void port.read().then(sequence=>{
      if(!active||sequence<=received.current)return;
      received.current=sequence;
      // The saved preference applies on opening, never while editing it.
      // The ordinary navigation guard still protects an unfinished edit.
      actions.current.requestPage(actions.current.defaultPage);
    }).catch(error=>{if(active)actions.current.onError(String(error));});};
    const unsubscribe=port.subscribe(read);
    read();
    return()=>{active=false;unsubscribe();};
  },[port,enabled]);
}
