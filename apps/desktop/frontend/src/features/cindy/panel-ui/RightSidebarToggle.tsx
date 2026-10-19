import {SidebarSimple} from '@phosphor-icons/react';
import {ChromeIconButton} from './ChromeIconButton';
import {useTranslation} from 'react-i18next';
export function RightSidebarToggle({onToggle}:{action?:string;collapsed?:boolean;side?:'left'|'right';onToggle:()=>void}){
 const {t}=useTranslation();return <ChromeIconButton aria-label={t('chat.showDetails')} onClick={onToggle}><SidebarSimple size={15}/></ChromeIconButton>;
}
