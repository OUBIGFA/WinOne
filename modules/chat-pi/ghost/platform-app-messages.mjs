// Async Go/PiInbox seam for Cindy's App message contract. Reuse the original
// command escaping, authored history, external provenance and Main-only guard.
// There is no second queue and no synchronous enqueue pretending IPC accepted.
import {randomUUID} from 'node:crypto';
import {appMessageInput, MCP_APP_MESSAGE_GUARD, MCP_APP_MESSAGE_CLIENT_PREFIX, queuedAppMessageGuard} from '../cindy/ghost/apps/desktop/src/main/mcp-apps/message.ts';

const unavailable = () => Error('MCP_APP_CARD_UNAVAILABLE');
const key = (sessionId, requestId) => JSON.stringify([sessionId, requestId]);

export function createNativeAppMessageSender({host}) {
  // Only actual closures live here. Nothing serialized, restored from history,
  // or supplied by the App can recreate the source Symbol's positive grant.
  const guards = new Map();
  let closed = false;
  async function send(sessionId, record, content, assertCurrent, files) {
    if (closed || guards.size >= 256) throw Error('MCP_APP_MESSAGE_LIMIT');
    await assertCurrent();
    const snapshot = await host('session-snapshot', {sessionId});
    if (!snapshot || snapshot.remoteHostId != null || snapshot.archived === true) throw unavailable();
    await assertCurrent();
    if (closed) throw unavailable();
    const input = appMessageInput(content);
    const plugin = record.tool._meta?.['cindy/plugin'];
    const presentationSource = {pluginId: `mcp-app:${record.serverId}`, name: typeof plugin?.name === 'string' ? plugin.name : record.serverId};
    const clientId = MCP_APP_MESSAGE_CLIENT_PREFIX + randomUUID();
    const guardKey = key(sessionId, clientId);
    guards.set(guardKey, queuedAppMessageGuard({clientId, sourcePlugin: presentationSource, [MCP_APP_MESSAGE_GUARD]: assertCurrent}));
    try {
      const receipt = await host('task', {op: 'mcp-message', sessionId, clientId, text: input.text,
        authoredText: input.persistedContent, presentationSource,
        expectedEpoch: snapshot.mcpAppEpoch ?? snapshot.clearedAt ?? null,
        ...(files?.length ? {files} : {})});
      // The current NativePiInbox/DispatchPresentedInput owns queue/dispatch and
      // checks ExpectedMcpAppEpoch in its session gate. Only its real receipt
      // confirms acceptance; an emitted request or {ok:true} is not sufficient.
      if (receipt?.accepted !== true) throw Error('MCP_APP_MESSAGE_NOT_ACCEPTED');
    } catch (error) {guards.delete(guardKey); throw error;}
  }
  async function assert({sessionId, requestId}) {
    const guard = !closed ? guards.get(key(sessionId, requestId)) : undefined;
    if (guard) {await guard(); return;}
    // Reuse the original denial for JSON/crash-restored and legacy App rows.
    await queuedAppMessageGuard({clientId: requestId ?? '', sourcePlugin: {pluginId: 'mcp-app:unrestorable'}})();
  }
  return {
    send, assert,
    settled({sessionId, requestId}) {guards.delete(key(sessionId, requestId));},
    cancelSession(sessionId) {for (const id of guards.keys()) if (JSON.parse(id)[0] === sessionId) guards.delete(id);},
    close() {closed = true; guards.clear();},
  };
}
