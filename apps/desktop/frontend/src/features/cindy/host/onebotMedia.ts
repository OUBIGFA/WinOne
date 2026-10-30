// WinOne seam for Cindy shared/remoteMediaUrl in the OneBot pages (Cindy
// b7e46bc, Apache-2.0). A teammate avatar is a Cindy-managed image address
// (`cindy-media://blobs/<sha256>.<ext>`) whose bytes the OneBot Host stores.
// WebView2 cannot load that scheme, so the bytes are read once through the
// Host and shown as a data URL. Other computers were removed with Cindy cloud,
// so a remote media origin never applies.
import {chatCall} from '../../../bridge';
import {isManagedBotAvatarUrl} from '../shared/botAvatarValue';
export {isRemoteMediaUrl, parseRemoteMediaUrl, type RemoteMediaOrigin} from '../media/shared/remoteMediaUrl';

const dataUrls = new Map<string, string>();
const pending = new Map<string, Promise<void>>();
/** Bytes read through the bounded group attachment path, kept under their stable address. */
export function registerManagedMedia(url: string, mimeType: string, base64: string) {dataUrls.set(url, 'data:' + mimeType + ';base64,' + base64);}
export function hasManagedMedia(url: string) {return dataUrls.has(url);}

/** Read a managed image before a view that shows it renders; other values are ignored. */
export async function cacheManagedMedia(value: unknown): Promise<void> {
  if (!isManagedBotAvatarUrl(value) || dataUrls.has(value)) return;
  let request = pending.get(value);
  if (!request) {
    request = chatCall<{base64: string; mimeType: string}>('onebot.media.read', {url: value})
      .then(image => { dataUrls.set(value, `data:${image.mimeType};base64,${image.base64}`); })
      // A missing blob shows the teammate's initial, as upstream does when an image fails to load.
      .catch(() => undefined)
      .finally(() => { pending.delete(value); });
    pending.set(value, request);
  }
  await request;
}

/** The displayable address of a managed image that was read; any other value is returned unchanged. */
export function rewriteToRemoteMediaOrigin(url: string, _origin?: unknown): string {
  return dataUrls.get(url) ?? url;
}
