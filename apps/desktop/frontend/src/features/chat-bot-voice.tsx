// The native Chat store remains the authority. Cindy's ordinary question card
// supplies both the visible wizard and its narrowly scoped spoken-answer bridge.
import {useCallback,useSyncExternalStore} from 'react';
import type {Approval,ChatMessage as NativeMessage} from './chat-state';
import {messageText} from './chat-state';
import {AskUserQuestionPrompt} from './cindy/group-shared/renderer/components/new-chat/AskUserQuestionPrompt';
import type {VoiceQuestionBridge} from './cindy/group-shared/renderer/components/new-chat/voiceQuestionSelection';
import {makerChatStore,pendingQuestion,type ChatMessage} from './cindy/host/onebotTaskActions';
import {toast} from './cindy/components/ui/toast';
import {hasVoiceCallContent} from './cindy/bots/voice/voiceCallPresentation';

/** The open panel owns its live record; saved cards retain their history position. */
export function visibleNativeVoiceMessages(messages:NativeMessage[],activeCallId:string|null){return messages.filter(message=>{const record=message.agentMeta?.voiceCall;return !record||(record.callId!==activeCallId&&hasVoiceCallContent(record));});}

export function voiceTranscriptRows(messages:readonly NativeMessage[]):ChatMessage[]{
 return messages.filter(message=>message.role==='user'||message.role==='assistant').map(message=>({
  clientId:message.role==='user'?message.requestId??message.id??'':message.id??'',role:message.role as 'user'|'assistant',content:messageText(message),createdAt:message.timestamp,replyToClientId:message.turnId,
  turnCompleted:message.stopReason==='stop'||message.stopReason==='length',isStreaming:message.role==='assistant'&&!message.stopReason&&!message.errorMessage,assistantPhase:message.stopReason==='toolUse'?'commentary':'final_answer',
  ...(message.agentMeta?.voiceCall?{systemCardType:'voice-call',systemCardData:{record:message.agentMeta.voiceCall}}:{}),
 }));
}
export function OneBotQuestion({sessionId,workingDir,request,voiceBridge,onAnswer}:{sessionId:string;workingDir:string;request:Approval;voiceBridge?:VoiceQuestionBridge;onAnswer:(id:string,answers:string[])=>Promise<unknown>}){
 const subscribe=useCallback((listener:()=>void)=>makerChatStore.subscribeLight(sessionId,listener),[sessionId]);
 const light=useSyncExternalStore(subscribe,()=>makerChatStore.getLightSnapshot(sessionId));
 const pending=pendingQuestion(request)!;
 return <AskUserQuestionPrompt sessionId={sessionId} workingDir={workingDir} pending={pending} voiceBridge={voiceBridge}
  onAnswer={(id,answers)=>{void onAnswer(id,pending.questions.map(question=>answers[question.question]??'')).catch(error=>toast.error(String(error)));}}
  draft={light.askUserDraft} onDraftChange={value=>makerChatStore.setAskUserDraft(sessionId,value)} viewerState={light.askUserViewerState} onViewerStateChange={value=>makerChatStore.setAskUserViewerState(sessionId,value)}/>;
}
