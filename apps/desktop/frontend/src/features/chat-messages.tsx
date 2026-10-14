import {useState,type ReactNode} from "react";
import {ChatMessageEdit} from './chat-message-edit';
import {ConversationMessage,AIResponse,IconButton,Button} from 'chatgpt-desktop-kit';
import './chat-markdown.css';
import {Pencil,Check,Copy,RotateCcw,Terminal,FileText,GitBranch,LoaderCircle} from "lucide-react";
import {useTranslation} from "../i18n";
import {messageText,type ChatMessage,type ToolActivity} from "./chat-state";
import {ThinkingCard} from "./cindy/ThinkingCard";
import type {AIProvider} from "./ai-configuration";
import {modelLabel} from "./ai-model-label";
import {ChatTurnChanges} from './chat-turn-changes';
import {ChatGeneratedFiles} from './chat-generated-files';
import {UserMessageBody} from './cindy/UserMessageBody';
import {UserAttachmentChip} from './cindy/UserAttachmentChip';

export {ChatMarkdown} from './chat-markdown';
import {ChatMarkdown} from './chat-markdown';
import {ChatFileNavigation} from './chat-file-navigation';
// The caller supplies onOpenReview only for a turn with real successful writes.
// No file count is inferred from generated text or the rendered process node.
export function ChatMessageView({message,onEdit,onStopForEdit,turnId,changeVersion="",onCopy,onFork,onRetry,onOpenReview,canFork,running=false,showReasoning=true,conversationId="",process,providers=[],files=[],onOpenFile}:{message:ChatMessage;onEdit?:(text:string,restoreRevision?:string)=>Promise<void>;onStopForEdit?:()=>Promise<void>;turnId?:string;changeVersion?:string;onCopy:(text:string)=>void;onFork:(id:string)=>void;onRetry?:()=>void;onOpenReview?:(path?:string)=>void;canFork:boolean;running?:boolean;showReasoning?:boolean;conversationId?:string;process?:ReactNode;providers?:AIProvider[];files?:string[];onOpenFile?:(path:string,line?:number)=>void}){
 const {t}=useTranslation(),[editing,setEditing]=useState(false);
 const text=messageText(message);
 // Historical call metadata takes precedence over mutable service settings.
 const provider=providers.find(value=>value.id===message.provider);
 const source=message.role==="assistant"?[message.providerName||provider?.name||message.provider,message.modelName||(provider&&message.model?modelLabel(provider,message.model,t("ai.free")):message.model)].filter(Boolean).join(" · "):"";
 if(message.role==="toolResult")return <details className="chat-tool-result"><summary><Terminal size={15}/><span>{message.toolName||t("chat.tool")}</span>{message.isError?<span>{t("chat.failed")}</span>:<Check size={13}/>}</summary><pre>{text}</pre></details>;
 if(message.role!=="assistant"&&message.role!=="user")return null;
 const thinking=typeof message.content==="string"?[]:message.content.map((part,index)=>({part,index})).filter(({part})=>part.type==="thinking"&&(part.thinking||part.thinkingStartedAt));
 if(!text&&!thinking.length&&!message.errorMessage&&!process&&!message.attachments?.length)return null;
 return <ChatFileNavigation id={conversationId||undefined} onOpen={onOpenFile} files={message.attachments}><ConversationMessage role={message.role} label={t(message.role==='user'?'chat.userMessage':'chat.assistantMessage')} actions={text||onRetry||onEdit?<>
  {text&&<IconButton aria-label={t('chat.copy')} onClick={()=>onCopy(text)}><Copy size={16}/></IconButton>}
  {message.id&&canFork&&<IconButton aria-label={t('chat.branchFrom')} onClick={()=>onFork(message.id!)}><GitBranch size={16}/></IconButton>}
  {onRetry&&<IconButton aria-label={t('chat.retry')} disabled={running} onClick={onRetry}><RotateCcw size={16}/></IconButton>}
  {onEdit&&<IconButton aria-label={t('chat.editMessage')} onClick={()=>setEditing(true)}><Pencil size={16}/></IconButton>}
  {source&&<small className="chat-message-source" title={source}>{source}</small>}
 </>:undefined}>
   <div className="chat-answer-content">
    {process??(showReasoning&&thinking.length>0&&<div className="chat-thinking-group">{thinking.map(({part,index})=><ThinkingCard key={index} content={part.thinking??""} isStreaming={running&&part.thinkingStartedAt!==undefined&&part.thinkingDurationMs===undefined} startedAt={part.thinkingStartedAt} durationMs={part.thinkingDurationMs} blockKey={`${conversationId}:${message.timestamp??message.id}:${index}`} aborted={message.stopReason==="aborted"}/>)}</div>)}
    {editing&&onEdit&&message.id&&<ChatMessageEdit id={conversationId} entry={message.id} text={text} hasAttachments={!!message.attachments?.length} onStop={onStopForEdit} onCancel={()=>setEditing(false)} onCommit={onEdit}/>}
    {text&&!editing&&(message.role==='user'?<UserMessageBody text={text}/>:<ChatMarkdown text={text}/>)}
    {message.role==='user'&&!!message.attachments?.length&&<div className="cindy-user-attachments">{message.attachments.map(file=><UserAttachmentChip key={file.path} id={conversationId} file={file}/>)}</div>}
    {message.role==='assistant'&&files.length>0&&onOpenFile&&<ChatGeneratedFiles id={conversationId} paths={files} onOpen={onOpenFile}/>}
    {message.role==='assistant'&&onOpenReview&&turnId&&<ChatTurnChanges id={conversationId} turnId={turnId} version={changeVersion} disabled={!canFork} onReview={onOpenReview} onOpen={onOpenFile}/>}
    {message.role==='assistant'&&onOpenReview&&!turnId&&<div className="client-change-summary chat-change-summary"><FileText size={24}/><Button size="sm" actionId="client.changes.open" onClick={()=>onOpenReview()}>{t('chat.reviewViewChanges')}</Button></div>}
   </div>
   {message.errorMessage&&<p role="alert">{message.errorMessage}</p>}
 </ConversationMessage></ChatFileNavigation>;
}
export function ChatToolView({tool}:{tool:ToolActivity}){
 const Icon=tool.name==="powershell"?Terminal:FileText;
 return <details className="chat-tool-result"><summary>{tool.running?<LoaderCircle size={15} className="animate-spin"/>:<Icon size={15}/>}<span>{tool.name}</span><code>{String(tool.args.path??tool.args.command??"").slice(0,100)}</code></summary><pre>{JSON.stringify(tool.args,null,2)}</pre>{tool.result!==undefined&&<pre>{typeof tool.result==="string"?tool.result:JSON.stringify(tool.result,null,2)}</pre>}</details>;
}
