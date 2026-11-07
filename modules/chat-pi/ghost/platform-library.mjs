// Persistent Library rules, staging, migration and download receipts are Cindy's
// real implementations. Only SQLite and trusted file/browser UI are host seams.
import fs from 'node:fs';
import path from 'node:path';
import {randomUUID} from 'node:crypto';
import {Readable} from 'node:stream';
import {fileURLToPath} from 'node:url';
import {GhostLibrarySlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/librarySlot.ts';
import {LibraryBindingStore} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/libraryBinding.ts';
import {LibraryStagingStore} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/libraryStaging.ts';
import {LibraryVault} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/libraryVault.ts';
import {LibrarySqlService} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/librarySqlService.ts';
import {importPickedFiles} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/pickedFiles.ts';
import {PluginDownloadSlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/downloadSlot.ts';
import {createDownloader} from '../cindy/ghost/apps/desktop/src/main/downloader/index.ts';
import {migrateGhostLibrary} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/libraryMigrate.ts';
import {ghostInstallApprovalToken} from '../cindy/ghost/apps/desktop/src/shared/ghost.ts';
import {installedGhostStoragePart} from '../cindy/ghost/apps/desktop/src/shared/pluginIdentity.ts';
import {ghostFileMime} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/runtime/ghostFiles.ts';
import {BunSqliteDatabase} from './library-sqlite.mjs';
export {BunSqliteDatabase} from './library-sqlite.mjs';

export function createGhostLibrary({root,find,current,sendToGhost,showSave,host,log,mediaStore,stopNode}) {
  let closed=false;
  const inRoot=(...parts)=>path.join(root,...parts);
  const bindingDeps={getFile:()=>inRoot('libraries-binding.json'),getManagedRoots:()=>[root],getDefaultRoot:id=>inRoot('libraries',id),log};
  const bindingStore=new LibraryBindingStore(bindingDeps);
  const getDiskFreeBytes=async directory=>{const stats=fs.statfsSync(directory);return Number(stats.bavail)*Number(stats.bsize);};
  const slot=new GhostLibrarySlot({
    getGhost:id=>closed?null:find(id),bindingStore,getDefaultRoot:bindingDeps.getDefaultRoot,getStagingRoot:id=>inRoot('library-staging',id),
    createStagingStore:deps=>new LibraryStagingStore(deps),captureMutationOwner:()=>closed?null:'local',beginMutation:()=>()=>{},captureOwnerScope:()=>closed?null:'local',
    createVault:deps=>new LibraryVault(deps),createSqlService:deps=>new LibrarySqlService(deps),getDiskFreeBytes,
    workerScriptPath:()=>process.env.WINONE_GHOST_LIBRARY_WORKER??fileURLToPath(new URL('./libraryDbWorker.js',import.meta.url)),
    betterSqliteModulePath:()=>process.env.WINONE_GHOST_LIBRARY_SQLITE??fileURLToPath(new URL('./library-sqlite.js',import.meta.url)),log,
    syncAgentReadonlyExtraDir:async(ghostId,libraryRoot)=>host('library-read-root',{ghostId,root:libraryRoot}),
    showItemInFolder:absPath=>{void host('show-item',{path:absPath}).catch(error=>log.warn(error.message));},
    showSaveDialog:async options=>{const picked=await showSave({ghostName:options.ghostName,defaultPath:options.defaultPath});return picked?.path?{canceled:false,filePath:picked.path}:{canceled:true};},
    writeClipboardPng:async bytes=>{
      // Preserve the original 20 MB PNG contract without putting base64 in
      // the bounded Host/native UI protocol. The private file lives only until
      // the real clipboard acknowledgement or cancellation; it grants no files.
      const directory=inRoot('clipboard-staging');await fs.promises.mkdir(directory,{recursive:true,mode:0o700});
      const file=path.join(directory,randomUUID()+'.png');
      await fs.promises.writeFile(file,bytes,{flag:'wx',mode:0o600});
      try{
        if(closed)throw Error('Library owner is closed');
        const receipt=await host('clipboard-png',{path:file});
        if(receipt?.copied!==true)throw Error(receipt?.message??'Clipboard write was not confirmed');
      }finally{await fs.promises.rm(file,{force:true});}
    },
  });
  // One receipt/lifecycle namespace per plugin, sharing the real downloader's
  // queue. Stopping A must not revoke B's active downloads or cached tokens.
  const downloader=createDownloader(), downloadSlots=new Map();
  const downloadSlot=id=>{
    let item=downloadSlots.get(id);
    if(!item){item=new PluginDownloadSlot({getGhost:ghostId=>closed?null:find(ghostId),root:ghostId=>inRoot('plugin-downloads',ghostId),scope:()=> 'local',send:sendToGhost,download:downloader});downloadSlots.set(id,item);}
    return item;
  };
  const isCurrent=ghost=>!closed&&current(ghost);
  const capture=id=>{const ghost=find(id);return ()=>!!ghost&&isCurrent(ghost);};

  async function readFile(id,relative,signal,range) {
    signal?.throwIfAborted();
    const active=capture(id);
    const absolute=await slot.resolvePanelFilePath(id,relative);
    if(!absolute||!active())return new Response(null,{status:404});
    const info=await fs.promises.stat(absolute);
    if(!active())return new Response(null,{status:410});
    const headers={'Content-Type':ghostFileMime(absolute),'Accept-Ranges':'bytes','Content-Length':String(info.size)};
    let start=0,end=info.size-1,status=200;
    if(range){
      const match=/^bytes=(\d*)-(\d*)$/.exec(range);
      if(!match||(!match[1]&&!match[2]))return new Response(null,{status:416,headers:{'Content-Range':`bytes */${info.size}`}});
      if(!match[1])start=Math.max(0,info.size-Number(match[2]));else start=Number(match[1]);
      if(match[1]&&match[2])end=Math.min(end,Number(match[2]));
      if(!Number.isSafeInteger(start)||!Number.isSafeInteger(end)||start<0||start>end||start>=info.size)return new Response(null,{status:416,headers:{'Content-Range':`bytes */${info.size}`}});
      status=206;headers['Content-Range']=`bytes ${start}-${end}/${info.size}`;headers['Content-Length']=String(end-start+1);
    }
    const stream=fs.createReadStream(absolute,{start,...(info.size?{end}:{}),signal});
    return new Response(Readable.toWeb(stream),{status,headers});
  }

  const result={
    request:(id,message)=>slot.handleLibraryRequest(id,message),
    download:(id,message)=>downloadSlot(id).handle(id,message,capture(id)),
    withNodeDownloads:(id,message,run)=>downloadSlot(id).withNodeDownloads(id,message,run,capture(id)),
    readFile,saveStream:(id,input)=>slot.importStream(id,input),
    resolvePanelFilePath:(id,relative)=>slot.resolvePanelFilePath(id,relative),
    fileSize:async(id,relative)=>{const active=capture(id);const file=await slot.resolvePanelFilePath(id,relative);return file&&active()?(await fs.promises.stat(file)).size:undefined;},
    endpoint:async(ghost,route,req,res)=>{
      if(!route.startsWith('/library/'))return false;
      if(req.method!=='GET'){res.writeHead(405);res.end();return true;}
      let relative;try{relative=decodeURIComponent(route.slice('/library/'.length));}catch{res.writeHead(400);res.end();return true;}
      const controller=new AbortController();res.on('close',()=>controller.abort());
      const response=await readFile(installedGhostStoragePart(ghost),relative,controller.signal,req.headers.range);
      res.writeHead(response.status,Object.fromEntries(response.headers));
      if(response.body)Readable.fromWeb(response.body).on('error',()=>res.destroy()).pipe(res);else res.end();
      return true;
    },
    importPicked:grantMedia=>(id,paths,input)=>importPickedFiles({
      tempRoot:inRoot('picked-temp'),usageBytes:id=>mediaStore.usage(id),importLibrary:(ghostId,options)=>slot.importStream(ghostId,options),
      deposit:async(ghostId,filePath,mimeType,assertCurrent)=>{const {ingestMedia}=await import('./seams/media.ts');assertCurrent();const written=await ingestMedia({filePath,mimeType,assertStillValid:assertCurrent});await grantMedia(ghostId,written.hash);return written;},
    },id,paths,input),
    action:async({pluginId,action,target})=>{
      if(action==='status')return {...await slot.handleLibraryRequest(pluginId,{op:'status'}),binding:await bindingStore.resolveLibraryRoot(pluginId)};
      if(!['relocate','rebind'].includes(action))throw Object.assign(Error('Unknown library action'),{code:'INVALID_PARAMS'});
      const ghost=find(pluginId);if(!ghost?.manifest.library||!isCurrent(ghost))throw Object.assign(Error('Library unavailable'),{code:'PLUGIN_UNAVAILABLE'});
      const token=ghostInstallApprovalToken(ghost.approval);
      const assertCurrent=()=>{if(!isCurrent(ghost)||ghostInstallApprovalToken(find(pluginId)?.approval)!==token)throw Error('Library installation changed');};
      const candidate=target==='default'?path.dirname(bindingDeps.getDefaultRoot(pluginId)):target;
      if(typeof candidate!=='string'||!path.isAbsolute(candidate))throw Object.assign(Error('Library target must be absolute'),{code:'INVALID_PARAMS'});
      const answer=await host('ui',{kind:'confirm',ghostName:ghost.manifest.name,body:candidate,trusted:true});
      assertCurrent();if(answer?.confirmed!==true)return {ok:false,reason:'cancelled'};
      slot.setRelocating(pluginId,true);
      try {
        await stopNode(pluginId);await slot.disposeGhost(pluginId);assertCurrent();
        if(action==='rebind'){const bound=await bindingStore.setBinding(pluginId,candidate,getDiskFreeBytes);assertCurrent();return bound;}
        const resolution=await bindingStore.resolveLibraryRoot(pluginId);
        if(!resolution.root)return {ok:false,phase:'precheck',message:'Library location unavailable; reconnect the original disk or rebind explicitly'};
        if(path.resolve(resolution.root)===path.resolve(path.join(candidate,pluginId)))return {ok:true,unchanged:true};
        return await migrateGhostLibrary({ghostId:pluginId,fromRoot:resolution.root,candidate,allowInsideManagedRoot:target==='default',deps:{...bindingDeps,getDiskFreeBytes,
          copySqlite:async(from,to)=>{const db=new BunSqliteDatabase(from,{readonly:true});try{await db.backup(to);}finally{db.close();}},
          checkSqliteHealthy:async file=>{const db=new BunSqliteDatabase(file,{readonly:true});try{return db.pragma('quick_check').every(row=>row.quick_check==='ok');}finally{db.close();}},
        },applyBinding:async location=>{assertCurrent();if(target==='default'){await bindingStore.removeBinding(pluginId);return {ok:true};}const bound=await bindingStore.setBinding(pluginId,location,getDiskFreeBytes);assertCurrent();return bound;}});
      } finally {slot.setRelocating(pluginId,false);}
    },
    async stop(id){const item=downloadSlots.get(id);if(item){await item.stopAndWait();if(downloadSlots.get(id)===item)downloadSlots.delete(id);}await slot.invalidateGhost(id);},
    async remove(id){await downloadSlot(id).removePlugin(id);await slot.disposeGhost(id);},
    async close(){closed=true;await Promise.all([...downloadSlots.values()].map(item=>item.stopAndWait()));await slot.disposeAll();},
  };
  return result;
}
