// WinOne has one local data owner, so Cindy's account boundaries never move.
import path from 'node:path';
import {createHash} from 'node:crypto';

let root = '';
export function setGhostDataRoot(value: string): void { root = value; }
export function ghostDataRoot(): string {
  if (!root) throw new Error('Plugin data directory is not set');
  return root;
}

export function ownerScopedUserDataPath(...parts: string[]): string { return path.join(ghostDataRoot(), ...parts); }
export function activeOwnerScopeKey(): string { return 'local'; }
// Same storage-key contract as Cindy; the actual data root remains isolated by Host.
export function dataOwnerStorageKey(ownerId: string): string {
  return createHash('sha256').update(ownerId).digest('hex').slice(0, 20);
}
export function isAppSessionBoundaryPending(): boolean { return false; }
// Device-link setup is deliberately outside this local Host. Direct local
// OAuth and authenticated native forms use the original coordinator instead.
export function supportsRemotePluginOauth(): boolean { return false; }
export function getActiveDataOwnerPushStamp() { return {dataOwnerId: 'local', ownerGeneration: 1}; }
export function getActiveAppSession() { return {mode: 'local', dataOwnerId: 'local', generation: 1}; }
