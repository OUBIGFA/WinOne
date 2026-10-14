import {useSyncExternalStore} from "react";

export const sidebarStorageKey="winone.sidebar.v1";
const startupKey="winone.sidebar.startup.v1",changeEvent="winone-sidebar-startup";
export type SidebarStartup="collapsed"|"expanded";

function readStartup():SidebarStartup{
 try{return localStorage.getItem(startupKey)==="expanded"?"expanded":"collapsed";}
 catch(error){console.warn("Sidebar startup preference unavailable",error);return "collapsed";}
}

function subscribe(callback:()=>void){
 const storage=(event:StorageEvent)=>{if(event.key===startupKey||event.key===null)callback();};
 window.addEventListener(changeEvent,callback);window.addEventListener("storage",storage);
 return ()=>{window.removeEventListener(changeEvent,callback);window.removeEventListener("storage",storage);};
}
export function useSidebarStartup(){return useSyncExternalStore(subscribe,readStartup,()=>"collapsed" as const);}

// Startup state is explicit; manually folding navigation never changes it.
// A failed write is reported by the settings field, not shown as saved.
export function saveSidebarStartup(value:SidebarStartup){
 localStorage.setItem(startupKey,value);window.dispatchEvent(new Event(changeEvent));
}

export function readSidebarLayout(){
 let expandedWidth=256;
 try{
  const value=JSON.parse(localStorage.getItem(sidebarStorageKey)||"null");
  const savedWidth=value?.expandedWidth??value?.width;
  if(Number.isFinite(savedWidth))expandedWidth=Math.max(180,Math.min(360,savedWidth));
 }
 catch(error){console.warn("Sidebar preference unavailable",error);}
 const collapsed=readStartup()==="collapsed";
 return {width:collapsed?64:expandedWidth,expandedWidth,collapsed};
}
