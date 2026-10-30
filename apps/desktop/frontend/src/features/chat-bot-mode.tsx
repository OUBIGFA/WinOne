// WinOne host adapter for a OneBot teammate's chat (Cindy b7e46bc, Apache-2.0:
// CCAgentSessionView / MessageStream with simplifiedBotConversation). WinOne's
// Chat surface renders the conversation; this module supplies what Cindy's
// teammate pages read from it and the companion-only timeline presentation.
import {useCallback,useEffect,useMemo,useRef,useState} from 'react';
import {chatCall} from '../bridge';
import {useTranslation} from '../i18n';
import {messageText,type ChatMessage as WinOneMessage} from './chat-state';
import type {ProcessRenderItem} from './chat-process';
import {pendingPermission,pendingQuestion,publishOneBotLight,publishOneBotTranscript,type ChatMessage} from './cindy/host/onebotTaskActions';
import {subscribeOneBotChanges} from './cindy/host/onebotAPI';
import type {BotChatQuoteValue} from './cindy/bots/botChatQuoteContext';
import {setPendingBotReplyQuote} from './cindy/bots/botDmQuote';
import {useBotReplyTargetPreference} from './cindy/bots/useBotReplyTargetPreference';
import {collectBotMessageTimeGroups,findFirstUnreadBotReplyClientId,formatBotMessageGroupTime} from './cindy/bots/botConversationTimeline';
import type {Approval} from './chat-state';

const isImage=(file:{mimeType?:string})=>!!file.mimeType?.startsWith('image/');

/** Cindy's clientId of a WinOne row: the accepted input id of an owner message, the Pi entry id otherwise. */
export const botClientId=(message:WinOneMessage)=>message.role==='user'?message.requestId??message.id:message.id;

/**
 * The rows Cindy's companion pages read from an open chat (topic roots, quote
 * originals): owner messages the owner wrote and replies with visible text,
 * each with the input it answers. Hidden host instructions are not rows.
 */
export function botTranscriptRows(messages:readonly WinOneMessage[]):ChatMessage[]{
 const rows:ChatMessage[]=[];
 for(const message of messages){
  const clientId=botClientId(message);
  if(!clientId||message.hiddenInput||message.presentationOnly||message.role!=='user'&&message.role!=='assistant')continue;
  const content=messageText(message);
  if(message.role==='assistant'&&(!content.trim()||message.stopReason==='toolUse'))continue;
  const attachments=message.attachments??[];
  rows.push({clientId,role:message.role,content,createdAt:message.timestamp,
   ...(message.role==='user'?{images:attachments.filter(isImage).map(file=>({originalName:file.name})),files:attachments.filter(file=>!isImage(file)).map(file=>({name:file.name}))}:{replyToClientId:message.turnId})});
 }
 return rows;
}

/** Publishes the open chat to Cindy's chat-store seam while it is mounted. */
export function usePublishBotChat(sessionId:string,enabled:boolean,rows:readonly ChatMessage[],approvals:readonly Approval[],queuePaused:boolean){
 useEffect(()=>{if(!enabled||!sessionId)return;publishOneBotTranscript(sessionId,rows);},[enabled,sessionId,rows]);
 const question=approvals.find(request=>request.kind==='question')??null,permission=approvals.find(request=>request.kind!=='question')??null;
 useEffect(()=>{if(!enabled||!sessionId)return;publishOneBotLight(sessionId,{pendingPermission:pendingPermission(permission),pendingAskUser:pendingQuestion(question),queuePaused});},[enabled,sessionId,permission,question,queuePaused]);
 useEffect(()=>()=>{if(enabled&&sessionId){publishOneBotTranscript(sessionId,null);publishOneBotLight(sessionId,null);}},[enabled,sessionId]);
}

/** Cindy's quote context for the open chat's bubbles; stable while replies stream. */
export function useBotChatQuoteValue({sessionId,companion,enabled,readOnly,rows,locate}:{sessionId:string;companion:string;enabled:boolean;readOnly:boolean;rows:readonly ChatMessage[];locate:(clientId:string)=>void}):BotChatQuoteValue|null{
 const {t}=useTranslation();
 const {enabled:showReplyTargets}=useBotReplyTargetPreference();
 const rowsRef=useRef(rows);rowsRef.current=rows;
 const locateRef=useRef(locate);locateRef.current=locate;
 const getMessage=useCallback((clientId:string)=>rowsRef.current.find(row=>row.clientId===clientId),[]);
 const owner=t('bots.dmThread.you');
 const names=useMemo(()=>({owner,companion}),[owner,companion]);
 return useMemo<BotChatQuoteValue|null>(()=>enabled&&sessionId?{
  sessionId,names,getMessage,
  // The turn's last loaded reply carries the marker, so it shows once per turn.
  isFirstReplyFor:(assistantClientId,replyToClientId)=>{
   const loaded=rowsRef.current;
   for(let index=loaded.length-1;index>=0;index-=1){const row=loaded[index];if(row.role==='assistant'&&row.replyToClientId===replyToClientId)return row.clientId===assistantClientId;}
   return false;
  },
  showReplyTargets,
  quote:value=>{if(!readOnly)setPendingBotReplyQuote(sessionId,value);},
  locate:clientId=>locateRef.current(clientId),
 }:null,[enabled,sessionId,names,getMessage,readOnly,showReplyTargets]);
}

/** Learning receipts by reply id (Cindy messages.agent_meta.botLearning). */
type BotLearningReceipts=Readonly<Record<string,unknown>>;

/**
 * Cindy stores what a teammate learned from a reply in that reply's message
 * metadata. WinOne keeps the receipts beside the Pi transcript: the open chat
 * reads them by reply id and reads again when the Host reports a change for
 * this chat (a learning review result, or a finished turn's own saves).
 */
export function useBotLearningReceipts(sessionId:string,enabled:boolean):BotLearningReceipts|null{
 const [loaded,setLoaded]=useState<{sessionId:string;receipts:BotLearningReceipts}|null>(null);
 useEffect(()=>{
  if(!enabled||!sessionId)return;
  let active=true,generation=0;
  // Only the newest read may land: an older answer never replaces a newer one.
  const load=()=>{
   const request=++generation;
   chatCall<{receipts:BotLearningReceipts}>('onebot.messages.learning',{sessionId})
    .then(value=>{if(active&&request===generation)setLoaded({sessionId,receipts:value.receipts});})
    .catch(console.error);
  };
  load();
  const stop=subscribeOneBotChanges(event=>{
   if(event.reset||event.type==='message-meta-changed'&&(event.payload as {sessionId?:unknown}|undefined)?.sessionId===sessionId)load();
  });
  return()=>{active=false;stop();};
 },[enabled,sessionId]);
 return enabled&&loaded?.sessionId===sessionId?loaded.receipts:null;
}

/** The transcript with each reply's receipts in its agentMeta; other rows keep their identity. */
export function withBotLearningReceipts(messages:WinOneMessage[],receipts:BotLearningReceipts|null):WinOneMessage[]{
 if(!receipts)return messages;
 let changed=false;
 const next=messages.map(message=>{
  const value=message.role==='assistant'&&message.id?receipts[message.id]:undefined;
  if(value===undefined||message.agentMeta?.botLearning===value)return message;
  changed=true;
  return {...message,agentMeta:{...message.agentMeta,botLearning:value}};
 });
 return changed?next:messages;
}

/** Cindy's clientId of a rendered transcript item (an owner message or a turn's reply). */
export function botItemClientId(item:ProcessRenderItem):string|undefined{
 if(item.kind==='user'||item.kind==='presentation')return botClientId(item.message);
 return item.answer?botClientId(item.answer):undefined;
}

/**
 * Five-minute time groups and the first unread reply after the read position
 * captured on entry (Cindy collectBotMessageTimeGroups and
 * findFirstUnreadBotReplyClientId), keyed by rendered item.
 */
export function useBotTimeline(items:readonly ProcessRenderItem[],enabled:boolean,boundaryAt:number|null|undefined){
 return useMemo(()=>{
  if(!enabled)return {groups:new Map<string,number>(),unreadKey:null as string|null,latestReplyAt:0};
  const rows=items.flatMap(item=>{
   const message=item.kind==='response'?item.answer:item.message;
   if(!message||message.hiddenInput)return [];
   // A teammate-message trace is not a reply to the owner (Cindy bot-direct-message).
   return [{clientId:item.kind+':'+item.key,createdAt:message.timestamp,role:message.role,settled:item.kind!=='response'||!item.running,
    ...(message.agentMeta?.botDirectMessage?{systemCardType:'bot-direct-message'}:{})}];
  });
  // A reply still streaming is neither unread history nor read through yet.
  const settled=rows.filter(row=>row.settled);
  const latestReplyAt=settled.reduce((latest,row)=>row.role==='assistant'&&typeof row.createdAt==='number'?Math.max(latest,row.createdAt):latest,0);
  return {groups:collectBotMessageTimeGroups(rows),unreadKey:findFirstUnreadBotReplyClientId(settled,boundaryAt),latestReplyAt};
 },[items,enabled,boundaryAt]);
}

/** The centred time above a group's first message, and the unread boundary line. */
export function BotTimelineMarkers({timestamp,unread}:{timestamp?:number;unread:boolean}){
 const {t,i18n}=useTranslation();
 return <>
  {unread&&<div className="chat-bot-unread" role="separator" aria-label={t('bots.chat.newMessagesBoundary')}><span>{t('bots.chat.newMessagesBoundary')}</span></div>}
  {timestamp!==undefined&&<time className="chat-bot-time" dateTime={new Date(timestamp).toISOString()}>{formatBotMessageGroupTime(timestamp,i18n.resolvedLanguage??i18n.language)}</time>}
 </>;
}
