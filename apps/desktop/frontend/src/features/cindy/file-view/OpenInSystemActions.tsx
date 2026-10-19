// Cindy OpenInSystemActions controls; saved-session file methods replace shell IPC.
import {ExternalLink,FolderOpen} from 'lucide-react';
import {useTranslation} from 'react-i18next';
import {Button} from '../panel-ui/button';
import {cn} from '../adapter';
import {useChatSessionFile,toast} from '../media/host';
import {chatCall} from '../../../bridge';
export function OpenInSystemActions({absPath,folderPath:_folderPath,className}:{absPath:string;folderPath:string;className?:string}){
 const {t}=useTranslation(),{sessionId}=useChatSessionFile();
 async function run(method:string){try{await chatCall(method,{id:sessionId,path:absPath});}catch(error){toast.error(String(error));}}
 return <div className={cn('flex items-center gap-2',className)}><Button variant="cta" size="md" compact type="button" onClick={()=>void run('file.open')}><ExternalLink size={14}/>{t('ccAgent.workdirBrowse.unrenderable.openInSystem')}</Button><Button variant="secondary" size="md" compact type="button" onClick={()=>void run('file.reveal')}><FolderOpen size={14}/>{t('ccAgent.workdirBrowse.unrenderable.revealFolder')}</Button></div>;
}
