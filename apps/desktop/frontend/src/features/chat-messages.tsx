import {useState,type ReactNode} from "react";
import {ChatMessageEdit} from './chat-message-edit';
import {ChatFeedback} from './chat-feedback';
import {IconButton} from 'chatgpt-desktop-kit';
import {ConversationMessage} from './cindy/ConversationMessage';
import './chat-markdown.css';
import {Pencil,Copy,RotateCcw,GitBranch} from "lucide-react";
import {useTranslation} from "../i18n";
import {messageText,type ChatMessage,type ToolActivity} from "./chat-state";
import {WorkGroupBlock} from './cindy/WorkGroupBlock';
import {AgentActionRow} from './cindy/AgentActionRow';
import type {AIProvider} from "./ai-configuration";
import {modelLabel} from "./ai-model-label";
import {ChatGeneratedFiles} from './chat-generated-files';
import {UserMessageBody} from './cindy/UserMessageBody';
import {UserAttachmentChip} from './cindy/UserAttachmentChip';

export {ChatMarkdown} from './chat-markdown';
import {ChatMarkdown} from './chat-markdown';
import {ChatFileNavigation} from './chat-file-navigation';
// File changes are owned exclusively by the transcript's per-turn footer.
export function ChatMessageView({message,onEdit,onStopForEdit,onCopy,onFork,onRetry,canFork,running=false,busy=false,showReasoning=true,conversationId="",process,providers=[],files=[],onOpenFile}:{message:ChatMessage;onEdit?:(text:string,restoreRevision?:string)=>Promise<void>;onStopForEdit?:()=>Promise<void>;onCopy:(text:string)=>void;onFork:(id:string)=>void;onRetry?:()=>void;canFork:boolean;running?:boolean;busy?:boolean;showReasoning?:boolean;conversationId?:string;process?:ReactNode;providers?:AIProvider[];files?:string[];onOpenFile?:(path:string,line?:number)=>void}){
 const {t}=useTranslation(),[editing,setEditing]=useState(false),[editWidth,setEditWidth]=useState(0);
 // Like Cindy's Pi translator, hold a failed attempt until the native run
 // settles. A retrying request is not a terminal error or a second reply.
 const visibleError=running?undefined:message.errorMessage;
 const text=messageText(message);
 // Historical call metadata takes precedence over mutable service settings.
 const provider=providers.find(value=>value.id===message.provider);
 const source=message.role==="assistant"?[message.providerName||provider?.name||message.provider,message.modelName||(provider&&message.model?modelLabel(provider,message.model,t("ai.free")):message.model)].filter(Boolean).join(" · "):"";
 if(message.role==="toolResult")return <ChatToolView tool={{id:message.toolCallId??message.id??String(message.timestamp),name:message.toolName??'tool',args:{},running:false,result:text,isError:message.isError}}/>;
 if(message.role!=="assistant"&&message.role!=="user")return null;
 const thinking=typeof message.content==="string"?[]:message.content.map((part,index)=>({part,index})).filter(({part})=>part.type==="thinking"&&part.thinking?.trim());
 if(!text&&!thinking.length&&!visibleError&&!process&&!message.attachments?.length)return null;
 const hasBody=!!(text||!process&&showReasoning&&thinking.length||visibleError||message.attachments?.length||files.length);
 return <ChatFileNavigation id={conversationId||undefined} onOpen={onOpenFile} files={message.attachments}>{process}{hasBody&&<ConversationMessage role={message.role} label={t(message.role==='user'?'chat.userMessage':'chat.assistantMessage')} actions={text||onRetry||onEdit?<>
  {text&&<IconButton aria-label={t('chat.copy')} onClick={()=>onCopy(text)}><Copy size={16}/></IconButton>}
  {message.id&&canFork&&!message.pending&&<IconButton aria-label={t('chat.branchFrom')} onClick={()=>onFork(message.id!)}><GitBranch size={16}/></IconButton>}
  {onRetry&&<IconButton aria-label={t('chat.retry')} disabled={running||busy} onClick={onRetry}><RotateCcw size={16}/></IconButton>}
  {onEdit&&<IconButton aria-label={t('chat.editMessage')} disabled={busy} onClick={event=>{setEditWidth(event.currentTarget.closest('article')?.querySelector('.chat-message-bubble')?.getBoundingClientRect().width??0);setEditing(true);}}><Pencil size={16}/></IconButton>}
  {source&&<small className="chat-message-source" title={source}>{source}</small>}
 </>:undefined}>
   <div className="chat-answer-content">
    {!process&&showReasoning&&thinking.length>0&&<div className="cindy-chat-ui chat-work-process"><WorkGroupBlock blockId={`${conversationId}:${message.timestamp??message.id}:thinking`} isStreaming={running} startedAtMs={thinking[0].part.thinkingStartedAt??message.timestamp} durationMs={running?undefined:thinking.every(({part})=>part.thinkingDurationMs!==undefined)?thinking.reduce((total,{part})=>total+(part.thinkingDurationMs??0),0):undefined} childItems={thinking.map(({part,index})=>({kind:'thinking',key:String(index),message:{clientId:`${conversationId}:${message.timestamp??message.id}:${index}`,content:part.thinking!}}))}/></div>}
    {editing&&onEdit&&message.id&&<ChatMessageEdit id={conversationId} entry={message.id} text={text} minimumWidth={editWidth} hasAttachments={!!message.attachments?.length} onStop={onStopForEdit} onCancel={()=>setEditing(false)} onCommit={onEdit}/>}
    {text&&!editing&&(message.role==='user'?<UserMessageBody text={text}/>:<ChatMarkdown text={text} isStreaming={running} streamFadeKey={`${conversationId}:${message.id??message.timestamp??'answer'}`}/>)}
    {message.role==='user'&&!!message.attachments?.length&&<div className="cindy-user-attachments">{message.attachments.map(file=><UserAttachmentChip key={file.path} id={conversationId} file={file}/>)}</div>}
    {message.role==='assistant'&&files.length>0&&onOpenFile&&<ChatGeneratedFiles id={conversationId} paths={files} onOpen={onOpenFile}/>}
   </div>
   {visibleError&&<ChatFeedback message={visibleError} role="alert" className="chat-message-error"/>}
 </ConversationMessage>}</ChatFileNavigation>;
}
export function ChatToolView({tool}:{tool:ToolActivity}){
 // Compatibility callers use the same Cindy row as grouped tools; no legacy
 // native <details>/<pre> surface or separate fixed-height payload remains.
 return <div className="cindy-chat-ui chat-work-process"><AgentActionRow message={{clientId:tool.id,content:'',toolName:tool.name,toolInput:tool.args,isError:tool.isError}} status={tool.running?'running':'done'} showRawCommand toolResult={tool.result===undefined?undefined:typeof tool.result==='string'?tool.result:JSON.stringify(tool.result,null,2)}/></div>;
}
