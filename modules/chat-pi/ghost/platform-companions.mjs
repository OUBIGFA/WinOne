// Native OneBot facts + Cindy's unchanged interaction projection/decision rules.
// The plugin never chooses a raw task/request id; opaque refs resolve again at
// every read, answer and post-confirmation boundary.
import {createHash} from 'node:crypto';
import {resolveTeammateStatus} from '../../../third_party/cindy/maker-shared/teammateStatus.ts';
import {companionInteractionKind,companionInteractionDetail,companionInteractionDecision,PLUGIN_COMPANION_REPLY_MAX,PLUGIN_COMPANION_MESSAGE_MAX} from '../cindy/ghost/apps/desktop/src/main/maker-ipc/pluginCompanionHost.ts';
import {answerKey,normalizeAskQuestions,describePermissionAction} from '../cindy/wechat/packages/maker-shared/src/interaction.ts';
import {isSingleBotAvatarGrapheme} from '../cindy/bots/shared/botAvatarValue.ts';
import {visibleRows} from '../onebot/bots-service.mjs';
import {readBranch} from '../onebot/history.mjs';

const fail=code=>{throw Object.assign(Error(code),{code});};
const limit=(value,max)=>Array.from(value??'').slice(0,max).join('');
export function createGhostCompanions({host,ui,canAnswer}){
 let confirming=false;
 const snapshot=async guard=>{guard();const rows=await host('task',{op:'companion-snapshot'});guard();return rows;};
 const requests=row=>(row.pending??[]).map(entry=>{
  const native=entry.request;
  const ref=createHash('sha256').update([entry.sessionId,native.id,native.requestId??''].join('\n')).digest('hex').slice(0,32);
  const input=describePermissionAction(native.tool??'',native.args??{}).kind==='command'
   ?{...native.args,cwd:native.args?.cwd??native.args?.workdir??entry.workingDir}:native.args;
  const request=native.kind==='question'?{requestId:native.id,kind:'ask_user_question',questions:native.questions?.map(question=>({...question,options:question.options?.map(option=>typeof option==='string'?{label:option}:option)}))}
   :{requestId:native.id,kind:'permission',toolName:native.tool,input,metadata:native.metadata};
  return {...entry,ref,native,request};
 });
 const lookup=async(companionId,guard)=>{const row=(await snapshot(guard)).find(row=>row.id===companionId);if(!row)fail('TARGET_UNAVAILABLE');return row;};
 const findRequest=async(companionId,ref,guard)=>{const row=await lookup(companionId,guard),item=requests(row).find(item=>item.ref===ref);if(!item)fail('TARGET_UNAVAILABLE');return {row,item};};
 const messages=async(row,guard)=>{const rows=row.canonical?visibleRows(await readBranch(row.canonical)):[];guard();return rows;};
 return {
  async list(guard){
   const rows=await snapshot(guard),out=[];
   for(const row of rows){
    const waiting=requests(row),history=await messages(row,guard),reply=history.findLast(item=>item.role==='assistant');
    const active=(row.activities??[]).find(item=>item.phase==='needs-interaction')??(row.activities??[]).find(item=>item.phase==='running')??row.activities?.[0];
    out.push({id:row.id,name:row.name,description:row.description??'',home:{isSelf:true,online:true,deviceName:null},
     status:resolveTeammateStatus({activities:row.activities,waiting:waiting.length>0}),
     activity:{phase:active?.phase??'idle',workingPhase:null,summary:null,startedAt:active?.startedAtMs??null,lastActivityAt:active?.lastActivityAtMs??null},
     interactions:waiting.map(item=>({ref:item.ref,kind:companionInteractionKind(item.request),title:item.request.kind==='ask_user_question'?limit(item.request.questions?.[0]?.question,200):item.request.toolName??null})),lastReplyAt:reply?.at??null});
   }
   return out;
  },
  async avatar(companionId,guard){const row=await lookup(companionId,guard);if(isSingleBotAvatarGrapheme(row.avatar))return {src:null,fallback:row.avatar};const avatar=await host('task',{op:'companion-avatar',companionId});guard();return avatar??{src:null,fallback:Array.from(row.name)[0]??'?'};},
  async reply(companionId,guard){const row=await lookup(companionId,guard),reply=(await messages(row,guard)).findLast(item=>item.role==='assistant');return reply?{text:limit(reply.text,PLUGIN_COMPANION_REPLY_MAX),createdAt:reply.at,truncated:Array.from(reply.text).length>PLUGIN_COMPANION_REPLY_MAX}:null;},
  async history(companionId,count,guard){const row=await lookup(companionId,guard),rows=await messages(row,guard);return {messages:rows.slice(-count).map(item=>({role:item.role,text:limit(item.text,PLUGIN_COMPANION_MESSAGE_MAX),createdAt:item.at,truncated:Array.from(item.text).length>PLUGIN_COMPANION_MESSAGE_MAX})),more:rows.length>count};},
  async open(companionId,ref,guard){const row=await lookup(companionId,guard);const item=ref===undefined?null:requests(row).find(item=>item.ref===ref);if(ref!==undefined&&!item)fail('TARGET_UNAVAILABLE');await host('task',{op:'companion-open',companionId,sessionId:item?.sessionId??row.canonical?.id});guard();},
  async interaction(companionId,ref,guard){const {item}=await findRequest(companionId,ref,guard);return companionInteractionDetail(item.request,ref);},
  async answer(companionId,ref,answer,{ghostId,guard:baseGuard}){
   const guard=()=>{baseGuard();if(!canAnswer(ghostId))fail('PERMISSION_DENIED');};
   let found=await findRequest(companionId,ref,guard);const decision=companionInteractionDecision(found.item.request,answer);
   if(answer.kind!=='question'&&answer.decision==='allow'){
    const detail=companionInteractionDetail(found.item.request,ref);if(!detail.allowHere)fail('ALLOW_IN_CINDY');
    if(confirming)fail('TASK_BUSY');confirming=true;let accepted;
    try{accepted=(await ui('confirm',{ghostId,ghostName:found.row.name,body:`${found.row.name}\n\n${detail.kind==='plan'?detail.plan:detail.preview}`,confirmText:'Allow',cancelText:'Cancel'}))?.confirmed===true;}
    finally{confirming=false;}
    guard();if(!accepted)return {answered:false};
    const current=await findRequest(companionId,ref,guard);if(current.item.fingerprint!==found.item.fingerprint)fail('TARGET_UNAVAILABLE');found=current;
   }
   guard();const {item}=found;
   const result=await host('task',{op:'companion-answer',companionId,sessionId:item.sessionId,nativeId:item.native.id,requestId:item.native.requestId,
    fingerprint:item.fingerprint,kind:decision.kind==='ask_user_question'?'question':'permission',allow:decision.behavior==='allow',
    answers:decision.kind==='ask_user_question'?normalizeAskQuestions(item.request.questions).map(question=>decision.answers[answerKey(question)]):undefined});
   return {answered:result.resolved===true};
  },
 };
}
