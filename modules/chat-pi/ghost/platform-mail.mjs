// Provider adapters and Cindy's mail runtime remain in the actual plugin.
// WinOne reuses the original host-owned draft/card/reader rules and the existing
// card database boundary; opening a draft never sends mail by itself.
import {randomUUID} from 'node:crypto';
import {MailCardService} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/mailCards.ts';
import {createMailReader} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/mailReader.ts';
import {installedGhostStoragePart,installedGhostLogicalIdentity,pluginStoragePart} from '../cindy/ghost/apps/desktop/src/shared/pluginIdentity.ts';

export function createGhostMail({manager,find,current,ownerScope,host,cards,dispatcher,read,list,persist,push,assessment}){
 const service=new MailCardService({cards,read,list,persist,broadcast:card=>push('card',card)});
 const reader=createMailReader({
  plugins:()=>manager.list().flatMap(ghost=>{
   if(!ghost.enabled||ghost.approval.state!=='approved'||ghost.manifest.mail!==true||!ghost.manifest.mailReader)return [];
   if(!ghost.manifest.tools?.some(tool=>tool.name===ghost.manifest.mailReader.tool))return [];
   return [{id:installedGhostStoragePart(ghost),namespace:ghost.namespace,name:ghost.manifest.name,enabled:true,
    ready:assessment(installedGhostStoragePart(ghost)).state==='ready',reader:ghost.manifest.mailReader}];
  }),
  authorize:async sessionId=>{const snapshot=await host('session-snapshot',{sessionId});if(!snapshot||snapshot.remoteHostId!=null||snapshot.sharedTask===true)throw Error('MAIL_SESSION_UNAVAILABLE');},
  captureScope:()=>{const scope=ownerScope.capture();return ()=>ownerScope.isCurrent(scope)&&ownerScope.isStable();},
  call:async(plugin,sessionId,url,opened)=>{
   const ghost=find(plugin.id);if(!ghost||!current(ghost))return {ok:false};
   const id=installedGhostStoragePart(ghost),callId=randomUUID();
   if(opened)cards.registerCall(callId,{ghostId:id,sessionId,toolUseId:null,logicalGhostId:pluginStoragePart(installedGhostLogicalIdentity(ghost)),remoteHostId:null});
   try{
    const result=await dispatcher.callGhostTool({ghostId:id,tool:plugin.reader.tool,args:{url},sessionId,callId,...(opened?{mailReaderOpened:true}:{})});
    if(!opened||!result.ok||!current(ghost))return result;
    const draftCallId=await service.resolveLink(id,sessionId,callId,result.result?.accountId,result.result?.draftId,()=>current(ghost));
    return {...result,...(draftCallId?{draftCallId}:{})};
   }finally{if(opened)cards.finalizeCall(callId);}
  },
 });
 return {service,preview:({sessionId,url,read,opened=false,providerId})=>reader(sessionId,url,read,opened,providerId),
  request:(ghost,message)=>{
   const id=installedGhostStoragePart(ghost);if(!ghost.enabled||ghost.manifest.mail!==true)return {ok:false};
   return message.type==='mail-draft-read'?service.restore(id,message,()=>current(ghost)):service.handle(id,message,()=>current(ghost));
  }};
}
