// ComposerAttachments/ThumbnailItem adapted from Cindy a152497a,
// Copyright 2026 XD Inc., Apache-2.0. WinOne's managed-file preview replaces
// Electron URLs; composer and persisted messages share the same preview.
import {lazy,Suspense,useEffect,useState} from 'react';
import {FileText,X,Pen} from 'lucide-react';
import {openMedia} from './media/host';
import {useTranslation} from '../../i18n';
import {Dialog,DialogContent,DialogTitle,DialogDescription} from '../../components/ui/dialog';
import type {ChatAttachment} from '../chat-types';
import {formatBytes} from './fileMeta';
import {AttachmentTypeThumb} from './AttachmentTypeThumb';
const ChatFilePreview=lazy(()=>import('../chat-file-preview').then(module=>({default:module.ChatFilePreview})));

export function AttachmentPreview({id,file,onClose}:{id:string;file:ChatAttachment;onClose:()=>void}){
 const {t}=useTranslation();
 const image=file.mimeType.startsWith('image/');
 useEffect(()=>{if(image)openMedia({id,src:file.path,attachment:file,onClose});},[id,file.path,image]);
 if(image)return null;
 return <Dialog open onOpenChange={open=>{if(!open)onClose();}}><DialogContent className="chat-tool-dialog cindy-chat-ui"><DialogTitle>{file.name}</DialogTitle><DialogDescription>{file.path}</DialogDescription><Suspense fallback={<p role="status">{t('chat.loadingResources')}</p>}><ChatFilePreview id={id} path={file.path} onDirty={()=>{}} onHandle={()=>{}}/></Suspense></DialogContent></Dialog>;
}

export function ComposerAttachments({id,attachments,onRemove,disabled}:{id:string;attachments:ChatAttachment[];onRemove:(path:string)=>void;disabled:boolean}){
 const {t}=useTranslation(),[preview,setPreview]=useState<ChatAttachment>();
 if(!attachments.length)return null;
 return <><div className="chat-attachments cindy-attachment-strip" aria-label={t('chat.attachments')}>{attachments.map(file=>{
  const extension=file.name.includes('.')?file.name.split('.').pop()?.toUpperCase():'';
  const meta=[extension,file.size>0?formatBytes(file.size):undefined].filter(Boolean).join(' · ');
  return <div key={file.path} className="cindy-attachment-card" data-slot="cindy-attachment">
   <button type="button" className="cindy-attachment-preview" onClick={()=>setPreview(file)} disabled={!id} aria-label={file.name} title={file.path}><span className="cindy-attachment-type"><AttachmentTypeThumb id={id} file={file}/></span><span className="cindy-attachment-label"><span>{file.annotation?`#${file.annotation.markerNumber} ${file.annotation.comment}`:file.name}</span>{meta&&<small>{meta}</small>}</span></button>
   {file.mimeType.startsWith('image/')&&<button type="button" className="cindy-attachment-edit" disabled={disabled||!id} aria-label={t('chat.media.annotate')+' · '+file.name} onClick={()=>openMedia({id,src:file.path,attachment:file,edit:true,autoAnnotate:true})}><Pen size={12}/></button>}
   <button type="button" className="cindy-attachment-remove" disabled={disabled} aria-label={t('chat.removeAttachment')+' · '+file.name} onClick={()=>onRemove(file.path)}><X size={12}/></button>
  </div>;
 })}</div>{preview&&<AttachmentPreview id={id} file={preview} onClose={()=>setPreview(undefined)}/>}</>;
}
