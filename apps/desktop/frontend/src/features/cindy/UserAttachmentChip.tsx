// UserAttachmentChip adapted from Cindy a152497a, Copyright 2026 XD Inc.,
// Apache-2.0. The shared WinOne preview retains the original managed file.
import {useState} from 'react';
import {FileText} from 'lucide-react';
import {useTranslation} from '../../i18n';
import {DropdownMenu,DropdownMenuContent,DropdownMenuItem,DropdownMenuTrigger} from '../../components/ui/dropdown-menu';
import {chatCall} from '../../bridge';
import {AttachmentPreview} from './ComposerAttachments';
import type {ChatAttachment} from '../chat-types';
export function UserAttachmentChip({id,file}:{id:string;file:ChatAttachment}){
 const {t}=useTranslation(),[open,setOpen]=useState(false),[menu,setMenu]=useState(false),[error,setError]=useState('');
 const action=async(method:string)=>{try{await chatCall(method,{id,path:file.path});setError('');}catch(error){setError(String(error));}};
 return <><DropdownMenu open={menu} onOpenChange={setMenu}><DropdownMenuTrigger render={<button type="button" className="cindy-user-attachment-chip" title={file.path} onClick={event=>{event.preventDefault();setOpen(true);}} onContextMenu={event=>{event.preventDefault();setMenu(true);}}/>}><FileText size={14}/><span>{file.name}</span></DropdownMenuTrigger><DropdownMenuContent className="chat-row-menu"><DropdownMenuItem onClick={()=>setOpen(true)}>{file.name}</DropdownMenuItem><DropdownMenuItem onClick={()=>{void navigator.clipboard.writeText(file.path).catch(error=>setError(String(error)));}}>{t('chat.copyPath')}</DropdownMenuItem><DropdownMenuItem onClick={()=>void action('file.reveal')}>{t('chat.revealFile')}</DropdownMenuItem></DropdownMenuContent></DropdownMenu>{error&&<span role="alert">{error}</span>}{open&&<AttachmentPreview id={id} file={file} onClose={()=>setOpen(false)}/>}</>;
}
