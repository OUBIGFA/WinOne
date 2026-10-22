import {useEffect,useState} from 'react';
import {Pencil,Plus,Trash2} from 'lucide-react';
import {useTranslation} from '../i18n';
import {Input} from '../components/ui/input';
import {Field,FieldLabel} from '../components/ui/field';
import {SettingsDisclosure,SettingsOptions} from '../components/settings-layout';
import {Button} from '../ui/button';
import type {AIRedactRule,MagpieSettings} from './ai-configuration';

type Props={settings:MagpieSettings;disabled:boolean;onChange:(patch:Partial<MagpieSettings>)=>void};
const secrets=['API_KEY','PRIVATE_KEY','TOKEN','PASSWORD','SECRET'];
const personal=['EMAIL','PHONE','ID_CARD','BANK_CARD','PERSON','ADDRESS','ACCOUNT','SSN','PASSPORT','IBAN','BIRTHDAY','MAC','SERIAL','USERNAME','HOSTNAME','IP','BUCKET'];
// Same kind normalization as Magpie's redact.RuleKind; saved unknown kinds
// stay editable even when their finder is disabled.
const ruleKind=(kind:string)=>kind.trim().toUpperCase().replace(/[ .-]/g,'_').replace(/[^A-Z0-9_]/g,'').replace(/^_+|_+$/g,'')||'CUSTOM';
export function AIPrivacyActions({settings:s,disabled,onChange}:Props){
 const {t}=useTranslation(),[allow,setAllow]=useState((s.redactAllow??[]).join(', '));
 useEffect(()=>setAllow((s.redactAllow??[]).join(', ')),[s.redactAllow]);
 const kinds=[...new Set([...(s.redact?secrets:[]),...(s.redact?(s.redactRules??[]).map(r=>ruleKind(r.kind)):[]),...(s.redactWords?.length?['TERM']:[]),...Object.keys(s.redactActions??{}),...(s.redactPersonal?personal:[])])];
 return <SettingsDisclosure title={t('ai.redactActions')} defaultOpen={!!Object.keys(s.redactActions??{}).length||!!s.redactAllow?.length}>
  <p className="settings-help">{t('ai.redactActionsHint')}</p>
  {kinds.map(kind=><SettingsOptions key={kind} label={`${t('ai.redactType'+kind,{defaultValue:kind})} · ${kind}`} hint={t('ai.redactActionHint'+(s.redactActions?.[kind]??'mask'))} value={s.redactActions?.[kind]??'mask'} disabled={disabled} items={(['mask','warn','block'] as const).map(value=>({value,label:t('ai.redactAction'+value)}))} onChange={value=>{const actions={...s.redactActions};if(value==='mask')delete actions[kind];else actions[kind]=value as 'warn'|'block';onChange({redactActions:actions});}}/>)}
  <Field><FieldLabel htmlFor="ai-redact-allow">{t('ai.redactAllow')}</FieldLabel><Input id="ai-redact-allow" value={allow} disabled={disabled} onChange={e=>setAllow(e.target.value)} onBlur={()=>onChange({redactAllow:allow.split(/[,，\n]/).map(v=>v.trim()).filter(Boolean)})}/><p className="settings-help">{t('ai.redactAllowHint')}</p></Field>
 </SettingsDisclosure>;
}
export function AIPrivacyRules({settings:s,disabled,onChange}:Props){
 const {t}=useTranslation(),[editing,setEditing]=useState<number|null>(null),[kind,setKind]=useState(''),[by,setBy]=useState('prefix'),[match,setMatch]=useState('');
 const rules=s.redactRules??[];
 const reset=()=>{setEditing(null);setKind('');setBy('prefix');setMatch('');};
 const apply=()=>{const rule:AIRedactRule={kind:kind.trim()||'SECRET',[by]:match.trim()};onChange({redactRules:editing===null?[...rules,rule]:rules.map((r,n)=>n===editing?rule:r)});reset();};
 return <div className="space-y-3"><h3 className="settings-group-title">{t('ai.redactRules')}</h3>
  <Field><FieldLabel htmlFor="ai-redact-rule-kind">{t('ai.redactRuleKind')}</FieldLabel><Input id="ai-redact-rule-kind" placeholder="API_KEY" value={kind} disabled={disabled} onChange={e=>setKind(e.target.value)}/></Field>
  <SettingsOptions label={t('ai.redactRuleMatch')} value={by} disabled={disabled} items={['prefix','regex'].map(value=>({value,label:t('ai.'+value)}))} onChange={setBy}/>
  <Field><FieldLabel htmlFor="ai-redact-rule-content">{t('ai.redactRuleContent')}</FieldLabel><Input id="ai-redact-rule-content" value={match} disabled={disabled} onChange={e=>setMatch(e.target.value)}/></Field>
  <div className="flex flex-wrap gap-2"><Button icon={editing===null?Plus:Pencil} disabled={disabled||!match.trim()} onClick={apply}>{t(editing===null?'ai.addRedactRule':'ai.applyRedactRule')}</Button>{editing!==null&&<Button disabled={disabled} onClick={reset}>{t('common.cancel')}</Button>}</div>
  {rules.map((rule,index)=><div key={index} className="flex items-center gap-2 border-t border-border pt-3"><div className="min-w-0 flex-1 break-all text-sm">{rule.kind} · {t(rule.prefix?'ai.prefix':'ai.regex')}: <code>{rule.prefix||rule.regex}</code></div><Button icon={Pencil} size="icon" disabled={disabled} aria-label={t('ai.editRedactRule',{index:index+1})} onClick={()=>{setEditing(index);setKind(rule.kind);setBy(rule.prefix?'prefix':'regex');setMatch(rule.prefix||rule.regex||'');}}/><Button icon={Trash2} size="icon" disabled={disabled} aria-label={t('ai.removeRedactRule',{index:index+1})} onClick={()=>{onChange({redactRules:rules.filter((_,n)=>n!==index)});if(editing!==null){if(editing===index)reset();else if(editing>index)setEditing(editing-1);}}}/></div>)}
 </div>;
}
