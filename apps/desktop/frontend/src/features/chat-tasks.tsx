import {ListTree,Pause,Play} from 'lucide-react';
import {Button} from '../ui/button';
import {Popover,PopoverContent,PopoverTrigger} from '../components/ui/popover';
import {useTranslation} from '../i18n';
import type {ChatTask,ChatTaskGraph} from './chat-state';

// Compose the shared popover and native nested lists. Show only the inspector's
// task metadata; tool arguments and checkpoints never enter this view.
export function ChatTasks({graph,paused,running,busy,onPause,onResume}:{graph?:ChatTaskGraph;paused:boolean;running:boolean;busy:boolean;onPause?:()=>void;onResume?:()=>void}){
 const {t}=useTranslation();
 const tasks=graph?.tasks??[],ids=new Set(tasks.map(task=>task.id));
 function row(task:ChatTask,ancestors:number[]=[]){
  const children=tasks.filter(child=>child.owner===task.id&&!ancestors.includes(child.id)&&child.id!==task.id);
  const name=task.kind==='pi.generation'?t('chat.taskGeneration'):task.kind==='pi.tool'?t('chat.taskTool'):task.kind;
  return <li key={task.id} className="min-w-0 py-1 text-sm"><div className="flex min-w-0 flex-wrap items-center gap-x-2"><span className="break-all" title={task.kind}>{name} · #{task.id}</span><span className="text-muted-foreground">{t(paused?'chat.taskWaitingForResume':'chat.taskState_'+task.state)}</span>{task.background&&<span className="text-muted-foreground">{t('chat.taskBackground')}</span>}</div>{children.length>0&&<ul className="ml-3 list-none border-l border-border pl-3">{children.map(child=>row(child,[...ancestors,task.id]))}</ul>}</li>;
 }
 return <>
  {!!graph?.total&&<Popover><PopoverTrigger render={<Button variant="ghost" size="sm" icon={ListTree} aria-label={t('chat.tasks')+' · '+graph.total} disabled={busy}>{t('chat.tasks')} · {graph.total}</Button>}/><PopoverContent side="top" align="start" className="chat-row-menu max-h-80 overflow-y-auto"><p className="m-0 text-sm text-muted-foreground">{t('chat.pauseTaskHelp')}</p><p className="m-0 mt-2 text-sm text-muted-foreground">{t(paused?'chat.taskPaused':'chat.taskScheduling_'+graph.scheduling)}</p><ul aria-label={t('chat.tasks')} className="m-0 list-none p-0">{tasks.filter(task=>task.owner===undefined||!ids.has(task.owner)).map(task=>row(task))}</ul>{graph.total>tasks.length&&<p className="text-sm text-muted-foreground">{t('chat.taskLimit',{shown:tasks.length,total:graph.total})}</p>}</PopoverContent></Popover>}
  {paused&&onResume?<Button variant="ghost" size="sm" icon={Play} aria-label={t('chat.resumeTask')} disabled={busy} onClick={onResume}>{t('chat.resumeTask')}</Button>:running&&onPause?<Button variant="ghost" size="sm" icon={Pause} aria-label={t('chat.pauseTask')} title={t('chat.pauseTaskHelp')} disabled={busy} onClick={onPause}>{t('chat.pauseTask')}</Button>:null}
 </>;
}
