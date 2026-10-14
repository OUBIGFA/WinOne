// BrowserPanel from ChatGPT-Desktop-UI/examples/demo/ClientPane.tsx (MIT).
// Preserve its pane chrome, glyphs, capsule shell, address bar and tool grid.
// Only controlled tabs, translations and real Host callbacks replace demo state.
import {forwardRef,useEffect,useLayoutEffect,useImperativeHandle,useRef,useState,type ReactNode} from 'react';
import {ArrowLeft,ArrowRight,ArrowClockwise,ArrowsOutSimple,ArrowsInSimple,DotsThree,Globe,Plus,PlusSquare,X,Files,Terminal,NotePencil,ChatCircle,SidebarSimple,DownloadSimple,UploadSimple,Books,PushPin,Stop} from '@phosphor-icons/react';
import {Button,IconButton,DesktopMenu,InternalScrollArea,Input,Tooltip,type DesktopMenuItem} from 'chatgpt-desktop-kit';
import {useTranslation} from '../i18n';
import {parseOmnibox} from './cindy/browser/parseOmnibox';
import type {ChatPanelTab} from './chat-panel-state';
import './chat-upstream-pane.css';

const icons={home:Globe,browser:Globe,files:Files,resources:Books,file:NotePencil,terminal:Terminal,review:PlusSquare};
export function UpstreamPaneTabs({tabs,selected,label,dirty,full,onSelect,onClose,onAdd,onFull,onHide,onReorder}:{tabs:ChatPanelTab[];selected:string;label:(tab:ChatPanelTab)=>string;dirty:Set<string>;full:boolean;onSelect:(id:string)=>void;onClose:(id:string)=>void;onAdd:()=>void;onFull:()=>void;onHide:()=>void;onReorder:(from:string,to:string)=>void}){
 const {t}=useTranslation(),drag=useRef(''),strip=useRef<HTMLDivElement>(null);
 useLayoutEffect(()=>{
  const viewport=strip.current;if(!viewport)return;
  const reveal=()=>{const tab=document.getElementById('chat-tab-'+selected)?.closest<HTMLElement>('.client-browser-tab');if(tab)viewport.scrollLeft=tab.offsetLeft-viewport.offsetLeft;};
  const observer=new ResizeObserver(reveal);observer.observe(viewport);reveal();return()=>observer.disconnect();
 },[selected,tabs.length]);
 return <div className="client-tabbar" aria-label={t('chat.panelTabs')}>
  <div ref={strip} className="client-tabbar chat-pane-tabstrip" role="tablist" aria-label={t('chat.panelTabs')}>
  {tabs.map(tab=>{const Icon=icons[tab.kind];return <span key={tab.id} className="client-browser-tab" draggable onDragStart={()=>{drag.current=tab.id;}} onDragOver={event=>event.preventDefault()} onDrop={()=>onReorder(drag.current,tab.id)}>
   <button type="button" data-slot="pane-tab" className="chat-pane-tab-target" id={'chat-tab-'+tab.id} role="tab" aria-controls={'chat-tabpanel-'+tab.id} aria-selected={selected===tab.id} tabIndex={selected===tab.id?0:-1} onClick={()=>onSelect(tab.id)} onKeyDown={event=>{const index=tabs.indexOf(tab),next=event.key==='ArrowRight'?(index+1)%tabs.length:event.key==='ArrowLeft'?(index+tabs.length-1)%tabs.length:event.key==='Home'?0:event.key==='End'?tabs.length-1:-1;if(next>=0){event.preventDefault();onSelect(tabs[next].id);document.getElementById('chat-tab-'+tabs[next].id)?.focus();}}} title={tab.path||tab.url||label(tab)}>
    {tab.pinned?<PushPin size={14}/>:<Icon size={14}/>}<span>{label(tab)}</span>{dirty.has(tab.id)&&<small aria-label={t('chat.unsavedFile')}>*</small>}
   </button>
   <IconButton aria-label={t('chat.closeTab',{name:label(tab)})} actionId="client.tab.close" onClick={()=>onClose(tab.id)}><X size={12}/></IconButton>
  </span>;})}</div>
  <IconButton aria-label={t('chat.newPanelTab')} actionId="client.tab.add" onClick={onAdd}><Plus size={16}/></IconButton>
  <Tooltip label={t(full?'chat.restorePanel':'chat.expandPanel')}><IconButton aria-label={t(full?'chat.restorePanel':'chat.expandPanel')} actionId="client.pane.expand" onClick={onFull}>{full?<ArrowsInSimple size={14}/>:<ArrowsOutSimple size={14}/>}</IconButton></Tooltip>
  <Tooltip label={t('chat.hideDetails')}><IconButton aria-label={t('chat.hideDetails')} actionId="client.pane.hide" onClick={onHide}><SidebarSimple size={16}/></IconButton></Tooltip>
 </div>;
}

export type UpstreamBrowserBarHandle={focusUrlBar:()=>void};
export const UpstreamBrowserBar=forwardRef<UpstreamBrowserBarHandle,{url?:string;canBack?:boolean;canForward?:boolean;loading?:boolean;readOnly?:boolean;onNavigate?:(url:string)=>void;onBack?:()=>void;onForward?:()=>void;onReload?:()=>void;onStop?:()=>void;onComment?:()=>void;commentActive?:boolean;items?:DesktopMenuItem[];onMenuOpen?:(open:boolean)=>void}>(function UpstreamBrowserBar({url='',canBack=false,canForward=false,loading=false,readOnly=false,onNavigate,onBack,onForward,onReload,onStop,onComment,commentActive,items=[],onMenuOpen},ref){
 const {t}=useTranslation(),input=useRef<HTMLInputElement>(null),[address,setAddress]=useState(url==='about:blank'?'':url);
 useEffect(()=>setAddress(url==='about:blank'?'':url),[url]);
 useImperativeHandle(ref,()=>({focusUrlBar(){input.current?.focus();input.current?.select();}}),[]);
 return <div className="client-browserbar">
  <div className="client-browser-history">
   <IconButton aria-label={t('chat.browserBack')} actionId="client.browser.back" disabled={!canBack} onClick={onBack}><ArrowLeft size={15}/></IconButton>
   <IconButton aria-label={t('chat.browserForward')} actionId="client.browser.forward" disabled={!canForward} onClick={onForward}><ArrowRight size={15}/></IconButton>
   <IconButton aria-label={t(loading?'chat.browserStop':'chat.refresh')} actionId="client.browser.refresh" disabled={!onReload&&!onStop} onClick={loading?onStop:onReload}>{loading?<Stop size={16}/>:<ArrowClockwise size={16}/>}</IconButton>
  </div>
  <Input ref={input} aria-label={t('chat.browserAddress')} placeholder={t('chat.browserAddress')} readOnly={readOnly} value={address} onChange={event=>setAddress(event.target.value)} onKeyDown={event=>{if(event.key==='Escape'){setAddress(url==='about:blank'?'':url);input.current?.blur();}if(event.key==='Enter'&&!event.nativeEvent.isComposing&&address.trim())onNavigate?.(parseOmnibox(address,{ctrlEnter:event.ctrlKey||event.metaKey}));}}/>
  <IconButton aria-label={t('chat.browserComment')} actionId="client.tab.tool.side-chat" disabled={!onComment} aria-pressed={commentActive} onClick={onComment}><ChatCircle size={16}/></IconButton>
  <DesktopMenu align="end" onOpenChange={onMenuOpen} trigger={<Button className="desktop-icon-control" size="sm" variant="ghost" iconOnly icon={<DotsThree size={18}/>} actionId="client.browser.actions.open">{t('chat.paneActions')}</Button>} items={[
   {id:'client.browser.downloads',label:t('chat.paneDownloads'),icon:<DownloadSimple/>,disabled:true},
   {id:'client.browser.share',label:t('chat.paneShare'),icon:<UploadSimple/>,disabled:true},...items,
  ]}/>
 </div>;
});

export function UpstreamNewTab({onTool,suggested}:{onTool:(kind:'review'|'terminal'|'files'|'new-page'|'resources')=>void;suggested:{id:string;label:string;icon?:ReactNode;open:()=>void}[]}){
 const {t}=useTranslation();
 return <InternalScrollArea><div className="client-new-tab">
  <h2>{t('chat.paneTools')}</h2>
  <div className="client-tools-grid">{[
   {id:'review' as const,label:t('chat.paneChanges'),icon:PlusSquare,shortcut:'Ctrl+Shift+G'},
   {id:'terminal' as const,label:t('chat.terminal'),icon:Terminal,shortcut:'Ctrl+\u0060'},
   {id:'files' as const,label:t('chat.files'),icon:Files,shortcut:'Ctrl+P'},
   {id:'new-page' as const,label:t('chat.paneNewPage'),icon:NotePencil,shortcut:''},
   {id:'resources' as const,label:t('chat.resources'),icon:Books,shortcut:''},
  ].map(item=><Button key={item.id} actionId={'client.tab.tool.'+item.id} icon={<item.icon size={15}/>} onClick={()=>onTool(item.id)}>{item.label}{item.shortcut&&<kbd>{item.shortcut}</kbd>}</Button>)}</div>
  <h2>{t('chat.paneSuggested')}</h2>
  <div className="client-suggested">{suggested.map(item=><Button key={item.id} actionId="client.tab.suggested.open" onClick={item.open}>{item.icon??<Globe size={28}/>}<span>{item.label}</span></Button>)}</div>
 </div></InternalScrollArea>;
}
