import {useCallback,useEffect,useRef,useState,type ReactNode} from 'react';
import {Copy,FilePlus,FolderPlus,ClipboardPaste,Play,Square,QrCode,RefreshCw,Trash2,Send,Download,Cloud} from 'lucide-react';
import {interconnectCall,pickFiles,pickDirectory,readClipboardFiles} from '../bridge';
import {useTranslation} from '../i18n';
import {Button} from '../ui/button';
import {Notice} from '../ui/notice';
import {usePageActivity,type PageActivityChanged} from '../ui/page-navigation';
import {PathDropArea} from '../ui/path-drop-zone';
import {SettingsSection,SettingsDisclosure,SettingsOptions,SettingsToggle} from '../components/settings-layout';
import {Input} from '../components/ui/input';
import {Textarea} from '../components/ui/textarea';
import {Label} from '../components/ui/label';
import {PeerTask,type PeerRecord} from './interconnect-peer';
import type {InterconnectIntent} from './interconnect-intents';
import {Select,SelectContent,SelectItem,SelectTrigger,SelectValue} from '../components/ui/select';

type Form={derpMapURL:string;allowWrites:boolean;listen:string;paths:string[];text:string;code:string;outputDir:string;options:Record<string,string|boolean|string[]>;socks5:string;httpProxy:string;storeOrigin:string;downloads:number;expirationHours:number};
type RecordItem={id:string;created:string;spec:Partial<Form>&{mode:string};state:PeerRecord['state']&{status:string;code?:string;qr?:string;output?:string;detail?:string;error?:string;persistenceError?:string;cleanupError?:string;notificationError?:string;prompt?:{id:number;text:string};progress?:{Progress?:{CurrentNum:number;Max:number;Description:string;KBsPerSecond:number}};storeProgress?:{TotalBytes:number;TotalSize:number;FileName:string};result?:{code?:string;link?:string;qr?:string;expiresAt?:string;downloads?:number;revoked?:boolean;textPath?:string;manifest?:{f:{n:string;s:number}[]}}}};
const initial=():Form=>({derpMapURL:'',allowWrites:false,listen:'127.0.0.1:0',paths:[],text:'',code:'',outputDir:'',options:{},socks5:'',httpProxy:'',storeOrigin:'https://getcroc.com',downloads:1,expirationHours:24});
function Field({label,children}:{label:string;children:ReactNode}){return <div className="min-w-0 space-y-2"><Label>{label}</Label>{children}</div>;}
const size=(n:number)=>new Intl.NumberFormat(undefined,{maximumFractionDigits:1}).format(n>=1048576?n/1048576:n/1024)+(n>=1048576?' MiB':' KiB');

export type CodeTransferState={status:string;code?:string;error?:string;detail?:string;output?:string;prompt?:{id:number;text:string};progress?:{Progress?:{CurrentNum:number;Max:number;Description:string;KBsPerSecond:number}}};
function CodeTransferStatus({id,state,busy,action,copy}:{id:string;state:CodeTransferState;busy:boolean;action:(work:()=>Promise<unknown>)=>Promise<void>;copy:(value:string)=>Promise<void>}){
 const {t}=useTranslation(),tr=(key:string)=>t('interconnect.'+key),[answer,setAnswer]=useState('');
 const progress=state.progress?.Progress;
 return <div className="space-y-3">
  <p className="settings-help">{tr('transfer')} · {tr(state.status==='completed'?'codeDelivered':state.status==='failed'?'failedTask':state.status==='running'?'runningTask':'cancelledTask')}</p>
  {state.code&&<p className="break-all select-all">{state.code}</p>}
  {progress&&progress.Max>0&&<><p className="settings-help break-all">{progress.Description} · {size(progress.CurrentNum)} / {size(progress.Max)} · {size(progress.KBsPerSecond*1024)}/s</p><progress className="w-full" value={progress.CurrentNum} max={progress.Max} aria-label={tr('fileProgress')}/></>}
  {state.error&&<Notice>{state.error}</Notice>}
  {state.prompt&&<div className="space-y-2"><p className="whitespace-pre-wrap break-all">{state.output?.slice(-4096)}{state.prompt.text}</p><div className="flex flex-wrap gap-2">{['yes','no'].map(value=><Button key={value} disabled={busy} onClick={()=>void action(()=>interconnectCall('share_code_answer',{id,promptId:state.prompt!.id,answer:value}))}>{tr(value)}</Button>)}<Input aria-label={tr('answer')} value={answer} onChange={e=>setAnswer(e.target.value)}/><Button disabled={busy||!answer} onClick={()=>void action(()=>interconnectCall('share_code_answer',{id,promptId:state.prompt!.id,answer}))}>{tr('reply')}</Button></div></div>}
  <div className="flex flex-wrap gap-2">{state.code&&<Button icon={Copy} onClick={()=>void copy(state.code!)}>{tr('codeCopy')}</Button>}{state.status==='running'?<Button icon={Square} disabled={busy} onClick={()=>void action(()=>interconnectCall('share_code_stop',{id}))}>{tr('stopCodeTransfer')}</Button>:<Button icon={Play} disabled={busy} onClick={()=>void action(()=>interconnectCall('share_code_restart',{id}))}>{tr('sendCodeAgain')}</Button>}</div>
  {(state.output||state.detail)&&<SettingsDisclosure title={tr('details')}><pre className="whitespace-pre-wrap break-all text-sm">{state.detail}{state.output}</pre></SettingsDisclosure>}
 </div>;
}

// Panels share the WinOne controls and native drop adapter. No file content is
// read by React; the upstream process writes directly to the selected folder.
export function TransferPanel({stored,active,onActivity,intent,onIntentConsumed,settingsPending=false}:{stored:boolean;active:boolean;settingsPending?:boolean;onActivity:PageActivityChanged;intent?:InterconnectIntent;onIntentConsumed?:()=>void}){
 const {t}=useTranslation(),tr=(key:string)=>t('interconnect.'+key);
 const [form,setForm]=useState(initial),[saved,setSaved]=useState(()=>JSON.stringify(initial()));
 const [direction,setDirection]=useState('send'),[payload,setPayload]=useState('files');
 const [items,setItems]=useState<RecordItem[]>([]),[busy,setBusy]=useState(false),[error,setError]=useState(''),[message,setMessage]=useState('');
 const [qr,setQr]=useState(''),[answers,setAnswers]=useState<Record<string,string>>({});
 const mounted=useRef(true),epoch=useRef(0);
 useEffect(()=>{if(!intent)return;setDirection(intent.receive?'receive':'send');setPayload('files');setForm(current=>({...current,paths:[...new Set([...current.paths,...intent.paths])],code:intent.code??current.code}));onIntentConsumed?.();},[intent]);
 useEffect(()=>{mounted.current=true;return()=>{mounted.current=false;epoch.current++;};},[]);
 const refresh=useCallback(async()=>{const version=++epoch.current;const all=await interconnectCall<RecordItem[]>('transfer_list');if(mounted.current&&version===epoch.current)setItems(all);},[]);
 useEffect(()=>{if(!active)return;void refresh().catch(e=>setError(String(e)));const interval=window.setInterval(()=>{if(document.visibilityState==='visible')void refresh().catch(e=>setError(String(e)));},750);return()=>clearInterval(interval);},[active,refresh]);
 usePageActivity(onActivity,JSON.stringify(form)!==saved,busy,stored?'interconnect-store':'interconnect-transfer');
 const set=<K extends keyof Form>(key:K,value:Form[K])=>setForm(current=>({...current,[key]:value}));
 const option=(key:string,value:string|boolean|string[])=>setForm(current=>({...current,options:{...current.options,[key]:value}}));
 async function action(work:()=>Promise<unknown>){setBusy(true);setError('');setMessage('');try{await work();await refresh();}catch(e){if(mounted.current)setError(String(e));}finally{if(mounted.current)setBusy(false);}}
 const add=(paths:string[])=>setForm(current=>({...current,paths:[...new Map([...current.paths,...paths].map(p=>[p.toLowerCase(),p])).values()]}));
 async function copy(value:string){try{await navigator.clipboard.writeText(value);setMessage(tr('copiedTransfer'));}catch(e){setError(String(e));}}
 async function start(mode:string,override?:Partial<Form>){
  const draft={...form,...override};
  const spec={...draft,mode,paths:payload==='text'&&mode==='send'?[]:draft.paths,text:payload==='text'&&mode==='send'?draft.text:'',options:{...draft.options,...(mode==='receive'?{transport:'auto'}:{})}};
  if(!stored&&mode==='send'){
   // A single worker owns the LAN pool, persistent relay and code sender.
   await interconnectCall('transfer_start',{mode:'share_server',paths:spec.paths,text:spec.text,derpMapURL:draft.derpMapURL,allowWrites:draft.allowWrites,transfer:{code:draft.code,options:draft.options,socks5:draft.socks5,httpProxy:draft.httpProxy}});
  }else if(!stored&&mode==='receive'&&draft.code.trim().startsWith('tc')){
   await interconnectCall('transfer_start',{mode:'share_client',code:draft.code.trim(),listen:draft.listen,derpMapURL:draft.derpMapURL});
  }else{
   const {derpMapURL,allowWrites,listen,...transferSpec}=spec;
   await interconnectCall('transfer_start',transferSpec);
  }
  const next={...form,paths:[],text:''};setForm(next);setSaved(JSON.stringify(next));
 }
 const field=(key:keyof Form,label:string,type='text')=><Field label={tr(label)}><Input aria-label={tr(label)} type={type} value={String(form[key])} disabled={busy} onChange={e=>set(key,(type==='number'?Number(e.target.value):e.target.value) as never)}/></Field>;
 const optionField=(key:string,label:string,placeholder?:string)=><Field label={tr(label)}><Input aria-label={tr(label)} placeholder={placeholder} value={String(form.options[key]??'')} disabled={busy} onChange={e=>option(key,e.target.value)}/></Field>;
 const toggle=(key:string,label:string)=><SettingsToggle label={tr(label)} checked={form.options[key]===true} disabled={busy} onChange={v=>option(key,v)}/>;
 const select=(label:string,value:string,choices:{value:string;label:string}[],change:(v:string)=>void)=><Field label={tr(label)}><Select value={value} disabled={busy} onValueChange={v=>change(v??'')}><SelectTrigger className="w-full" aria-label={tr(label)}><SelectValue>{tr(choices.find(c=>c.value===value)?.label??value)}</SelectValue></SelectTrigger><SelectContent>{choices.map(c=><SelectItem key={c.value} value={c.value}>{tr(c.label)}</SelectItem>)}</SelectContent></Select></Field>;
 const records=items.filter(item=>stored?item.spec.mode.startsWith('store_'):['send','receive','share_server','share_client'].includes(item.spec.mode));
 return <div className="settings-stack">
  {error&&<Notice>{error}</Notice>}{message&&<p role="status" className="settings-help">{message}</p>}
  <SettingsSection title={tr(stored?'store':'fileTransfer')} description={tr(stored?'storeHint':'unifiedSendHint')} icon={stored?Cloud:Send} className="interconnect-wide">
   <SettingsOptions label={tr('direction')} value={direction} onChange={setDirection} items={[{value:'send',label:tr('send')},{value:'receive',label:tr('receive')}]}/>
   {direction==='send'?<>
    {!stored&&<SettingsOptions label={tr('send')} value={payload} onChange={setPayload} items={[{value:'files',label:tr('sendFiles')},{value:'text',label:tr('sendText')}]}/>}
    {payload==='text'&&!stored?<Textarea aria-label={tr('sendText')} value={form.text} disabled={busy} onChange={e=>set('text',e.target.value)}/>:<PathDropArea multiple kind={stored?'file':'any'} disabled={busy||!active} onPaths={add} onError={setError} className="interconnect-drop">
     <p className="settings-help">{tr(stored?'dropFiles':'drop')}</p>
     <div className="flex flex-wrap gap-2"><Button icon={FilePlus} disabled={busy} onClick={()=>void action(async()=>add(await pickFiles()))}>{tr('chooseFiles')}</Button>{!stored&&<Button icon={FolderPlus} disabled={busy} onClick={()=>void action(async()=>{const folder=await pickDirectory();if(folder)add([folder]);})}>{tr('chooseFolder')}</Button>}<Button icon={ClipboardPaste} disabled={busy} onClick={()=>void action(async()=>add(await readClipboardFiles()))}>{tr('paste')}</Button></div>
     {form.paths.length>0&&<><div className="flex items-center justify-between gap-2"><span>{t('interconnect.selectedFiles',{count:form.paths.length})}</span><Button variant="ghost" onClick={()=>set('paths',[])}>{tr('clear')}</Button></div><ul className="space-y-2">{form.paths.map(path=><li key={path} className="flex min-w-0 items-start gap-2"><span className="min-w-0 flex-1 break-all" title={path}>{path}</span><Button variant="ghost" icon={Trash2} aria-label={tr('remove')+' '+path} onClick={()=>set('paths',form.paths.filter(p=>p!==path))}/></li>)}</ul></>}
    </PathDropArea>}
    {stored?<><p className="settings-help">{tr('storeFilesOnly')}</p><div className="interconnect-fields">{field('storeOrigin','origin')}{field('downloads','downloads','number')}{field('expirationHours','expiration','number')}</div></>:null}
   </>:<>
    {field('code','code')}<Button icon={ClipboardPaste} disabled={busy} onClick={()=>void action(async()=>set('code',await navigator.clipboard.readText()))}>{tr('clipboardCode')}</Button>
    <div className="flex flex-wrap items-end gap-2"><div className="min-w-0 flex-1">{field('outputDir','output')}</div><Button icon={FolderPlus} disabled={busy} onClick={()=>void action(async()=>{const folder=await pickDirectory();if(folder)set('outputDir',folder);})}>{tr('choose')}</Button></div>
   </>}
   <SettingsDisclosure title={tr('networkOptions')}><p className="settings-help">{tr('advancedHint')}</p><div className="interconnect-fields">
    {!stored&&<>{field('derpMapURL','derpMap')}{direction==='receive'&&field('listen','listenPort')}{optionField('relayAddress','relay',tr('automaticRelay'))}{optionField('relayAddress6','relay6')}{optionField('relayPassword','relayPassword')}{optionField('IP','senderIP')}{optionField('multicastAddress','multicast')}
    <Field label={tr('ports')}><Input aria-label={tr('ports')} value={(form.options.relayPorts as string[]??[]).join(',')} onChange={e=>option('relayPorts',e.target.value.split(',').map(v=>v.trim()).filter(Boolean))}/></Field></>}
    {field('socks5','socks5')}{field('httpProxy','httpProxy')}
   </div>{!stored&&<>{toggle('onlyLocal','localOnly')}{toggle('disableLocal','disableLocal')}</>}</SettingsDisclosure>
   {!stored&&direction==='send'&&<SettingsToggle label={tr('allowWrites')} checked={form.allowWrites} onChange={allowWrites=>set('allowWrites',allowWrites)}/>}
   {!stored&&<SettingsDisclosure title={tr('transferOptions')}><div className="interconnect-fields">{direction==='send'&&field('code','customCode')}
    {direction==='send'&&select('transport',String(form.options.transport??'auto'),[{value:'auto',label:'auto'},{value:'derp',label:'derp'},{value:'relay',label:'relayOnly'}],v=>option('transport',v))}
    {select('collision',form.options.rename?'rename':form.options.overwrite?'overwrite':'ask',[{value:'ask',label:'ask'},{value:'rename',label:'rename'},{value:'overwrite',label:'overwrite'}],v=>setForm(f=>({...f,options:{...f.options,rename:v==='rename',overwrite:v==='overwrite'}})))}
    {optionField('curve','curve')}{direction==='send'&&<>{optionField('hashAlgorithm','hash')}{optionField('throttleUpload','throttle')}</>}
   </div>{direction==='send'&&<>{toggle('zipFolder','zip')}{toggle('gitIgnore','gitIgnore')}{toggle('noCompress','noCompress')}{toggle('noMultiplexing','noMulti')}{toggle('ask','verifyPeer')}
   {(['exclude','excludeFile'] as const).map(key=><Field key={key} label={tr(key)}><Input aria-label={tr(key)} value={(form.options[key] as string[]??[]).join(',')} onChange={e=>option(key,e.target.value.split(',').map(v=>v.trim()).filter(Boolean))}/></Field>)}</>}</SettingsDisclosure>}
   {settingsPending&&<p className="settings-help">{tr('saveBeforeSending')}</p>}
   <div><Button variant="primary" icon={direction==='send'?Send:Download} busy={busy} disabled={settingsPending||(direction==='send'?(payload==='text'&&!stored?!form.text.trim():!form.paths.length):!form.code.trim())} onClick={()=>void action(()=>start(stored?(direction==='send'?'store_send':'store_inspect'):(direction==='send'?'send':'receive')))}>{tr(stored?(direction==='send'?'publishStore':'inspect'):(direction==='send'?'sendSelection':'receiveNow'))}</Button></div>
  </SettingsSection>
  <SettingsSection title={tr('tasks')} icon={RefreshCw} className="interconnect-wide">
   {records.length===0?<p className="settings-help py-4">{tr('noTasks')}</p>:<div className="divide-y divide-border">{records.map(item=>{
    if(item.spec.mode.startsWith('share_'))return <PeerTask key={item.id} item={item} kind="share" busy={busy} action={action} copy={copy}>{item.state.transfer&&<CodeTransferStatus id={item.id} state={item.state.transfer} busy={busy} action={action} copy={copy}/>}</PeerTask>;
    const state=item.state,result=state.result,bar=state.progress?.Progress,progress=state.storeProgress;
    const code=result?.code??state.code??'',image=result?.qr??state.qr;
    const maximum=progress?.TotalSize??bar?.Max??0,current=progress?.TotalBytes??bar?.CurrentNum??0;
    const status=tr(({running:'runningTask',completed:'completedTask',failed:'failedTask',cancelled:'cancelledTask',interrupted:'interruptedTask'} as Record<string,string>)[state.status]??state.status);
    return <article key={item.id} className="interconnect-task">
     <div className="flex flex-wrap justify-between gap-2"><p className="font-medium break-all">{item.spec.paths?.map(p=>p.split(/[\\/]/).pop()).join(' · ')||tr(item.spec.mode==='store_inspect'?'inspect':item.spec.mode==='store_revoke'?'revoke':item.spec.mode==='send'?'sendText':'receive')}</p><span className="text-sm text-muted-foreground">{status}</span></div>
     {maximum>0&&<div className="space-y-2"><p className="settings-help break-all">{progress?.FileName??bar?.Description} · {tr('fileProgress')} {size(current)} / {size(maximum)}{bar?.KBsPerSecond?' · '+size(bar.KBsPerSecond*1024)+'/s':''}</p><progress className="w-full" aria-label={tr('fileProgress')} value={current} max={maximum}/></div>}
     {state.status==='running'&&!maximum&&!state.prompt&&<p className="settings-help">{tr('waitTask')}</p>}
     {code&&<p className="break-all select-all text-sm">{code}</p>}
     {result?.expiresAt&&<p className="settings-help">{tr('expiresAt')} {new Date(result.expiresAt).toLocaleString()} · {tr('downloads')} {result.downloads}</p>}
     {result?.revoked&&<p>{tr('revoked')}</p>}
     {state.error&&<Notice>{state.error}</Notice>}{[state.persistenceError,state.cleanupError,state.notificationError].filter(Boolean).map((warning,i)=><Notice key={i}>{warning}</Notice>)}
     {state.prompt&&<div className="interconnect-confirm"><p className="font-medium">{tr('confirmation')}</p><pre className="whitespace-pre-wrap break-all text-sm">{state.output?.slice(-4096)}{state.prompt.text}</pre><div className="flex flex-wrap gap-2">{['yes','no'].map(answer=><Button key={answer} disabled={busy} onClick={()=>void action(()=>interconnectCall('transfer_answer',{id:item.id,promptId:state.prompt!.id,answer}))}>{tr(answer)}</Button>)}<Input aria-label={tr('answer')} className="min-w-0 flex-1" value={answers[item.id]??''} onChange={e=>setAnswers({...answers,[item.id]:e.target.value})}/><Button disabled={busy||!answers[item.id]} onClick={()=>void action(()=>interconnectCall('transfer_answer',{id:item.id,promptId:state.prompt!.id,answer:answers[item.id]}))}>{tr('reply')}</Button></div></div>}
     {result?.manifest&&<><ul className="space-y-1">{result.manifest.f.map(file=><li key={file.n} className="flex flex-wrap justify-between gap-2"><span className="break-all">{file.n}</span><span>{size(file.s)}</span></li>)}</ul><div><Button icon={Download} disabled={busy} onClick={()=>void action(()=>start('store_receive',{code:item.spec.code,outputDir:item.spec.outputDir}))}>{tr('accept')}</Button></div></>}
     {result?.textPath&&<div className="space-y-2"><p className="break-all">{tr('textReceived')} {result.textPath}</p><div className="flex flex-wrap gap-2"><Button onClick={()=>void action(()=>interconnectCall('transfer_text_open',{id:item.id}))}>{tr('openText')}</Button><Button icon={Copy} onClick={()=>void action(async()=>copy(await interconnectCall<string>('transfer_text',{id:item.id})))}>{tr('copyReceivedText')}</Button></div></div>}
     <div className="flex flex-wrap gap-2">
      {code&&<Button icon={Copy} onClick={()=>void copy(code)}>{tr('codeCopy')}</Button>}{result?.link&&<Button icon={Copy} onClick={()=>void copy(result.link!)}>{tr('linkCopy')}</Button>}{image&&<Button icon={QrCode} onClick={()=>setQr(qr===item.id?'':item.id)}>{tr(qr===item.id?'hideQR':'qr')}</Button>}
      {state.status==='running'?<Button icon={Square} disabled={busy} onClick={()=>void action(()=>interconnectCall('transfer_stop',{id:item.id}))}>{tr('cancel')}</Button>:<>
       {item.spec.mode==='store_send'&&result?.code&&!result.revoked?<Button disabled={busy} onClick={()=>void action(()=>interconnectCall('share_revoke',{id:item.id}))}>{tr('revoke')}</Button>:<Button variant="ghost" icon={Trash2} disabled={busy} onClick={()=>void action(()=>interconnectCall('transfer_remove',{id:item.id}))}>{tr('forget')}</Button>}
       {item.spec.mode!=='store_revoke'&&<Button variant="ghost" icon={Play} onClick={()=>{setForm({...initial(),...item.spec});setDirection(item.spec.mode.endsWith('send')?'send':'receive');setPayload(item.spec.text?'text':'files');setMessage(tr('restoreDraft'));}}>{tr('reuse')}</Button>}
      </>}
     </div>
     {qr===item.id&&image&&<img src={image} alt={tr('qr')} className="h-auto w-44 max-w-full"/>}
     {(state.output||state.detail)&&<SettingsDisclosure title={tr('details')}><pre className="whitespace-pre-wrap break-all text-sm">{state.detail}{state.detail?'\n':''}{state.output}</pre></SettingsDisclosure>}
    </article>;
   })}</div>}
  </SettingsSection>
 </div>;
}
