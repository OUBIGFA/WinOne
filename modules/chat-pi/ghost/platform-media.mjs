// One persistent ownership/quota ledger for plugin media. Bytes alone, or a
// remembered hash in an in-process Set, are never evidence of permission.
import fs from 'node:fs/promises';
import * as blobs from '../cindy/ghost/apps/desktop/src/main/cindy-media/blobStore.ts';
import * as ledger from '../cindy/ghost/apps/desktop/src/main/cindy-media/ledger.ts';
import {ingestMedia} from '../cindy/ghost/apps/desktop/src/main/cindy-media/ingest.ts';
import {captureMediaRefCompensationScope} from '../cindy/ghost/apps/desktop/src/main/cindy-media/refCompensationJournal.ts';
import {recordGhostCallMedia} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostMediaLedger.ts';

export function createGhostMediaStore({database,find,current,callInfo}){
 let closed=false;
 const operations=new Set();
 const db=()=>database().drizzle;
 const guardFor=id=>{const captured=find(id);return()=>{if(closed||!captured||!current(captured))throw Object.assign(Error('Plugin media authority changed'),{code:'PERMISSION_DENIED'});};};
 const owned=async(id,hash)=>{
  if(!await ledger.ghostCanRead(hash,id,db()))return null;
  const info=await ledger.getBlobInfo(hash,db());
  if(!info)return null;
  const resolved=blobs.resolveHashRef(hash,info.ext);
  return resolved?{...resolved,...info}:null;
 };
 async function write(id,source,ref){
  // A model call already admitted by Cindy may finish after the plugin is
  // disabled. Preserve that paid result for the same data owner, as upstream
  // does; only owner teardown invalidates persistence, not the UI toggle.
  const guard=()=>{if(closed)throw Error('Plugin media owner is closing');};
  guard();
  return ingestMedia({...source,isCache:false,refs:[{refId:id,...ref}],assertStillValid:guard,refCompensationScope:captureMediaRefCompensationScope('local')},db());
 }
 const api = {
  async generated(params){
   const {ghostId,buffer,mimeType,label,callId,ownerScopeKey}=params;
   if(ownerScopeKey!==undefined&&ownerScopeKey!=='local')throw Error('Plugin media owner changed');
   const call=callId?callInfo(callId):null;
   const written=await write(ghostId,{buffer,mimeType},{refKind:'ghost-gallery',originKind:'ghost',originId:ghostId,label,
    ...(call?.ghostId===ghostId&&call.sessionId?{originSessionId:call.sessionId}:{})});
   if(call?.ghostId===ghostId)recordGhostCallMedia(ghostId,callId,written.url);
   return written;
  },
  deposit:({ghostId,buffer,mimeType,label})=>write(ghostId,{buffer,mimeType},{refKind:'ghost-deposit',originKind:'user',label}),
  usage:id=>ledger.ghostDepositUsageBytes(id,db()),
  release:({ghostId,hash})=>ledger.removeGhostDepositRef({ghostId,hash},db()),
  handoff:(id,source,sessionId)=>write(id,source,{refKind:'ghost-tool-grant',originKind:'tool',originId:'attachment-handoff',...(sessionId?{originSessionId:sessionId}:{})}),
  async grant(id,hash,sessionId){
   const guard=guardFor(id);guard();
   if(!await ledger.getBlobInfo(hash,db()))throw Error('Media is not registered');guard();
   if(!await ledger.hasRef({hash,refKind:'ghost-tool-grant',refId:id},db())){
    await ledger.addRef({hash,refKind:'ghost-tool-grant',refId:id,originKind:'tool',...(sessionId?{originSessionId:sessionId}:{})},db());
   }
   guard();
  },
  async canHandOff(id,hash,sessionId){return await ledger.ghostCanRead(hash,id,db())||!!sessionId&&await ledger.sessionCanRead(hash,sessionId,db());},
  async resolve(id,hash){return (await owned(id,hash))?.absPath??null;},
  async read(id,hash){const value=await owned(id,hash);return value?{buffer:await fs.readFile(value.absPath),mimeType:value.mimeType}:null;},
  async deliver(id,sessionId,text){
   // Only media actually returned by its owning plugin can become a task
   // attachment. An arbitrary hash quoted by another plugin grants nothing.
   const urls=new Set(text.match(/cindy-media:\/\/blobs\/[a-f0-9]{64}\.[a-z0-9]+/g)??[]);
   if(!urls.size)return;
   await ledger.withSessionMediaRefLock(db(),sessionId,async()=>{
    for(const url of urls){
     const parsed=blobs.parseBlobUrl(url);
     if(!parsed||!await ledger.ghostCanRead(parsed.hash,id,db()))continue;
     if(!await ledger.hasRef({hash:parsed.hash,refKind:'session-attachment',refId:sessionId},db()))await ledger.addRef({hash:parsed.hash,refKind:'session-attachment',refId:sessionId,originSessionId:sessionId,originKind:'tool'},db());
    }
   });
  },
  // Uninstall releases only the plugin-owned deposit references. Its produced
  // gallery and already delivered task media retain their original ownership.
  removeDeposits:id=>ledger.removeRefs({refKind:'ghost-deposit',refId:id},db()),
 };
 return {...Object.fromEntries(Object.entries(api).map(([name,fn])=>[name,(...args)=>{
  if(closed)return Promise.reject(Error('Plugin media store is closing'));
  const work=Promise.resolve().then(()=>fn(...args));operations.add(work);
  void work.then(()=>operations.delete(work),()=>operations.delete(work));return work;
 }])),async close(){closed=true;await Promise.allSettled([...operations]);}};
}
