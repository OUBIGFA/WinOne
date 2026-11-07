// Account connection is a Host-owned interaction, not a plugin business tool.
// Reuse Cindy's ordinary coordinator and durable teammate service; credentials
// go straight from the native Password form to the encrypted vault/executor.
import fs from 'node:fs/promises';
import path from 'node:path';
import {randomUUID} from 'node:crypto';
import {GhostSetupCoordinator, toExplicitReauthInteractionAssessment, toReauthInteractionAssessment} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostSetupCoordinator.ts';
import {GhostSetupInteractionBridge, sanitizeGhostSetupSnapshotForDesktop} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostSetupInteractionBridge.ts';
import {executeGhostSetupInlineSubmission} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostSetupInlineExecutor.ts';
import {parseOauthConnectSecretKey} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostSetupStatus.ts';
import {BotAuthorizationService, buildBotAuthorizationContinuation} from '../cindy/ghost/apps/desktop/src/main/maker-ipc/botAuthorizationService.ts';
import {readBotAuthorizationCard} from '../cindy/ghost/apps/desktop/src/shared/botAuthorization.ts';
import {classifyGhostVisibility, classifyInstalledGhostVisibility} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostVisibility.ts';
import {installedGhostStoragePart, deliveryNamespaceFields} from '../cindy/ghost/apps/desktop/src/shared/pluginIdentity.ts';
import {ghostInstallApprovalToken} from '../cindy/ghost/apps/desktop/src/shared/ghost.ts';
import {pluginToolVisible} from '../cindy/ghost/packages/plugin-protocol/src/index.ts';

const REQUEST_ID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const unavailable = () => Error('Plugin connection is no longer current');

export function createGhostConnections({root, find, current, visibility, assessment, changeBus, accounts,
  secrets, secretName, ownerScope, host, push, log, requestHostAccount}) {
  let closed = false;
  const owner = ownerScope.capture();
  const requests = new Map(), presentations = new Map(), epochs = new Map(), setupRequests = new Map(), cardEpochs = new Map();
  const cleared = () => Object.assign(unavailable(), {code: 'REMOTE_OPTIMISTIC_INPUT_CLEARED'});
  const cardsRoot = path.join(root, 'connection-cards');
  const checkOwner = () => { if (closed || !ownerScope.isCurrent(owner) || !ownerScope.isStable()) throw unavailable(); };
  const epochOf = snapshot => snapshot.mcpAppEpoch ?? snapshot.clearedAt ?? null;
  const sessionSnapshot = async sessionId => {
    checkOwner();
    const snapshot = await host('session-snapshot', {sessionId});
    checkOwner();
    if (!snapshot || snapshot.remoteHostId != null || snapshot.archived === true) throw unavailable();
    epochs.set(sessionId, epochOf(snapshot));
    return snapshot;
  };
  const targetToken = id => { const ghost = find(id); return ghost && current(ghost) ? ghostInstallApprovalToken(ghost.approval) : null; };
  const validateTarget = (id, tool, workingDir) => {
    checkOwner();
    const ghost = find(id);
    if (!ghost) return {ok: false, errorCode: 'GHOST_NOT_FOUND', message: '插件不存在'};
    const visible = classifyInstalledGhostVisibility(ghost, workingDir ?? null, visibility);
    if (!visible.ok) return visible;
    if (tool && !(ghost.manifest.tools ?? []).some(value => value.name === tool && pluginToolVisible(ghost.manifest, tool, 'model')))
      return {ok: false, errorCode: 'TOOL_NOT_FOUND', message: '插件没有此工具'};
    return {ok: true};
  };
  const identity = id => { const ghost = find(id); return ghost ? {id, name: ghost.manifest.name, ...(ghost.iconDataUrl ? {iconDataUrl: ghost.iconDataUrl} : {})} : null; };
  const inline = input => executeGhostSetupInlineSubmission({
    getAssessment: assessment, getManifest: id => find(id)?.manifest ?? null,
    storeSecret: (id, key, value) => secrets.write(secretName(id, key), value),
    emitChange: (id, key) => changeBus.emit(id, {source: 'secret', ref: key}), logger: log,
  }, input);
  async function executeAction({sessionId, ghostId, action, assertCurrent}) {
    checkOwner(); assertCurrent?.();
    const ghost = find(ghostId);
    if (!ghost || !current(ghost)) return {ok: false, errorCode: 'TARGET_UNAVAILABLE'};
    if (action.kind === 'oauth_connect') {
      const key = parseOauthConnectSecretKey(action.id);
      if (!key) return {ok: false, errorCode: 'ACTION_STALE'};
      const result = await accounts.action({pluginId: ghostId, action: 'oauth-connect', key});
      checkOwner(); assertCurrent?.();
      return result.ok ? {ok: true} : {ok: false, errorCode: 'ACTION_FAILED'};
    }
    const kind = action.kind === 'open_client_settings' ? 'plugin-client-settings' : 'plugin-settings';
    const result = await host('ui', {kind, sessionId, pluginId: ghostId, request: {actionId: action.id, kind: action.kind}});
    checkOwner(); assertCurrent?.();
    return result?.ok === true ? {ok: true, waitingExternal: true} : {ok: false, errorCode: 'ACTION_FAILED'};
  }

  // The same native choice card supports all any-of alternatives. Replies are
  // private UI data; neither the value nor its public action ID nominates a key.
  function present(sessionId, snapshot, submit, action) {
    if (snapshot.terminal || !snapshot.steps.some(step => step.action && ['pending', 'failed'].includes(step.phase)) || presentations.has(snapshot.requestId)) return;
    const controller = new AbortController();
    const flight = {controller, revision: snapshot.revision, ghostId: snapshot.ghost.id};
    presentations.set(snapshot.requestId, flight);
    void (async () => {
      try {
        const answer = await host('ui', {kind: 'plugin-setup', sessionId, ghostName: snapshot.ghost.name,
          request: sanitizeGhostSetupSnapshotForDesktop(snapshot)}, controller.signal);
        checkOwner();
        if (controller.signal.aborted) return;
        const command = {kind: 'plugin_setup', expectedRevision: snapshot.revision};
        if (answer?.action === 'submit') await submit({actionId: answer.actionId, expectedRevision: snapshot.revision, value: answer.value});
        else await action({...command, ...(answer?.action === 'run_action' ? {action: 'run_action', actionId: answer.actionId} : {action: 'cancel'})});
      } catch {
        if (!controller.signal.aborted && !closed) await action({kind: 'plugin_setup', action: 'cancel', expectedRevision: snapshot.revision});
      } finally {
        if (presentations.get(snapshot.requestId) === flight) presentations.delete(snapshot.requestId);
      }
    })().catch(() => log.warn('Plugin account interaction could not complete'));
  }
  const bridge = new GhostSetupInteractionBridge({logger: log, broadcast(channel, payload) {
    push('plugin-setup', {channel, ...payload});
    const snapshot = payload.request;
    if (snapshot && !snapshot.terminal) setupRequests.set(snapshot.requestId, snapshot);
    if (!snapshot || snapshot.terminal) {
      const requestId = snapshot?.requestId ?? payload.requestId;
      setupRequests.delete(requestId);
      presentations.get(requestId)?.controller.abort();
      return;
    }
    present(payload.sessionId, snapshot,
      input => bridge.submitInline(snapshot.requestId, input),
      input => bridge.resolve(snapshot.requestId, input));
  }});
  const coordinator = new GhostSetupCoordinator({
    changeBus, bridge, assess: assessment, validateTarget, getTargetToken: targetToken, getGhostIdentity: identity,
    executeAction, executeInlineAction: async input => inline(input), logger: log,
  });

  const cardPath = requestId => {
    if (!REQUEST_ID.test(requestId)) throw unavailable();
    return path.join(cardsRoot, requestId + '.json');
  };
  const loadCard = async requestId => {
    let envelope;
    try { envelope = JSON.parse(await fs.readFile(cardPath(requestId), 'utf8')); }
    catch (error) { if (error.code === 'ENOENT') return null; throw error; }
    const card = readBotAuthorizationCard(envelope.card);
    if (!card) return null;
    const snapshot = await sessionSnapshot(card.sessionId);
    if (envelope.epoch !== epochOf(snapshot)) return null;
    if (!card.snapshot.terminal) cardEpochs.set(requestId, envelope.epoch);
    return card;
  };
  const savedCards = async sessionId => {
    await sessionSnapshot(sessionId);
    const names = await fs.readdir(cardsRoot).catch(error => { if (error.code === 'ENOENT') return []; throw error; });
    const cards = [];
    for (const name of names) {
      const id = name.slice(0, -5);
      if (!name.endsWith('.json') || !REQUEST_ID.test(id)) continue;
      const card = await loadCard(id).catch(() => null);
      if (card?.sessionId === sessionId) cards.push(card);
    }
    return cards;
  };
  const service = new BotAuthorizationService({
    captureRequestGuard(sessionId) {
      const epoch = epochs.get(sessionId);
      return () => { checkOwner(); if (epochs.get(sessionId) !== epoch) throw unavailable(); };
    },
    async adapter(sessionId, target) {
      const snapshot = await sessionSnapshot(sessionId);
      if (snapshot.isCompanion !== true || target.kind !== 'plugin') throw unavailable();
      const visible = classifyGhostVisibility(target.id, snapshot.workingDir ?? null, visibility, target.namespace);
      if (!visible.ok) throw unavailable();
      const ghost = visible.ghost, id = installedGhostStoragePart(ghost), token = targetToken(id), completed = new Set(), originalEpoch = epochOf(snapshot);
      const validate = async () => {
        const latest = await sessionSnapshot(sessionId);
        if (epochOf(latest) !== originalEpoch) throw cleared();
        if (latest.isCompanion !== true || targetToken(id) !== token || !validateTarget(id, undefined, latest.workingDir).ok) throw unavailable();
      };
      return {
        pluginApprovalToken: ghostInstallApprovalToken(ghost.approval), identity: identity(id),
        async assess() {
          await validate();
          const current = assessment(id);
          return target.reauthorize ? toReauthInteractionAssessment(current) ?? toExplicitReauthInteractionAssessment(current, completed) ?? current : current;
        },
        subscribe: wake => changeBus.subscribe(id, event => {
          const state = assessment(id);
          if (target.reauthorize) for (const item of state.groups.flatMap(group => group.items)) {
            if ((event.ref === item.ref.slice(item.ref.indexOf(':') + 1) || event.ref === undefined && event.source === 'oauth' && item.kind === 'oauth') &&
                ['secret', 'oauth', 'connection'].includes(event.source)) item.actions.forEach(action => completed.add(action.id));
          }
          wake();
        }),
        async execute(action, _sender, value, _onUrl, assertCurrent) {
          await validate(); assertCurrent?.();
          if (action.kind === 'inline_form') {
            if (value === undefined) return {ok: false, errorCode: 'INLINE_UNAVAILABLE'};
            const bound = assessment(id).groups.flatMap(group => group.items).find(item => item.actions.some(candidate => candidate.id === action.id));
            if (!bound) return {ok: false, errorCode: 'ACTION_STALE'};
            const result = inline({ghostId: id, action, value, commit: {
              replaceExisting: target.reauthorize === true,
              assertCurrent(ghostId, actionId, ref) { checkOwner(); assertCurrent?.(); if (ghostId !== id || actionId !== action.id || ref !== bound.ref || targetToken(id) !== token) throw unavailable(); },
              onCommitted() { completed.add(action.id); },
            }});
            return result;
          }
          const result = await executeAction({sessionId, ghostId: id, action, assertCurrent});
          if (result.ok && action.kind === 'oauth_connect') completed.add(action.id);
          return result;
        },
      };
    },
    async save(card, assertCurrent) {
      const snapshot = await sessionSnapshot(card.sessionId);
      assertCurrent?.();
      const requestId = card.snapshot.requestId;
      if (!cardEpochs.has(requestId)) {
        // New cards have the original service's captured creation guard. A
        // later save/restoration may not rebind old presentation to a new epoch.
        if (!assertCurrent) throw unavailable();
        cardEpochs.set(requestId, epochOf(snapshot));
      }
      const expectedEpoch = cardEpochs.get(requestId);
      if (expectedEpoch !== epochOf(snapshot)) throw cleared();
      const safe = {...card, snapshot: sanitizeGhostSetupSnapshotForDesktop(card.snapshot)};
      const target = cardPath(card.snapshot.requestId), temporary = target + '.' + randomUUID() + '.tmp';
      await fs.mkdir(cardsRoot, {recursive: true, mode: 0o700});
      try {
        await fs.writeFile(temporary, JSON.stringify({epoch: expectedEpoch, card: safe}), {flag: 'wx', mode: 0o600});
        const latest = await sessionSnapshot(card.sessionId);
        if (epochOf(latest) !== expectedEpoch) throw cleared();
        assertCurrent?.();
        await fs.rename(temporary, target);
      } finally { await fs.rm(temporary, {force: true}); }
      push('plugin-authorization', {sessionId: card.sessionId, clientId: 'bot-authorization:' + card.snapshot.requestId, card: safe});
      if (safe.snapshot.terminal) {presentations.get(safe.snapshot.requestId)?.controller.abort();cardEpochs.delete(safe.snapshot.requestId);}
      else presentAuthorizationCard(safe);
    },
    load: loadCard,
    findPending: async (sessionId, target) => (await savedCards(sessionId)).find(card => !card.snapshot.terminal && card.target.kind === target.kind &&
      card.target.id === target.id && card.target.namespace === target.namespace && !!card.target.reauthorize === !!target.reauthorize) ?? null,
    async resume(card, assertCurrent) {
      const requestId = card.snapshot.requestId;
      if (!cardEpochs.has(requestId)) throw unavailable();
      const expectedEpoch = cardEpochs.get(requestId), snapshot = await sessionSnapshot(card.sessionId);
      if (epochOf(snapshot) !== expectedEpoch) throw cleared();
      assertCurrent();
      const continuation = buildBotAuthorizationContinuation(card);
      const result = await host('connection-resume', {requestId, expectedEpoch, ...continuation});
      checkOwner(); assertCurrent();
      if (result?.ok !== true) throw unavailable();
    },
    openExternal: url => host('open-external', {url}), warn: () => log.warn('Plugin account interaction could not complete'),
  });

  function presentAuthorizationCard(card) {
    present(card.sessionId, card.snapshot,
      input => service.submit(card.snapshot.requestId, input),
      input => service.resolve(card.snapshot.requestId, input, {id: 0, isDestroyed: () => closed, send() {}}));
  }
  async function ensure({ghost, session, tool, plan, reauthorize, signal}) {
    return coordinator.ensureReady({ghostId: installedGhostStoragePart(ghost), sessionId: session?.id ?? null,
      tool, workingDir: session?.workingDir ?? null, plan, reauthorize, signal});
  }
  async function connect({session, kind, id, namespace, reauthorize, cancelKey}, signal) {
    if (kind !== 'plugin') return requestHostAccount ? requestHostAccount({session, kind, id, reauthorize}, signal) : {ok: false, errorCode: 'UNSUPPORTED_CONNECTION'};
    if (!session?.id) return {ok: false, errorCode: 'NO_SESSION_CONTEXT'};
    const controller = new AbortController(), abort = () => controller.abort();
    signal?.addEventListener('abort', abort, {once: true});
    if (signal?.aborted) abort();
    if (cancelKey) requests.set(cancelKey, {sessionId: session.id, controller});
    try {
      const snapshot = await sessionSnapshot(session.id);
      controller.signal.throwIfAborted();
      const target = classifyGhostVisibility(id, snapshot.workingDir ?? null, visibility, namespace);
      if (!target.ok) return target;
      const instance = installedGhostStoragePart(target.ghost), setup = assessment(instance);
      if (setup.state === 'ready' && setup.groups.length === 0) return {ok: false, errorCode: 'PLUGIN_CONNECTION_METHOD_REQUIRED', ghostId: id,
        settingsAvailable: !!target.ghost.manifest.settingsHtml, message: '此插件没有 Host 配置动作，请使用插件已有设置或文档中的登录工具。不要在对话里提交凭证。'};
      if (snapshot.isCompanion === true) {
        const pending = await service.request(session.id, {kind: 'plugin', id: target.ghost.manifest.id,
          ...(target.ghost.namespaceState === 'pending' ? {} : deliveryNamespaceFields(target.ghost)), ...(reauthorize ? {reauthorize: true} : {})});
        if (pending.requestId) {
          const card = await loadCard(pending.requestId);
          if (card && !card.snapshot.terminal) presentAuthorizationCard(card);
        }
        return pending;
      }
      const result = await ensure({ghost: target.ghost, session: {...session, workingDir: snapshot.workingDir}, reauthorize, signal: controller.signal});
      if (!result.ok) return result;
      controller.signal.throwIfAborted();
      const latest = classifyGhostVisibility(id, snapshot.workingDir ?? null, visibility, namespace);
      if (!latest.ok || installedGhostStoragePart(latest.ghost) !== instance) return latest.ok ? {ok: false, errorCode: 'GHOST_NOT_FOUND'} : latest;
      if (assessment(instance).state !== 'ready') return {ok: false, errorCode: 'SETUP_REQUIRED'};
      return {ok: true, status: 'ready', ghostId: id, message: 'Host 配置已完成，没有执行插件业务操作；服务权限由后续实际操作验证。'};
    } catch (error) {
      if (controller.signal.aborted) return {ok: false, errorCode: 'SETUP_CANCELLED'};
      throw error;
    } finally {signal?.removeEventListener('abort', abort); if (cancelKey) requests.delete(cancelKey);}
  }
  return {
    connect, ensure, bridge, service,
    pending: savedCards,
    cancel(key) {requests.get(key)?.controller.abort();},
    cancelSession(sessionId) {for (const entry of requests.values()) if (entry.sessionId === sessionId) entry.controller.abort(); bridge.cleanupForSession(sessionId, 'session_aborted');},
    action: async ({requestId, op, input}) => {
      if (op === 'open') {
        const card = await loadCard(requestId);
        if (!card || card.snapshot.terminal || targetToken(card.snapshot.ghost.id) !== card.pluginApprovalToken) return {requested: false};
        presentAuthorizationCard(card);
        return {requested: true};
      }
      if (op === 'submit') return {accepted: bridge.submitInline(requestId, input) || await service.submit(requestId, input)};
      if (op !== 'command') return {accepted: false};
      return {accepted: bridge.resolve(requestId, input) || await service.resolve(requestId, input, {id: 0, isDestroyed: () => closed, send() {}})};
    },
    stop(id) {
      // Public pendingSnapshots intentionally omits revoked installations. Keep
      // the authoritative ids from broadcasts so stop can cancel those too.
      for (const snapshot of setupRequests.values()) if (snapshot.ghost.id === id)
        bridge.resolve(snapshot.requestId, {kind: 'plugin_setup', action: 'cancel', expectedRevision: snapshot.revision});
      for (const flight of presentations.values()) if (flight.ghostId === id) flight.controller.abort();
      service.invalidatePlugin(id);
    },
    async close() {
      for (const entry of requests.values()) entry.controller.abort();
      bridge.cleanupAll('session_closed');
      for (const flight of presentations.values()) flight.controller.abort();
      await service.dispose();
      await coordinator.waitForActionsIdle();
      cardEpochs.clear();
      closed = true;
    },
  };
}
