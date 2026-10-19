// Host adapter around Cindy's actual Windows TabBar. It preserves every
// existing workbench action while using the source menu and tab components.
import {Fragment,useEffect,useState} from 'react';
import {MoreHorizontal} from 'lucide-react';
import {useTranslation} from 'react-i18next';
import type {DesktopMenuItem} from 'chatgpt-desktop-kit';
import type {ChatPanelTab} from '../../chat-panel-state';
import {TabBar} from './TabBar';
import {ChromeIconButton} from './ChromeIconButton';
import {DropdownMenu,DropdownMenuTrigger,DropdownMenuContent,DropdownMenuItem,DropdownMenuLabel,DropdownMenuSeparator,DropdownMenuSub,DropdownMenuSubTrigger,DropdownMenuSubContent,DropdownMenuCheckboxItem} from './dropdown-menu';
import './locales';
import './theme.css';
import './foundation.css';
import './sortable.css';

export function PanelMenuItems({items}:{items:DesktopMenuItem[]}){
 return <>{items.map(item=><Fragment key={item.id}>{item.separator&&<DropdownMenuSeparator/>}{item.heading?<DropdownMenuLabel>{item.label}</DropdownMenuLabel>:item.children?<DropdownMenuSub><DropdownMenuSubTrigger disabled={item.disabled}>{item.label}</DropdownMenuSubTrigger><DropdownMenuSubContent><PanelMenuItems items={item.children}/></DropdownMenuSubContent></DropdownMenuSub>:typeof item.checked==='boolean'?<DropdownMenuCheckboxItem checked={item.checked} disabled={item.disabled} onSelect={()=>item.onSelect?.()}>{item.label}</DropdownMenuCheckboxItem>:<DropdownMenuItem disabled={item.disabled} onSelect={()=>item.onSelect?.()}><span className="flex-1">{item.label}</span>{item.shortcut&&<span className="pl-4 text-12 text-[var(--text-tertiary)]">{item.shortcut}</span>}</DropdownMenuItem>}</Fragment>)}</>;
}

export function PanelTabs({tabs,selected,dirty,label,full,visible,onSelect,onClose,onAdd,onFull,onHide,onReorder,onCloseOthers,onCloseAll,items,onPopupOpen}:{tabs:ChatPanelTab[];selected:string;dirty:Set<string>;label:(tab:ChatPanelTab)=>string;full:boolean;visible:boolean;onSelect:(id:string)=>void;onClose:(id:string)=>void;onAdd:(kind:string)=>void;onFull:()=>void;onHide:()=>void;onReorder:(ids:string[])=>void;onCloseOthers:(id:string)=>void;onCloseAll:()=>void;items:DesktopMenuItem[];onPopupOpen:(open:boolean)=>void}){
 const {t}=useTranslation(),[menu,setMenu]=useState(false);
 useEffect(()=>{if(!visible)setMenu(false);},[visible]);
 const actions=<DropdownMenu open={menu} onOpenChange={open=>{setMenu(open);onPopupOpen(open);}}><DropdownMenuTrigger asChild><ChromeIconButton aria-label={t('chat.paneActions')}><MoreHorizontal size={15}/></ChromeIconButton></DropdownMenuTrigger><DropdownMenuContent align="end"><PanelMenuItems items={items}/></DropdownMenuContent></DropdownMenu>;
 return <TabBar tabs={tabs.map(tab=>({id:tab.id,kind:tab.kind,state:{kind:tab.kind,title:label(tab),dirty:dirty.has(tab.id),pinned:tab.pinned}}))} activeTabId={selected} onActivate={onSelect} onClose={onClose} onReorder={onReorder} onAdd={onAdd} onCloseOthers={onCloseOthers} onCloseAll={onCloseAll} actions={actions} onPopupOpen={onPopupOpen} shellVisible={visible} showWindowControls onMaximize={onFull} isMaximized={full} onCloseSidebar={onHide} chromeWindowDrag={false}/>;
}
