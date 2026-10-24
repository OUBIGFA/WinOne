import {useLayoutEffect,useRef,useState,type ReactNode,type CSSProperties} from "react";
import {usePanelRef} from "react-resizable-panels";
import type {LucideIcon} from "lucide-react";
import {SidebarSimple} from "@phosphor-icons/react";
import {useTranslation} from "../i18n";
import {Sidebar,SidebarHeader,SidebarContent,SidebarFooter,SidebarInset,SidebarMenu,SidebarMenuButton,SidebarMenuItem,SidebarProvider} from "./ui/sidebar";
import {ResizablePanelGroup,ResizablePanel,ResizableHandle} from "./ui/resizable";
import {Button} from "../ui/button";
import {TooltipProvider} from "./ui/tooltip";
import {WindowTitlebar,type WindowFramePort} from "./window-titlebar";
import {readSidebarLayout,sidebarStorageKey,useSidebarStartup} from "./sidebar-preference";
import {PageScrollControls} from "./page-scroll-controls";
import {SettingsFooterTarget} from "./settings-save-bar";
import "../styles/settings.css";

interface ShellPage {id:string;title:string;icon:LucideIcon}

// A viewport-sized shell gives content its own scroll area. Inactive feature
// pages still unmount; the fixed navigation does not keep their workers alive.
export function AppShell({page,pages,logo,onNavigate,children,windowPort,onWindowReady,toolbar}:{page:string;pages:ShellPage[];logo:string;onNavigate:(id:string)=>void;children:ReactNode;windowPort?:WindowFramePort;onWindowReady?:()=>void;toolbar?:ReactNode}) {
 const {t}=useTranslation();
 // Portalled menus/dialogs must inherit the same settings tokens as the page.
 // Remove the scope before Chat paints; its imported design system is intact.
 useLayoutEffect(()=>{
  if(page==="chat")return;
  document.documentElement.dataset.settingsUi="true";
  return()=>{delete document.documentElement.dataset.settingsUi;};
 },[page]);
 const [sidebar,setSidebar]=useState(readSidebarLayout);
 const startup=useSidebarStartup(),previousStartup=useRef(startup);
 const initialSidebarWidth=useRef(sidebar.width);
 const panel=usePanelRef();
 const [currentWidth,setCurrentWidth]=useState(sidebar.width);
 const compact=currentWidth<148;
 const group=useRef<HTMLDivElement>(null),separator=useRef<HTMLDivElement>(null);
 function rememberSidebar(value:typeof sidebar){setSidebar(value);try{localStorage.setItem(sidebarStorageKey,JSON.stringify(value));}catch(error){console.warn("Sidebar preference could not be saved",error);}}
 // The splitter preserves pixels as the window resizes. A viewport breakpoint
 // must not force a width or disable dragging; the content's minimum bounds it.
 useLayoutEffect(()=>{
  if(previousStartup.current===startup)return;
  previousStartup.current=startup;
  const next={...sidebar,width:startup==="collapsed"?64:sidebar.expandedWidth,collapsed:startup==="collapsed"};
  setSidebar(next);panel.current?.resize(next.width);
 },[startup,sidebar]);
 const scroll=useRef<HTMLDivElement>(null);
 const [footerTarget,setFooterTarget]=useState<HTMLDivElement|null>(null);
 const [scrollbarWidth,setScrollbarWidth]=useState(0);
 // Match the page's usable width, including a native scrollbar when present.
 useLayoutEffect(()=>{const viewport=scroll.current;if(!viewport)return;let frame=0;const measure=()=>{frame=0;const width=viewport.offsetWidth-viewport.clientWidth;setScrollbarWidth(previous=>previous===width?previous:width);};const schedule=()=>{if(!frame)frame=requestAnimationFrame(measure);};const observer=new ResizeObserver(schedule);observer.observe(viewport);schedule();return()=>{observer.disconnect();if(frame)cancelAnimationFrame(frame);};},[page]);
 useLayoutEffect(()=>{if(scroll.current)scroll.current.scrollTop=0;},[page]);
 const renderPage=(item:ShellPage,rail=false)=><SidebarMenuItem key={item.id}>
  {item.id==="ai-services"&&pages.some(item=>item.id==="chat")&&<div role="separator" className="mx-3 my-2 border-t border-border"/>}
  <SidebarMenuButton size="lg" data-page-id={item.id} className={rail?"h-[48px] justify-start pl-[14px] pr-0 [&>svg]:size-[20px]":"h-[48px] pl-[14px] pr-3 [&>svg]:size-[20px]"} title={item.title} aria-label={item.title} isActive={page===item.id} aria-current={page===item.id?"page":undefined} onClick={()=>onNavigate(item.id)}>
   <item.icon aria-hidden="true" strokeWidth={1.6}/><span className="app-navigation-label">{item.title}</span>
  </SidebarMenuButton>
 </SidebarMenuItem>;
 return <TooltipProvider><div data-navigation-compact={compact?"true":undefined} className="shell flex h-svh min-h-0 flex-col overflow-hidden"><WindowTitlebar logo={logo} port={windowPort} onReady={onWindowReady} context={page==="chat"?undefined:pages.find(item=>item.id===page)?.title} toolbar={page==="chat"?undefined:toolbar}/><SidebarProvider enableShortcut={false} className="min-h-0 flex-1 overflow-hidden">
  <ResizablePanelGroup elementRef={group} orientation="horizontal" onLayoutChanged={(layout,meta)=>{if(meta.isUserInteraction&&group.current){
   // This callback precedes the rendered panel size. Use the completed
   // layout so quick keyboard-resize / collapse sequences keep the new width.
   const available=group.current.clientWidth-(separator.current?.getBoundingClientRect().width??0);
   const width=Math.round(available*layout["app-sidebar"]/100);
   rememberSidebar({width,expandedWidth:width>=148?width:sidebar.expandedWidth,collapsed:width===64});
  }}}>
  {/* Keep registration constraints stable: changing default/max size at the
      breakpoint re-registers the panel before its resize observer catches up,
      carrying a narrow-window percentage into the wider window. */}
  <ResizablePanel id="app-sidebar" panelRef={panel} defaultSize={initialSidebarWidth.current} minSize={64} maxSize={360} onResize={size=>setCurrentWidth(size.inPixels)} groupResizeBehavior="preserve-pixel-size">
  <Sidebar collapsible="none" className="app-navigation w-full shrink-0">
   <SidebarHeader className="items-start px-[8px] py-2"><Button className="w-[48px] min-h-[48px] [&_svg]:size-[20px]" variant="ghost" size="icon" aria-label={t(compact?"navigation.expandSidebar":"navigation.collapseSidebar")} title={t(compact?"navigation.expandSidebar":"navigation.collapseSidebar")} onClick={()=>{const width=compact?sidebar.expandedWidth:64;panel.current?.resize(width);rememberSidebar({...sidebar,width,collapsed:width===64});}}><SidebarSimple size={20}/></Button></SidebarHeader>
   <SidebarContent><nav aria-label={t("nav.label")} className="px-[8px]"><SidebarMenu>{pages.filter(item=>item.id!=="settings"&&item.id!=="ai-services"&&item.id!=="chat"&&item.id!=="flash-chat").map(item=>renderPage(item,compact))}</SidebarMenu></nav></SidebarContent>
   <SidebarFooter className="px-[8px] py-3"><SidebarMenu>{pages.filter(item=>item.id==="chat"||item.id==="flash-chat"||item.id==="ai-services"||item.id==="settings").map(item=>renderPage(item,compact))}</SidebarMenu></SidebarFooter>
  </Sidebar>
  </ResizablePanel>
  <ResizableHandle elementRef={separator} className="app-navigation-divider" aria-label={t("navigation.resizeSidebar")}/>
  <ResizablePanel id="app-page" minSize={320}>
  <SidebarInset className="settings-frame relative m-0 h-full min-h-0 max-w-none overflow-hidden p-0" style={{"--settings-scrollbar-width":`${scrollbarWidth}px`} as CSSProperties}>
   <SettingsFooterTarget.Provider value={page==="chat"?null:footerTarget}>
   <div ref={scroll} data-slot="page-scroll" data-page={page} className={page==="chat"?"min-h-0 flex-1 overflow-hidden":"min-h-0 flex-1 overflow-y-auto overscroll-contain"}>
    <div data-slot="page-content" data-settings-page={page==="chat"?undefined:page} className={page==="chat"?"flex h-full min-h-0 flex-col [&>.chat-workspace]:flex-1":"settings-surface mx-auto w-full"}>{children}</div>
   </div>
   {page!=="chat"&&<div data-slot="settings-footer" className="settings-surface settings-footer"><div ref={setFooterTarget} className="settings-footer-actions"/><PageScrollControls key={page} viewportRef={scroll}/></div>}
   </SettingsFooterTarget.Provider>
  </SidebarInset>
  </ResizablePanel>
  </ResizablePanelGroup>
 </SidebarProvider></div></TooltipProvider>;
}
