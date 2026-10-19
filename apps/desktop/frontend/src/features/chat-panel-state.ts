export type ChatPanelTab={id:string;kind:'home'|'files'|'resources'|'file'|'browser'|'terminal'|'review';turnId?:string;title?:string;path?:string;line?:number;url?:string;pinned?:boolean};
export type ChatPanelSection='library'|'tabs';
export type ChatLibraryTab='files'|'resources';
export type ChatPanelState={tabs:ChatPanelTab[];selected:string;section:ChatPanelSection;libraryTab:ChatLibraryTab;directory?:string};
export const initialPanelTabs:ChatPanelTab[]=[{id:'home',kind:'home'}];

/** Keep document identity/order; migrate the old singleton tabs into fixed views.
 * The v1 key is retained so existing file/browser/terminal tabs are not lost. */
export function restorePanelTabs(id:string):ChatPanelState{
 try{
  const saved=JSON.parse(localStorage.getItem(`winone.chat.panel.v1:${id}`)||'null');
  if(saved&&Array.isArray(saved.tabs)){
   const valid:ChatPanelTab[]=saved.tabs.filter((tab:ChatPanelTab)=>tab&&typeof tab.id==='string'&&['home','files','resources','file','browser','terminal','review'].includes(tab.kind)&&(tab.kind!=='file'||typeof tab.path==='string'));
   const previous=valid.find(tab=>tab.id===saved.selected),tabs=valid.filter(tab=>tab.kind!=='files'&&tab.kind!=='resources');
   const libraryTab=previous?.kind==='resources'||saved.libraryTab==='resources'?'resources':'files';
   return {tabs,selected:tabs.some(tab=>tab.id===saved.selected)?saved.selected:tabs[0]?.id??'',section:saved.section==='library'||previous?.kind==='files'||previous?.kind==='resources'?'library':'tabs',libraryTab,directory:typeof saved.directory==='string'?saved.directory:valid.find(tab=>tab.kind==='files')?.path};
  }
 }catch(error){console.warn('Could not restore side panel tabs',error);}
 return {tabs:initialPanelTabs,selected:'home',section:'tabs',libraryTab:'files'};
}

export const FILE_REFERENCE_MIME='application/x-winone-file-reference';
