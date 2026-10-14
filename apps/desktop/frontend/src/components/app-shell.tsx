import {useLayoutEffect,useRef,useState,useSyncExternalStore,type ReactNode} from "react";
import {usePanelRef} from "react-resizable-panels";
import {PanelLeftClose,PanelLeftOpen,type LucideIcon} from "lucide-react";
import {useTranslation} from "../i18n";
import {Sidebar,SidebarHeader,SidebarContent,SidebarFooter,SidebarInset,SidebarMenu,SidebarMenuButton,SidebarMenuItem,SidebarProvider} from "./ui/sidebar";
import {ResizablePanelGroup,ResizablePanel,ResizableHandle} from "./ui/resizable";
import {Sheet,SheetContent,SheetHeader,SheetTitle,SheetClose} from "./ui/sheet";
import {Button} from "../ui/button";
import {TooltipProvider} from "./ui/tooltip";
import {WindowTitlebar,type WindowFramePort} from "./window-titlebar";
import {readSidebarLayout,sidebarStorageKey,useSidebarStartup} from "./sidebar-preference";
import {PageScrollControls} from "./page-scroll-controls";

interface ShellPage {id:string;title:string;icon:LucideIcon}
const narrowQuery="(max-width: 767px)";
function subscribeNarrow(callback:()=>void){const query=matchMedia(narrowQuery);query.addEventListener("change",callback);return ()=>query.removeEventListener("change",callback);}

// A viewport-sized shell gives content its own scroll area. Inactive feature
// pages still unmount; the fixed navigation does not keep their workers alive.
export function AppShell({page,pages,logo,onNavigate,children,windowPort,onWindowReady}:{page:string;pages:ShellPage[];logo:string;onNavigate:(id:string)=>void;children:ReactNode;windowPort?:WindowFramePort;onWindowReady?:()=>void}) {
 const {t}=useTranslation();
 const [sidebar,setSidebar]=useState(readSidebarLayout),[drawerOpen,setDrawerOpen]=useState(false);
 const startup=useSidebarStartup(),previousStartup=useRef(startup);
 const narrow=useSyncExternalStore(subscribeNarrow,()=>matchMedia(narrowQuery).matches,()=>false);
 const initialSidebarWidth=useRef(narrow?64:sidebar.width);
 const panel=usePanelRef();
 const [currentWidth,setCurrentWidth]=useState(sidebar.width);
 const compact=narrow||currentWidth<148;
 const group=useRef<HTMLDivElement>(null),separator=useRef<HTMLDivElement>(null);
 const sidebarLayout=useRef(sidebar);
 useLayoutEffect(()=>{sidebarLayout.current=sidebar;},[sidebar]);
 function rememberSidebar(value:typeof sidebar){setSidebar(value);try{localStorage.setItem(sidebarStorageKey,JSON.stringify(value));}catch(error){console.warn("Sidebar preference could not be saved",error);}}
 // The page stays mounted across both breakpoints and manual collapse, so
 // resizing navigation cannot reset form drafts or running operations.
 useLayoutEffect(()=>{
  if(!narrow)setDrawerOpen(false);
  if(!group.current)return;
  let frame=0;
  // Wait until the panel library has measured the new viewport. Imperative
  // resizing during the breakpoint render mixes the old axis size with the
  // new percentage and can turn the 64px rail into an expanded sidebar.
  const observer=new ResizeObserver(()=>{
   cancelAnimationFrame(frame);
   frame=requestAnimationFrame(()=>panel.current?.resize(narrow?64:sidebarLayout.current.width));
  });
  observer.observe(group.current);
  return ()=>{observer.disconnect();cancelAnimationFrame(frame);};
 },[narrow]);
 useLayoutEffect(()=>{
  if(previousStartup.current===startup)return;
  previousStartup.current=startup;
  const next={...sidebar,width:startup==="collapsed"?64:sidebar.expandedWidth,collapsed:startup==="collapsed"};
  setSidebar(next);panel.current?.resize(narrow?64:next.width);
 },[startup,narrow,sidebar]);
 const scroll=useRef<HTMLDivElement>(null);
 useLayoutEffect(()=>{if(scroll.current)scroll.current.scrollTop=0;},[page]);
 const renderPage=(item:ShellPage,rail=false)=><SidebarMenuItem key={item.id}>
  {item.id==="ai-services"&&pages.some(item=>item.id==="chat")&&<div role="separator" className="mx-3 my-2 border-t border-border"/>}
  <SidebarMenuButton size="lg" className={rail?"h-[48px] justify-start pl-[14px] pr-0 [&>svg]:size-[20px]":"h-[48px] pl-[14px] pr-3 [&>svg]:size-[20px]"} title={item.title} aria-label={item.title} isActive={page===item.id} aria-current={page===item.id?"page":undefined} onClick={()=>{setDrawerOpen(false);onNavigate(item.id);}}>
   <item.icon aria-hidden="true" strokeWidth={1.6}/><span className="app-navigation-label">{item.title}</span>
  </SidebarMenuButton>
 </SidebarMenuItem>;
 return <TooltipProvider><div data-navigation-compact={compact?"true":undefined} className="shell flex h-svh min-h-0 flex-col overflow-hidden"><WindowTitlebar logo={logo} port={windowPort} onReady={onWindowReady}/><SidebarProvider enableShortcut={false} className="min-h-0 flex-1 overflow-hidden">
  <ResizablePanelGroup elementRef={group} orientation="horizontal" onLayoutChanged={(layout,meta)=>{if(meta.isUserInteraction&&!narrow&&group.current){
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
   <SidebarHeader className="items-start px-[8px] py-2"><Button className="w-[48px] min-h-[48px] [&_svg]:size-[20px]" variant="ghost" size="icon" icon={compact?PanelLeftOpen:PanelLeftClose} aria-label={t(compact?"navigation.expandSidebar":"navigation.collapseSidebar")} title={t(compact?"navigation.expandSidebar":"navigation.collapseSidebar")} onClick={()=>{if(narrow)setDrawerOpen(true);else {const width=compact?sidebar.expandedWidth:64;panel.current?.resize(width);rememberSidebar({...sidebar,width,collapsed:width===64});}}}/></SidebarHeader>
   <SidebarContent><nav aria-label={t("nav.label")} className="px-[8px]"><SidebarMenu>{pages.filter(item=>item.id!=="settings"&&item.id!=="ai-services"&&item.id!=="chat").map(item=>renderPage(item,compact))}</SidebarMenu></nav></SidebarContent>
   <SidebarFooter className="px-[8px] py-3"><SidebarMenu>{pages.filter(item=>item.id==="chat"||item.id==="ai-services"||item.id==="settings").map(item=>renderPage(item,compact))}</SidebarMenu></SidebarFooter>
  </Sidebar>
  </ResizablePanel>
  <ResizableHandle elementRef={separator} disabled={narrow} className="app-navigation-divider" aria-label={t("navigation.resizeSidebar")}/>
  <ResizablePanel id="app-page" minSize={320}>
  <SidebarInset className="relative m-0 h-full min-h-0 max-w-none overflow-hidden p-0">
   <div ref={scroll} data-slot="page-scroll" className={page==="chat"?"min-h-0 flex-1 overflow-hidden":"min-h-0 flex-1 overflow-y-auto overscroll-contain"}>
    <div data-slot="page-content" className={page==="chat"?"flex h-full min-h-0 flex-col [&>.chat-workspace]:flex-1":"mx-auto w-full max-w-6xl px-6 py-8 lg:px-10"}>{children}</div>
   </div>
   {page!=="chat"&&<PageScrollControls key={page} viewportRef={scroll}/>}
  </SidebarInset>
  </ResizablePanel>
  </ResizablePanelGroup>
  <Sheet open={drawerOpen&&narrow} onOpenChange={setDrawerOpen}><SheetContent side="left" showCloseButton={false} className="max-w-72 gap-0">
   <SheetHeader className="flex-row items-center justify-between"><SheetTitle>{t("nav.label")}</SheetTitle><SheetClose render={<Button variant="ghost" size="icon" icon={PanelLeftClose} aria-label={t("navigation.collapseSidebar")} title={t("navigation.collapseSidebar")}/>}/></SheetHeader>
   <SidebarContent><nav aria-label={t("nav.label")} className="px-3"><SidebarMenu>{pages.filter(item=>item.id!=="settings"&&item.id!=="ai-services"&&item.id!=="chat").map(item=>renderPage(item))}</SidebarMenu></nav></SidebarContent>
   <SidebarFooter className="p-3"><SidebarMenu>{pages.filter(item=>item.id==="chat"||item.id==="ai-services"||item.id==="settings").map(item=>renderPage(item))}</SidebarMenu></SidebarFooter>
  </SheetContent></Sheet>
 </SidebarProvider></div></TooltipProvider>;
}
