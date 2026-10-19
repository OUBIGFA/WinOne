import {useEffect,useState} from 'react';
import {RefreshCw,RotateCcw,Blocks,Puzzle,Package} from 'lucide-react';
import {captureCall} from '../bridge';
import {useTranslation} from '../i18n';
import {Button} from '../ui/button';
import {Notice} from '../ui/notice';
import {Input} from '../components/ui/input';
import {Tabs,TabsList,TabsTrigger,TabsContent} from '../components/ui/tabs';
import {SettingsSection,SettingsToggle,SettingsOptions,SettingsDisclosure} from '../components/settings-layout';
import {CaptureOptionSelect} from '../ui/capture-option-select';
import type {MagpieSettings} from './ai-configuration';

type Property={type:string;title:string;description?:string;enum?:unknown[];minimum?:number;maximum?:number;default:unknown};
export type BuiltinPlugin={id:string;name:string;sub:string;essential:boolean;on:boolean;chosen:boolean;state:string;inUse:number;why?:string;requires?:string[];config?:Record<string,unknown>;schema?:{properties:Record<string,Property>;'x-order':string[]}};
type BundledPlugin={directory:string;package:string;name:string;version:string;kind?:string;summary?:Record<string,string>;providers:string[]};
type PluginCatalog={builtinPlugins:BuiltinPlugin[];bundledPlugins:BundledPlugin[]};

/** React adaptation of Magpie 674b6ca renderBuiltins/ui.form: every manifest
 * and ordered schema field is retained. WinOne owns the common save draft,
 * rather than maintaining independent account-local plugins.yml files. */
export function AIPlugins({settings,onChange,disabled,revision,active=true,onValidityChange}:{settings:MagpieSettings;onChange:(patch:Partial<MagpieSettings>)=>void;disabled:boolean;revision?:number;active?:boolean;onValidityChange?:(invalid:boolean)=>void}){
 const {t,i18n}=useTranslation(),[catalog,setCatalog]=useState<PluginCatalog>(),[error,setError]=useState(''),[busy,setBusy]=useState(false),[reload,setReload]=useState(0);
 const [invalid,setInvalid]=useState<Set<string>>(new Set());
 useEffect(()=>{onValidityChange?.(invalid.size>0);},[invalid,onValidityChange]);
 useEffect(()=>{if(!active)return;let alive=true;setBusy(true);setError('');void captureCall<PluginCatalog>('ai.magpie.catalog',{}).then(value=>{if(alive)setCatalog(value);}).catch(error=>{if(alive)setError(String(error));}).finally(()=>{if(alive)setBusy(false);});return()=>{alive=false;};},[active,revision,reload]);
 const changeBuiltin=(id:string,patch:{enabled?:boolean;config?:Record<string,unknown>})=>onChange({builtinPlugins:{...settings.builtinPlugins,[id]:{...settings.builtinPlugins?.[id],...patch}}});
 return <div className="settings-stack">
  <p className="settings-help">{t('aiPlugins.hint')}</p>
  {error&&<Notice actions={<Button icon={RefreshCw} disabled={busy} onClick={()=>setReload(n=>n+1)}>{t('common.retry')}</Button>}>{error}</Notice>}
  {busy&&<p role="status">{t('aiPlugins.loading')}</p>}
  {catalog&&<Tabs defaultValue="builtin"><TabsList variant="line"><TabsTrigger value="builtin">{t('aiPlugins.builtin')}</TabsTrigger><TabsTrigger value="bundled">{t('aiPlugins.bundled')}</TabsTrigger></TabsList>
   <TabsContent value="builtin" className="settings-section-stack">
    <SettingsSection icon={Blocks} title={t('aiPlugins.preset')} description={t('aiPlugins.presetHint')}><SettingsOptions label={t('aiPlugins.preset')} value={settings.builtinPreset||'all'} disabled={disabled} onChange={builtinPreset=>onChange({builtinPreset})} items={[{value:'all',label:t('aiPlugins.all')},{value:'essentials',label:t('aiPlugins.essentials')}]}/><Button icon={RotateCcw} disabled={disabled||!Object.keys(settings.builtinPlugins??{}).length} onClick={()=>onChange({builtinPlugins:{}})}>{t('aiPlugins.overrides')}</Button></SettingsSection>
    {!catalog.builtinPlugins?.length&&<Notice>{t('aiPlugins.noPlugins')}</Notice>}
    {catalog.builtinPlugins?.map(plugin=>{
     const choice=settings.builtinPlugins?.[plugin.id],on=plugin.inUse>0||(choice?.enabled??((settings.builtinPreset||'all')==='all'||plugin.essential));
     const name=t('aiPlugins.'+(plugin.id==='tools'?'pluginTools':plugin.id),{defaultValue:plugin.name});
     return <SettingsSection key={plugin.id} icon={Puzzle} title={name} description={plugin.sub}>
      <SettingsToggle label={name} description={plugin.inUse>0?t('aiPlugins.inUse'):plugin.why||undefined} checked={on} disabled={disabled||plugin.inUse>0} onChange={enabled=>changeBuiltin(plugin.id,{enabled})}/>
      {on&&plugin.schema?.['x-order'].map(key=>{
       const field=plugin.schema!.properties[key],value=choice?.config?.[key]??plugin.config?.[key]??field.default;
       const change=(next:unknown)=>changeBuiltin(plugin.id,{config:{...choice?.config,[key]:next}});
       if(field.type==='boolean')return <SettingsToggle key={key} label={field.title} description={field.description} checked={value===true} disabled={disabled} onChange={change}/>;
       if(field.enum)return <CaptureOptionSelect key={key} stacked label={field.title} value={String(value)} disabled={disabled} items={field.enum.map(option=>({value:String(option),label:String(option)}))} onChange={next=>change(field.type==='string'?next:Number(next))}/>;
       return <label className="settings-stack" key={key}>{field.title}<Input aria-label={field.title} type={field.type==='string'?'text':'number'} min={field.minimum} max={field.maximum} step={field.type==='integer'?1:'any'} value={String(value??'')} disabled={disabled} onChange={event=>change(field.type==='string'?event.target.value:Number(event.target.value))}/>{field.description&&<span className="settings-help">{field.description}</span>}</label>;
      })}
      <SettingsDisclosure title={t('aiPlugins.details')}><p className="settings-help">{plugin.id} · {t('aiPlugins.'+plugin.state,{defaultValue:plugin.state})}</p><p className="settings-help">{t('aiPlugins.dependencies')}：{plugin.requires?.join(', ')||'—'}</p></SettingsDisclosure>
     </SettingsSection>;
    })}
   </TabsContent>
   <TabsContent value="bundled" className="settings-section-stack">{catalog.bundledPlugins?.map(plugin=>{
    const choice=settings.bundledPlugins?.[plugin.directory],on=choice?.enabled??plugin.providers.length>0;
    const change=(patch:{enabled?:boolean;options?:Record<string,unknown>})=>onChange({bundledPlugins:{...settings.bundledPlugins,[plugin.directory]:{enabled:on,...choice,...patch}}});
    return <SettingsSection key={plugin.directory} icon={Package} title={`${plugin.name} · ${plugin.version}`} description={plugin.summary?.[i18n.language.startsWith('zh')?'zh':'en']}>
     <SettingsToggle label={plugin.name} checked={on} disabled={disabled} onChange={enabled=>change({enabled})}/>
     <p className="settings-help">{t('aiPlugins.'+(plugin.kind||'providers'))}{plugin.providers.length?` · ${plugin.providers.join(', ')}`:''}</p>
     <SettingsDisclosure title={t('aiPlugins.details')}><p className="settings-help break-all">{plugin.package}</p><PluginOptions value={choice?.options??{}} disabled={disabled} onValidity={valid=>setInvalid(current=>{const next=new Set(current);if(valid)next.delete(plugin.directory);else next.add(plugin.directory);return next;})} onChange={options=>change({options})}/></SettingsDisclosure>
    </SettingsSection>;
   })}</TabsContent>
  </Tabs>}
 </div>;
}
function PluginOptions({value,disabled,onChange,onValidity}:{value:Record<string,unknown>;disabled:boolean;onChange:(value:Record<string,unknown>)=>void;onValidity:(valid:boolean)=>void}){
 const {t}=useTranslation(),[text,setText]=useState(JSON.stringify(value,null,2)),[error,setError]=useState(false);
 useEffect(()=>{setText(JSON.stringify(value,null,2));setError(false);},[JSON.stringify(value)]);
 return <label className="settings-stack">{t('aiPlugins.options')}<textarea className="w-full min-w-0 rounded-md border border-input bg-transparent p-2 text-sm" aria-label={t('aiPlugins.options')} aria-invalid={error} rows={5} value={text} disabled={disabled} onChange={event=>{const next=event.target.value;setText(next);try{const parsed=JSON.parse(next);if(!parsed||Array.isArray(parsed)||typeof parsed!=='object')throw Error();setError(false);onValidity(true);onChange(parsed);}catch{setError(true);onValidity(false);}}}/>{error&&<span role="alert">{t('aiPlugins.invalidOptions')}</span>}</label>;
}
