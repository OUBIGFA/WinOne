// Adapted from Cindy TerminalTabBody/xtermPool (a152497a, Apache-2.0).
// WinOne keeps tab bodies mounted; the native ConPTY belongs to this tab until
// it is closed. Poll offsets preserve UTF-8 bytes across response boundaries.
import {useEffect,useRef,useState} from 'react';
import {Terminal} from '@xterm/xterm';
import {FitAddon} from '@xterm/addon-fit';
import {WebLinksAddon} from '@xterm/addon-web-links';
import '@xterm/xterm/css/xterm.css';
import {chatCall} from '../bridge';
import {CINDY_TERMINAL_OPTIONS} from './cindy/terminal/options';
import {ErrorOverlay,ExitedOverlay} from './cindy/terminal/TerminalOverlays';
import {Spinner} from './cindy/panel-ui/spinner';
import './cindy/panel-ui/locales';
import type {DesktopMenuItem} from 'chatgpt-desktop-kit';
import {useTranslation} from '../i18n';
import {createTerminalInput} from './chat-terminal-input';

type Poll={data:string;offset:number;reset:boolean;exit?:number;error?:string};
export function ChatTerminalView({sessionId,tabId,active=true,onRunningChange,onActions}:{sessionId:string;tabId:string;active?:boolean;onRunningChange?:(running:boolean)=>void;onActions?:(items:DesktopMenuItem[])=>void}){
 const {t}=useTranslation(),slot=useRef<HTMLDivElement>(null),terminal=useRef<Terminal|undefined>(undefined),fit=useRef<()=>void>(()=>{}),visible=useRef(active),chain=useRef(Promise.resolve());visible.current=active;
 const [shell,setShell]=useState('powershell'),[generation,setGeneration]=useState(0),[error,setError]=useState(''),[exit,setExit]=useState<number>(),[loading,setLoading]=useState(true);
 const runningCallback=useRef(onRunningChange);runningCallback.current=onRunningChange;
 const actionsCallback=useRef(onActions);actionsCallback.current=onActions;
 useEffect(()=>{
  // Cindy's shared tab menu keeps WinOne's shell and process controls together.
  actionsCallback.current?.([
   {id:'terminal.shell',label:t('chat.terminalShell'),children:[['powershell','Windows PowerShell'],['pwsh','PowerShell 7'],['cmd','Command Prompt']].map(([value,label])=>({id:'terminal.shell.'+value,label,checked:shell===value,onSelect:()=>setShell(value)}))},
   {id:'terminal.restart',label:t('chat.terminalRestart'),onSelect:()=>setGeneration(value=>value+1)},
   {id:'terminal.clear',label:t('chat.terminalClear'),onSelect:()=>terminal.current?.clear()},
   {id:'terminal.copy',label:t('chat.copy'),onSelect:()=>void navigator.clipboard.writeText(terminal.current?.getSelection()??'').catch(error=>setError(String(error)))},
  ]);
  return()=>actionsCallback.current?.([]);
 },[shell,t]);
 useEffect(()=>{
  let alive=true,attached=false,timer:ReturnType<typeof setTimeout>,offset=0;
  const term=new Terminal({...CINDY_TERMINAL_OPTIONS,cursorBlink:!(typeof matchMedia==='function'&&matchMedia('(prefers-reduced-motion: reduce)').matches)}),addon=new FitAddon();terminal.current=term;
  term.loadAddon(addon);term.loadAddon(new WebLinksAddon((_event,url)=>{void chatCall('link.open',{url}).catch(report);}));term.open(slot.current!);
  const call=<T,>(method:string,data:Record<string,unknown>={})=>chatCall<T>('terminal.'+method,{sessionId,tabId,...data});
  function report(value:unknown){if(alive){setError(String(value));setLoading(false);}}
  term.attachCustomKeyEventHandler(event=>{if(event.type==='keydown'&&(event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='c'&&term.hasSelection()){event.preventDefault();void navigator.clipboard.writeText(term.getSelection()).catch(report);return false;}return true;});
  const inputQueue=createTerminalInput(data=>call('write',{data}),report);
  const input=term.onData(data=>inputQueue.enqueue(data));
  const resize=()=>{if(!alive||!visible.current||!slot.current?.clientWidth||!slot.current?.clientHeight)return;addon.fit();if(attached)void call('resize',{cols:Math.max(2,term.cols),rows:Math.max(2,term.rows)}).catch(report);};fit.current=resize;
  const observer=new ResizeObserver(resize);observer.observe(slot.current!);
  async function poll(){if(!alive||!attached)return;try{const result=await call<Poll>('poll',{offset});if(!alive)return;offset=result.offset;if(result.reset)term.reset();if(result.data)term.write(Uint8Array.from(atob(result.data),char=>char.charCodeAt(0)));if(result.error)setError(result.error);if(result.exit!==undefined){setExit(result.exit);runningCallback.current?.(false);return;}}catch(error){report(error);return;}timer=setTimeout(()=>void poll(),visible.current?100:1000);}
  setError('');setExit(undefined);setLoading(true);
  // StrictMode/remount and restart share a chain: dispose the previous native
  // owner before a new create, and never let a late cleanup kill a new shell.
  chain.current=chain.current.then(async()=>{if(!alive)return;resize();await call('create',{shell,cols:Math.max(2,term.cols),rows:Math.max(2,term.rows)});attached=true;if(alive){inputQueue.attach();setLoading(false);runningCallback.current?.(true);if(visible.current)term.focus();void poll();}}).catch(report);
  return()=>{alive=false;inputQueue.close();runningCallback.current?.(false);clearTimeout(timer);observer.disconnect();input.dispose();term.dispose();chain.current=chain.current.then(async()=>{if(attached)await call('dispose');}).catch(console.warn);};
 },[sessionId,tabId,shell,generation]);
 useEffect(()=>{if(active){fit.current();terminal.current?.focus();}},[active]);
 return <section className="chat-terminal relative">
  {error&&<div role="alert"><ErrorOverlay message={error} restarting={loading} onRetry={()=>setGeneration(value=>value+1)} t={t}/></div>}{loading&&!error&&<div role="status" className="absolute inset-0 flex items-center justify-center gap-2 text-13 text-white/60"><Spinner size={14}/>{t('chat.terminalStarting')}</div>}{exit!==undefined&&<div role="status"><ExitedOverlay exit={{code:exit,signal:null}} restarting={loading} onRestart={()=>setGeneration(value=>value+1)} t={t}/></div>}
  <div ref={slot} className="chat-terminal-slot"/>
 </section>;
}
