import {useEffect,useState} from 'react';
import {Play,Square,PanelTop,PanelTopClose,RefreshCw} from 'lucide-react';
import {wintrayCall} from '../bridge';
import {Button} from '../ui/button';
import {useTranslation} from '../i18n';
import {Dialog,DialogContent,DialogHeader,DialogTitle,DialogDescription,DialogFooter} from '../components/ui/dialog';

type ProgramState={running:boolean;hasWindow:boolean;visible:boolean};
// WinTray's live window actions, composed from WinOne's existing controls.
// Runtime state is separate from settings: refreshing never replaces a draft.
export function WinTrayProgramActions({id,name,disabled,busy,onRun}:{id:string;name:string;disabled:boolean;busy:boolean;onRun:(method:string,params:unknown)=>Promise<void>}){
 const {t}=useTranslation(),[state,setState]=useState<ProgramState>(),[error,setError]=useState(''),[confirm,setConfirm]=useState(false),[refresh,setRefresh]=useState(0);
 useEffect(()=>{
  if(disabled||busy)return;let alive=true,timer:ReturnType<typeof setTimeout>|undefined;
  async function update(){
   if(document.visibilityState==='hidden'){timer=setTimeout(update,1500);return;}
   try{const next=await wintrayCall<ProgramState>('program.state',{id});if(typeof next?.running!=='boolean')throw Error(t('wintray.unknownResult'));
    if(alive){setState(previous=>previous?.running===next.running&&previous?.hasWindow===next.hasWindow&&previous?.visible===next.visible?previous:next);setError('');}
   }catch(error){if(alive){setState(undefined);setError(String(error));}}
   if(alive)timer=setTimeout(update,1500);
  }
  void update();return()=>{alive=false;if(timer)clearTimeout(timer);};
 },[id,disabled,busy,refresh,t]);
 async function act(method:string,params:unknown){await onRun(method,params);setRefresh(value=>value+1);}
 const blocked=disabled||busy;
 return <>
  <Button icon={state?.running?Square:Play} disabled={blocked} onClick={()=>state?.running?setConfirm(true):void act('launch',{id})}>{t(state?.running?'wintray.stop':'wintray.launch')}</Button>
  <Button icon={state?.visible?PanelTopClose:PanelTop} disabled={blocked||!state?.hasWindow} title={!state?.hasWindow?t('wintray.noWindow'):undefined} onClick={()=>void act('program.window',{id,visible:!state?.visible})}>{t(state?.visible?'wintray.hideWindow':'wintray.showWindow')}</Button>
  {error&&<span role="alert" className="text-sm text-muted-foreground" title={error}>{t('wintray.stateUnavailable')} <Button variant="ghost" size="icon" icon={RefreshCw} disabled={blocked} aria-label={t('common.retry')} onClick={()=>setRefresh(value=>value+1)}/></span>}
  <Dialog open={confirm} onOpenChange={open=>{if(!busy)setConfirm(open);}}>
   <DialogContent showCloseButton={false}><DialogHeader><DialogTitle>{t('wintray.stopTitle',{name})}</DialogTitle><DialogDescription>{t('wintray.stopDescription')}</DialogDescription></DialogHeader>
    <DialogFooter><Button disabled={busy} onClick={()=>setConfirm(false)}>{t('wintray.cancelReset')}</Button><Button variant="danger" disabled={blocked} onClick={()=>{setConfirm(false);void act('program.stop',{id,confirmed:true});}}>{t('wintray.confirmStop')}</Button></DialogFooter>
   </DialogContent>
  </Dialog>
 </>;
}
