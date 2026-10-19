import {createContext,useContext,useLayoutEffect,useRef,useState,type ReactNode} from "react";
import {createPortal} from "react-dom";

export const SettingsFooterTarget=createContext<HTMLDivElement|null>(null);

// Keep each feature's draft, callbacks and React context at their source, but
// place its actions beside page navigation outside the scrolling viewport.
export function SettingsSaveBar({children,disabled=false}:{children:ReactNode;disabled?:boolean}){
 const target=useContext(SettingsFooterTarget);
 const source=useRef<HTMLSpanElement>(null);
 const [inherited,setInherited]=useState({hidden:false,disabled:false});
 useLayoutEffect(()=>{
  if(!target||!source.current)return;
  const ancestors:HTMLElement[]=[];
  for(let node=source.current.parentElement;node;node=node.parentElement)ancestors.push(node);
  // Portals do not inherit DOM fieldset/tabpanel state. Mirror it so busy
  // forms stay locked and kept-mounted, inactive tabs never expose actions.
  const update=()=>{
   const hidden=ancestors.some(node=>node.hidden||getComputedStyle(node).display==="none");
   const disabled=ancestors.some(node=>node instanceof HTMLFieldSetElement&&node.disabled);
   setInherited(old=>old.hidden===hidden&&old.disabled===disabled?old:{hidden,disabled});
  };
  const observer=new MutationObserver(update);
  for(const node of ancestors)observer.observe(node,{attributes:true,attributeFilter:["hidden","disabled","class","style"]});
  update();
  return()=>observer.disconnect();
 },[target]);
 const actions=<fieldset className="plain-fieldset settings-save-bar" hidden={!!target&&inherited.hidden} disabled={disabled||!!target&&inherited.disabled}>{children}</fieldset>;
 return target?<><span ref={source} hidden/>{createPortal(actions,target)}</>:actions;
}
