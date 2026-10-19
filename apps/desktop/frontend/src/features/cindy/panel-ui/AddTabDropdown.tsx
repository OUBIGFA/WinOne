// Cindy AddTabDropdown/AddTabItem layout with WinOne's complete workbench
// registry. Selecting an existing singleton focuses it instead of cloning it.
import {useEffect,type RefObject} from 'react';
import {useTranslation} from 'react-i18next';
import {FilePlus,Globe,Terminal,FileDiff,type LucideIcon} from 'lucide-react';
import {DropdownMenuContent,DropdownMenuItem,DropdownMenuLabel} from './dropdown-menu';
// Files and resources belong to the fixed library, not the add-tab menu.
const entries:Array<{kind:string;key:string;icon:LucideIcon;singleton?:boolean}>=[
 {kind:'review',key:'chat.review',icon:FileDiff,singleton:true},
 {kind:'browser',key:'chat.browser',icon:Globe},
 {kind:'terminal',key:'chat.terminal',icon:Terminal},
 {kind:'new-page',key:'chat.paneNewPage',icon:FilePlus},
];
export function AddTabDropdown({open,anchorRef,onClose,onSelect,existingKinds}:{open:boolean;anchorRef:RefObject<HTMLElement|null>;sessionId?:string|null;onClose:()=>void;onSelect:(kind:string)=>void;existingKinds?:ReadonlySet<string>;subagentsAvailable?:boolean}){
 const {t}=useTranslation();
 useEffect(()=>{if(!open)return;let frame=0;const track=()=>{const anchor=anchorRef.current,pane=anchor?.closest('[data-panel-drag-root="right-tabs"]');if(!anchor||pane?.hasAttribute('data-pane-collapsed')){onClose();return;}frame=requestAnimationFrame(track);};track();return()=>cancelAnimationFrame(frame);},[open,anchorRef,onClose]);
 return <DropdownMenuContent align="start" data-rsb-territory="" onCloseAutoFocus={event=>{if(anchorRef.current?.closest('[data-panel-drag-root="right-tabs"]')?.hasAttribute('data-pane-collapsed'))event.preventDefault();}}>
  <DropdownMenuLabel>{t('rightSidebar.tabs.menu.addLabel')}</DropdownMenuLabel>
  {entries.map(({kind,key,icon:Icon,singleton})=><DropdownMenuItem key={kind} className="gap-2" onSelect={()=>{onSelect(kind);onClose();}}><Icon size={14} className="shrink-0"/><span className="flex-1">{t(key)}</span>{singleton&&existingKinds?.has(kind)&&<span className="pl-4 text-12 font-normal text-[var(--cmd-palette-item-meta)]">{t('rightSidebar.tabs.menu.alreadyOpen')}</span>}</DropdownMenuItem>)}
 </DropdownMenuContent>;
}
