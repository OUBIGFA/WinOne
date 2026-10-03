import {useLayoutEffect,useRef,type ReactNode} from "react";
import type {LucideIcon} from "lucide-react";
import {useTranslation} from "../i18n";
import {Sidebar,SidebarContent,SidebarFooter,SidebarInset,SidebarMenu,SidebarMenuButton,SidebarMenuItem,SidebarProvider} from "./ui/sidebar";
import {TooltipProvider} from "./ui/tooltip";
import {WindowTitlebar,type WindowFramePort} from "./window-titlebar";

interface ShellPage {id:string;title:string;icon:LucideIcon}

// A viewport-sized shell gives content its own scroll area. Inactive feature
// pages still unmount; the fixed navigation does not keep their workers alive.
export function AppShell({page,pages,logo,onNavigate,children,windowPort,onWindowReady}:{page:string;pages:ShellPage[];logo:string;onNavigate:(id:string)=>void;children:ReactNode;windowPort?:WindowFramePort;onWindowReady?:()=>void}) {
 const {t}=useTranslation();
 const scroll=useRef<HTMLDivElement>(null);
 useLayoutEffect(()=>{if(scroll.current)scroll.current.scrollTop=0;},[page]);
 const renderPage=(item:ShellPage)=><SidebarMenuItem key={item.id}>
  <SidebarMenuButton size="lg" isActive={page===item.id} aria-current={page===item.id?"page":undefined} onClick={()=>onNavigate(item.id)}>
   <item.icon aria-hidden="true"/><span>{item.title}</span>
  </SidebarMenuButton>
 </SidebarMenuItem>;
 return <TooltipProvider><div className="shell flex h-svh min-h-0 flex-col overflow-hidden"><WindowTitlebar logo={logo} port={windowPort} onReady={onWindowReady}/><SidebarProvider enableShortcut={false} className="min-h-0 flex-1 overflow-hidden">
  <Sidebar collapsible="none" className="shrink-0 border-r pt-3">
   <SidebarContent><nav aria-label={t("nav.label")} className="px-3"><SidebarMenu>{pages.filter(item=>item.id!=="settings").map(renderPage)}</SidebarMenu></nav></SidebarContent>
   <SidebarFooter className="p-3"><SidebarMenu>{pages.filter(item=>item.id==="settings").map(renderPage)}</SidebarMenu></SidebarFooter>
  </Sidebar>
  <SidebarInset className="m-0 h-full min-h-0 max-w-none overflow-hidden p-0">
   <div ref={scroll} data-slot="page-scroll" className="min-h-0 flex-1 overflow-y-auto overscroll-contain">
    <div data-slot="page-content" className="mx-auto w-full max-w-6xl px-6 py-8 lg:px-10">{children}</div>
   </div>
  </SidebarInset>
 </SidebarProvider></div></TooltipProvider>;
}
