import {useContext,useState,type ReactNode} from "react";
import {ChatMessageEdit} from './chat-message-edit';
import {ChatFeedback} from './chat-feedback';
import {IconButton} from 'chatgpt-desktop-kit';
import {ConversationMessage} from './cindy/ConversationMessage';
import './chat-markdown.css';
import {Pencil,Copy,RotateCcw,GitBranch,MessagesSquare,Volume2,Square,LoaderCircle} from "lucide-react";
import {MessageReadAloudContext} from './cindy/components/chat/MessageReadAloudContext';
import {VoiceCallTranscript} from './cindy/bots/voice/VoiceCallTranscript';
import {useTranslation} from "../i18n";
import {messageText,type ChatMessage,type ToolActivity} from "./chat-state";
import {WorkGroupBlock} from './cindy/WorkGroupBlock';
import {AgentActionRow} from './cindy/AgentActionRow';
import type {AIProvider} from "./ai-configuration";
import {modelLabel} from "./ai-model-label";
import {ChatGeneratedFiles} from './chat-generated-files';
import {UserMessageBody} from './cindy/UserMessageBody';
import {UserAttachmentChip} from './cindy/UserAttachmentChip';
import {BotSessionTaskCard,BotSessionTaskMessageTrace} from './cindy/bots/BotCollaborationCard';
import {BotSessionTaskResultCard} from './cindy/bots/BotSessionTaskResultCard';
import {BotDirectMessageCard} from './cindy/bots/BotDirectMessageCard';
import {readBotCollaborationMeta} from '../../../../../modules/chat-pi/cindy/tasks/shared/botCollaboration';
import {readBotStarterOptions,stripStarterOptionList} from '../../../../../modules/chat-pi/cindy/bots/shared/botStarterOptions';
import {hasRichAnswer} from '../../../../../third_party/cindy/maker-shared/richAnswer';
import {useBotChatQuote} from './cindy/bots/botChatQuoteContext';
import {BotChatQuoteInBubble,BotChatReplyTarget} from './cindy/bots/BotChatQuoteViews';
import {botReplyQuoteFor} from './cindy/bots/botDmQuote';
import {useBotDmThreadForMessage} from './cindy/bots/botDmThreadsContext';
import {BotDmThreadPreview} from './cindy/bots/BotDmThreadPreview';
import {BotStarterOptions} from './cindy/bots/BotStarterOptions';
import {BotLearningFooter} from './cindy/bots/BotLearningFooter';
import {ChatReplyIcon} from './cindy/components/chat/ChatReplyIcon';

export {ChatMarkdown} from './chat-markdown';
import {ChatMarkdown} from './chat-markdown';
import {ChatFileNavigation} from './chat-file-navigation';

/** What a message adds inside a OneBot teammate's chat (Cindy simplifiedBotConversation). */
export interface ChatBotMessage {
 /** The teammate's avatar beside its replies; work steps keep the same column. */
 avatar:ReactNode;
 /** Cindy's clientId: the accepted input id of an owner message, the Pi entry id of a reply. */
 clientId?:string;
 /** The owner input this reply answers (Cindy replyToClientId). */
 replyToClientId?:string;
 /** Once the owner has written in this chat, a Market opening's starter buttons retire. */
 starterOptionsLocked?:boolean;
}
const isImage=(file:{mimeType?:string})=>!!file.mimeType?.startsWith('image/');

// File changes are owned exclusively by the transcript's per-turn footer.
export function ChatMessageView({message,onEdit,onStopForEdit,onCopy,onFork,onRetry,canFork,running=false,busy=false,showReasoning=true,conversationId="",process,providers=[],files=[],onOpenFile,bot}:{message:ChatMessage;onEdit?:(text:string,restoreRevision?:string)=>Promise<void>;onStopForEdit?:()=>Promise<void>;onCopy:(text:string)=>void;onFork:(id:string)=>void;onRetry?:()=>void;canFork:boolean;running?:boolean;busy?:boolean;showReasoning?:boolean;conversationId?:string;process?:ReactNode;providers?:AIProvider[];files?:string[];onOpenFile?:(path:string,line?:number)=>void;bot?:ChatBotMessage}){
 const {t}=useTranslation(),[editing,setEditing]=useState(false),[editWidth,setEditWidth]=useState(0);
 const readAloud=useContext(MessageReadAloudContext);
 // Cindy's companion chat: "Reply" quotes this message above the composer and
 // a topic opens beside the chat; both apply only to the teammate's own chat.
 const botQuote=useBotChatQuote(bot?conversationId:undefined);
 const dmThread=useBotDmThreadForMessage(bot&&!message.pending&&!message.hiddenInput?conversationId:undefined,bot?.clientId);
 // Like Cindy's Pi translator, hold a failed attempt until the native run
 // settles. A retrying request is not a terminal error or a second reply.
 const visibleError=running?undefined:message.errorMessage;
 const starterOptions=bot&&message.role==='assistant'?readBotStarterOptions(message.agentMeta?.botStarterOptions):[];
 const text=messageText(message),displayText=starterOptions.length?stripStarterOptionList(text,starterOptions):text;
 // Historical call metadata takes precedence over mutable service settings.
 const provider=providers.find(value=>value.id===message.provider);
 const source=message.role==="assistant"&&!bot?[message.providerName||provider?.name||message.provider,message.modelName||(provider&&message.model?modelLabel(provider,message.model,t("ai.free")):message.model)].filter(Boolean).join(" · "):"";
 if(message.hiddenInput)return null;
 if(message.agentMeta?.voiceCall)return <VoiceCallTranscript record={message.agentMeta.voiceCall} onOpenTask={bot?clientId=>window.dispatchEvent(new CustomEvent('winone-voice-task',{detail:{id:conversationId,clientId}})):undefined}/>;
 const collaboration=readBotCollaborationMeta(message.agentMeta?.botCollaboration);
 const data=message.agentMeta?.botCollaboration;
 // Task cards and teammate-message traces (Cindy bot-session-task* and
 // bot-direct-message system cards) sit in a teammate's reply column.
 const card=collaboration&&data?collaboration.role==='delegation-request'?<BotSessionTaskCard data={data} sessionId={conversationId}/>:collaboration.role==='delegation-result'?<BotSessionTaskResultCard data={data} sessionId={conversationId}/>:collaboration.role==='interjection'?<BotSessionTaskMessageTrace data={data}/>:null
  :message.agentMeta?.botDirectMessage?<BotDirectMessageCard data={message.agentMeta.botDirectMessage} sessionId={conversationId}/>:null;
 if(card)return bot?<div className="chat-bot-reply-row"><span className="chat-bot-avatar chat-bot-avatar--spacer" aria-hidden="true">{bot.avatar}</span><div className="chat-bot-reply-column">{card}</div></div>:card;
 if(message.role==="toolResult")return <ChatToolView tool={{id:message.toolCallId??message.id??String(message.timestamp),name:message.toolName??'tool',args:{},running:false,result:text,isError:message.isError}}/>;
 if(message.role!=="assistant"&&message.role!=="user")return null;
 const thinking=typeof message.content==="string"?[]:message.content.map((part,index)=>({part,index})).filter(({part})=>part.type==="thinking"&&part.thinking?.trim());
 if(!text&&!thinking.length&&!visibleError&&!process&&!message.attachments?.length)return null;
 const hasBody=!!(text||!process&&showReasoning&&thinking.length||visibleError||message.attachments?.length||files.length);
 const quote=()=>{
  if(!botQuote||!bot?.clientId)return;
  const attachments=message.attachments??[];
  const value=botReplyQuoteFor({clientId:bot.clientId,role:message.role as 'user'|'assistant',content:text,images:attachments.filter(isImage).map(file=>({originalName:file.name})),files:attachments.filter(file=>!isImage(file)).map(file=>({name:file.name}))});
  if(value)botQuote.quote(value);
 };
 const botActions=bot&&!message.pending&&!running?<>
  {botQuote&&bot.clientId&&<IconButton aria-label={t('chat.messageActionBar.reply')} title={t('chat.messageActionBar.reply')} onClick={quote}><ChatReplyIcon size={16}/></IconButton>}
  {dmThread&&<IconButton aria-label={dmThread.thread?t('bots.groupChat.server.openTopic',{count:dmThread.thread.replyCount}):t('bots.groupChat.server.startTopic')} title={dmThread.thread?t('bots.groupChat.server.openTopic',{count:dmThread.thread.replyCount}):t('bots.groupChat.server.startTopic')} onClick={dmThread.open}><MessagesSquare size={16}/></IconButton>}
 </>:null;
 const actions=text||onRetry||onEdit||botActions?<>
  {bot&&readAloud?.available&&readAloud.sessionId===conversationId&&message.id&&readAloud.canRead(displayText)&&<IconButton aria-label={t(readAloud.messageId===message.id?'bots.voiceMode.stopReadAloud':'bots.voiceMode.readAloud')} aria-pressed={readAloud.messageId===message.id} aria-busy={readAloud.messageId===message.id&&readAloud.phase==='preparing'} disabled={running||readAloud.disabled&&readAloud.messageId!==message.id} onClick={()=>readAloud.toggle(message.id!,displayText)}>{readAloud.messageId===message.id?(readAloud.phase==='preparing'?<LoaderCircle size={16} className="animate-spin motion-reduce:animate-none"/>:<Square size={16}/>):<Volume2 size={16}/>}</IconButton>}
  {text&&<IconButton aria-label={t('chat.copy')} onClick={()=>onCopy(displayText)}><Copy size={16}/></IconButton>}
  {botActions}
  {!bot&&message.id&&canFork&&!message.pending&&<IconButton aria-label={t('chat.branchFrom')} onClick={()=>onFork(message.id!)}><GitBranch size={16}/></IconButton>}
  {onRetry&&<IconButton aria-label={t('chat.retry')} disabled={running||busy} onClick={onRetry}><RotateCcw size={16}/></IconButton>}
  {onEdit&&<IconButton aria-label={t('chat.editMessage')} disabled={busy} onClick={event=>{setEditWidth(event.currentTarget.closest('article')?.querySelector('.chat-message-bubble')?.getBoundingClientRect().width??0);setEditing(true);}}><Pencil size={16}/></IconButton>}
  {source&&<small className="chat-message-source" title={source}>{source}</small>}
 </>:undefined;
 // Under the bubble: the topic opened on this message, then (replies only)
 // a Market opening's starter choices and what the teammate just learned.
 const footer=bot&&!running?<>
  {dmThread?.thread&&<BotDmThreadPreview align={message.role==='user'?'right':'left'} thread={dmThread.thread} bot={dmThread.bot} unread={dmThread.unread} onOpen={dmThread.open}/>}
  {message.role==='assistant'&&text.trim()&&starterOptions.length>0&&<BotStarterOptions options={starterOptions} locked={bot.starterOptionsLocked===true}/>}
  {message.role==='assistant'&&text.trim()&&<BotLearningFooter receipts={message.agentMeta?.botLearning}/>}
 </>:undefined;
 const body=hasBody&&<ConversationMessage role={message.role} bot={!!bot} label={t(message.role==='user'?'chat.userMessage':'chat.assistantMessage')} actions={actions} footer={footer}>
   <div className={"chat-answer-content"+(bot&&message.role==='assistant'&&hasRichAnswer(displayText)?" chat-answer-content--rich":"")}>
    {bot&&message.role==='assistant'&&botQuote&&bot.clientId&&<BotChatReplyTarget message={{clientId:bot.clientId,replyToClientId:bot.replyToClientId}} context={botQuote}/>}
    {bot&&message.role==='user'&&message.replyQuote&&botQuote&&<BotChatQuoteInBubble quote={message.replyQuote} context={botQuote} align="left"/>}
    {!process&&showReasoning&&thinking.length>0&&<div className="cindy-chat-ui chat-work-process"><WorkGroupBlock blockId={`${conversationId}:${message.timestamp??message.id}:thinking`} isStreaming={running} startedAtMs={thinking[0].part.thinkingStartedAt??message.timestamp} durationMs={running?undefined:thinking.every(({part})=>part.thinkingDurationMs!==undefined)?thinking.reduce((total,{part})=>total+(part.thinkingDurationMs??0),0):undefined} childItems={thinking.map(({part,index})=>({kind:'thinking',key:String(index),message:{clientId:`${conversationId}:${message.timestamp??message.id}:${index}`,content:part.thinking!}}))}/></div>}
    {editing&&onEdit&&message.id&&<ChatMessageEdit id={conversationId} entry={message.id} text={text} minimumWidth={editWidth} hasAttachments={!!message.attachments?.length} onStop={onStopForEdit} onCancel={()=>setEditing(false)} onCommit={onEdit}/>}
    {text&&!editing&&(message.role==='user'?<UserMessageBody text={text}/>:<ChatMarkdown text={displayText} isStreaming={running} streamFadeKey={`${conversationId}:${message.id??message.timestamp??'answer'}`}/>)}
    {message.role==='user'&&!!message.attachments?.length&&<div className="cindy-user-attachments">{message.attachments.map(file=><UserAttachmentChip key={file.path} id={conversationId} file={file}/>)}</div>}
    {message.role==='assistant'&&files.length>0&&onOpenFile&&<ChatGeneratedFiles id={conversationId} paths={files} onOpen={onOpenFile}/>}
   </div>
   {visibleError&&<ChatFeedback message={visibleError} role="alert" className="chat-message-error"/>}
 </ConversationMessage>;
 if(bot&&message.role==='assistant'){
  // Cindy withAssistantAvatar: the face sits beside the reply bubble; the work
  // process above it keeps the same column through an invisible spacer.
  return <ChatFileNavigation id={conversationId||undefined} onOpen={onOpenFile} files={message.attachments}>
   {process&&<div className="chat-bot-reply-row"><span className="chat-bot-avatar chat-bot-avatar--spacer" aria-hidden="true">{bot.avatar}</span><div className="chat-bot-reply-column">{process}</div></div>}
   {body&&<div className="chat-bot-reply-row"><span className="chat-bot-avatar" aria-hidden="true">{bot.avatar}</span><div className="chat-bot-reply-column">{body}</div></div>}
  </ChatFileNavigation>;
 }
 return <ChatFileNavigation id={conversationId||undefined} onOpen={onOpenFile} files={message.attachments}>{process}{body}</ChatFileNavigation>;
}
export function ChatToolView({tool}:{tool:ToolActivity}){
 // Compatibility callers use the same Cindy row as grouped tools; no legacy
 // native <details>/<pre> surface or separate fixed-height payload remains.
 return <div className="cindy-chat-ui chat-work-process"><AgentActionRow message={{clientId:tool.id,content:'',toolName:tool.name,toolInput:tool.args,isError:tool.isError}} status={tool.running?'running':'done'} showRawCommand toolResult={tool.result===undefined?undefined:typeof tool.result==='string'?tool.result:JSON.stringify(tool.result,null,2)}/></div>;
}
