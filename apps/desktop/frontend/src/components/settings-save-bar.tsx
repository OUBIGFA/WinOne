import {Children,cloneElement,createContext,isValidElement,useContext,useLayoutEffect,useRef,useState,type ReactNode} from "react";
import {createPortal} from "react-dom";
import {RotateCcw,Save} from "lucide-react";
import {useTranslation} from "../i18n";
import {Button} from "./ui/button";

// target is the shell's action rail. setActive tells the shell that a page
// currently shows its own Save, so the disabled placeholder can step aside.
export interface SettingsActions{target:HTMLDivElement;setActive:(id:object,active:boolean)=>void}
export const SettingsActionsTarget=createContext<SettingsActions|null>(null);
type ActionProps={variant?:string;icon?:unknown;children?:ReactNode;className?:string;title?:string;size?:string};
export interface SettingsUndo{disabled?:boolean;onClick:()=>void}

// Pages without a manual-save transaction keep the same two controls in the
// same place, disabled, so Save never jumps between pages or tabs.
export function SettingsSavePlaceholder(){
 const {t}=useTranslation();
 return <fieldset className="plain-fieldset settings-save-bar settings-save-placeholder" disabled>
  <Button size="icon" className="settings-global-save" disabled title={t("navigation.saveChanges")}><Save aria-hidden="true"/><span className="sr-only">{t("navigation.saveChanges")}</span></Button>
  <Button variant="outline" size="icon" className="settings-global-undo bg-background" disabled title={t("navigation.undoChanges")}><RotateCcw aria-hidden="true"/><span className="sr-only">{t("navigation.undoChanges")}</span></Button>
 </fieldset>;
}

// Keep the original save callback, disabled state and React context. Save and
// Undo move to the shell's icon rail, in that order; delete actions, notices
// and status text remain in the owning page.
export function SettingsSaveBar({children,disabled=false,undo}:{children:ReactNode;disabled?:boolean;undo?:SettingsUndo}){
 const {t}=useTranslation();
 const actions=useContext(SettingsActionsTarget);
 const source=useRef<HTMLSpanElement>(null);
 const [identity]=useState(()=>({}));
 const [inherited,setInherited]=useState({hidden:false,disabled:false});
 const parts=Children.toArray(children);
 const primary=parts.find(child=>isValidElement<ActionProps>(child)&&child.props.variant==="primary")??parts.find(child=>isValidElement(child)&&child.type==="button");
 const portalled=!!actions&&isValidElement<ActionProps>(primary);
 useLayoutEffect(()=>{
  if(!portalled||!actions||!source.current)return;
  const ancestors:HTMLElement[]=[];
  for(let node=source.current.parentElement;node;node=node.parentElement)ancestors.push(node);
  // Portals do not inherit DOM fieldset/tabpanel state. Keep inactive tabs and
  // busy forms locked without changing their transactions or draft lifetimes.
  const update=()=>{
   const hidden=ancestors.some(node=>node.hidden||getComputedStyle(node).display==="none");
   const disabled=ancestors.some(node=>node instanceof HTMLFieldSetElement&&node.disabled);
   setInherited(old=>old.hidden===hidden&&old.disabled===disabled?old:{hidden,disabled});
  };
  const observer=new MutationObserver(update);
  for(const node of ancestors)observer.observe(node,{attributes:true,attributeFilter:["hidden","disabled","class","style"]});
  update();return()=>observer.disconnect();
 },[portalled,actions]);
 // Layout timing removes the placeholder before paint, so the rail never
 // shows two Save buttons while a page mounts its own transaction.
 useLayoutEffect(()=>{
  if(!portalled||!actions||inherited.hidden)return;
  actions.setActive(identity,true);return()=>actions.setActive(identity,false);
 },[portalled,actions,inherited.hidden,identity]);
 const undoLabel=t("navigation.undoChanges");
 if(!actions||!isValidElement<ActionProps>(primary)){
  const others=parts.filter(child=>child!==primary);
  return <fieldset className="plain-fieldset settings-inline-actions" disabled={disabled}>{primary}{undo&&<Button variant="outline" disabled={undo.disabled} onClick={undo.onClick}><RotateCcw aria-hidden="true"/>{undoLabel}</Button>}{others}</fieldset>;
 }
 const save=cloneElement(primary,{size:"icon",className:`${primary.props.className??""} settings-global-save`,title:primary.props.title??(typeof primary.props.children==="string"?primary.props.children:undefined),children:<>{!primary.props.icon&&<Save aria-hidden="true"/>}<span className="sr-only">{primary.props.children}</span></>});
 // A page without an undo transaction still shows the control disabled, so
 // Save and Undo keep identical positions everywhere.
 const undoButton=<Button variant="outline" size="icon" className="settings-global-undo bg-background" title={undoLabel} disabled={!undo||undo.disabled} onClick={undo?.onClick}><RotateCcw aria-hidden="true"/><span className="sr-only">{undoLabel}</span></Button>;
 return <><span ref={source} hidden/>
  {parts.length>1&&<fieldset className="plain-fieldset settings-inline-actions" disabled={disabled}>{parts.filter(child=>child!==primary)}</fieldset>}
  {createPortal(<fieldset className="plain-fieldset settings-save-bar" hidden={inherited.hidden} disabled={disabled||inherited.disabled}>{save}{undoButton}</fieldset>,actions.target)}
 </>;
}
