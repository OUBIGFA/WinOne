import {useTranslation} from '../i18n';
import {Label} from './ui/label';
import {Select,SelectTrigger,SelectValue,SelectContent,SelectItem} from './ui/select';
import type {LucideIcon} from 'lucide-react';

// Receive the actual sidebar catalog so the menu cannot omit new modules.
export function DefaultPageSetting({value,pages,disabled,onChange}:{value:string;pages:{id:string;title:string;icon?:LucideIcon}[];disabled:boolean;onChange:(page:string)=>void}){
 const {t}=useTranslation();
 const selected=pages.find(page=>page.id===value),SelectedIcon=selected?.icon;
 return <div className="settings-choice-field">
  <Label id="default-page-label" htmlFor="default-page">{t('settingsDesign.defaultPage')}</Label>
  <Select value={value} disabled={disabled} onValueChange={page=>{if(page)onChange(page);}}>
   <SelectTrigger id="default-page" aria-labelledby="default-page-label" title={selected?.title} className="w-full min-w-0 max-w-lg">
    <SelectValue>{SelectedIcon&&<SelectedIcon aria-hidden="true" className="size-4 shrink-0"/>}<span className="min-w-0 truncate">{selected?.title??t('nav.overview')}</span></SelectValue>
   </SelectTrigger>
   <SelectContent>{pages.map(page=>{
    const Icon=page.icon;
    return <SelectItem key={page.id} value={page.id}>{Icon&&<Icon aria-hidden="true" className="size-4 shrink-0"/>}<span className="min-w-0 [overflow-wrap:anywhere]">{page.title}</span></SelectItem>;
   })}</SelectContent>
  </Select>
 </div>;
}
