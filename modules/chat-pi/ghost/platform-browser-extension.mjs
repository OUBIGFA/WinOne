// Native WinOne supplies only its owner directory and confirmation. Cindy's
// original bridge owns pairing, job relay, page control and no-replay outcomes.
import path from 'node:path';
import {GhostBrowserExtensionSlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/browserExtensionSlot.ts';
import {installedGhostStoragePart} from '../cindy/ghost/apps/desktop/src/shared/pluginIdentity.ts';
import {createNativeBinding} from '../cindy/ghost/apps/desktop/src/main/browser-extension/native-binding.ts';

export function createGhostBrowserExtension({root,manager,find,current,ownerScope,sendToGhost,runtime,ui,log,nativeOptions={},createBridge}){
 const owner=ownerScope.capture();
 const slot=new GhostBrowserExtensionSlot({getGhost:find,
  // A starting helper can accept stdin without its SDK/document being ready.
  // Returning false makes the original policy relay use its wake/readiness
  // path rather than acknowledge a premature, lost message.
  sendToGhost:(id,payload)=>runtime.stateOf(id)==='running'&&sendToGhost(id,payload),
  wake:async id=>{const ghost=find(id);return !!ghost&&current(ghost)&&(await runtime.spawn(ghost)).ok&&await runtime.waitUntilRunning(installedGhostStoragePart(ghost));},
  // WinOne has no Cindy public-market provenance; a matching manifest ID must
  // not inherit Cindy's old official registration or unstamped pairings.
  isVerifiedOfficial:()=>false,
  policyDir:()=>path.join(root,'browser-extension'),
  ownerScopeKey:()=>ownerScope.isCurrent(owner)&&ownerScope.isStable()?root:'closed',
  createBridge,
  createNativeBinding:options=>createNativeBinding({...options,...nativeOptions,root:path.join(root,'browser-native')}),
  confirmNativeTakeover:async ghost=>{
   const answer=await ui('confirm',{ghostName:ghost.manifest.name,body:'将切换浏览器原生连接：'+ghost.manifest.browserExtension.nativeHost.name+'。已有其它应用的连接会被替换。',danger:true});
   return answer?.confirmed===true&&current(ghost)&&ownerScope.isCurrent(owner)&&ownerScope.isStable();
  },log});
 slot.reconcile(manager.list());
 return {slot,reconcile:()=>slot.reconcile(manager.list()),request:(id,message)=>slot.handleRequest(id,message),
  stop:id=>slot.stop(id),close:()=>slot.dispose()};
}
