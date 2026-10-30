import {useCallback,useEffect,useRef,useState} from 'react';
import {FolderOpen,Network,Send,Cloud,Save} from 'lucide-react';
import {interconnectCall,pickDirectory} from '../bridge';
import {useTranslation} from '../i18n';
import {Button} from '../ui/button';
import {Notice} from '../ui/notice';
import {usePageActivity,type PageActivityChanged} from '../ui/page-navigation';
import {SettingsSection} from '../components/settings-layout';
import {SettingsSaveBar} from '../components/settings-save-bar';
import {Input} from '../components/ui/input';
import {Label} from '../components/ui/label';
import {Select,SelectContent,SelectItem,SelectTrigger,SelectValue} from '../components/ui/select';
import {Tabs,TabsList,TabsTrigger,TabsContent} from '../components/ui/tabs';
import {IconLabel} from '../ui/icon-label';
import './interconnect.css';
import {TransferPanel} from './interconnect-transfer';
import {PeerPanel} from './interconnect-peer';
import {InterconnectGeneralPanel,type InterconnectGeneral} from './interconnect-general';
import type {InterconnectIntent} from './interconnect-intents';

type Preferences={bind:string;port:number;downloadDir:string};
type Address={name?:string;ip:string;url?:string;qr?:string};
type State={running:boolean;settings:Preferences;addresses:Address[];snapshot:null|{info:{url:string;qr:string;addresses:Address[]};items:unknown[];downloads:{itemId:string;speedBps:number;activeCount:number}[]}};

// Compose existing WinOne controls. File bytes stay inside the upstream service.
export function InterconnectPage({onActivity,intent,onIntentConsumed}:{onActivity:PageActivityChanged;intent?:InterconnectIntent;onIntentConsumed?:()=>void}){
 const {t}=useTranslation();
 const [state,setState]=useState<State>(),[preferences,setPreferences]=useState<Preferences>();
 const [general,setGeneral]=useState<InterconnectGeneral>(),[generalDraft,setGeneralDraft]=useState<InterconnectGeneral>();
 const [busy,setBusy]=useState(false),[error,setError]=useState(''),[notice,setNotice]=useState('');
 const [tab,setTab]=useState('files');
 const [clipboard,setClipboard]=useState<{kind:'share'|'transfer'|'store';code:string}>(),[clipboardIntent,setClipboardIntent]=useState<InterconnectIntent>(),[storeCode,setStoreCode]=useState<InterconnectIntent>();
 useEffect(()=>{let live=true;const read=async()=>{try{const settings=await interconnectCall<InterconnectGeneral>('general.get');if(!live||!settings?.clipboard)return;const code=(await navigator.clipboard.readText()).trim();if(!live||code.length>4096)return;const kind=code.startsWith('tc')&&code.length>40?'share':code.startsWith('croc-store-v1.')||code.startsWith('https://getcroc.com/s/')||code.startsWith('https://gofile.io/d/')?'store':/^[0-9]+-[a-z0-9-]+$/i.test(code)?'transfer':undefined;if(kind)setClipboard({kind,code});}catch(e){if(live)setError(String(e));}};window.addEventListener('focus',read);void read();return()=>{live=false;window.removeEventListener('focus',read);};},[]);
 useEffect(()=>{if(intent)setTab('files');},[intent]);
 const alive=useRef(true),version=useRef(0);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;version.current++;};},[]);
 const refresh=useCallback(async(reset=false)=>{
  const result=await interconnectCall<State>('get');
  if(!alive.current)return;
  setState(result);setPreferences(current=>reset||!current?result.settings:current);
 },[]);
 const loadGeneral=useCallback(async()=>{const value=await interconnectCall<InterconnectGeneral>('general.get');if(alive.current){setGeneral(value);setGeneralDraft(current=>current??value);}},[]);
 useEffect(()=>{void Promise.all([refresh(),loadGeneral()]).catch(e=>setError(String(e)));},[refresh,loadGeneral]);
 const preferencesDirty=!!(preferences&&state&&JSON.stringify(preferences)!==JSON.stringify(state.settings));
 const generalDirty=!!(generalDraft&&general&&JSON.stringify(generalDraft)!==JSON.stringify(general));
 const settingsDirty=preferencesDirty||generalDirty;
 const dirty=settingsDirty;
 // Sharing survives navigation; only unfinished input and current actions need protection.
 const undoSettings=()=>{setPreferences(state?.settings);setGeneralDraft(general);setError('');setNotice('');};
 usePageActivity(onActivity,dirty,busy,'interconnect',undoSettings);
 async function action(work:()=>Promise<unknown>){
  setBusy(true);setError('');setNotice('');version.current++;
  try{await work();await refresh();}catch(e){if(alive.current)setError(String(e));}finally{if(alive.current)setBusy(false);}
 }
 async function copy(value:string){try{await navigator.clipboard.writeText(value);setNotice(t('interconnect.copied'));}catch(e){setError(String(e));}}
 async function saveSettings(){
  // Acknowledge each successful write separately. If a later write fails,
  // its draft remains dirty and retry does not repeat completed work.
  if(preferencesDirty){const saved=await interconnectCall<Preferences>('preferences.save',preferences);setState(current=>current?{...current,settings:saved}:current);setPreferences(saved);}
  if(generalDirty){const saved=await interconnectCall<InterconnectGeneral>('general.save',generalDraft);setGeneral(saved);setGeneralDraft(saved);}
  setNotice(t('common.saved'));
 }
 const options=[{ip:'0.0.0.0',name:t('interconnect.all')},{ip:'127.0.0.1',name:t('interconnect.local')},...(state?.addresses??[]).filter(a=>a.ip!=='127.0.0.1')];
 return <Tabs value={tab} onValueChange={value=>setTab(String(value))}>
  {clipboard&&<Notice actions={<Button onClick={()=>{if(clipboard.kind==='store')setStoreCode({requested:true,paths:[],receive:true,code:clipboard.code});else setClipboardIntent({requested:true,paths:[],receive:true,code:clipboard.code});setTab(clipboard.kind==='store'?'store':'files');setClipboard(undefined);}}>{t('interconnect.useClipboard')}</Button>}>{t('interconnect.clipboardFound')}</Notice>}
  <TabsList aria-label={t('interconnect.title')}><TabsTrigger value="files" title={t('interconnect.fileTransfer')}><IconLabel icon={Send}>{t('interconnect.fileTransfer')}</IconLabel></TabsTrigger><TabsTrigger value="store" title={t('interconnect.store')}><IconLabel icon={Cloud}>{t('interconnect.store')}</IconLabel></TabsTrigger><TabsTrigger value="tunnel" title={t('interconnect.tunnel')}>{t('interconnect.tunnel')}</TabsTrigger><TabsTrigger value="general" title={t('interconnect.general')}>{t('interconnect.general')}</TabsTrigger></TabsList>
  {error&&<Notice actions={<Button disabled={busy} onClick={()=>void Promise.all([refresh(),loadGeneral()]).then(()=>setError('')).catch(e=>setError(String(e)))}>{t('interconnect.retry')}</Button>}>{error}</Notice>}
  {notice&&<p role="status" className="text-sm text-muted-foreground">{notice}</p>}
  <TabsContent value="files" keepMounted><TransferPanel stored={false} active={tab==='files'} settingsPending={settingsDirty} onActivity={onActivity} intent={intent??clipboardIntent} onIntentConsumed={()=>{setClipboardIntent(undefined);onIntentConsumed?.();}}/></TabsContent>
  <TabsContent value="store" keepMounted><TransferPanel stored active={tab==='store'} onActivity={onActivity} intent={storeCode} onIntentConsumed={()=>setStoreCode(undefined)}/></TabsContent>
  <TabsContent value="tunnel" keepMounted><PeerPanel kind="tunnel" active={tab==='tunnel'} onActivity={onActivity}/></TabsContent>
  <TabsContent value="general" keepMounted className="settings-stack">
   <SettingsSection title={t('interconnect.configuration')} icon={Network} description={t('interconnect.settingsForNextSend')}>
    {preferences?<div className="grid min-w-0 gap-4 sm:grid-cols-2">
      <div className="min-w-0 space-y-2"><Label htmlFor="interconnect-bind">{t('interconnect.network')}</Label><Select value={preferences.bind} disabled={busy} onValueChange={bind=>setPreferences({...preferences,bind:bind??'0.0.0.0'})}><SelectTrigger id="interconnect-bind" className="w-full"><SelectValue>{options.find(a=>a.ip===preferences.bind)?.name??preferences.bind}</SelectValue></SelectTrigger><SelectContent>{options.map(a=><SelectItem key={a.ip} value={a.ip}>{a.name} {a.ip!=='0.0.0.0'&&a.ip!=='127.0.0.1'?a.ip:''}</SelectItem>)}</SelectContent></Select></div>
      <div className="space-y-2"><Label htmlFor="interconnect-port">{t('interconnect.port')}</Label><Input id="interconnect-port" type="number" min={0} max={65535} disabled={busy} value={preferences.port} onChange={e=>setPreferences({...preferences,port:Number(e.target.value)})}/></div>
      <div className="min-w-0 space-y-2 sm:col-span-2"><Label htmlFor="interconnect-directory">{t('interconnect.directory')}</Label><div className="flex min-w-0 flex-wrap gap-2"><Input id="interconnect-directory" className="min-w-0 flex-1" value={preferences.downloadDir} disabled={busy} onChange={e=>setPreferences({...preferences,downloadDir:e.target.value})}/><Button icon={FolderOpen} disabled={busy} onClick={()=>void action(async()=>{const path=await pickDirectory();if(path)setPreferences({...preferences,downloadDir:path});})}>{t('interconnect.choose')}</Button></div></div>
     </div>:<p>{t('interconnect.loading')}</p>}
   </SettingsSection>
   <InterconnectGeneralPanel active={tab==='general'} onActivity={onActivity} general={generalDraft} onChange={setGeneralDraft} disabled={busy}/>
  </TabsContent>
  <SettingsSaveBar disabled={busy} undo={{disabled:!settingsDirty,onClick:undoSettings}}>
   <Button icon={Save} variant="primary" disabled={!settingsDirty||!preferences||!generalDraft||!Number.isInteger(preferences.port)||preferences.port<0||preferences.port>65535} onClick={()=>void action(saveSettings)}>{t('interconnect.saveSettings')}</Button>
  </SettingsSaveBar>
 </Tabs>;
}
