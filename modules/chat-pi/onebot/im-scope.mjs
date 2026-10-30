// Host facts for Cindy's original per-account IM record store. Each WinOne
// data directory is a local owner scope; there is no Cindy cloud account.
import {AsyncLocalStorage} from 'node:async_hooks';
import path from 'node:path';
import {createLogger} from './logger.ts';
const scope=new AsyncLocalStorage();
export const withIMScope=(owner,run)=>scope.run(owner,run);
function current(){const value=scope.getStore();if(!value||value.closed)throw Error('OneBot IM owner scope is not active');return value;}
export const oneBotOwnerScope=()=>current();
export const activeOwnerScopeKey=()=>current().key;
export const isAppSessionBoundaryPending=()=>!scope.getStore()||scope.getStore().closed;
// One local data owner per WinOne data directory (Cindy getActiveAppSession).
export const getActiveAppSession=()=>({dataOwnerId:isAppSessionBoundaryPending()?null:'winone-local'});
export const ownerScopedImUserDataPath=(...parts)=>{const owner=current();const file=path.join(owner.paths.root,...parts);owner.paths.assertPath(file);return file;};
export const ownerScopedUserDataPath=ownerScopedImUserDataPath;
export const desktopMakerLogger={child:createLogger};
