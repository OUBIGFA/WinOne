import {useCallback,useEffect,useRef,useState,type ReactNode} from 'react';
import {Copy,FolderPlus,FilePlus,Play,Square,QrCode,Trash2,Network,FolderOpen,Download} from 'lucide-react';
import {Dialog,DialogContent,DialogHeader,DialogTitle,DialogFooter} from '../components/ui/dialog';
import {Checkbox} from '../components/ui/checkbox';
import {Textarea} from '../components/ui/textarea';
import {interconnectCall,pickFiles,pickDirectory} from '../bridge';
import {useTranslation} from '../i18n';
import {Button} from '../ui/button';
import {Notice} from '../ui/notice';
import {PathDropArea} from '../ui/path-drop-zone';
import {usePageActivity,type PageActivityChanged} from '../ui/page-navigation';
import {SettingsSection,SettingsDisclosure,SettingsOptions,SettingsToggle} from '../components/settings-layout';
import {Input} from '../components/ui/input';
import {Label} from '../components/ui/label';
import type {CodeTransferState} from './interconnect-transfer';
import {Select,SelectContent,SelectItem,SelectTrigger,SelectValue} from '../components/ui/select';
import {Tabs,TabsList,TabsTrigger,TabsContent} from '../components/ui/tabs';

export type PeerRecord={id:string;spec:{mode:string;paths?:string[];target?:string;listen?:string;code?:string};state:{transfer?:CodeTransferState;status:string;ready?:boolean;relayReady?:boolean;relayError?:string;code?:string;qr?:string;address?:string;error?:string;persistenceError?:string;cleanupError?:string;detail?:string;updating?:boolean;sent?:number;received?:number;active?:number;peers?:{direct:boolean;relay?:string;sent:number;received:number}[];share?:{info?:{url:string;qr:string;addresses:{name?:string;ip:string;url:string;qr:string}[]};items:{id:string;title:string;content?:string;size:number;exists:boolean}[];downloads?:{itemId:string;speedBps:number;activeCount:number}[]}}};
const initial=()=>({paths:[] as string[],code:'',target:'127.0.0.1:8080',listen:'127.0.0.1:0',remotePort:1,derpMapURL:'',allowWrites:false});

// Both panels operate on the same supervised tasks; changing tabs does not
// interrupt a published share, a mapped port, or a file preparation operation.
export function PeerPanel({kind,active,onActivity,incomingCode,onCodeConsumed}:{kind:'share'|'tunnel';active:boolean;onActivity:PageActivityChanged;incomingCode?:string;onCodeConsumed?:()=>void}){
 const {t}=useTranslation(),tr=(key:string)=>t('interconnect.'+key);
 const [form,setForm]=useState(initial),[saved,setSaved]=useState(()=>JSON.stringify(initial())),[direction,setDirection]=useState('server');
 useEffect(()=>{if(incomingCode){setDirection('client');setForm(value=>({...value,code:incomingCode}));onCodeConsumed?.();}},[incomingCode]);
 const [records,setRecords]=useState<PeerRecord[]>([]),[busy,setBusy]=useState(false),[error,setError]=useState(''),[message,setMessage]=useState(''),[qr,setQr]=useState(''),[browser,setBrowser]=useState('');
 const mounted=useRef(true),sequence=useRef(0);
 const [selectedAddresses,setSelectedAddresses]=useState<Record<string,string>>({});
 useEffect(()=>{mounted.current=true;return()=>{mounted.current=false;sequence.current++;};},[]);
 const refresh=useCallback(async()=>{const version=++sequence.current,items=await interconnectCall<PeerRecord[]>('transfer_list');if(mounted.current&&version===sequence.current)setRecords(items);},[]);
 useEffect(()=>{if(!active)return;let alive=true;const update=()=>{if(alive&&document.visibilityState==='visible')void refresh().catch(e=>{if(alive)setError(String(e));});};update();const timer=setInterval(update,1000);return()=>{alive=false;clearInterval(timer);};},[active,refresh]);
 usePageActivity(onActivity,JSON.stringify(form)!==saved,busy,'interconnect-'+kind,()=>setForm(JSON.parse(saved)));
 async function action(work:()=>Promise<unknown>){setBusy(true);setError('');try{await work();await refresh();}catch(e){setError(String(e));}finally{setBusy(false);}}
 const add=(paths:string[])=>setForm(f=>({...f,paths:[...new Set([...f.paths,...paths])]}));
 const field=(key:'code'|'target'|'listen'|'derpMapURL',label:string)=><div className="space-y-2"><Label>{tr(label)}</Label><Input aria-label={tr(label)} value={form[key]} disabled={busy} onChange={e=>setForm({...form,[key]:e.target.value})}/></div>;
 async function copy(value:string){await navigator.clipboard.writeText(value);setMessage(tr('copied'));}
 return <div className="settings-stack">
  {error&&<Notice>{error}</Notice>}{message&&<p role="status" className="settings-help">{message}</p>}
  <SettingsSection title={tr(kind==='share'?'persistent':'tunnel')} icon={kind==='share'?FolderOpen:Network} description={tr(kind==='share'?'persistentHint':'tunnelHint')} className="interconnect-wide">
   <SettingsOptions label={tr('direction')} value={direction} onChange={setDirection} items={[{value:'server',label:tr(kind==='share'?'publishShare':'exposePort')},{value:'client',label:tr(kind==='share'?'receiveShare':'mapPort')}]}/>
   {direction==='server'?(kind==='share'?<PathDropArea kind="any" multiple disabled={busy||!active} onPaths={add} onError={setError} className="interconnect-drop">
    <p className="settings-help">{tr('drop')}</p><div className="flex flex-wrap gap-2"><Button icon={FilePlus} onClick={()=>void action(async()=>add(await pickFiles()))}>{tr('chooseFiles')}</Button><Button icon={FolderPlus} onClick={()=>void action(async()=>{const path=await pickDirectory();if(path)add([path]);})}>{tr('chooseFolder')}</Button><Button variant="ghost" onClick={()=>setForm({...form,paths:[]})}>{tr('clear')}</Button></div>
    <ul className="space-y-2">{form.paths.map(path=><li key={path} className="flex min-w-0 gap-2"><span className="min-w-0 flex-1 break-all">{path}</span><Button variant="ghost" icon={Trash2} aria-label={tr('remove')+' '+path} onClick={()=>setForm({...form,paths:form.paths.filter(p=>p!==path)})}/></li>)}</ul>
    <p className="settings-help">{tr('cacheHint')}</p>
   </PathDropArea>:field('target','targetPort')):field('code','tailcatCode')}
   <SettingsDisclosure title={tr('networkOptions')}><div className="interconnect-fields">
    {field('derpMapURL','derpMap')}
    {direction==='client'&&field('listen','listenPort')}
    {kind==='tunnel'&&<div className="space-y-2"><Label>{tr('remotePort')}</Label><Input aria-label={tr('remotePort')} type="number" min={1} max={65535} value={form.remotePort} onChange={e=>setForm({...form,remotePort:Number(e.target.value)})}/></div>}
   </div>{kind==='share'&&direction==='server'&&<SettingsToggle label={tr('allowWrites')} checked={form.allowWrites} onChange={allowWrites=>setForm({...form,allowWrites})}/>}</SettingsDisclosure>
   <div><Button variant="primary" icon={Play} busy={busy} disabled={direction==='client'?!form.code.trim():kind==='share'&&!form.paths.length} onClick={()=>void action(async()=>{await interconnectCall('transfer_start',{mode:kind+'_'+direction,...form});setSaved(JSON.stringify(form));})}>{tr(direction==='server'?'startService':'connectService')}</Button></div>
  </SettingsSection>
  <SettingsSection title={tr('services')} icon={Network} className="interconnect-wide">
   {records.filter(r=>r.spec.mode.startsWith(kind+'_')).length===0&&<p className="settings-help py-4">{tr('noServices')}</p>}
   <div className="divide-y divide-border">{records.filter(r=>r.spec.mode.startsWith(kind+'_')).map(item=>{
    return <PeerTask key={item.id} item={item} kind={kind} busy={busy} action={action} copy={copy}/>;
   })}</div>
  </SettingsSection>
 </div>;
}

// A file send and a port mapping use the same task controls. Embedded code
// progress stays inside this record rather than creating another send history.
export function PeerTask({item,kind,busy,action,copy,children}:{item:PeerRecord;kind:'share'|'tunnel';busy:boolean;action:(work:()=>Promise<unknown>)=>Promise<void>;copy:(value:string)=>Promise<void>;children?:ReactNode}){
 const {t}=useTranslation(),tr=(key:string)=>t('interconnect.'+key);
 const [qr,setQr]=useState(''),[browser,setBrowser]=useState(''),[text,setText]=useState(''),[save,setSave]=useState<{itemId:string;path:string;replace:boolean}>();
 const [selectedAddresses,setSelectedAddresses]=useState<Record<string,string>>({});
    const s=item.state,server=item.spec.mode.endsWith('server'),published=kind==='share'&&server;
    const info=s.share?.info,addresses=info?.addresses??[],selected=addresses.find(address=>address.ip===selectedAddresses[item.id])??addresses[0];
    const phoneURL=selected?.url??info?.url,phoneQR=selected?.qr??info?.qr;
    return <article id={'interconnect-task-'+item.id} className="interconnect-task">
     <div className="flex flex-wrap justify-between gap-2"><p className="font-medium break-all">{(s.share?.items.map(file=>file.title)??item.spec.paths?.map(p=>p.split(/[\\/]/).pop()))?.join(' · ')||item.spec.target||tr(server?'publishShare':'receiveShare')}</p><span className="settings-help">{tr(s.status==='running'?(s.ready?'serviceReady':'servicePreparing'):s.status==='failed'?'failedTask':'serviceStopped')}</span></div>
     {/* Keep route selection local to this task, so polling never resets it.
         Mounted panels preserve confirmation drafts; inactive routes stay hidden. */}
     {published&&s.ready&&<Tabs navigation={false} defaultValue={children?'internet':'lan'} className="interconnect-receive-tabs">
      <TabsList aria-label={tr('receiveMethod')} className="interconnect-receive-tablist">
       {children&&<TabsTrigger value="internet">{tr('scanToReceive')}</TabsTrigger>}
       <TabsTrigger value="lan">{tr('lanDirect')}</TabsTrigger>
      </TabsList>
      {children&&<TabsContent value="internet" keepMounted>{children}</TabsContent>}
      <TabsContent value="lan" keepMounted>
      <section className="interconnect-receive-route" aria-label={tr('lanReceive')}>
       <h3>{tr('lanReceive')}</h3><p className="settings-help">{tr('lanReceiveHint')}</p>
       {phoneQR&&<img src={'data:image/svg+xml;charset=utf-8,'+encodeURIComponent(phoneQR)} className="interconnect-qr" alt={tr('lanQR')}/>}
       {phoneURL&&<Button icon={Copy} onClick={()=>void action(()=>copy(phoneURL))}>{tr('copyPhoneLink')}</Button>}
       {addresses.length>1&&<Select value={selected?.ip??''} onValueChange={ip=>setSelectedAddresses(current=>({...current,[item.id]:ip??''}))}><SelectTrigger aria-label={tr('chooseAddress')} className="w-full"><SelectValue>{selected?.name} {selected?.ip}</SelectValue></SelectTrigger><SelectContent>{addresses.map(address=><SelectItem key={address.ip} value={address.ip}>{address.name} {address.ip}</SelectItem>)}</SelectContent></Select>}
       <p className="settings-help">{tr('accessHint')}</p>
      </section>
      </TabsContent>
     </Tabs>}
     {published&&<SettingsDisclosure title={tr('winoneLiveShare')}>
      <p className="settings-help">{tr('nativeShareSteps')}</p>
      <p className="settings-help">{tr(s.status!=='running'?'serviceStopped':s.relayReady?'serviceReady':s.relayError?'crossNetworkFailed':'crossNetworkConnecting')}</p>
      {s.code&&<><p className="select-all break-all text-sm">{s.code}</p><Button icon={Copy} onClick={()=>void action(()=>copy(s.code!))}>{tr('copyNativeAddress')}</Button></>}
      {s.relayError&&s.status==='running'&&<Notice>{tr('relayFailureHint')}<details><summary>{tr('errorDetails')}</summary><p className="break-all">{s.relayError}</p></details></Notice>}
     </SettingsDisclosure>}
     {!published&&s.code&&<p className="select-all break-all text-sm">{s.code}</p>}{s.address&&<p className="select-all break-all">{tr('localAddress')} {s.address}</p>}
     {s.error&&<Notice>{s.error}</Notice>}{s.persistenceError&&<Notice>{s.persistenceError}</Notice>}{s.cleanupError&&<Notice>{s.cleanupError}</Notice>}{s.detail&&<p className="settings-help break-all">{s.detail}</p>}
     {kind==='tunnel'&&s.ready&&<p className="settings-help">{tr('activeConnections')} {s.active??0} · ↑ {s.sent??0} B · ↓ {s.received??0} B</p>}
     {s.peers?.map((p,i)=><p key={i} className="settings-help">{tr(p.direct?'directPath':'relayPath')} · ↑ {p.sent} B · ↓ {p.received} B</p>)}
     {s.share&&<SettingsDisclosure title={tr('manageShare')+' · '+s.share.items.length}>
     {published&&s.ready&&<div className="space-y-3"><Textarea aria-label={tr('text')} placeholder={tr('textPlaceholder')} value={text} disabled={busy||s.updating} onChange={e=>setText(e.target.value)}/><Button disabled={busy||s.updating||!text.trim()} onClick={()=>void action(async()=>{await interconnectCall('share_add_text',{id:item.id,text});setText('');})}>{tr('publish')}</Button></div>}
     <ul className="space-y-2">{s.share.items.map(file=>{const progress=s.share?.downloads?.find(download=>download.itemId===file.id);const body=file.content?<span className="min-w-0 flex-1 break-all whitespace-pre-wrap">{file.content}</span>:<span className="min-w-0 flex-1 break-all">{file.title} ({file.size} B{!file.exists?' '+tr('missing'):''}{progress?.activeCount?' '+t('interconnect.receivers',{count:progress.activeCount}):''})</span>;const actions=file.content?<Button variant="ghost" icon={Copy} disabled={busy} onClick={()=>void action(()=>copy(file.content!))}>{tr('copy')}</Button>:(published&&s.ready&&phoneURL&&<Button variant="ghost" icon={Copy} disabled={busy||!file.exists} onClick={()=>{const link=new URL(phoneURL);link.pathname='/api/items/'+file.id+'/download';void action(()=>copy(link.toString()));}}>{tr('downloadLink')}</Button>);const saveButton=<Button variant="ghost" icon={Download} disabled={busy||!file.exists} onClick={()=>void action(async()=>{const general=await interconnectCall<{downloadDir:string}>('general.get');setSave({itemId:file.id,path:(general.downloadDir?general.downloadDir+'\\':'')+file.title,replace:false});})}>{tr('save')}</Button>;return <li key={file.id} className="flex min-w-0 flex-wrap items-center gap-2">{body}{actions}{!file.content&&saveButton}<Button variant="ghost" icon={Trash2} disabled={busy||s.updating} onClick={()=>void action(()=>interconnectCall('share_remove',{id:item.id,itemId:file.id}))}>{tr('remove')}</Button></li>;})}</ul>
     {s.ready&&published&&<div className="flex flex-wrap gap-2"><Button icon={FilePlus} disabled={busy||s.updating} onClick={()=>void action(async()=>{const paths=await pickFiles();if(paths.length)await interconnectCall('share_add',{id:item.id,paths});})}>{tr('files')}</Button><Button icon={FolderPlus} disabled={busy||s.updating} onClick={()=>void action(async()=>{const path=await pickDirectory();if(path)await interconnectCall('share_add',{id:item.id,paths:[path]});})}>{tr('chooseFolder')}</Button></div>}
     </SettingsDisclosure>}
     <div className="flex flex-wrap gap-2">
      {!published&&s.code&&<Button icon={Copy} onClick={()=>void action(()=>copy(s.code!))}>{tr('copyAddress')}</Button>}
      {!published&&s.code&&s.qr&&<Button icon={QrCode} onClick={()=>setQr(qr===item.id?'':item.id)}>{tr(qr===item.id?'hideQR':'qr')}</Button>}
      {s.ready&&kind==='share'&&!server&&<><Button icon={FolderOpen} onClick={()=>setBrowser(browser===item.id?'':item.id)}>{tr('browseShare')}</Button><Button icon={Copy} onClick={()=>void action(()=>copy('http://'+s.address))}>{tr('link')}</Button></>}
      {s.status==='running'?<Button icon={Square} disabled={busy} onClick={()=>void action(()=>interconnectCall('transfer_stop',{id:item.id}))}>{tr('stopService')}</Button>:<><Button icon={Play} disabled={busy} onClick={()=>void action(()=>interconnectCall('transfer_restart',{id:item.id}))}>{tr('resumeService')}</Button><Button variant="ghost" icon={Trash2} disabled={busy} onClick={()=>void action(()=>interconnectCall('transfer_remove',{id:item.id}))}>{tr('forget')}</Button></>}
     </div>
     {!published&&qr===item.id&&s.qr&&<img src={s.qr} className="interconnect-qr" alt={tr('qr')}/>}
     {browser===item.id&&s.ready&&s.address&&<iframe title={tr('browseShare')} src={'http://'+s.address} sandbox="allow-scripts allow-same-origin allow-forms allow-downloads" className="h-[480px] w-full rounded-md border border-border"/>}
     <Dialog open={!!save} onOpenChange={open=>{if(!open&&!busy)setSave(undefined);}}>
      <DialogContent><DialogHeader><DialogTitle>{tr('save')}</DialogTitle></DialogHeader>
       {save&&<div className="space-y-4">
        <Label htmlFor={'save-'+save.itemId}>{tr('savePath')}</Label><Input id={'save-'+save.itemId} value={save.path} disabled={busy} onChange={e=>setSave({...save,path:e.target.value})}/>
        <Button icon={FolderOpen} disabled={busy} onClick={()=>void action(async()=>{const folder=await pickDirectory();if(folder){const name=save.path.split(/[\\/]/).pop();setSave({...save,path:folder+'\\'+name});}})}>{tr('choose')}</Button>
        <label className="flex items-center gap-2"><Checkbox checked={save.replace} disabled={busy} onCheckedChange={replace=>setSave({...save,replace:replace===true})}/>{tr('replaceExisting')}</label>
       </div>}
       <DialogFooter><Button disabled={busy} onClick={()=>setSave(undefined)}>{t('common.close')}</Button><Button variant="primary" busy={busy} disabled={!save?.path} onClick={()=>void action(async()=>{await interconnectCall('share_save_as',{id:item.id,itemId:save!.itemId,path:save!.path,replace:save!.replace});setSave(undefined);})}>{tr('save')}</Button></DialogFooter>
      </DialogContent>
     </Dialog>
    </article>;
}
