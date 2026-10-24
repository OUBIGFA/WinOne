import {createContext,useCallback,useContext,useLayoutEffect,useMemo,useRef,useState,type ReactNode} from 'react';
import {createPortal,flushSync} from 'react-dom';
import {ChevronsUp} from 'lucide-react';
import {Button} from '../ui/button';
import {useTranslation} from '../i18n';

type CollapseTarget={element:HTMLElement;host:HTMLElement;left:number;top:number;collapse:()=>void};
type Navigation={register:(key:string,target:CollapseTarget|undefined)=>void;target?:CollapseTarget};
const ProcessNavigation=createContext<Navigation|null>(null);
const ProcessScrollOwner=createContext(false);
export const useExpandedProcess=()=>useContext(ProcessScrollOwner);
export function ProcessNavigationProvider({children}:{children:ReactNode}){
 const entries=useRef(new Map<string,CollapseTarget>()),[revision,setRevision]=useState(0);
 const register=useCallback((key:string,target:CollapseTarget|undefined)=>{
  const previous=entries.current.get(key);
  if(target)entries.current.set(key,target);else entries.current.delete(key);
  if(previous?.element!==target?.element||previous?.host!==target?.host||previous?.left!==target?.left||previous?.top!==target?.top)setRevision(value=>value+1);
 },[]);
 const value=useMemo(()=>({register,target:[...entries.current.values()].at(-1)}),[register,revision]);
 return <ProcessNavigation.Provider value={value}>{children}</ProcessNavigation.Provider>;
}
export function ProcessCollapseAction(){
 const navigation=useContext(ProcessNavigation),{t}=useTranslation(),target=navigation?.target;
 if(!target)return null;
 // The button belongs to this visible work group, never the composer and never
 // every expanded group. Its own right-hand inset keeps all payloads readable.
 return createPortal(<Button variant="ghost" size="icon" icon={ChevronsUp} aria-label={t('chat.collapseProcess')} title={t('chat.collapseProcess')} onClick={target.collapse} className="process-collapse-action" style={{left:target.left,top:target.top}}/>,target.host);
}

// Only the transcript scrolls. Long outer work bodies reserve one small local
// action inset; nested tool/thinking bodies share it instead of stacking rails.
export function ProcessBody({children,blockKey,onCollapse,version,onLongChange}:{children:ReactNode;blockKey:string;onCollapse?:()=>void;onLongChange?:(value:boolean)=>void;follow?:boolean;version?:unknown}){
 const navigation=useContext(ProcessNavigation),nested=useContext(ProcessScrollOwner),register=navigation?.register;
 const [content,setContent]=useState<HTMLDivElement|null>(null),[long,setLong]=useState(false);
 const anchor=useRef<{element:HTMLElement;viewportTop:number;viewportBottom:number;bottom:number}|undefined>(undefined);
 const collapseRef=useRef(onCollapse);collapseRef.current=onCollapse;
 const lengthRef=useRef(onLongChange);lengthRef.current=onLongChange;
 useLayoutEffect(()=>{lengthRef.current?.(long);},[long]);
 useLayoutEffect(()=>{
  if(!content)return;
  const transcript=content.closest<HTMLElement>('.chat-transcript-viewport,.chat-transcript');
  const host=content.closest<HTMLElement>('.chat-reading-pane');
  const footer=content.closest('.chat-main')?.querySelector<HTMLElement>('.chat-input-area');
  let frame=0;
  const measure=()=>{
   const bounds=transcript?.getBoundingClientRect(),body=content.getBoundingClientRect();
   // This action belongs to the work body. The composer is an overlay, not
   // its scroll boundary; resizing that overlay must not move the action.
   const bottom=bounds?.bottom??window.innerHeight;
   // Keep the existing long-content threshold: the overlay reduces reading
   // space, but it is not the anchor for an already-visible collapse action.
   const readingBottom=footer?Math.min(bottom,footer.getBoundingClientRect().top-6):bottom;
   // Capture a visible location when this body enters the reading pane. Only
   // a change to the pane's geometry resets it. Scrolling, stream growth and
   // composer resizing never pull the button toward the body's natural end.
   if(bounds&&(!anchor.current||anchor.current.element!==content||anchor.current.viewportTop!==bounds.top||anchor.current.viewportBottom!==bounds.bottom)){
    anchor.current={element:content,viewportTop:bounds.top,viewportBottom:bounds.bottom,bottom:readingBottom};
   }
   const available=bounds&&bounds.height>0?Math.max(0,readingBottom-bounds.top):transcript?.clientHeight||window.innerHeight;
   const isLong=content.scrollHeight>Math.max(1,available)+1;setLong(isLong);
   if(nested||!register)return;
   // Long work stays collapsible as soon as it grows, including while its
   // natural bottom is visible. Keep the action inside the visible body.
   const visible=isLong&&host&&bounds&&onCollapse&&body.top<bottom-32&&body.bottom>bounds.top+32;
   const origin=host?.getBoundingClientRect();
   register(blockKey,visible&&origin?{element:content,host,left:body.right-origin.left-30,top:(anchor.current?.bottom??readingBottom)-origin.top-30,collapse:()=>collapseRef.current?.()}:undefined);
  };
  const schedule=()=>{cancelAnimationFrame(frame);frame=requestAnimationFrame(measure);};
  measure();window.addEventListener('resize',schedule);transcript?.addEventListener('scroll',schedule,{passive:true});
  const observer=typeof ResizeObserver!=='undefined'?new ResizeObserver(()=>flushSync(measure)):undefined;
  observer?.observe(content);if(transcript)observer?.observe(transcript);if(footer)observer?.observe(footer);
  return()=>{cancelAnimationFrame(frame);window.removeEventListener('resize',schedule);transcript?.removeEventListener('scroll',schedule);observer?.disconnect();if(!nested)register?.(blockKey,undefined);};
 },[content,version,nested,register,blockKey,!!onCollapse]);
 return <ProcessScrollOwner.Provider value={true}><div ref={setContent} className={nested?'process-nested':'process-body'} data-process-long={long||undefined}><div className="process-body-content">{children}</div></div></ProcessScrollOwner.Provider>;
}
