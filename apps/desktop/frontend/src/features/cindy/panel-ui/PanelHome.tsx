// ActionRow and empty-pane composition from Cindy RightSidebar/EmptyState,
// 779b5237, Apache-2.0. Entries use the complete WinOne workbench registry.
import {ChevronRight,FileDiff,FilePlus,FolderOpen,Globe,Puzzle,Terminal,type LucideIcon} from 'lucide-react';
import {useTranslation} from 'react-i18next';
const actions:Array<{kind:string;icon:LucideIcon;label:string;hint:string}>=[
 {kind:'files',icon:FolderOpen,label:'rightSidebar.tabs.empty.openFile',hint:'rightSidebar.tabs.empty.fileSub'},
 {kind:'review',icon:FileDiff,label:'rightSidebar.tabs.empty.openReview',hint:'rightSidebar.tabs.empty.reviewSub'},
 {kind:'browser',icon:Globe,label:'rightSidebar.tabs.empty.openBrowser',hint:'rightSidebar.tabs.empty.browserSub'},
 {kind:'terminal',icon:Terminal,label:'rightSidebar.tabs.empty.openTerminal',hint:'rightSidebar.tabs.empty.terminalSub'},
 {kind:'resources',icon:Puzzle,label:'chat.resources',hint:'chat.resourcesSummary'},
 {kind:'new-page',icon:FilePlus,label:'chat.paneNewPage',hint:'chat.paneNewPageHint'},
];
export function PanelHome({onTool}:{onTool:(kind:string)=>void}){
 const {t}=useTranslation();return <div className="flex min-h-0 flex-1 flex-col items-start gap-8 overflow-y-auto px-10 pb-8 pt-16">
  <div className="flex w-full flex-col gap-2"><span className="text-11 font-semibold uppercase tracking-[0.08em] text-[var(--text-tertiary)]">{t('rightSidebar.tabs.empty.eyebrow')}</span><span className="text-20 font-semibold leading-tight text-[var(--text-primary)]">{t('rightSidebar.tabs.empty.title')}</span><span className="text-13 leading-relaxed text-[var(--text-tertiary)]">{t('rightSidebar.tabs.empty.desc')}</span></div>
  <div className="flex w-full flex-col">{actions.map(({kind,icon:Icon,label,hint})=><button key={kind} type="button" onClick={()=>onTool(kind)} className="group flex w-full items-center gap-3.5 border-b border-[var(--border-default)] px-1 py-3 text-left transition-colors hover:bg-[var(--surface-hover)]"><Icon size={16} className="shrink-0 text-[var(--text-secondary)]"/><span className="flex min-w-0 flex-1 flex-col gap-0.5"><span className="text-14 font-medium text-[var(--text-primary)]">{t(label)}</span><span className="text-11 text-[var(--text-tertiary)]">{t(hint)}</span></span><ChevronRight size={14} className="shrink-0 text-[var(--text-tertiary)]"/></button>)}</div>
  <p className="px-1 text-11 text-[var(--text-tertiary)]">{t('rightSidebar.tabs.empty.addMoreHint')}</p>
 </div>;
}
