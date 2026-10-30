import {VoiceConversationService} from '../cindy/voice/main/voice-conversation/service.ts';
import {CompanionCallDialogue} from '../cindy/voice/main/voice-conversation/dialogue.ts';
import {speechModels,resolveSpeechConnection} from '../cindy/voice/main/voice-conversation/models.ts';
import {companionVoicePreference,updateCompanionVoicePreference,updateSentencePlaybackPreference} from '../cindy/voice/main/voice-conversation/preferences.ts';
import {classifyVoiceExit} from '../cindy/voice/main/voice-conversation/semanticExit.ts';
import {VoiceInputDataStore} from '../cindy/voice/main/voice-input/VoiceInputDataStore.ts';
import {LiteLlmTranscriptionProvider} from '../cindy/voice/main/voice-input/LiteLlmTranscriptionProvider.ts';
import {FallbackAsrProvider} from '../cindy/voice/main/voice-input/FallbackAsrProvider.ts';
import {SystemAudioMuteGuard} from '../cindy/voice/main/voice-input/SystemAudioMuteGuard.ts';
import {hasVoiceConversationLease} from '../cindy/voice/main/voice-conversation/lease.ts';
import {BUNDLED_CATALOG,buildUserProvider} from '../cindy/voice/packages/model-providers/src/index.ts';
import {VoiceInputController,VoiceTimelineLogger,DictationRefiner,DictationDictionaryAdvisor} from './voice-core.mjs';
import {normalizeVoiceConversationSelection} from '../../../apps/desktop/frontend/src/features/cindy/shared/voiceConversation.ts';
import {requestExplicitUtilityText} from './voice-host.mjs';
import {oneBotError} from './content-service.mjs';
import {createLogger} from './logger.ts';
import {branchRows} from './learning-data.mjs';

const SENDER=1;
const audioWire=async promise=>{const result=await promise;return {...result,pcm:Buffer.from(result.pcm).toString('base64')};};
const audioBuffer=value=>{const bytes=Buffer.from(value,'base64');return bytes.buffer.slice(bytes.byteOffset,bytes.byteOffset+bytes.byteLength);};

export function createOneBotVoice(scope){
 const data=new VoiceInputDataStore();
 // Cross-device dictionary synchronization belongs to the excluded Cindy
 // device service. Its local journal, learning and settings are still reused.
 if(data.getSettings().dictionarySyncEnabled)data.updateSettings({dictionarySyncEnabled:false});
 let resources={providers:[]}, catalog={...BUNDLED_CATALOG,providers:[]}, dictation=null, currentRun='', closed=false, starting=false, inputEpoch=0;
 const muteGuard=new SystemAudioMuteGuard();
 const log=createLogger('voice');
 const emit=payload=>scope.event({type:'voice-input',payload});
 let activeSessionId='',projectedRows,projection;
 // The upstream dialogue's own polling reads through the existing Pi branch
 // projection. There is no second polling loop or duplicate model transcript.
 scope.refreshVoiceProjection=()=>{
  if(!activeSessionId||!scope.taskStore)return Promise.resolve();
  if(projection)return projection;
  projection=(async()=>{
   await scope.refreshTaskSession(activeSessionId);
   const rows=await branchRows(await scope.host.conversation(activeSessionId));
   if(rows===projectedRows)return;
   const messages=rows.filter(row=>row.role==='user'||row.role==='assistant'),live=new Set(messages.map(row=>row.id)),db=scope.taskStore;
   db.transaction(()=>{
    for(const row of db.query("SELECT id,client_id AS clientId FROM messages WHERE session_id=? AND id LIKE 'pi-voice:%' AND rewind_at IS NULL",[activeSessionId]))if(!live.has(row.clientId))db.exec('UPDATE messages SET rewind_at=? WHERE id=?',[Date.now(),row.id]);
    for(const row of messages)db.exec("INSERT INTO messages (id,session_id,client_id,role,content,agent_meta,created_at) VALUES (?,?,?,?,?,?,?) ON CONFLICT(session_id,client_id) DO UPDATE SET content=excluded.content,agent_meta=json_patch(COALESCE(messages.agent_meta,'{}'),excluded.agent_meta),rewind_at=NULL",['pi-voice:'+activeSessionId+':'+row.id,activeSessionId,row.id,row.role,row.content,JSON.stringify(row.meta),row.createdAt]);
   });
   projectedRows=rows;
  })().finally(()=>{projection=undefined;});return projection;
 };
 async function refresh(){
  resources=await scope.host.voiceResources();
  catalog={...BUNDLED_CATALOG,providers:resources.providers.map(provider=>buildUserProvider({id:provider.id,name:provider.name,runtimes:{pi:{baseUrl:provider.baseUrl,wireProtocol:'openai-chat',headerOverride:provider.headers,models:provider.models.map(model=>typeof model==='string'?{id:model,name:model}:model)}}},{modelRegistry:BUNDLED_CATALOG.modelRegistry}))};
 }
 const catalogDeps=()=>({catalog,gatewayAllowed:false,disabled:()=>false,readKey:id=>resources.providers.find(provider=>provider.id===id)?.key??null,gatewayConnection:()=>{throw Error('Cindy gateway is excluded');}});
 async function createProvider(language=data.getSettings().language){
  scope.voiceInputAbort=new AbortController();
  const candidates=catalog.providers.flatMap(provider=>(provider.audioModels??[]).filter(model=>model.mode==='audio_transcription'&&model.defaultEnabled!==false).map(model=>({kind:'litellm',create:async()=>{const source=resources.providers.find(item=>item.id===provider.id);return new LiteLlmTranscriptionProvider({proxyApiKey:source.key,baseUrl:source.baseUrl,model:model.id,sourceLanguage:language});}})));
  if(!candidates.length)throw oneBotError('ASR_UNAVAILABLE','请在 AI 服务中配置并启用语音识别模型');
  return new FallbackAsrProvider(candidates,{sharedAccountRateLimit:false});
 }
 const assertCompanion=async(botId,sessionId)=>{const bot=scope.profiles.get(botId),link=scope.profiles.findSession(botId);if(bot.status!=='active'||link?.sessionId!==sessionId)throw oneBotError('NOT_CANONICAL','语音仅能使用当前伙伴主对话');if(activeSessionId!==sessionId){activeSessionId=sessionId;projectedRows=undefined;}await scope.refreshVoiceProjection();};
 const service=new VoiceConversationService({owner:()=>closed?'closed':scope.key,hasDictation:()=>!!dictation,assertCompanion,models:()=>speechModels(catalogDeps()),connection:selection=>resolveSpeechConnection(catalogDeps(),selection),createProvider,createDialogue:(selection,onRecord)=>new CompanionCallDialogue({...selection,emit:onRecord}),hasReplyHook:()=>false,classifyExit:input=>classifyVoiceExit(input),fetch:(...args)=>fetch(...args),emit:(_sender,event)=>scope.event({type:'voice',payload:event}),log});
 const textClient={async requestJson(input){const result=await requestExplicitUtilityText(JSON.stringify(input.user),{systemPrompt:input.system,maxTokens:4096,timeoutMs:60000,onText:input.onTextSnapshot});if(!result.ok)throw Error(result.reason??'语音整理失败');return JSON.parse(result.text);}};
 async function startDictation(params){
  if(dictation||starting||hasVoiceConversationLease())throw oneBotError('BUSY','语音输入或通话正在运行');
  starting=true;const epoch=++inputEpoch;
  try {
  await refresh();if(closed||epoch!==inputEpoch)throw oneBotError('CANCELLED','语音输入已取消');const settings=data.getSettings(),asr=await createProvider(params.sourceLanguage??settings.language);
  const refiner=settings.refinementEnabled?new DictationRefiner({client:textClient,model:'',systemPrompt:settings.refinementInstructions,contextProvider:()=>params.refinementContext??{},historyProvider:()=>data.getHistoryForRefinement().map(entry=>entry.text)}):undefined;
  const controller=new VoiceInputController({asr,refiner,pauseRefinementEnabled:true,logger:new VoiceTimelineLogger(),callbacks:{
   onStateChanged:(state,outcome)=>{if(currentRun)emit({type:'state',runId:currentRun,state,outcome});},
   onDraftChanged:(text,segment,source)=>{if(currentRun)emit({type:'draft',runId:currentRun,text,segment,source});},
   onSubmitted:(text,segment)=>{if(currentRun)emit({type:'submitted',runId:currentRun,text,segment});return {id:segment.id,segmentIds:[segment.id],startOffset:0,endOffset:text.length,userTouched:false};},
   onRefinementPreview:(text,segment,range)=>emit({type:'refinement-preview',runId:currentRun,text,segment,range}),
   applyRefinement:(range,text)=>{emit({type:'refined',runId:currentRun,text,segment:{id:range.id+':refined',text,source:'mic',status:'refined',updatedAt:Date.now()},range});return true;},
   onError:(message,code,details)=>emit({type:'error',runId:currentRun,message,code,transcriptKept:details?.transcriptKept}),
  }});
  dictation=controller;
  try{currentRun=await controller.start();if(closed||epoch!==inputEpoch)throw oneBotError('CANCELLED','语音输入已取消');return {ok:true,runId:currentRun};}catch(error){dictation=null;await controller.cancel();throw error;}
  }finally{starting=false;}
 }
 const methods={
  models:()=>service.models(),voices:p=>service.voices(p.selection),
  preview:p=>service.startPreview(SENDER,p.input),readPreview:p=>audioWire(service.readPreview(SENDER,p.previewId)),stopPreview:p=>{service.stopPreview(SENDER,p.previewId);return {ok:true};},
  preference:p=>{scope.profiles.get(p.botId);return companionVoicePreference(data.getSettings(),p.botId);},
  setPreference:p=>{scope.profiles.get(p.botId);const settings=data.getSettings(),selection=p.selection===null?null:normalizeVoiceConversationSelection(p.selection);if(p.selection!==null&&!selection)throw oneBotError('INVALID_PARAMS','语音设置无效');data.updateSettings({conversationSelections:updateCompanionVoicePreference(settings,p.botId,selection),...(p.sentencePlayback!==undefined?{conversationSentencePlayback:updateSentencePlaybackPreference(settings,p.botId,p.sentencePlayback)}:{})});return companionVoicePreference(data.getSettings(),p.botId);},
  start:p=>service.start(SENDER,p.input),startPlayback:p=>service.startPlayback(SENDER,p.input),promoteTurn:p=>service.promoteTurn(SENDER,p.callId,p.inputId),submitTurn:p=>service.submitTurn(SENDER,p.callId,p.inputId,p.text,p.forceHandoff===true),
  classifyExit:p=>service.classifyExit(SENDER,p.callId,p.utteranceId,p.inputGeneration),audio:p=>{service.audio(SENDER,p.callId,audioBuffer(p.pcm),p.trace,p.inputGeneration);return {ok:true};},
  setInputMuted:p=>service.setInputMuted(SENDER,p.callId,p.muted),finishUtterance:p=>{service.finishUtterance(SENDER,p.callId);return {ok:true};},end:async p=>{scope.voiceInputAbort?.abort();await service.end(SENDER,p.callId);return {ok:true};},
  speak:p=>service.speak(SENDER,p.input.callId,p.input.requestId,p.input.text,p.input.streaming),readSpeech:p=>audioWire(service.readSpeech(SENDER,p.callId,p.requestId)),interrupt:p=>{service.interrupt(SENDER,p.callId);return {ok:true};},
  'input.data':()=>data.getSnapshot(),'input.settings':p=>data.updateSettings(p.patch),'input.migrate':p=>data.migrateLegacyRendererStorage(p),
  'input.readiness':async()=>{await refresh();return catalog.providers.some(provider=>(provider.audioModels??[]).some(model=>model.mode==='audio_transcription'&&model.defaultEnabled!==false))?{ok:true}:{ok:false,error:'请在 AI 服务中配置并启用语音识别模型'};},
  'input.history':p=>data.getHistory(p.limit),'input.historyForRefinement':()=>data.getHistoryForRefinement(),'input.recordHistory':p=>data.recordHistory(p.text),'input.updateHistory':p=>data.updateHistoryEntry(p.id,p.text),'input.deleteHistory':p=>data.deleteHistoryEntry(p.id),
  'input.dictionaryRecord':p=>data.recordDictionaryLearningActions(p.actions),'input.dictionaryDelete':p=>data.deleteDictionaryEntries(p.ids),'input.dictionaryAdd':p=>data.addManualDictionaryEntry(p.text),'input.dictionaryImport':p=>data.importManualDictionaryEntries(p.texts),'input.dictionaryRename':p=>data.renameDictionaryEntry(p.id,p.text),'input.dictionaryEdit':p=>data.editDictionaryEntry(p.id,p.text,p.aliases),
  'input.start':startDictation,'input.audio':p=>{if(p.runId===currentRun)dictation?.appendAudio(audioBuffer(p.pcm),p.trace);return {ok:true};},
  'input.stop':async p=>{if(p.runId!==currentRun||!dictation)return {ok:false,error:'语音输入已结束'};const active=dictation;try{await active.stop();if(dictation===active){dictation=null;currentRun='';}return {ok:true};}finally{await muteGuard.restore(SENDER);}},
  'input.cancel':async p=>{if(p.runId&&p.runId!==currentRun)return {ok:true};inputEpoch++;scope.voiceInputAbort?.abort();const active=dictation;dictation=null;currentRun='';try{await active?.cancel();return {ok:true};}finally{await muteGuard.restore(SENDER);}},
  'input.mute':async()=>{if(hasVoiceConversationLease())return {ok:false,error:'语音通话正在进行'};await muteGuard.mute(SENDER);return {ok:true};},
  'input.restore':async()=>{await muteGuard.restore(SENDER);return {ok:true};},
 };
 return {data,async call(method,params){const run=methods[method];if(!run)throw oneBotError('UNKNOWN_METHOD','不支持的语音操作');if(['models','voices','preview','start','startPlayback'].includes(method))await refresh();return (await run(params))??{ok:true};},async close(){closed=true;inputEpoch++;scope.voiceInputAbort?.abort();await service.end(SENDER);service.stopPreview(SENDER);await dictation?.cancel();dictation=null;await muteGuard.restoreAll();}};
}
