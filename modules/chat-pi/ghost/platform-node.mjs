// A plugin's bundled Node services and stdio MCP servers. The broker's rules are
// unchanged; a worker is a Bun child of this host (Node APIs, no Electron), reached
// through the bootstrap Cindy's utilityProcess runs (see seams/electron.ts).
import {GhostNodeRuntimeBroker} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/nodeRuntimeBroker.ts';

export function createGhostNode({find, dispatcher, secrets, secretName, accounts, network, sendToGhost, ownerScope, log, ui, changeBus}) {
  const broker = new GhostNodeRuntimeBroker({
    getGhost: find, startBlockedReason: () => null,
    getCallSignal: (id, callId) => dispatcher.getPendingCallSignal(id, callId),
    getCallSessionId: (id, callId) => dispatcher.getPendingCallSessionId(id, callId),
    openSecretInput: async input => {
      // The broker supplies method/entry, not a single secret key. Bindings are
      // conjunctive and must be read from the current approved manifest.
      input.assertCurrent();
      const ghost = find(input.ghostId);
      const bindings = (ghost?.manifest.node?.secretBindings ?? []).filter(binding => !binding.oauthSecret && binding.methods.includes(input.method) && (binding.entry ?? ghost.manifest.node.entry) === input.entry);
      if (!bindings.length) return false;
      for (const binding of bindings) {
        input.assertCurrent();
        input.signal.throwIfAborted();
        if (!input.force && secrets.read(secretName(input.ghostId, binding.key)) !== null) continue;
        const answer = await ui('secret', {ghostId: input.ghostId, ghostName: ghost.manifest.name, sessionId: input.sessionId, key: binding.key, label: binding.label});
        input.assertCurrent();
        input.signal.throwIfAborted();
        if (!answer?.value || !secrets.write(secretName(input.ghostId, binding.key), answer.value)) return false;
        changeBus.emit(input.ghostId, {source: 'secret', ref: binding.key});
      }
      return true;
    },
    openDeviceAuthorization: input => accounts.deviceAuthorization(input),
    openAuthorization: input => accounts.pluginAuthorization(input),
    ownerScope,
    readSecret: (id, key) => secrets.read(secretName(id, key)),
    secretSaved: (id, key) => secrets.read(secretName(id, key)) !== null,
    authenticatedFetch: (id, request, authorization, assertCurrent, signal) => network.handleAuthenticatedNodeFetch(id, request, authorization, assertCurrent, signal),
    resolveOauthSecret: (id, key, accountId) => accounts.freshToken(id, key, accountId),
    sendToGhost: (id, payload) => { sendToGhost(id, payload); },
    getStartAttemptContext: () => ({observedMainWindowState: 'unknown', observedScreenState: 'unknown'}),
    log,
  });
  return {request: (id, message) => broker.handleRequest(id, message), start: ghost => broker.startResident(ghost), stop: id => broker.stopAndWait(id), close() { broker.destroyAll(); }};
}
