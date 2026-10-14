import {useTranslation} from '../i18n';
import {Label} from './ui/label';
import {Select,SelectTrigger,SelectValue,SelectContent,SelectItem} from './ui/select';

// Receive the actual sidebar catalog so the menu cannot omit new modules.
export function DefaultPageSetting({value,pages,disabled,onChange}:{value:string;pages:{id:string;title:string}[];disabled:boolean;onChange:(page:string)=>void}){
 const {t}=useTranslation();
 return <div className="setting-row"><Label id="default-page-label">{t('shell.defaultPage')}</Label><Select value={value} disabled={disabled} onValueChange={page=>{if(page)onChange(page);}}><SelectTrigger aria-labelledby="default-page-label" className="min-w-48"><SelectValue>{pages.find(page=>page.id===value)?.title??t('nav.overview')}</SelectValue></SelectTrigger><SelectContent>{pages.map(page=><SelectItem key={page.id} value={page.id}>{page.title}</SelectItem>)}</SelectContent></Select></div>;
}
