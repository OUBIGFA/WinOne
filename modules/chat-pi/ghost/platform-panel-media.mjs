// Cindy's original focus/rate/media ownership gates on the same native page
// bindings and persistent media ledger. A private HTTP page URI is normalized
// only against its live sandbox capability, never by trusting an HTML hostname.
import fs from 'node:fs/promises';
import path from 'node:path';
import {Readable} from 'node:stream';
import {GhostPreviewGate,resolveGhostPanelMedia,parseGhostPanelMediaUrl} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/previewGate.ts';
import {installedGhostStoragePart} from '../cindy/ghost/apps/desktop/src/shared/pluginIdentity.ts';
import * as blobs from '../cindy/ghost/apps/desktop/src/main/cindy-media/blobStore.ts';
import * as ledger from '../cindy/ghost/apps/desktop/src/main/cindy-media/ledger.ts';
import {buildRangedFileResponse} from '../cindy/ghost/apps/desktop/src/main/cindy-media/rangeResponse.ts';

export function createGhostPanelMedia({manager,find,current,adapter,runtime,database,pageBinding,push}){
 const db=()=>database().drizzle;
 const deps={ghostCanRead:(hash,id)=>ledger.ghostCanRead(hash,id,db()),getBlobInfo:hash=>ledger.getBlobInfo(hash,db()),
  blobUrl:blobs.blobUrl,blobAbsPath:(hash,ext)=>blobs.resolveHashRef(hash,ext)?.absPath,
  statSize:async file=>(await fs.stat(file)).size};
 const preview=new GhostPreviewGate(deps);
 function source(uri,{pageId,sourcePluginId}={}){
  const binding=pageId?pageBinding(pageId):null;
  if(pageId&&!binding)return null;
  if(binding&&sourcePluginId&&binding.pluginId!==sourcePluginId)return null;
  const explicit=binding?.pluginId??sourcePluginId;
  const candidates=explicit?[find(explicit)]:manager.list();
  for(const ghost of candidates){
   if(!ghost||!current(ghost))continue;
   const instanceId=installedGhostStoragePart(ghost);
   if(uri.startsWith(`cindy-ghost://${ghost.manifest.id}/`))return {ghost,instanceId,uri};
   try{
    const kind=ghost.manifest.panel?'panel':ghost.manifest.mainView?'main':ghost.manifest.taskView?'task':'settings';
    const issued=new URL(adapter.pageUrl(ghost,kind,'native-media'));
    const value=new URL(uri),prefix=issued.pathname.slice(0,issued.pathname.lastIndexOf('/'));
    if(value.origin!==issued.origin||value.search||value.hash||!value.pathname.startsWith(prefix+'/'))continue;
    const relative=value.pathname.slice(prefix.length);
    if(!/^\/(?:media|preview)\/[0-9a-f]{64}\.[a-z0-9]+$/.test(relative))continue;
    return {ghost,instanceId,uri:`cindy-ghost://${ghost.manifest.id}${relative}`};
   }catch{} // No live sandbox capability is not a resolvable panel source.
  }
  return null;
 }
 async function resolve(input){
  if(typeof input.uri!=='string'||!['attach','menu'].includes(input.purpose))throw Error('PANEL_MEDIA_UNAVAILABLE');
  const owner=source(input.uri,input);if(!owner)throw Error('PANEL_MEDIA_UNAVAILABLE');
  const result=await resolveGhostPanelMedia(owner.uri,input.purpose,deps,{ghostId:owner.ghost.manifest.id,instanceId:owner.instanceId});
  if(!result||!current(owner.ghost))throw Error('PANEL_MEDIA_UNAVAILABLE');
  const blob=blobs.resolveSafe(result.url),info=await ledger.getBlobInfo(blob.hash,db());
  if(!info||!current(owner.ghost))throw Error('PANEL_MEDIA_UNAVAILABLE');
  await ledger.touchBlob(blob.hash,db());
  return {path:blob.absPath,url:result.url,uri:owner.uri,pluginId:owner.instanceId,kind:result.kind,mimeType:info.mimeType,size:info.bytes,name:result.name??`${owner.ghost.manifest.id}-${blob.hash.slice(0,8)}${info.ext}`};
 }
 return {resolve,
  async navigation(input){
   const owner=source(input.url,{pageId:input.pageId});
   if(!owner||!owner.uri.includes('/preview/'))return false;
   const result=await preview.request({ghostId:owner.ghost.manifest.id,instanceId:owner.instanceId,url:owner.uri,isPanelFocused:()=>input.focused===true});
   if(result.ok&&current(owner.ghost))push('preview-media',{...result,uri:owner.uri,pluginId:owner.instanceId,pageId:input.pageId});
   return true;
  },
  async endpoint(ghost,route,req,res){
   const id=installedGhostStoragePart(ghost);
   if(route==='/wake'){
    res.setHeader('Content-Type','application/json');
    if(req.method!=='POST'&&req.method!=='GET'){res.writeHead(405);res.end();return true;}
    const result=current(ghost)?await runtime.spawn(ghost):{ok:false};
    res.writeHead(result.ok?200:409);res.end(JSON.stringify({ok:result.ok===true,state:runtime.stateOf(id)}));return true;
   }
   if(route==='/gallery'){
    if(req.method!=='GET'){res.writeHead(405);res.end();return true;}
    const rows=await ledger.listGhostGallery(id,db());
    if(!current(ghost)){res.writeHead(410);res.end();return true;}
    res.writeHead(200,{'Content-Type':'application/json'});
    res.end(JSON.stringify(rows.map(row=>({src:`cindy-ghost://${ghost.manifest.id}/media/${row.hash}${row.ext}`,caption:row.label??''}))));return true;
   }
   const media=/^\/media\/([0-9a-f]{64})\.[a-z0-9]+$/.exec(route);
   if(!media)return false;
   if(!['GET','HEAD'].includes(req.method)){res.writeHead(405);res.end();return true;}
   if(!await ledger.ghostCanRead(media[1],id,db())){res.writeHead(404);res.end();return true;}
   const info=await ledger.getBlobInfo(media[1],db());if(!info||!current(ghost)){res.writeHead(404);res.end();return true;}
   const opened=await blobs.openBlobFile(blobs.blobUrl(media[1],info.ext)),abort=new AbortController();res.on('close',()=>abort.abort());
   const response=await buildRangedFileResponse({...opened,rangeHeader:req.headers.range??null,cacheControl:'private, max-age=31536000, immutable',signal:abort.signal});
   res.writeHead(response.status,Object.fromEntries(response.headers));
   if(req.method==='HEAD'){await response.body?.cancel();res.end();}
   else if(response.body)Readable.fromWeb(response.body).on('error',()=>res.destroy()).pipe(res);else res.end();
   await ledger.touchBlob(media[1],db());return true;
  },
 };
}
