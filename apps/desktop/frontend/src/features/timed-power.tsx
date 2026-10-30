import {useEffect,useRef,useState} from 'react';
import {Timer,Power,Play,Square,Save,BellRing} from 'lucide-react';
import {timedPowerCall} from '../bridge';
import {useTranslation} from '../i18n';
import {Button} from '../ui/button';
import {Notice,InlineError} from '../ui/notice';
import {NumberField,validNumber} from '../ui/number-field';
import {usePageActivity,type PageActivityChanged} from '../ui/page-navigation';
import {SettingsSection,SettingsOptions,SettingsToggle} from '../components/settings-layout';
import {SettingsSaveBar} from '../components/settings-save-bar';
import {Label} from '../components/ui/label';
import {Checkbox} from '../components/ui/checkbox';
import {Select,SelectTrigger,SelectValue,SelectContent,SelectItem} from '../components/ui/select';
import {Dialog,DialogContent,DialogTitle,DialogDescription,DialogFooter} from '../components/ui/dialog';

export interface PowerPreferences {schemaVersion:number;revision:number;action:string;mode:string;delaySeconds:number;timeOfDay:string;weekdays:number[];reminderSeconds:number;force:boolean;dryRun:boolean}
export interface PowerSnapshot {state:string;active:boolean;task?:PowerPreferences;deadline:string;error?:string;noticeError?:string;lastEvent?:string}
const actions=['shutdown','restart','hibernate','sleep','lock','logoff'];
const closing=(action:string)=>['shutdown','restart','logoff'].includes(action);
type TimeParts={hours:number;minutes:number;seconds:number};
type PowerDraft=Omit<PowerPreferences,'delaySeconds'|'timeOfDay'>&{delay:TimeParts;time:TimeParts};
const validParts=(parts:TimeParts,maxHours:number)=>validNumber(parts.hours,0,maxHours)&&validNumber(parts.minutes,0,59)&&validNumber(parts.seconds,0,59);
function toDraft(p:PowerPreferences):PowerDraft{
 const {delaySeconds,timeOfDay,...rest}=p;
 const [hours,minutes,seconds=0]=timeOfDay.split(':').map(Number);
 return {...rest,delay:{hours:Math.floor(delaySeconds/3600),minutes:Math.floor(delaySeconds/60)%60,seconds:delaySeconds%60},time:{hours,minutes,seconds}};
}
function toPreferences(draft:PowerDraft):PowerPreferences{
 const {delay,time,...rest}=draft;
 return {...rest,delaySeconds:validParts(delay,168)?delay.hours*3600+delay.minutes*60+delay.seconds:NaN,timeOfDay:validParts(time,23)?[time.hours,time.minutes,time.seconds].map(n=>String(n).padStart(2,'0')).join(':'):''};
}
// Keep the three editable values separate, including temporarily empty fields.
// Conversion happens at the Host boundary, never while the user types a digit.
function TimeFields({label,value,maxHours,onChange}:{label:string;value:TimeParts;maxHours:number;onChange:(value:TimeParts)=>void}){
 const {t}=useTranslation();
 return <fieldset className="plain-fieldset space-y-2" aria-label={label}>
  <legend className="settings-field-label">{label}</legend>
  <div className="grid grid-cols-3 gap-3 min-w-0">{(['hours','minutes','seconds'] as const).map(part=><NumberField key={part} label={t('timedPower.'+part)} inlineLabel={t('timedPower.'+part+'Unit')} value={value[part]} min={0} max={part==='hours'?maxHours:59} onChange={next=>onChange({...value,[part]:next})}/>)}</div>
 </fieldset>;
}
export function validPowerPreferences(p:PowerPreferences){return actions.includes(p.action)&&['countdown','time','weekly'].includes(p.mode)&&validNumber(p.delaySeconds,1,604800)&&validNumber(p.reminderSeconds,0,3600)&&/^([01]\d|2[0-3]):[0-5]\d(?::[0-5]\d)?$/.test(p.timeOfDay)&&(p.mode!=='weekly'||p.weekdays.length>0);}
export function remainingTime(deadline:string,now:number){const seconds=Math.max(0,Math.ceil((Date.parse(deadline)-now)/1000));return [Math.floor(seconds/3600),Math.floor(seconds/60)%60,seconds%60].map(n=>String(n).padStart(2,'0')).join(':');}

// The form is a saved template. Live snapshots never replace an unfinished
// draft, and the active task is rendered from Host data rather than the form.
export function TimedPowerPage({disabled=false,actionsDisabled=false,onActivity}:{disabled?:boolean;actionsDisabled?:boolean;onActivity?:PageActivityChanged}){
 const {t,i18n}=useTranslation();
 const [draft,setDraft]=useState<PowerDraft>();const [saved,setSaved]=useState<PowerDraft>();
 const [status,setStatus]=useState<PowerSnapshot>();const [now,setNow]=useState(Date.now());
 const [busy,setBusy]=useState(false);const [error,setError]=useState('');const [notice,setNotice]=useState('');
 const [confirmation,setConfirmation]=useState<PowerPreferences>();
 const generation=useRef(0),mounted=useRef(true);
 const dirty=!!draft&&JSON.stringify(draft)!==JSON.stringify(saved);
 usePageActivity(onActivity,dirty,busy,'timed-power',()=>{setDraft(saved);setError('');setNotice('');});
 useEffect(()=>{mounted.current=true;return()=>{mounted.current=false;generation.current++;};},[]);
 async function load(){
  setError('');
  try{const p=await timedPowerCall<PowerPreferences>('settings.get');if(!mounted.current)return;const next=toDraft(p);setDraft(next);setSaved(next);
  }catch(e){if(mounted.current)setError(String(e));}
 }
 useEffect(()=>{void load();},[]);
 // Module state can change outside this form (for example on import). Refresh
 // only runtime state; never replace an unsaved template as a side effect.
 useEffect(()=>{
  if(disabled){setStatus(undefined);return;}
  let live=true;
  void timedPowerCall<PowerSnapshot>('snapshot').then(s=>{if(live)setStatus(s);}).catch(e=>{if(live)setError(String(e));});
  return()=>{live=false;};
 },[disabled]);
 useEffect(()=>{
  if(disabled||!status?.active)return;
  let live=true;let timer:ReturnType<typeof setTimeout>;
  const poll=async()=>{const version=generation.current;try{const next=await timedPowerCall<PowerSnapshot>('snapshot');if(live&&version===generation.current){setStatus(next);setNow(Date.now());}}catch(e){if(live)setError(String(e));}finally{if(live)timer=setTimeout(poll,1000);}};
  timer=setTimeout(poll,1000);return()=>{live=false;clearTimeout(timer);};
 },[disabled,status?.active]);
 async function run(operation:()=>Promise<void>){
  if(busy)return;generation.current++;setBusy(true);setError('');setNotice('');
  try{await operation();}catch(e){setError(String(e));}finally{generation.current++;if(mounted.current)setBusy(false);}
 }
 function change<K extends keyof PowerDraft>(key:K,value:PowerDraft[K]){if(draft)setDraft({...draft,[key]:value});}
 async function start(p:PowerPreferences,confirmed:boolean){setStatus(await timedPowerCall<PowerSnapshot>('start',{preferences:p,confirmed}));setNow(Date.now());setConfirmation(undefined);}
 const valid=!!draft&&validPowerPreferences(toPreferences(draft)),locked=busy||actionsDisabled||!!confirmation;
 const formatted=(value:string)=>new Date(value).toLocaleString(i18n.language);
 const taskLabel=(p:PowerPreferences)=>`${t('timedPower.'+p.action)} · ${t('timedPower.'+(p.dryRun?'dryRunTag':closing(p.action)?p.force?'forceTag':'normalTag':p.mode))}`;
 return <section className="settings-section-stack">
  {error&&<Notice actions={!draft?<Button onClick={()=>void load()}>{t('common.retry')}</Button>:undefined}>{error}</Notice>}
  {disabled&&<p className="settings-help">{t('timedPower.disabled')}</p>}
  <SettingsSection icon={Timer} title={t('timedPower.current')}>
   <p role="status">{t('timedPower.'+(status?.state??'idle'))}</p>
   {status?.active&&status.task&&<>
    <p className="text-sm">{taskLabel(status.task)} · {formatted(status.deadline)}</p>
    <p className="text-3xl font-medium tabular-nums" aria-label={t('timedPower.remaining')}>{remainingTime(status.deadline,now)}</p>
    <div><Button icon={Square} disabled={busy||status.state==='executing'} onClick={()=>void run(async()=>{setStatus(await timedPowerCall<PowerSnapshot>('cancel'));})}>{t('timedPower.cancel')}</Button></div>
    <p className="settings-help">{t('timedPower.activeHint')}</p>
   </>}
   {status?.lastEvent==='missed'&&status.active&&<p className="settings-help">{t('timedPower.missed')}</p>}
   {status?.error&&<Notice>{status.error}</Notice>}
   {status?.noticeError&&<Notice>{t('timedPower.noticeError',{message:status.noticeError})}</Notice>}
  </SettingsSection>
  {!draft&&!error&&<p role="status">{t('timedPower.loading')}</p>}
  {draft&&<>
   <fieldset disabled={locked} className="plain-fieldset settings-section-stack">
    <SettingsSection icon={Timer} title={t('timedPower.schedule')}>
     <SettingsOptions label={t('timedPower.mode')} value={draft.mode} items={['countdown','time','weekly'].map(value=>({value,label:t('timedPower.'+value)}))} onChange={value=>change('mode',value)} disabled={locked}/>
     {draft.mode==='countdown'?<>
      <TimeFields label={t('timedPower.delay')} value={draft.delay} maxHours={168} onChange={value=>change('delay',value)}/>
      {/* Light outlined shortcuts stay discoverable without competing with the inputs. */}
      <div className="timed-power-presets flex flex-wrap gap-2" role="group" aria-label={t('timedPower.presets')}>{[15,30,60,90,120,150].map(minutes=><Button key={minutes} variant="secondary" className="rounded-full border-border/60 bg-muted/30 text-muted-foreground font-normal hover:border-border hover:bg-muted/60 hover:text-foreground" onClick={()=>change('delay',{hours:Math.floor(minutes/60),minutes:minutes%60,seconds:0})}>{t('timedPower.preset'+minutes)}</Button>)}</div>
     </>:<div className="space-y-2"><TimeFields label={t('timedPower.timeOfDay')} value={draft.time} maxHours={23} onChange={value=>change('time',value)}/><p className="settings-help">{t('timedPower.timeHint')}</p></div>}
     {draft.mode==='weekly'&&<div role="group" aria-label={t('timedPower.days')} className="flex flex-wrap gap-x-5 gap-y-3">{[1,2,3,4,5,6,0].map(day=><Label key={day} className="flex items-center gap-2"><Checkbox checked={draft.weekdays.includes(day)} onCheckedChange={checked=>change('weekdays',checked?[...draft.weekdays,day].sort():draft.weekdays.filter(value=>value!==day))}/>{t('timedPower.day'+day)}</Label>)}</div>}
    </SettingsSection>
    <SettingsSection icon={Power} title={t('timedPower.power')}>
     <div className="space-y-2"><Label id="power-action-label">{t('timedPower.action')}</Label><Select value={draft.action} onValueChange={value=>{if(value)change('action',value);}}><SelectTrigger aria-labelledby="power-action-label" className="w-full"><SelectValue>{t('timedPower.'+draft.action)}</SelectValue></SelectTrigger><SelectContent>{actions.map(action=><SelectItem key={action} value={action}>{t('timedPower.'+action)}</SelectItem>)}</SelectContent></Select></div>
     {closing(draft.action)&&<SettingsToggle label={t('timedPower.force')} description={t(draft.force?'timedPower.forceHint':'timedPower.normalHint')} checked={draft.force} onChange={value=>change('force',value)}/>}
    </SettingsSection>
    <SettingsSection icon={BellRing} title={t('timedPower.dryRunTag')}>
     <NumberField label={t('timedPower.reminder')} value={draft.reminderSeconds} min={0} max={3600} onChange={value=>change('reminderSeconds',value)}/>
     <SettingsToggle label={t('timedPower.dryRun')} description={t('timedPower.dryRunHint')} checked={draft.dryRun} onChange={value=>change('dryRun',value)}/>
    </SettingsSection>
   </fieldset>
   {!valid&&<InlineError>{t('timedPower.invalid')}</InlineError>}
   <div><Button icon={Play} disabled={locked||disabled||!valid||!!status?.active} onClick={()=>draft.dryRun?void run(()=>start(toPreferences(draft),false)):setConfirmation(toPreferences(draft))}>{t('timedPower.start')}</Button></div>
   <p className="settings-help">{t('timedPower.lifetime')}</p>
   <SettingsSaveBar disabled={locked} undo={{disabled:!dirty,onClick:()=>{setDraft(saved);setError('');setNotice('');}}}>
    <Button icon={Save} variant="primary" disabled={!dirty||!valid} onClick={()=>void run(async()=>{const next=toDraft(await timedPowerCall<PowerPreferences>('settings.save',toPreferences(draft)));setDraft(next);setSaved(next);setNotice(t('timedPower.saved'));})}>{t('timedPower.save')}</Button>
    <p role="status">{dirty?t('timedPower.dirty'):notice}</p>
   </SettingsSaveBar>
  </>}
  <Dialog open={!!confirmation} onOpenChange={open=>{if(!open&&!busy)setConfirmation(undefined);}}><DialogContent><DialogTitle>{t('timedPower.confirmTitle')}</DialogTitle><DialogDescription>{t(confirmation?.force&&closing(confirmation.action)?'timedPower.confirmForce':'timedPower.confirmNormal')}</DialogDescription>
   {confirmation&&<p>{taskLabel(confirmation)} · {confirmation.mode==='countdown'?remainingTime(new Date(Date.now()+confirmation.delaySeconds*1000).toISOString(),Date.now()):confirmation.timeOfDay}</p>}
   {error&&<Notice>{error}</Notice>}
   <DialogFooter><Button disabled={busy} onClick={()=>setConfirmation(undefined)}>{t('timedPower.back')}</Button><Button disabled={busy} onClick={()=>confirmation&&void run(()=>start(confirmation,true))}>{t('timedPower.confirm')}</Button></DialogFooter>
  </DialogContent></Dialog>
 </section>;
}
