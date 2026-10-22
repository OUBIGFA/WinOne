// Adapted from Cindy a44ac274 useFileChipContextMenu.tsx (Apache-2.0,
// Copyright 2026 XD Inc.). Local actions retain the flat menu and gesture/focus
// contract; WinOne supplies scoped paths, native clipboard and sidebar callbacks.
import {useRef,useState,type MouseEvent} from 'react';
import {AppWindow,ClipboardCopy,Copy,FileCode,FolderOpen,FolderTree,Globe,PanelRight} from 'lucide-react';
import {useTranslation} from '../../i18n';
import {chatCall,chatMediaCall} from '../../bridge';
import {copyText} from '../clipboard';
import {DropdownMenu,DropdownMenuContent,DropdownMenuItem,DropdownMenuSeparator,DropdownMenuShortcut,DropdownMenuTrigger} from './panel-ui/dropdown-menu';
import {isSeparateLinkActivation,linkMenuPosition} from './linkInteraction';

export function useFileChipContextMenu({id,fileName,getAbsPath,sidebarFileBrowserKind='file',onOpenInSidebar,onOpenInBrowser,onViewSource,location,onError}:{
 id:string;fileName:string;getAbsPath:()=>Promise<string>|string;sidebarFileBrowserKind?:'file'|'directory';
 onOpenInSidebar?:()=>void|Promise<void>;onOpenInBrowser?:()=>void|Promise<void>;onViewSource?:()=>void|Promise<void>;
 location?:string;onError:(error:unknown)=>void;
}){
 const {t}=useTranslation(),[menuPos,setMenuPos]=useState<{x:number;y:number}|null>(null),triggerRef=useRef<HTMLElement|null>(null);
 const html=/\.(?:html?|xhtml)$/i.test(fileName)&&sidebarFileBrowserKind==='file';
 const run=(action:()=>void|Promise<void>)=>()=>{setMenuPos(null);void Promise.resolve().then(action).catch(onError);};
 const native=async(method:string)=>{const path=await getAbsPath();await chatCall(method,{id,path});};
 const copyFile=async()=>{const path=await getAbsPath();await chatMediaCall('copy-file',{id,path});};
 const openDefault=async()=>{
  if(!html){await native('file.open');return;}
  // The default HTML file association can be an editor. Open the scoped HTTP
  // preview explicitly in the browser, as Cindy's page/source split requires.
  const path=await getAbsPath(),file=await chatCall<{url:string}>('file.preview',{id,path});
  await chatCall('link.open',{url:file.url});
 };
 const openAt=(x:number,y:number,trigger:HTMLElement)=>{triggerRef.current=trigger;setMenuPos({x,y});};
 const onContextMenu=(event:MouseEvent<HTMLElement>)=>{event.preventDefault();event.stopPropagation();const {x,y}=linkMenuPosition(event);openAt(x,y,event.currentTarget);};
 const onActivate=(event:MouseEvent<HTMLElement>,open:()=>void|Promise<void>)=>{
  event.preventDefault();event.stopPropagation();
  run(isSeparateLinkActivation(event)?openDefault:open)();
 };
 const canLocate=!!id&&!!onOpenInSidebar,canDefault=sidebarFileBrowserKind==='file';
 const menu=<DropdownMenu open={menuPos!==null} onOpenChange={open=>{if(!open)setMenuPos(null);}}>
  <DropdownMenuTrigger asChild><span aria-hidden tabIndex={-1} data-fixed-menu-anchor style={{position:'fixed',left:menuPos?.x??0,top:menuPos?.y??0,width:0,height:0,pointerEvents:'none'}}/></DropdownMenuTrigger>
  <DropdownMenuContent align="start" sideOffset={2} onClick={event=>event.stopPropagation()} onCloseAutoFocus={event=>{event.preventDefault();if(triggerRef.current?.isConnected)triggerRef.current.focus({preventScroll:true});}}>
   {html?<>
    {onOpenInBrowser&&<DropdownMenuItem onSelect={run(onOpenInBrowser)}><PanelRight size={16}/>{t('chat.fileMenuSidebarBrowser')}</DropdownMenuItem>}
    <DropdownMenuItem disabled={!id} onSelect={run(openDefault)}><Globe size={16}/>{t('chat.fileMenuDefaultBrowser')}<DropdownMenuShortcut aria-hidden>Shift+Click</DropdownMenuShortcut></DropdownMenuItem>
    {onViewSource&&<DropdownMenuItem onSelect={run(onViewSource)}><FileCode size={16}/>{t('chat.fileMenuSource')}</DropdownMenuItem>}
   </>:<>
    {canLocate&&<DropdownMenuItem onSelect={run(onOpenInSidebar!)}><FolderTree size={16}/>{t('chat.fileMenuSidebar')}</DropdownMenuItem>}
    {canDefault&&<DropdownMenuItem disabled={!id} onSelect={run(openDefault)}><AppWindow size={16}/>{t('chat.openSystem')}<DropdownMenuShortcut aria-hidden>Shift+Click</DropdownMenuShortcut></DropdownMenuItem>}
   </>}
   {(html||canLocate||canDefault)&&<DropdownMenuSeparator/>}
   <DropdownMenuItem disabled={!id} onSelect={run(()=>native('file.reveal'))}><FolderOpen size={16}/>{t('chat.revealFile')}</DropdownMenuItem>
   {!html&&<DropdownMenuItem disabled={!id} onSelect={run(copyFile)}><Copy size={16}/>{t('chat.fileMenuCopyFile')}</DropdownMenuItem>}
   <DropdownMenuItem onSelect={run(async()=>copyText(await getAbsPath()))}><ClipboardCopy size={16}/>{t('chat.copyPath')}</DropdownMenuItem>
   {location&&sidebarFileBrowserKind==='file'&&<DropdownMenuItem onSelect={run(()=>copyText(location))}><ClipboardCopy size={16}/>{t('chat.fileMenuCopyLocation')}</DropdownMenuItem>}
  </DropdownMenuContent>
 </DropdownMenu>;
 return {onContextMenu,openAt,onActivate,menu};
}
