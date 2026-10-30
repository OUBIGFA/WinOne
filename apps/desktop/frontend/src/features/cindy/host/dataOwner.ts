// WinOne seam for Cindy contexts/dataOwnerGeneration (b7e46bc). WinOne has one
// local data owner and no account switching, so every push is current. This
// matches Cindy's own rule that an absent owner stamp is current.
export interface DataOwnerGeneration {
  readonly dataOwnerId: string | null;
  readonly generation: number;
}
// The data owner is fixed for this desktop process. Request generations remain
// component-owned, so a stale capability response still cannot replace a new one.
// The id matches useAuth, so owner-keyed local caches are enabled.
const LOCAL_OWNER: DataOwnerGeneration = Object.freeze({dataOwnerId: 'winone-local', generation: 0});

export function isDataOwnerPushCurrent(_ownerStamp: unknown): boolean {
  return true;
}
export function getDataOwnerGeneration(): DataOwnerGeneration { return LOCAL_OWNER; }
export function isDataOwnerGenerationCurrent(owner: DataOwnerGeneration): boolean { return owner === LOCAL_OWNER; }
export function isDataOwnerIdCurrent(owner: DataOwnerGeneration): boolean { return owner.dataOwnerId === LOCAL_OWNER.dataOwnerId; }
