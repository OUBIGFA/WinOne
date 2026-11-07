// Cindy's pure owner-intent projection over Pi's append-only, receipt-addressed
// branch. Plugin prose, old unverified rows and referenced material never become
// a new owner grant; this is the Host boundary, not another intent parser.
import {createAutoReviewIntentProjection} from './cindy/tasks/core/autoReviewIntent.ts';

const projection=createAutoReviewIntentProjection();
export function authorizationHistory(manager){
 const branch=manager.getBranch(),inputs=new Map(),history=[];
 for(const entry of branch){
  if(entry.type==='custom'&&entry.customType==='winone.input')inputs.set(entry.data.id,entry.data);
  if(entry.type==='custom'&&entry.customType==='winone.context-reset'){history.length=0;inputs.clear();}
  if(entry.type==='custom'&&entry.customType==='winone.authorization-answer'){
   const {id,text,acceptedAt}=entry.data;
   history.push({clientId:id,role:'ask_user',content:{text},createdAt:acceptedAt,agentMeta:{autoReviewUserText:{text,acceptedAt}}});
  }
  if(entry.type!=='message'||entry.message.role!=='user')continue;
  const message=entry.message,input=inputs.get(message.requestId),text=input?.presentation?.text??input?.text;
  let agentMeta=null,content=message.content;
  if(input){
   agentMeta={delivery:input.deliveryMode==='steer'?'steer':'turn',autoReviewUserText:input.hostDispatched?(input.presentation?.reviewRequest??{kind:'delegated-continuation'}):text};
   content={text,files:input.attachments??[],...(input.presentation?.agentReferences?{agentReferences:input.presentation.agentReferences}:{})};
  }
  history.push({clientId:input?.id??entry.id,role:'user',content,createdAt:input?.timestamp??message.timestamp,agentMeta});
 }
 return history;
}
export const authorizationIntent=manager=>projection.restore(authorizationHistory(manager));
export const authorizationProjection=manager=>{
 const history=authorizationHistory(manager);
 return {history,sessionIntent:projection.restore(history),reviewIntent:projection.review(history,true)};
};
