// Host presentation of Cindy's interaction model. Transport-verified identities
// and stored choices are authoritative; callback payloads never contain grants.
import {randomUUID} from 'node:crypto';
import {composeInteractionModel, encodeAskOptionAnswers} from '../cindy/bots/im-policy/interactionCardModel.ts';

function questionRequest(request, index) {
 const question=request.questions[index];
 return {kind:'ask_user_question',questions:[{...question,options:(question.options??[]).map(option=>typeof option==='string'?{label:option}:option)}]};
}
const permissionLabels={'permission:allow-once':'允许一次','permission:deny':'拒绝'};

export function createOneBotInteractions({timeoutMs=30*60*1000}={}) {
 const pending=new Map();
 const lane=(item,message)=>item.record.id+':'+message.senderId;
 const speaker=message=>message.speaker?.id??message.senderId;
 function consume(item,message) {
  const entry=pending.get(lane(item,message));
  if(!entry||speaker(message)!==entry.speaker||message.messageId===entry.sourceId||message.attachments?.length)return false;
  if(entry.permission&&!entry.isOwner(message))return false;
  const text=message.text?.trim();if(!text)return false;
  let answer;
  if(entry.permission) {
   // A text reply confirms only the current operation, never a lasting grant.
   if(['允许','同意','确认','allow','yes','y'].includes(text.toLowerCase()))answer={allow:true};
   if(['拒绝','不允许','取消','deny','no','n'].includes(text.toLowerCase()))answer={allow:false};
  } else if(['取消','cancel'].includes(text.toLowerCase()))answer={cancel:true};
  else {
   const indices=text.split(/[,，\s]+/);
   const choices=entry.model.choices;
   const selected=indices.every(value=>/^[1-9]\d*$/.test(value)&&choices[Number(value)-1]?.answerText)
    ?indices.map(value=>choices[Number(value)-1].answerText):[text];
   answer={answer:encodeAskOptionAnswers(entry.model.question.multiSelect===true,selected)};
  }
  if(answer)entry.finish(answer);
  else void item.im.sendText(message.senderId,'请回复“允许”或“拒绝”；只对当前操作生效。').catch(entry.fail);
  return true;
 }
 async function request(item,message,request,isOwner) {
  if(request.kind==='question') {
   const answers=[];
   for(let index=0;index<request.questions.length;index++) {
    const model=composeInteractionModel(questionRequest(request,index));
    const result=await ask(item,message,model,isOwner);
    if(result.cancel)return {cancel:true};
    answers.push(result.answer);
   }
   return {answers};
  }
  if(!isOwner(message))return {allow:false};
  const model=composeInteractionModel({kind:'permission',toolName:request.tool,displayName:request.title??request.tool,description:request.description??request.reason??'',input:request.args??{}});
  return ask(item,message,model,isOwner);
 }
 function ask(item,message,model,isOwner) {
  const key=lane(item,message);
  if(pending.has(key))return Promise.reject(Error('此渠道已有待确认操作'));
  if(item.abort.signal.aborted)return Promise.reject(Error('IM 连接已停止'));
  const token=randomUUID(),permission=model.kind==='permission';
  // Session-wide approval is deliberately absent: IM policy is per input even
  // when this conversation uses full access on the desktop.
  const choices=model.choices.filter(choice=>choice.choiceId!=='permission:allow-always');
  const title=permission?'OneBot 操作确认':model.headerText;
  const body=permission?[model.displayTitle,model.description,JSON.stringify(model.input,null,2)].filter(Boolean).join('\n'):[model.questionBody||model.question.question,'可回复选项序号或直接输入答案；回复“取消”结束问答。'].join('\n');
  return new Promise((resolve,reject)=>{
   let settled=false,off=()=>{},cardId;
   const cleanup=()=>{clearTimeout(timer);off();item.abort.signal.removeEventListener('abort',abort);if(pending.get(key)===entry)pending.delete(key);};
   const finish=value=>{if(settled)return;settled=true;cleanup();resolve(value);
    if(cardId&&item.im.updateInteractiveCard)void item.im.updateInteractiveCard(cardId,{title,body:body+'\n\n'+(value.cancel?'已取消':permission?(value.allow?'已允许一次':'已拒绝'):'已回答'),buttons:[]}).catch(()=>{});
   };
   const fail=error=>{if(settled)return;settled=true;cleanup();reject(error);};
   const abort=()=>fail(Error('IM 连接已停止，确认已取消'));
   const timer=setTimeout(()=>finish(permission?{allow:false}:{cancel:true}),timeoutMs);timer.unref?.();
   const entry={speaker:speaker(message),sourceId:message.messageId,permission,model,isOwner,finish,fail};pending.set(key,entry);
   item.abort.signal.addEventListener('abort',abort,{once:true});
   if(item.im.sendInteractiveCard&&item.im.onCardAction&&!(model.kind==='ask_user_question'&&(model.degraded||model.question.multiSelect))) {
    off=item.im.onCardAction(action=>{
     if(action.payload?.onebotInteraction!==token||action.senderId!==message.senderId)return;
     const choice=choices.find(choice=>choice.choiceId===action.buttonId);if(!choice)return;
     finish(permission?{allow:choice.decision.behavior==='allow'}:{answer:choice.answerText});
    });
    void item.im.sendInteractiveCard(message.senderId,{title,body,buttons:choices.map(choice=>({id:choice.choiceId,label:choice.label??permissionLabels[choice.choiceId],type:choice.style,payload:{onebotInteraction:token}}))},{threadTs:message.threadTs,deliverToOwnerDm:permission,ownerDmSourceMessageId:message.messageId})
     .then(result=>{cardId=result?.messageId;}).catch(fail);
   } else {
    const options=permission?'回复“允许”执行一次，或“拒绝”取消。':model.degraded?'':choices.map((choice,index)=>`${index+1}. ${choice.label}`).join('\n');
    void item.im.sendText(message.senderId,[title,body,options].filter(Boolean).join('\n'),{threadTs:message.threadTs}).catch(fail);
   }
  });
 }
 return {consume,request};
}
