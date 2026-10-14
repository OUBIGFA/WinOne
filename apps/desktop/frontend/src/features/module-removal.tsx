import {useEffect,useRef,useState} from 'react';
import {Trash2} from 'lucide-react';
import {moduleCall} from '../bridge';
import {useTranslation} from '../i18n';
import {Button} from '../ui/button';
import {Notice} from '../ui/notice';
import {Dialog,DialogContent,DialogTitle,DialogDescription,DialogFooter} from '../components/ui/dialog';
import {usePageActivity,type PageActivityChanged} from '../ui/page-navigation';
import {packageName,type PackageMetadata} from './package-types';

export function ModuleRemoval({id,kind,manifestSha256,disabled,onChanged,onActivity}:{id:string;kind?:'runtime';manifestSha256?:string;disabled:boolean;onChanged:(warning?:string)=>void|Promise<void>;onActivity?:PageActivityChanged}){
 const {t,i18n}=useTranslation();const [open,setOpen]=useState(false),[busy,setBusy]=useState(false),[error,setError]=useState('');
 const [preview,setPreview]=useState<{id:string;metadata:PackageMetadata}>();
 const generation=useRef(0),current=useRef('');
 usePageActivity(onActivity,false,busy,'module-remove-'+id+(manifestSha256??''));
 useEffect(()=>()=>{generation.current++;if(current.current)void moduleCall('packagePreview.cancel',{id:current.current}).catch(()=>{});},[]);
 async function show(){const attempt=++generation.current;setOpen(true);setError('');setPreview(undefined);try{const next=await moduleCall<NonNullable<typeof preview>>('packageRemoval.preview',{id,...(kind==='runtime'?{kind,manifestSha256}:{})});if(attempt!==generation.current){void moduleCall('packagePreview.cancel',{id:next.id}).catch(()=>{});return;}current.current=next.id;setPreview(next);}catch(error){if(attempt===generation.current)setError(String(error));}}
 async function cancel(){if(busy)return;generation.current++;setOpen(false);const token=current.current;current.current='';setPreview(undefined);if(token)try{await moduleCall('packagePreview.cancel',{id:token});}catch(error){setError(String(error));}}
 async function remove(){if(!preview||busy)return;setBusy(true);setError('');try{const result=await moduleCall<{state:string;error?:string;warning?:string}>('removePackage',{id:preview.id});if(result.state!=='completed')throw Error(result.error||t('module.removeFailed'));current.current='';setOpen(false);await onChanged(result.warning);}catch(error){setError(String(error));setPreview(undefined);}finally{setBusy(false)}}
 const language=i18n.language==='en-US'?'en-US':'zh-CN';
 return <><Button icon={Trash2} variant="ghost" disabled={disabled||busy} title={disabled&&kind!=='runtime'?t('module.closeBeforeRemove'):undefined} onClick={()=>void show()}>{t(kind==='runtime'?'sharedPackages.remove':'module.removePackage')}</Button><Dialog open={open} onOpenChange={value=>{if(!value)void cancel()}}><DialogContent className="max-w-md"><DialogTitle>{t(kind==='runtime'?'sharedPackages.remove':'module.removePackage')}</DialogTitle><DialogDescription>{t(kind==='runtime'?'sharedPackages.removeDescription':'module.removeDescription')}</DialogDescription>{error&&<Notice>{error}</Notice>}{preview&&<p className="text-sm">{packageName(preview.metadata,language)} · {preview.metadata.version}</p>}<DialogFooter><Button disabled={busy} onClick={()=>void cancel()}>{t('module.cancelInstall')}</Button><Button busy={busy} disabled={!preview} onClick={()=>void remove()}>{t('module.confirmRemove')}</Button></DialogFooter></DialogContent></Dialog></>;
}
