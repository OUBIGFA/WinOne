import {useCallback,useEffect,useRef,useState} from 'react';
import {Copy,FilePlus,FolderOpen,Network,Play,Square,RefreshCw,Trash2,QrCode,ClipboardPaste,FileText,Download,Send,Cloud} from 'lucide-react';
import {interconnectCall,pickFiles,pickDirectory,readClipboardFiles} from '../bridge';
import {useTranslation} from '../i18n';
import {Button} from '../ui/button';
import {Notice} from '../ui/notice';
import {usePageActivity,type PageActivityChanged} from '../ui/page-navigation';
import {SettingsSection,SettingsDisclosure} from '../components/settings-layout';
import {Input} from '../components/ui/input';
import {Textarea} from '../components/ui/textarea';
import {Label} from '../components/ui/label';
import {Select,SelectContent,SelectItem,SelectTrigger,SelectValue} from '../components/ui/select';
import {Tabs,TabsList,TabsTrigger,TabsContent} from '../components/ui/tabs';
import {Dialog,DialogContent,DialogHeader,DialogTitle,DialogFooter} from '../components/ui/dialog';
import {Checkbox} from '../components/ui/checkbox';
import {IconLabel} from '../ui/icon-label';
import './interconnect.css';
import {TransferPanel} from './interconnect-transfer';
import {PeerPanel} from './interconnect-peer';
import {InterconnectGeneralPanel,type InterconnectGeneral} from './interconnect-general';
import type {InterconnectIntent} from './interconnect-intents';
import {PathDropArea} from '../ui/path-drop-zone';

type Preferences={bind:string;port:number;downloadDir:string};
type Address={name?:string;ip:string;url?:string;qr?:string};
type Item={id:string;kind:string;title:string;content?:string;size:number;exists:boolean};
type State={running:boolean;settings:Preferences;addresses:Address[];snapshot:null|{info:{url:string;qr:string;addresses:Address[]};items:Item[];downloads:{itemId:string;speedBps:number;activeCount:number}[]}};

// Compose existing WinOne controls. File bytes stay inside the upstream service.
export function InterconnectPage({onActivity,intent,onIntentConsumed}:{onActivity:PageActivityChanged;intent?:InterconnectIntent;onIntentConsumed?:()=>void}){
 const {t}=useTranslation();
 const [state,setState]=useState<State>(),[preferences,setPreferences]=useState<Preferences>();
 const [text,setText]=useState(''),[busy,setBusy]=useState(false),[error,setError]=useState(''),[notice,setNotice]=useState('');
 const [qr,setQr]=useState(false),[selectedIP,setSelectedIP]=useState('');
 const [save,setSave]=useState<{id:string;path:string;replace:boolean}>();
 const [tab,setTab]=useState('lan');
 const [clipboard,setClipboard]=useState<{kind:'share'|'transfer'|'store';code:string}>(),[clipboardIntent,setClipboardIntent]=useState<InterconnectIntent>(),[shareCode,setShareCode]=useState(''),[storeCode,setStoreCode]=useState<InterconnectIntent>();
 useEffect(()=>{let live=true;const read=async()=>{try{const settings=await interconnectCall<InterconnectGeneral>('general.get');if(!live||!settings?.clipboard)return;const code=(await navigator.clipboard.readText()).trim();if(!live||code.length>4096)return;const kind=code.startsWith('tc')&&code.length>40?'share':code.startsWith('croc-store-v1.')||code.startsWith('https://getcroc.com/s/')?'store':/^[0-9]+-[a-z0-9-]+$/i.test(code)?'transfer':undefined;if(kind)setClipboard({kind,code});}catch(e){if(live)setError(String(e));}};window.addEventListener('focus',read);void read();return()=>{live=false;window.removeEventListener('focus',read);};},[]);
 useEffect(()=>{if(intent)setTab('transfer');},[intent]);
 const alive=useRef(true),version=useRef(0);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;version.current++;};},[]);
 const refresh=useCallback(async(reset=false)=>{
  const request=++version.current,result=await interconnectCall<State>('get');
  if(!alive.current||request!==version.current)return;
  setState(result);setPreferences(current=>reset||!current?result.settings:current);
 },[]);
 useEffect(()=>{void refresh().catch(e=>setError(String(e)));},[refresh]);
 useEffect(()=>{
  if(!state?.running||busy||tab!=='lan')return;
  const id=window.setInterval(()=>{if(document.visibilityState==='visible')void refresh().catch(e=>setError(String(e)));},1000);
  return()=>window.clearInterval(id);
 },[state?.running,busy,refresh,tab]);
 const dirty=!!save||!!text.trim()||!!(preferences&&state&&JSON.stringify(preferences)!==JSON.stringify(state.settings));
 // Sharing survives navigation; only unfinished input and current actions need protection.
 usePageActivity(onActivity,dirty,busy,'interconnect');
 async function action(work:()=>Promise<unknown>){
  setBusy(true);setError('');setNotice('');version.current++;
  try{await work();await refresh();}catch(e){if(alive.current)setError(String(e));}finally{if(alive.current)setBusy(false);}
 }
 async function copy(value:string){try{await navigator.clipboard.writeText(value);setNotice(t('interconnect.copied'));}catch(e){setError(String(e));}}
 const addresses=state?.snapshot?.info.addresses??[];
 const selected=addresses.find(a=>a.ip===selectedIP)??addresses[0];
 const url=selected?.url??state?.snapshot?.info.url??'';
 const options=[{ip:'0.0.0.0',name:t('interconnect.all')},{ip:'127.0.0.1',name:t('interconnect.local')},...(state?.addresses??[]).filter(a=>a.ip!=='127.0.0.1')];
 return <Tabs value={tab} onValueChange={value=>setTab(String(value))}>
  {clipboard&&<Notice actions={<Button onClick={()=>{if(clipboard.kind==='share')setShareCode(clipboard.code);else if(clipboard.kind==='store')setStoreCode({requested:true,paths:[],receive:true,code:clipboard.code});else setClipboardIntent({requested:true,paths:[],receive:true,code:clipboard.code});setTab(clipboard.kind);setClipboard(undefined);}}>{t('interconnect.useClipboard')}</Button>}>{t('interconnect.clipboardFound')}</Notice>}
  <TabsList aria-label={t('interconnect.title')}><TabsTrigger value="lan" title={t('interconnect.lan')}><IconLabel icon={Network}>{t('interconnect.lan')}</IconLabel></TabsTrigger><TabsTrigger value="share" title={t('interconnect.persistent')}>{t('interconnect.persistent')}</TabsTrigger><TabsTrigger value="transfer" title={t('interconnect.transfer')}><IconLabel icon={Send}>{t('interconnect.transfer')}</IconLabel></TabsTrigger><TabsTrigger value="store" title={t('interconnect.store')}><IconLabel icon={Cloud}>{t('interconnect.store')}</IconLabel></TabsTrigger><TabsTrigger value="tunnel" title={t('interconnect.tunnel')}>{t('interconnect.tunnel')}</TabsTrigger><TabsTrigger value="general" title={t('interconnect.general')}>{t('interconnect.general')}</TabsTrigger></TabsList>
  <TabsContent value="general" keepMounted><InterconnectGeneralPanel active={tab==='general'} onActivity={onActivity}/></TabsContent>
  <TabsContent value="share" keepMounted><PeerPanel kind="share" active={tab==='share'} onActivity={onActivity} incomingCode={shareCode} onCodeConsumed={()=>setShareCode('')}/></TabsContent>
  <TabsContent value="tunnel" keepMounted><PeerPanel kind="tunnel" active={tab==='tunnel'} onActivity={onActivity}/></TabsContent>
  <TabsContent value="transfer" keepMounted><TransferPanel stored={false} active={tab==='transfer'} onActivity={onActivity} intent={intent??clipboardIntent} onIntentConsumed={()=>{setClipboardIntent(undefined);onIntentConsumed?.();}}/></TabsContent>
  <TabsContent value="store" keepMounted><TransferPanel stored active={tab==='store'} onActivity={onActivity} intent={storeCode} onIntentConsumed={()=>setStoreCode(undefined)}/></TabsContent>
  <TabsContent value="lan" keepMounted className="settings-stack">
   {error&&<Notice actions={<Button onClick={()=>void refresh().then(()=>setError('')).catch(e=>setError(String(e)))}>{t('interconnect.retry')}</Button>}>{error}</Notice>}
   {notice&&<p role="status" className="text-sm text-muted-foreground">{notice}</p>}
   {!state||!preferences?<p>{t('interconnect.loading')}</p>:<>
    <SettingsSection title={t(state.running?'interconnect.running':'interconnect.stopped')} icon={Network} description={t('interconnect.hint')}>
     <div className="flex flex-wrap items-center gap-2">
      <Button icon={state.running?Square:Play} variant={state.running?'secondary':'primary'} busy={busy} onClick={()=>void action(async()=>{await interconnectCall(state.running?'stop':'start',state.running?null:preferences);await refresh(true);})}>{t(state.running?'interconnect.stop':'interconnect.start')}</Button>
      {state.running&&<><Button icon={Copy} onClick={()=>void copy(url)}>{t('interconnect.link')}</Button><Button icon={QrCode} onClick={()=>setQr(v=>!v)}>{t(qr?'interconnect.hideQR':'interconnect.qr')}</Button></>}
     </div>
     {state.running?<>
      {addresses.length>1&&<div className="max-w-lg"><Label htmlFor="interconnect-address">{t('interconnect.chooseAddress')}</Label><Select value={selected?.ip??''} onValueChange={value=>setSelectedIP(value??'')}><SelectTrigger id="interconnect-address" className="w-full"><SelectValue>{selected?.name} {selected?.ip}</SelectValue></SelectTrigger><SelectContent>{addresses.map(a=><SelectItem key={a.ip} value={a.ip}>{a.name} {a.ip}</SelectItem>)}</SelectContent></Select></div>}
      {qr&&<img className="h-auto w-44 max-w-full" alt={t('interconnect.qr')} src={'data:image/svg+xml;charset=utf-8,'+encodeURIComponent(selected?.qr??state.snapshot?.info.qr??'')}/>}
      <p className="settings-help">{t('interconnect.accessHint')}</p>
     </>:<div className="grid min-w-0 gap-4 sm:grid-cols-2">
      <div className="min-w-0 space-y-2"><Label htmlFor="interconnect-bind">{t('interconnect.network')}</Label><Select value={preferences.bind} disabled={busy} onValueChange={bind=>setPreferences({...preferences,bind:bind??'0.0.0.0'})}><SelectTrigger id="interconnect-bind" className="w-full"><SelectValue>{options.find(a=>a.ip===preferences.bind)?.name??preferences.bind}</SelectValue></SelectTrigger><SelectContent>{options.map(a=><SelectItem key={a.ip} value={a.ip}>{a.name} {a.ip!=='0.0.0.0'&&a.ip!=='127.0.0.1'?a.ip:''}</SelectItem>)}</SelectContent></Select></div>
      <div className="space-y-2"><Label htmlFor="interconnect-port">{t('interconnect.port')}</Label><Input id="interconnect-port" type="number" min={0} max={65535} disabled={busy} value={preferences.port} onChange={e=>setPreferences({...preferences,port:Number(e.target.value)})}/></div>
      <div className="min-w-0 space-y-2 sm:col-span-2"><Label htmlFor="interconnect-directory">{t('interconnect.directory')}</Label><div className="flex min-w-0 flex-wrap gap-2"><Input id="interconnect-directory" className="min-w-0 flex-1" value={preferences.downloadDir} disabled={busy} onChange={e=>setPreferences({...preferences,downloadDir:e.target.value})}/><Button icon={FolderOpen} disabled={busy} onClick={()=>void action(async()=>{const path=await pickDirectory();if(path)setPreferences({...preferences,downloadDir:path});})}>{t('interconnect.choose')}</Button></div></div>
     </div>}
    </SettingsSection>
    {state.running&&<SettingsSection title={t('interconnect.items')} className="interconnect-wide" icon={FileText} description={t('interconnect.requestLimit')}>
     <PathDropArea kind="file" multiple disabled={busy||tab!=='lan'} onError={setError} onPaths={paths=>void action(()=>interconnectCall('add_files',paths))}>
     <div className="flex flex-wrap gap-2"><Button icon={FilePlus} disabled={busy} onClick={()=>void action(async()=>{const files=await pickFiles();if(files.length)await interconnectCall('add_files',files);})}>{t('interconnect.files')}</Button><Button icon={ClipboardPaste} disabled={busy} onClick={()=>void action(async()=>{const files=await readClipboardFiles();if(files.length)await interconnectCall('add_files',files);})}>{t('interconnect.paste')}</Button><Button icon={RefreshCw} disabled={busy} onClick={()=>void action(()=>refresh())}>{t('interconnect.refresh')}</Button></div>
     <SettingsDisclosure title={t('interconnect.text')}><div className="space-y-3"><Textarea aria-label={t('interconnect.text')} placeholder={t('interconnect.textPlaceholder')} value={text} disabled={busy} onChange={e=>setText(e.target.value)}/><Button disabled={busy||!text.trim()} onClick={()=>void action(async()=>{await interconnectCall('add_text',text);setText('');})}>{t('interconnect.publish')}</Button></div></SettingsDisclosure>
     <div className="min-w-0 divide-y divide-border">
      {!state.snapshot?.items.length&&<p className="py-12 text-center text-muted-foreground">{t('interconnect.empty')}</p>}
      {state.snapshot?.items.map(item=>{
       const progress=state.snapshot?.downloads.find(d=>d.itemId===item.id);
       return <div key={item.id} className="flex min-w-0 flex-wrap items-start gap-3 py-4"><div className="min-w-0 flex-[1_1_240px]">{!item.content&&<p className="break-all font-medium">{item.title}</p>}{item.content?<p className="mt-1 whitespace-pre-wrap break-all text-sm text-muted-foreground">{item.content}</p>:<p className="text-sm text-muted-foreground">{t('interconnect.bytes',{count:item.size})}{!item.exists?' · '+t('interconnect.missing'):''}{progress?.activeCount?' · '+t('interconnect.receivers',{count:progress.activeCount})+' · '+Math.round(progress.speedBps/1024)+' KiB/s':''}</p>}</div><div className="flex max-w-full flex-wrap gap-1">
        {item.content?<Button variant="ghost" icon={Copy} onClick={()=>void copy(item.content!)}>{t('interconnect.copy')}</Button>:<><Button variant="ghost" icon={Copy} disabled={!item.exists} onClick={()=>{const link=new URL(url);link.pathname='/api/items/'+item.id+'/download';void copy(link.toString());}}>{t('interconnect.downloadLink')}</Button><Button variant="ghost" icon={Download} disabled={busy||!item.exists} onClick={()=>void action(async()=>{const name=await interconnectCall<string>('download_name',item.id);setSave({id:item.id,path:preferences.downloadDir+'\\'+name,replace:false});})}>{t('interconnect.save')}</Button></>}
        <Button variant="ghost" icon={Trash2} disabled={busy} onClick={()=>void action(()=>interconnectCall('remove',item.id))}>{t('interconnect.remove')}</Button>
       </div></div>;
      })}
     </div>
     </PathDropArea>
    </SettingsSection>}
   </>}
   <Dialog open={!!save} onOpenChange={open=>{if(!open&&!busy)setSave(undefined);}}>
    <DialogContent><DialogHeader><DialogTitle>{t('interconnect.save')}</DialogTitle></DialogHeader>
     {save&&<div className="space-y-4">
      <Label htmlFor="interconnect-save-path">{t('interconnect.savePath')}</Label><Input id="interconnect-save-path" value={save.path} disabled={busy} onChange={e=>setSave({...save,path:e.target.value})}/>
      <Button icon={FolderOpen} disabled={busy} onClick={()=>void action(async()=>{const folder=await pickDirectory();if(folder){const name=save.path.split(/[\\/]/).pop();setSave({...save,path:folder+'\\'+name});}})}>{t('interconnect.choose')}</Button>
      <label className="flex items-center gap-2"><Checkbox checked={save.replace} disabled={busy} onCheckedChange={replace=>setSave({...save,replace:replace===true})}/>{t('interconnect.replaceExisting')}</label>
      {error&&<Notice>{error}</Notice>}
     </div>}
     <DialogFooter><Button disabled={busy} onClick={()=>setSave(undefined)}>{t('common.close')}</Button><Button variant="primary" busy={busy} disabled={!save?.path} onClick={()=>void action(async()=>{await interconnectCall('save_as',save);setSave(undefined);})}>{t('interconnect.save')}</Button></DialogFooter>
    </DialogContent>
   </Dialog>
  </TabsContent>
 </Tabs>;
}
