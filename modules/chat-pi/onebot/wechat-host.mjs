// Database/media facts for Cindy's unmodified personal-WeChat state machine.
import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {oneBotOwnerScope} from './im-scope.mjs';
export * from './import-core.mjs';
export {createLogger} from './logger.ts';
export {hasSessionPermissionUpdates} from '../cindy/wechat/permissionUpdates.ts';
export {isExpiredPermissionDecision} from '../cindy/wechat/expiredPermission.ts';
export const maskPath=()=>'<local path>';
export const getDbClient=()=>oneBotOwnerScope().taskStore;
export const tryGetDbClient=()=>getDbClient();
export const getCurrentDbClientSnapshot=getDbClient;
const extensions={'image/png':'png','image/jpeg':'jpg','image/webp':'webp','image/gif':'gif','audio/wav':'wav','audio/mpeg':'mp3','video/mp4':'mp4'};
export function resolveSafe(url){const match=/^cindy-media:\/\/blobs\/([a-f0-9]{64})\.([a-z0-9]+)$/.exec(url);if(!match||!Object.values(extensions).includes(match[2]))throw Error('INVALID_MEDIA');const absPath=path.join(oneBotOwnerScope().paths.root,'media','blobs',match[1]+'.'+match[2]);oneBotOwnerScope().paths.assertPath(absPath);return {absPath};}
export async function ingestMedia(input){
 const scope=oneBotOwnerScope(),buffer=Buffer.from(input.buffer),ext=extensions[input.mimeType];
 if(!ext||buffer.length>32*1024*1024)throw Error('INVALID_MEDIA');
 const hash=createHash('sha256').update(buffer).digest('hex'),url='cindy-media://blobs/'+hash+'.'+ext,file=resolveSafe(url).absPath;
 fs.mkdirSync(path.dirname(file),{recursive:true});if(!fs.existsSync(file))fs.writeFileSync(file,buffer,{flag:'wx'});
 const now=Date.now();scope.taskStore.exec('INSERT INTO media_blobs(hash,ext,mime_type,bytes,is_cache,created_at,last_access_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(hash) DO NOTHING',[hash,ext,input.mimeType,buffer.length,0,now,now]);
 return {url,hash,ext,mimeType:input.mimeType,bytes:buffer.length};
}

// Cindy 2d8d3c3 im/shared/receivedImageHistory.ts: personal WeChat indexes
// received images into Cindy's IM group history (hook_group_messages) so the
// teammate can look them up later with the group-history tools. WinOne has
// neither that table nor those tools (a gap that predates 2d8d3c3; see
// docs/mygo-parity/onebot.md), so the scope is reported and nothing is indexed.
export function receivedHistoryScope(channel, botId, chatId, threadId = '') {
  const provider = `${channel}-personal:${botId}`;
  return {access: 'lane', provider, lane: {provider, chatId, threadId}};
}
export async function recordReceivedImageMessage(_args) {}
