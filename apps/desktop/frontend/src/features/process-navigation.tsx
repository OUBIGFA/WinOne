import {createContext,useCallback,useContext,useEffect,useMemo,useRef,useState,type ReactNode} from "react";
import {ChevronsUp} from "lucide-react";
import {Button} from "../ui/button";
import {useTranslation} from "../i18n";

type Navigation={register:(key:string,collapse:(()=>void)|undefined)=>void;collapseAll:()=>void;count:number};
const ProcessNavigation=createContext<Navigation|null>(null);
const ProcessScrollOwner=createContext(false);
export const useExpandedProcess=()=>useContext(ProcessScrollOwner);
export function ProcessNavigationProvider({children}:{children:ReactNode}){
 const entries=useRef(new Map<string,()=>void>()),[count,setCount]=useState(0);
 const register=useCallback((key:string,collapse:(()=>void)|undefined)=>{if(collapse)entries.current.set(key,collapse);else entries.current.delete(key);setCount(entries.current.size);},[]);
 const collapseAll=useCallback(()=>{for(const collapse of entries.current.values())collapse();},[]);
 const value=useMemo(()=>({register,collapseAll,count}),[register,collapseAll,count]);
 return <ProcessNavigation.Provider value={value}>{children}</ProcessNavigation.Provider>;
}
export function ProcessCollapseAction(){
 const navigation=useContext(ProcessNavigation),{t}=useTranslation();
 if(!navigation?.count)return null;
 return <Button variant="ghost" size="icon" icon={ChevronsUp} aria-label={t("chat.collapseProcess")} title={t("chat.collapseProcess")} onClick={navigation.collapseAll} className="process-collapse-action"/>;
}

// Only the transcript scrolls. The action lives in its fixed composer footer,
// so even a very long expanded run can be collapsed without scrolling back.
export function ProcessBody({children,blockKey,onCollapse,version}:{children:ReactNode;blockKey:string;onCollapse?:()=>void;follow?:boolean;version?:unknown}){
 const navigation=useContext(ProcessNavigation),nested=useContext(ProcessScrollOwner);
 const [content,setContent]=useState<HTMLDivElement|null>(null),[long,setLong]=useState(false);
 const collapseRef=useRef(onCollapse);collapseRef.current=onCollapse;
 useEffect(()=>{
  if(!content||nested)return;
  const measure=()=>{const transcript=content.closest('.chat-transcript-viewport,.chat-transcript');const available=transcript?.clientHeight||window.innerHeight;setLong(content.scrollHeight>Math.max(160,available-80));};
  measure();window.addEventListener("resize",measure);
  const observer=typeof ResizeObserver!=="undefined"?new ResizeObserver(measure):undefined;
  observer?.observe(content);
  return()=>{window.removeEventListener("resize",measure);observer?.disconnect();};
 },[content,version,nested]);
 const register=navigation?.register;
 useEffect(()=>{if(!register)return;if(long&&onCollapse)register(blockKey,()=>collapseRef.current?.());else register(blockKey,undefined);return()=>register(blockKey,undefined);},[register,blockKey,long,!!onCollapse]);
 if(nested)return <div className="process-nested">{children}</div>;
 return <ProcessScrollOwner.Provider value={true}><div ref={setContent} className="process-body" data-process-long={long||undefined}>{children}</div></ProcessScrollOwner.Provider>;
}
