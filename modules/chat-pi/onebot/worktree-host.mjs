// Electron's app paths are scoped to this OneBot owner, including legacy lease
// discovery. A WinOne worker must never read Cindy's real profile or lease store.
import fs from 'node:fs/promises';
import path from 'node:path';
import {createCipheriv,createDecipheriv,randomBytes} from 'node:crypto';
import {oneBotOwnerScope} from './im-scope.mjs';

export const app = {
  isPackaged: true,
  getPath(name) {
    const root = oneBotOwnerScope().paths.root;
    if (name === 'userData') return root;
    if (name === 'appData') return path.join(root, 'runtime');
    throw Error('Unsupported worktree application path: ' + name);
  },
};
// Cindy's probe scheduler already bounds concurrent directory probes. There is
// no separate Electron utility process in the Bun worker.
export const realpathWorkingDirectory = directory => fs.realpath(directory);

// WinOne never borrows an Electron/Chromium profile from an older Cindy
// instance. Native Host ownership and the original per-resource runtime leases
// are its barriers; loss of this captured owner keeps the resource intact.
export async function withLegacyWorktreeRuntimeGuard(task) {
  const owner=oneBotOwnerScope();owner.paths.assertPath();
  return task(()=>!owner.closed);
}

const recoverySecrets=new Map();
export function configureWorktreeRecoverySecrets(root,secrets) {
  const entry={secrets,key:null};recoverySecrets.set(path.resolve(root),entry);
  return ()=>{if(recoverySecrets.get(path.resolve(root))===entry)recoverySecrets.delete(path.resolve(root));entry.key?.fill(0);};
}
function recoveryKey() {
  const root=oneBotOwnerScope().paths.root,entry=recoverySecrets.get(path.resolve(root));
  if(!entry?.secrets.isAvailable())throw Error('Worktree recovery encryption is unavailable');
  if(!entry.key){
    const name='worktree-recovery-key-v1',saved=entry.secrets.readResult(name);
    if(saved.kind==='error')throw Error('Worktree recovery key could not be read');
    const key=saved.kind==='value'?Buffer.from(saved.value,'base64'):randomBytes(32);
    if(key.length!==32)throw Error('Invalid worktree recovery key');
    if(saved.kind==='missing'&&!entry.secrets.write(name,key.toString('base64'))){key.fill(0);throw Error('Worktree recovery key could not be saved');}
    entry.key=key;
  }
  return {key:entry.key,aad:Buffer.from('winone.worktree.recovery.v1:'+path.resolve(root))};
}
// Reuse the existing private, DPAPI-backed Host credential plane. Keys are not
// portable or exposed to a plugin, and archive authentication precedes restore.
export const safeStorage={
  encryptString(text){
    const {key,aad}=recoveryKey(),nonce=randomBytes(12),cipher=createCipheriv('aes-256-gcm',key,nonce);cipher.setAAD(aad);
    const bytes=Buffer.concat([cipher.update(text,'utf8'),cipher.final()]);
    return Buffer.concat([Buffer.from([1]),nonce,cipher.getAuthTag(),bytes]);
  },
  decryptString(bytes){
    if(bytes.length<29||bytes[0]!==1)throw Error('Invalid worktree recovery key envelope');
    const {key,aad}=recoveryKey(),decipher=createDecipheriv('aes-256-gcm',key,bytes.subarray(1,13));decipher.setAAD(aad);decipher.setAuthTag(bytes.subarray(13,29));
    return Buffer.concat([decipher.update(bytes.subarray(29)),decipher.final()]).toString('utf8');
  },
};
