import type {ChatAttachment} from './chat-types';
import {chatCall} from '../bridge';
import {i18n} from '../i18n';

/** Same one-shot, bounded HTTP transport as image editing. File bytes never
 * enter the JSON RPC pipe; the Host validates content size and destination. */
export async function uploadChatAttachment(id:string,file:File):Promise<ChatAttachment>{
 if(file.size>(file.type.startsWith('image/')?100:20)*1024*1024)throw Error(i18n.t('chat.attachmentSizeLimit'));
 const {url}=await chatCall<{url:string}>('attachments.upload',{id,name:file.name||'image.png'});
 const response=await fetch(url,{method:'POST',headers:{'Content-Type':file.type||'application/octet-stream'},body:file});
 if(!response.ok)throw Error((await response.text()).slice(0,300)||i18n.t('chat.attachmentImportFailed'));
 return (await response.json() as {attachment:ChatAttachment}).attachment;
}

// New-chat browser files stay in the same in-memory draft as its text. Only
// after a conversation exists are they copied to immutable session storage.
export async function importChatAttachments(id:string,files:ChatAttachment[],pending:Map<string,File>){
 const imported:ChatAttachment[]=[];
 for(const file of files){
  const blob=pending.get(file.path);
  const added=blob?await uploadChatAttachment(id,blob):(await chatCall<{attachments:ChatAttachment[]}>('attachments.add',{id,paths:[file.path]})).attachments[0];
  imported.push({...added,annotation:file.annotation,imageAnnotation:file.imageAnnotation});
 }
 return imported;
}
