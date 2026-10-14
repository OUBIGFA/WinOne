export type ChatPanelTab={id:string;kind:'home'|'files'|'resources'|'file'|'browser'|'terminal'|'review';turnId?:string;title?:string;path?:string;line?:number;url?:string;pinned?:boolean};
export const initialPanelTabs:ChatPanelTab[]=[{id:'home',kind:'home'},{id:'resources',kind:'resources'}];

/** Keep tab identity and order per conversation; never persist native handles. */
export function restorePanelTabs(id:string):{tabs:ChatPanelTab[];selected:string}{
 try{const saved=JSON.parse(localStorage.getItem(`winone.chat.panel.v1:${id}`)||'null');if(saved&&Array.isArray(saved.tabs)){
  const tabs=saved.tabs.filter((tab:ChatPanelTab)=>tab&&typeof tab.id==='string'&&['home','files','resources','file','browser','terminal','review'].includes(tab.kind)&& (tab.kind!=='file'||typeof tab.path==='string'));
  if(tabs.length)return {tabs,selected:tabs.some((tab:ChatPanelTab)=>tab.id===saved.selected)?saved.selected:tabs[0].id};
 }}catch(error){console.warn('Could not restore side panel tabs',error);}
 return {tabs:initialPanelTabs,selected:'home'};
}

export const FILE_REFERENCE_MIME='application/x-winone-file-reference';
