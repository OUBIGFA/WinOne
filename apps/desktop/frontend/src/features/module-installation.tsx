import {usePathDrop} from '../ui/path-drop';
import {useEffect,useRef,useState} from 'react';
import {Plus,Check,RefreshCw} from 'lucide-react';
import {moduleCall,pickModulePackage} from '../bridge';
import {useTranslation} from '../i18n';
import {Button} from '../ui/button';
import {Notice} from '../ui/notice';
import {Dialog,DialogContent,DialogTitle,DialogDescription,DialogFooter} from '../components/ui/dialog';
import {usePageActivity,type PageActivityChanged} from '../ui/page-navigation';
import {packageName,type PackageMetadata,type PackageReference} from './package-types';

interface Preview {id:string;state:string;operation?:'install'|'update';previousVersion?:string;wasEnabled?:boolean;migrationFrom?:number;migrationTo?:number;migrationPaths?:string[];fileName:string;metadata?:PackageMetadata;publisher?:string;keyFingerprint?:string;manifestSha256?:string;payloadBytes?:number;canInstall:boolean;restoresSettings?:boolean;error?:string;dependencyLock?:{packages:PackageReference[]}}
interface InstallationResult {operation:'install'|'update';state:'completed'|'failed';warning?:string;kind?:'runtime'}

export function ModuleInstallation({disabled,onChanged,onActivity}:{disabled:boolean;onChanged:(result:InstallationResult)=>void|Promise<void>;onActivity?:PageActivityChanged}){
 const {t,i18n}=useTranslation();const [open,setOpen]=useState(false),[preview,setPreview]=useState<Preview>(),[error,setError]=useState(''),[busy,setBusy]=useState(false),[choosing,setChoosing]=useState(false);
 const current=useRef<Preview|undefined>(undefined),generation=useRef(0),file=useRef(''),mounted=useRef(true);
 const update=(next:Preview|undefined)=>{current.current=next;setPreview(next)};
 usePageActivity(onActivity,false,busy||choosing,'module-installation');
 useEffect(()=>{mounted.current=true;return()=>{mounted.current=false;generation.current++;const owned=current.current;if(owned)void moduleCall('packagePreview.cancel',{id:owned.id}).catch(()=>{});}},[]);
 useEffect(()=>{
  if(!preview||preview.state!=='checking')return;
  let alive=true;let timer:ReturnType<typeof setTimeout>;
  const poll=async()=>{try{const next=await moduleCall<Preview>('packagePreview.get',{id:preview.id});if(!alive)return;update(next);if(next.state==='checking')timer=setTimeout(()=>void poll(),200);}catch(error){if(alive){setError(String(error));update({...preview,state:'failed',canInstall:false});}}};
  timer=setTimeout(()=>void poll(),100);return()=>{alive=false;clearTimeout(timer)};
 },[preview?.id,preview?.state]);
 async function inspect(path:string){const owner=++generation.current;setError('');update(undefined);setOpen(true);file.current=path;try{const next=await moduleCall<Preview>('packagePreview.start',{path});if(owner!==generation.current){void moduleCall('packagePreview.cancel',{id:next.id}).catch(()=>{});return;}update(next);}catch(error){if(owner===generation.current)setError(String(error));}}
 async function choose(){setChoosing(true);try{const path=await pickModulePackage();if(path&&mounted.current)await inspect(path);}catch(error){if(mounted.current){setError(String(error));setOpen(true);}}finally{if(mounted.current)setChoosing(false)}}
 async function cancel(){if(busy)return;generation.current++;const owned=current.current;update(undefined);setOpen(false);if(owned)try{await moduleCall('packagePreview.cancel',{id:owned.id});}catch(error){setError(String(error));}}
 async function install(){
  if(!preview?.canInstall||busy)return;
  const operation=preview.operation==='update'?'update':'install';
  let outcome:InstallationResult={operation,state:'failed'};
  setBusy(true);setError('');
  try{
   const result=await moduleCall<{state:string;error?:string;warning?:string}>(operation==='update'?'updatePackage':'installPackage',{id:preview.id});
   if(result.state!=='completed')throw Error(result.error||t(operation==='update'?'module.updateFailed':'module.installFailed'));
   outcome={operation,state:'completed',...(preview.metadata?.kind==='runtime'?{kind:'runtime'}:{}),...(result.warning?{warning:result.warning}:{})};update(undefined);setOpen(false);
  }catch(error){setError(String(error));update({...preview,canInstall:false});}
  finally{
   // Failed switches can restore the old version or leave a recovery entry.
   // Refresh the catalog for either outcome without showing a false success.
   try{await onChanged(outcome);}catch(error){setError(current=>current||String(error));}
   setBusy(false);
  }
 }
 const packageDrop=usePathDrop({kind:'file',disabled:disabled||choosing||busy||open,onPaths:paths=>inspect(paths[0]),onError:setError});
 const language=i18n.language==='en-US'?'en-US':'zh-CN';
 const checking=open&&(!preview&&!error||preview?.state==='checking');
 return <>
  <span {...packageDrop} className="inline-flex"><Button icon={Plus} disabled={disabled||choosing||busy} busy={choosing} onClick={()=>void choose()}>{t('module.installPackage')}</Button></span>
  {!open&&error&&<Notice>{error}</Notice>}
  <Dialog open={open} onOpenChange={value=>{if(!value)void cancel()}}>
   <DialogContent className="max-w-lg">
    <DialogTitle>{t(preview?.operation==='update'?'module.updatePackage':'module.installPackage')}</DialogTitle>
    <DialogDescription>{t(preview?.operation==='update'?'module.updateDescription':'module.installDescription')}</DialogDescription>
    {checking&&<p role="status" className="text-sm text-muted-foreground">{t('module.inspectPackage')}</p>}
    {(error||preview?.error)&&<Notice>{error||preview?.error}</Notice>}
    {preview?.metadata&&<>
     <h3 className="text-base font-medium">{packageName(preview.metadata,language)}</h3>
     <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-2 text-sm">
      <dt className="text-muted-foreground">{t('module.packageVersion')}</dt><dd>{preview.operation==='update'?preview.previousVersion+' → '+preview.metadata.version:preview.metadata.version}</dd>
      <dt className="text-muted-foreground">{t('module.packagePublisher')}</dt><dd className="break-all">{preview.publisher}</dd>
      <dt className="text-muted-foreground">{t('module.packageFile')}</dt><dd className="break-all">{preview.fileName}</dd>
     </dl>
     <p className="flex items-center gap-2 text-sm text-muted-foreground"><Check className="size-4"/>{t('module.signatureVerified')}</p>
     <details className="text-sm text-muted-foreground"><summary className="cursor-pointer">{t('module.signatureDetails')}</summary><p className="mt-2 break-all font-mono">{preview.keyFingerprint}</p><p className="mt-2 break-all font-mono">{preview.manifestSha256}</p></details>
     {!!preview.dependencyLock?.packages.length&&<details className="text-sm text-muted-foreground"><summary className="cursor-pointer">{t('sharedPackages.dependencies')} · {preview.dependencyLock.packages.length}</summary><ul className="mt-2 max-h-36 space-y-1 overflow-y-auto">{preview.dependencyLock.packages.map(dependency=><li className="break-words" key={dependency.id}>{dependency.id} · {dependency.version}</li>)}</ul></details>}
     {!!preview.migrationFrom&&<><p className="text-sm text-muted-foreground">{t('module.migrationDescription',{from:preview.migrationFrom,to:preview.migrationTo})}</p>{!!preview.migrationPaths?.length&&<details className="text-sm text-muted-foreground"><summary className="cursor-pointer">{t('module.migrationFields')}</summary><ul className="mt-2 max-h-32 overflow-y-auto space-y-1">{preview.migrationPaths.map(path=><li className="break-all font-mono" key={path}>{path}</li>)}</ul></details>}</>}
     <p className="text-sm text-muted-foreground">{t(preview.metadata.kind==='runtime'?'sharedPackages.installDescription':preview.migrationFrom?(preview.operation==='update'?'module.migrationUpdated':'module.migrationInstalled'):preview.operation==='update'?'module.updateRetained':preview.restoresSettings?'module.installRetained':'module.installDisabled')}</p>
    </>}
    <DialogFooter>
     {/* Both a failed request and a completed check with an error can be retried. */}
     {(error||preview?.error)&&file.current&&<Button icon={RefreshCw} disabled={busy} onClick={()=>void inspect(file.current)}>{t('module.inspectAgain')}</Button>}
     <Button disabled={busy} onClick={()=>void cancel()}>{t('module.cancelInstall')}</Button>
     <Button variant="primary" busy={busy} disabled={!preview?.canInstall||checking} onClick={()=>void install()}>{t(preview?.operation==='update'?'module.confirmUpdate':'module.confirmInstall')}</Button>
    </DialogFooter>
   </DialogContent>
  </Dialog>
 </>;
}
