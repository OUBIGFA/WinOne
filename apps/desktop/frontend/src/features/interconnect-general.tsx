import {useEffect,useState} from 'react';
import {FolderOpen,Settings} from 'lucide-react';
import {interconnectCall,pickDirectory} from '../bridge';
import {useTranslation} from '../i18n';
import {SettingsSection,SettingsToggle} from '../components/settings-layout';
import {Button} from '../ui/button';
import {Notice} from '../ui/notice';
import {Input} from '../components/ui/input';
import {usePageActivity,type PageActivityChanged} from '../ui/page-navigation';

export type InterconnectGeneral={notify:boolean;clipboard:boolean;downloadDir:string};
export function InterconnectGeneralPanel({active,onActivity}:{active:boolean;onActivity:PageActivityChanged}){
 const {t}=useTranslation(),tr=(key:string)=>t('interconnect.'+key);
 const [general,setGeneral]=useState<InterconnectGeneral>(),[menu,setMenu]=useState<{enabled:boolean;partial?:boolean;conflict?:string}>(),[entry,setEntry]=useState<{visible:boolean;error?:string}>();
 const [busy,setBusy]=useState(false),[error,setError]=useState('');
 usePageActivity(onActivity,false,busy,'interconnect-general');
 useEffect(()=>{if(!active)return;let alive=true;void Promise.all([interconnectCall<InterconnectGeneral>('general.get'),interconnectCall<NonNullable<typeof menu>>('menu.get'),interconnectCall<NonNullable<typeof entry>>('entry.get')]).then(([g,m,e])=>{if(alive){setGeneral(g);setMenu(m);setEntry(e);}}).catch(e=>{if(alive)setError(String(e));});return()=>{alive=false;};},[active]);
 async function action(work:()=>Promise<unknown>){setBusy(true);setError('');try{await work();}catch(e){setError(String(e));}finally{setBusy(false);}}
 async function save(patch:Partial<InterconnectGeneral>){if(general)setGeneral(await interconnectCall('general.save',{...general,...patch}));}
 return <div className="settings-stack">{error&&<Notice>{error}</Notice>}<SettingsSection title={tr('general')} icon={Settings} className="interconnect-wide">
  {general&&<>
   <div className="space-y-2"><p>{tr('defaultDownload')}</p><div className="flex flex-wrap gap-2"><Input aria-label={tr('defaultDownload')} readOnly value={general.downloadDir} className="min-w-0 flex-1"/><Button icon={FolderOpen} disabled={busy} onClick={()=>void action(async()=>{const path=await pickDirectory();if(path)await save({downloadDir:path});})}>{tr('choose')}</Button></div></div>
   <SettingsToggle label={tr('completionNotice')} checked={general.notify} disabled={busy} onChange={notify=>void action(()=>save({notify}))}/>
   <SettingsToggle label={tr('clipboardRecognition')} description={tr('clipboardHint')} checked={general.clipboard} disabled={busy} onChange={clipboard=>void action(()=>save({clipboard}))}/>
  </>}
  {entry&&<><SettingsToggle label={tr('floatingEntry')} description={tr('floatingHint')} checked={entry.visible} disabled={busy} onChange={visible=>void action(async()=>setEntry(await interconnectCall(visible?'entry.show':'entry.hide')))}/>{entry.error&&<Notice>{entry.error}</Notice>}</>}
  {menu&&<><SettingsToggle label={tr('explorerEntry')} description={tr('explorerHint')} checked={menu.enabled} disabled={busy||!!menu.conflict} onChange={enabled=>void action(async()=>setMenu(await interconnectCall('menu.set',{enabled})))}/>{menu.conflict&&<Notice>{menu.conflict}</Notice>}{menu.partial&&<Notice>{tr('menuPartial')}</Notice>}</>}
  <p className="settings-help">{tr('backgroundHint')}</p>
 </SettingsSection></div>;
}
