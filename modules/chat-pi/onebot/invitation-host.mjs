// Host facts for Cindy's unchanged invitation worker (maker-ipc/botInvitation.ts,
// Cindy b7e46bc). The private Host bridge supplies the auxiliary model and the
// opening message. Portraits reuse the configured Magpie image service.
import {oneBotOwnerScope} from './im-scope.mjs';
import {oneBotError} from './content-service.mjs';
import {writeBotProfileFolder as writeProfileFolder} from '../cindy/bots/botProfileFolder.ts';
import {withBotProfileLocks} from '../cindy/bots/botProfileLock.ts';
export {patchMessageAgentMetaWithResult} from './learning-host.mjs';
/** A reply's learning receipts changed: the page rereads that conversation's receipts. */
export async function broadcastMessageAgentMetaUpdate(sessionId, clientId) {
  oneBotOwnerScope().event({type: 'message-meta-changed', payload: {sessionId, clientId}});
}

// Invitation preparation shares the content service's Home mutex. On Windows
// a settings read must not keep SOUL.md open while preparation replaces it.
export function writeBotProfileFolder(directory, botId, patch) {
  const owner = oneBotOwnerScope();
  if (directory !== owner.paths.root) throw oneBotError('INVALID_PATH', '伙伴准备目录不属于当前数据目录');
  return withBotProfileLocks([owner.paths.home(botId)], async () => {
    owner.paths.assertPath(owner.paths.home(botId));
    await writeProfileFolder(directory, botId, patch);
  });
}

// maker-host/newMakerDefaultsCache.ts: the Host reports AI service changes, and
// an invitation waiting for a model rechecks then (and on its own 5 s timer).
const defaultListeners = new Set();
export function subscribeNewMakerDefaults(listener) {
  defaultListeners.add(listener);
  return () => { defaultListeners.delete(listener); };
}
export function notifyNewMakerDefaultsChanged() {
  for (const listener of [...defaultListeners]) listener();
}

export async function prepareBotInvitationAvatar(...args){return (await import('../cindy/import/main/maker-ipc/botInvitationAvatar.ts')).prepareBotInvitationAvatar(...args);}
export async function finishBotInvitationAvatar(...args){return (await import('../cindy/import/main/maker-ipc/botInvitationAvatar.ts')).finishBotInvitationAvatar(...args);}

// maker-host/auxiliary-title-one-shot.ts: one bounded model call on the Host's
// auxiliary route (Chat's title model, then the teammate's own route).
export async function requestAuxiliaryText(request) {
  const host = oneBotOwnerScope().host;
  if (!host) return {ok: false, reason: 'host_unavailable'};
  return host.auxiliaryText(request);
}

// localDb/ipc/messages.ts: a host-written teammate message in its chat.
export async function createMessage(sessionId, row, options) {
  const host = oneBotOwnerScope().host;
  if (!host) throw oneBotError('NOT_READY', 'OneBot 会话尚未就绪');
  return host.createMessage(sessionId, row, {requireNoUserMessage: options?.requireNoUserMessage === true});
}

// device-link/broadcast-tap.ts: WinOne has one data owner and no other device.
export function captureDataOwnerBroadcastScope() { return undefined; }
