// Cindy keys share the host locale, without an independent language setting.
import {useTranslation as useHostTranslation} from '../../../i18n';
const aliases:Record<string,string>={
 'chat.markdownRenderer.copyCode':'chat.copyCode','chat.markdownRenderer.codeCopied':'chat.copied',
 'chat.markdownRenderer.copy':'chat.copy','chat.markdownRenderer.blockCopied':'chat.copied',
 'chat.media.copyFailed':'chat.copyFailed','chat.media.copyImage':'chat.copyImage',
};
export function useTranslation(){const {t}=useHostTranslation();return {t:(key:string,options?:Record<string,unknown>)=>t(aliases[key]??key,options)};}
