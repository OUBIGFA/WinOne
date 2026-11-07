// Existing helper callers use the same real Cindy media ingest/ledger as the
// model and network slots. The Host supplies one stable owner-local database.
import * as blobStore from '../../cindy/ghost/apps/desktop/src/main/cindy-media/blobStore.ts';
import {ingestMedia as ingestOriginal, type IngestRef} from '../../cindy/ghost/apps/desktop/src/main/cindy-media/ingest.ts';
import {captureMediaRefCompensationScope} from '../../cindy/ghost/apps/desktop/src/main/cindy-media/refCompensationJournal.ts';
import {getDbClient} from './local-db.ts';

export const {blobUrl, extForMime, supportedMime} = blobStore;
export async function ingestMedia(params: blobStore.BlobSource & {mimeType: string; refs?: IngestRef[]; assertStillValid?: () => void}) {
  const refs = params.refs ?? [];
  return ingestOriginal({...params, refs,
    ...(params.assertStillValid && refs.length ? {refCompensationScope: captureMediaRefCompensationScope('local')} : {}),
  }, getDbClient().drizzle);
}
