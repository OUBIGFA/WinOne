import type {ChatAttachment} from './chat-types';
import {formatBrowserCommentsForSend,removeBrowserCommentAndRepairChains,type BrowserCommentDraftItem} from './cindy/browser/browserComments';

const comments=(files:ChatAttachment[]):BrowserCommentDraftItem[]=>files.flatMap(file=>file.annotation?[{...file.annotation,screenshot:file}]:[]);

/** Text and screenshot travel together; style/target data stays available to
 * the agent and is also persisted on the draft attachment before submission. */
export function browserAnnotatedPrompt(text:string,attachments:ChatAttachment[]):string{
 return formatBrowserCommentsForSend(comments(attachments),text);
}

/** Keep later annotations rooted in the original style after removing a chip. */
export function removeBrowserAttachment(files:ChatAttachment[],path:string):ChatAttachment[]{
 const removed=files.find(file=>file.path===path)?.annotation;
 if(!removed)return files.filter(file=>file.path!==path);
 const repaired=new Map(removeBrowserCommentAndRepairChains(comments(files),removed.id).map(item=>[item.id,item]));
 return files.filter(file=>file.path!==path).map(file=>{const item=file.annotation&&repaired.get(file.annotation.id);return item?{...file,annotation:{...file.annotation!,styleChanges:item.styleChanges}}:file;});
}
