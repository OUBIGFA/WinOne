// Translate actual Pi lifecycle facts with Cindy's original turn/activity
// state machines. Post-turn transformations are keyed to a saved message, not
// a mutable "last answer", and are persisted independently of model history.
import fs from 'node:fs/promises';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {sanitizeGhostCardHtml} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/cardSanitizer.ts';
import {GhostTurnTranslator,GhostActivityTracker,withGhostAssistantHookModel} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/subscriptionGateway.ts';
const hash=value=>createHash('sha256').update(value).digest('hex');
export function createGhostEvents({root,gateway,push,log}){
 const sessions=new Map(),pending=new Map();let closed=false;
 const directory=id=>path.join(root,'reply-overrides',hash(id));
 const file=(id,message)=>path.join(directory(id),hash(message)+'.json');
 function trackers(id,model){
  let value=sessions.get(id);if(value&&value.model===model)return value;
  value?.turn.dispose();value?.activity.finishAll();
  const activity=new GhostActivityTracker({sessionId:id,sink:{activity:(name,data)=>gateway.publish('activity',name,data)}});
  const turn=new GhostTurnTranslator({sessionId:id,agent:'pi',model,now:Date.now,sink:{
   turnStart:data=>{activity.beginTurn();gateway.publish('turn','did-turn-start',data);},
   turnEnd:data=>{activity.finishTurn();gateway.publish('turn','did-turn-end',data);},
  }});
  value={turn,activity,model};sessions.set(id,value);return value;
 }
 async function rewrite(input){
  const message=input.message;
  if(!message?.id||typeof message.text!=='string'||!message.text)return;
  const key=input.sessionId+':'+message.id,sourceSHA256=hash(message.text);
  if(pending.has(key))return;
  try{const saved=JSON.parse(await fs.readFile(file(input.sessionId,message.id),'utf8'));if(saved.sourceSHA256===sourceSHA256)return;}catch(error){if(error.code!=='ENOENT')throw error;}
  if(pending.has(key)||closed)return;
  const work=(async()=>{
   const verdict=await withGhostAssistantHookModel(Promise.resolve(input.model??''),()=>gateway.screenAssistantMessage({sessionId:input.sessionId,text:message.text}));
   if(closed||verdict.action==='allow')return;
   if(verdict.action==='render'){
    const sanitized=sanitizeGhostCardHtml(verdict.html);
    if(!sanitized.ok)return;
    verdict.html=sanitized.html;
   }
   const value={sessionId:input.sessionId,messageId:message.id,sourceSHA256,...verdict};
   await fs.mkdir(directory(input.sessionId),{recursive:true});
   const target=file(input.sessionId,message.id);await fs.writeFile(target+'.tmp',JSON.stringify(value));await fs.rename(target+'.tmp',target);
   if(!closed)push('assistant-rewritten',value);
  })().catch(error=>log.warn('Plugin answer presentation was not applied: '+error.message)).finally(()=>pending.delete(key));
  pending.set(key,work);
 }
 return {
  receive(input){
   if(closed)return {ok:false};
   const value=trackers(input.sessionId,input.model??'');
   switch(input.kind){
    case 'start':value.turn.handleEvent({type:'status',data:{isRunning:true}});break;
    case 'thinking':value.activity.handleEvent({type:'thinking',data:{blockId:input.blockId,stage:input.stage}});break;
    case 'interaction-start':value.activity.startInteraction(input.question?'ask_user_question':'permission',input.id);break;
    case 'interaction-end':value.activity.endInteraction(input.question?'ask_user_question':'permission',input.id);break;
    case 'settled':
     if(input.status==='completed')value.turn.handleEvent({type:'done',data:{usage:input.usage}});
     else if(input.status==='failed')value.turn.handleEvent({type:'error',data:{isTerminal:true,usage:input.usage}});
     else value.turn.dispose();
     if(input.status==='completed')void rewrite(input).catch(error=>log.warn(error.message));
     break;
    case 'closed':value.turn.dispose();value.activity.finishAll();sessions.delete(input.sessionId);break;
   }
   return {ok:true};
  },
  async read(sessionId){
   const rows=[];let names;
   try{names=await fs.readdir(directory(sessionId));}catch(error){if(error.code==='ENOENT')return [];throw error;}
   for(const name of names){if(!name.endsWith('.json'))continue;const value=JSON.parse(await fs.readFile(path.join(directory(sessionId),name),'utf8'));if(value.sessionId===sessionId)rows.push(value);}
   return rows;
  },
  close(){closed=true;for(const value of sessions.values()){value.turn.dispose();value.activity.finishAll();}sessions.clear();},
 };
}
