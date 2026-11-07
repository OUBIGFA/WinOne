// Plugin secrets, OAuth accounts and connections, and the same-origin endpoints a
// plugin's own settings page calls. Tokens stay in the encrypted WinOne vault and
// are injected by the real account-bound broker, never returned by these endpoints.
import path from 'node:path';
import {createGhostKvStore} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostKvStore.ts';
import {GhostOauthAccountManager} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostOauthAccounts.ts';
import {cancelActiveGhostOauthFlow} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostOauthFlow.ts';
import {GhostConnectionManager} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostConnections.ts';
import {handleGhostSecretsRequest} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/runtime/ghostSecretsEndpoint.ts';
import {handleGhostOauthRequest} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/runtime/ghostOauthEndpoint.ts';
import {handleGhostConnectionsRequest} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/runtime/ghostConnectionsEndpoint.ts';
import {handleGhostModelsRequest} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/runtime/ghostModelsEndpoint.ts';
import {guardedOutboundFetch, guardedResponseFetch} from './seams/outbound-fetch.ts';
import {checkGhostSecret} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/runtime/ghostSecretCheck.ts';
import {GHOST_NETWORK_MAX_CONNECTIONS_PER_DECL} from '../cindy/ghost/apps/desktop/src/shared/ghost.ts';
import {installedGhostStoragePart} from '../cindy/ghost/apps/desktop/src/shared/pluginIdentity.ts';
import {ghostInstallApprovalToken} from '../cindy/ghost/apps/desktop/src/shared/ghost.ts';

const MAX_BODY = 1 << 20;
async function bodyText(req) {
  const chunks = []; let count = 0;
  for await (const chunk of req) { count += chunk.length; if (count > MAX_BODY) throw Object.assign(Error('Request too large'), {status: 413}); chunks.push(chunk); }
  return Buffer.concat(chunks).toString('utf8');
}

export function createGhostAccounts({root, secrets, secretName, find, host, ui, kv, changeBus, log, ownerScope}) {
  let closed = false;
  const authorizations = new Set();
  const name = (id, key) => secretName(id, key);
  const vault = {
    read: (id, key) => secrets.read(name(id, key)),
    readStrict: (id, key) => {
      const result = secrets.readResult(name(id, key));
      if (result.kind === 'error') throw Error('Plugin credential storage unavailable');
      return result.kind === 'value' ? result.value : null;
    },
    store: (id, key, value) => secrets.write(name(id, key), value),
    remove: (id, key) => { secrets.remove(name(id, key)); },
    readTail: (id, key) => { const value = secrets.read(name(id, key)); return value && value.length >= 12 ? value.slice(-4) : null; },
  };
  let mutationTail=Promise.resolve();
  // Both the manager and protocol dispatcher pass (instanceKey, task).
  // Feeding the identity string to Promise.then silently skips persistence.
  const withMutationLock=(_id,work)=>{const pending=mutationTail.then(work);mutationTail=pending.catch(()=>{});return pending;};
  const oauth = new GhostOauthAccountManager({
    vault, fetchImpl: (url, init) => guardedResponseFetch(url, init, () => { if (closed || !ownerScope.isStable()) throw Error('Plugin account host closed'); }),
    openExternal: url => host('open-external', {url}),
    broker: null, resolveBrokerPublicUrl: () => { throw Error('WinOne has no Cindy token broker'); },
    logger: log, reclaimPort: async () => false,
    onAccountConnected: ({ghostId}) => changeBus.emit(ghostId, {source: 'oauth'}),
    onAccountStatusChanged: ({ghostId, secretKey}) => changeBus.emit(ghostId, {source: 'oauth', ref: secretKey}),
    captureConnectTarget: id => { const ghost = find(id); return ghost ? ghostInstallApprovalToken(ghost.approval) : null; },
    isConnectTargetCurrent: (id, key, decl, target) => { const ghost = find(id); return !closed && ownerScope.isStable() && !!ghost?.enabled && (target === undefined || ghostInstallApprovalToken(ghost.approval) === target) && !!ghost.manifest.network?.secrets?.some(secret => secret.key === key && secret.source === 'oauth'); },
    withMutationLock, isTokenBrokerAuthorized: () => false,
    fetchAvatarHop: (url, init) => guardedOutboundFetch(url, {...init, credentials: 'omit'}),
  });
  let connecting=null;
  const connectAccount=async(id,key,decl,options)=>{
    const target={id};connecting=target;
    try{return await oauth.connectAccount(id,key,decl,options);}
    finally{if(connecting===target)connecting=null;}
  };
  // Keep the real manager as receiver; its private state must not move to a
  // spread object. Track only the local browser transaction for stop/close.
  const endpointManager=Object.fromEntries(['clientConfigured','clientCustomized','setClientConfig','clearClientConfig','listAccounts','captureConnectTarget','disconnectAccount','setDefaultAccount','reportInsufficientScopes'].map(method=>[method,oauth[method].bind(oauth)]));
  endpointManager.connectAccount=connectAccount;
  const invalidate=id=>{
    if(connecting?.id===id)cancelActiveGhostOauthFlow();
    oauth.invalidateGhost(id);
    for(const handle of [...authorizations])if(handle.ghostId===id)handle.dispose();
  };
  const connections = new GhostConnectionManager({vault});
  const models = createGhostKvStore({getRootDir:()=>path.join(root,'published-models'),log});

  async function endpoint(ghost, route, req, res) {
    const id = installedGhostStoragePart(ghost), method = req.method, readBodyText = () => bodyText(req), isCurrent = () => {
      const latest=find(id);
      return !closed && ownerScope.isStable() && !!latest?.enabled && ghostInstallApprovalToken(latest.approval)===ghostInstallApprovalToken(ghost.approval);
    };
    const manifest = ghost.manifest, net = manifest.network?.secrets ?? [];
    let outcome;
    if (route === '/secrets' || route.startsWith('/secrets/')) {
      const nodeSecrets = (manifest.node?.secretBindings ?? []).filter(item => !item.oauthSecret);
      outcome = await handleGhostSecretsRequest({method, pathname: route, readBodyText, isCurrent, ghostId: id,
        userSecretKeys: net.filter(item => !['login-email', 'oauth', 'oidc-token'].includes(item.source)).map(item => item.key).concat(nodeSecrets.map(item => item.key)),
        identitySecretKeys: net.filter(item => item.source === 'login-email').map(item => item.key),
        managedSecretStates: net.filter(item => item.source === 'oidc-token').map(item => ({key:item.key,saved:false})),
        hostCredentialStates: await Promise.all(net.filter(item => item.source === 'gh-cli').map(async item => ({key:item.key,source:'gh-cli',available:!!await host('gh-token',{}).catch(()=>null)}))),
        checkSecretKeys: net.filter(item => item.check && (!item.source || item.source === 'user')).map(item => item.key),
        checkSecret: (key,value) => checkGhostSecret({instanceKey:id,secret:net.find(item=>item.key===key),value,isCurrent,
          fetch:(url,init)=>guardedResponseFetch(url,init,()=>{if(!isCurrent())throw Error('Plugin changed');})}),
        getLoginEmail: () => null,
        vault: {saved: (ghostId, key) => vault.read(ghostId, key) !== null, tail: vault.readTail,
          store: (ghostId, key, value) => { const stored = vault.store(ghostId, key, value); if (stored) changeBus.emit(ghostId, {source: 'secret', ref: key}); return stored; },
          remove: (ghostId, key) => { vault.remove(ghostId, key); changeBus.emit(ghostId, {source: 'secret', ref: key}); }}, log});
    } else if (route === '/oauth' || route.startsWith('/oauth/')) {
      const oauthSecrets = new Map(net.filter(item => item.source === 'oauth' && item.oauth).map(item => [item.key, item.oauth]));
      outcome = await handleGhostOauthRequest({method, pathname: route, readBodyText, isCurrent, oauthSecrets, networkHosts: manifest.network?.hosts, manager: endpointManager, ghostId: id, isTokenBrokerAuthorized: () => false, withMutationLock, onChanged: key => changeBus.emit(id, {source: 'oauth', ref: key}), log});
    } else if (route === '/connections' || route.startsWith('/connections/')) {
      const decls = new Map((manifest.network?.connections ?? []).map(item => [item.key, {label: item.label, maxConnections: item.maxConnections ?? GHOST_NETWORK_MAX_CONNECTIONS_PER_DECL}]));
      outcome = await handleGhostConnectionsRequest({method, pathname: route, readBodyText, isCurrent, decls, manager: connections, ghostId: id, confirmAddHost: async (label, address) => { const answer = await ui('confirm', {ghostId: id, ghostName: manifest.name, body: label + '\n' + address, confirmText: '', cancelText: '', danger: false, trusted: true}); return answer?.confirmed === true; }, onChanged: key => changeBus.emit(id, {source: 'connection', ref: key}), log});
    } else if (route === '/models') {
      outcome = await handleGhostModelsRequest({ghostId: id, method, readBodyText, isCurrent, needs: () => manifest.models ?? undefined, store: models, onChanged: () => {}});
    } else return false;
    res.setHeader('Content-Type', 'application/json; charset=utf-8');
    res.writeHead(outcome.status);
    res.end(outcome.body);
    return true;
  }

  const probes = (id, manifest) => {
    let snapshot = null;
    return {
      secretSaved: key => vault.read(id, key) !== null,
      oauthStatus: key => {
        const decl = manifest.network?.secrets?.find(secret => secret.key === key)?.oauth;
        const accounts = oauth.listAccounts(id, key);
        return {clientConfigured: oauth.clientConfigured(id, key, decl), connected: accounts.filter(account => account.status === 'connected').length, expired: accounts.filter(account => account.status === 'expired').length};
      },
      connectionCount: key => connections.list(id, key).length,
      kvValue: key => { snapshot ??= kv.readStrict?.(id) ?? kv.read(id) ?? {}; return snapshot[key]; },
    };
  };

  // The broker waits for these private promises, not a fire-and-forget browser
  // launch. Opening is not login success; the original CLI still exchanges and
  // verifies its own tokens. Closing/cancelling must release an awaiting RPC.
  function authorization(input, request) {
    let ended = false, rejectClosed;
    const cancelled = new Promise((_, reject) => { rejectClosed = reject; });
    const assertCurrent = () => {
      if (closed || ended || input.signal.aborted || !ownerScope.isStable()) throw Error('Authorization cancelled');
      input.assertCurrent();
    };
    const handle = {ghostId:input.ghost.id,finish() { this.dispose(); }, dispose() {
      if (ended) return;
      ended = true;
      input.signal.removeEventListener('abort', onAbort);
      authorizations.delete(handle);
      rejectClosed(Error('Authorization closed'));
    }};
    const onAbort = () => handle.dispose();
    authorizations.add(handle);
    input.signal.addEventListener('abort', onAbort, {once: true});
    const opening = (async () => {
      assertCurrent();
      const answer = await ui('confirm', {ghostId: input.ghost.id, ghostName: input.ghost.name, sessionId: input.sessionId,
        body: request.url + (request.userCode ? '\n' + request.userCode : ''), trusted: true, danger: false});
      assertCurrent();
      if (answer?.confirmed !== true) throw Error('Authorization cancelled');
      await host('open-external', {url: request.url});
      assertCurrent();
      return {kind: 'opened'};
    })();
    const result = Promise.race([opening, cancelled]);
    // Cleanup is not cancellation after successful opening. RPC settlement owns
    // the handle's remaining lifetime and may later call dispose again.
    void result.finally(() => {
      input.signal.removeEventListener('abort', onAbort);
      authorizations.delete(handle);
    }).catch(() => {});
    const opened = result.then(() => undefined);
    void opened.catch(() => {}); // Generic callers await result, legacy callers await opened.
    return {...handle, opened, result};
  }

  return {
    endpoint, probes,
    oauthTokens: {
      getFreshAccessToken:(id,key,decl,accountId)=>oauth.getFreshAccessToken(id,key,decl,accountId),
      invalidateAccessToken:(id,key,accountId)=>oauth.invalidateAccessToken(id,key,accountId),
    },
    networkConnections: {
      hostsFor:id=>(find(id)?.manifest.network?.connections??[]).flatMap(decl=>connections.hostsOf(id,decl.key)),
      tokenFor:(id,hostname)=>{
        for(const decl of find(id)?.manifest.network?.connections??[]){
          const value=connections.resolveTokenByHost(id,decl.key,hostname);
          if(value!==null)return {value,header:decl.inject.header,format:decl.inject.format};
        }
        return null;
      },
    },
    freshToken: (id, key, accountId) => {
      const decl = find(id)?.manifest.network?.secrets?.find(item => item.key === key);
      if (decl?.source !== 'oauth' || !decl.oauth) return {ok: false, error: 'INVALID_DECLARATION'};
      return oauth.getFreshAccessToken(id, key, decl.oauth, accountId);
    },
    resolveTokenByHost: (id, key, hostname) => connections.resolveTokenByHost(id, key, hostname),
    connectionHosts: (id, key) => connections.hostsOf(id, key),
    deviceAuthorization: input => authorization(input, {kind: 'device', url: input.url}),
    pluginAuthorization: input => authorization(input, input.request),
    invalidate,
    remove: (id, manifest) => {
      invalidate(id);
      // These derived vault keys cannot be enumerated by the hashed IM vault.
      // Use the original managers to remove each account/connection first.
      for (const secret of manifest.network?.secrets ?? []) {
        if (secret.source === 'oauth') {
          for (const account of oauth.listAccounts(id, secret.key)) oauth.disconnectAccount(id, secret.key, account.id);
          oauth.clearClientConfig(id, secret.key);
          vault.remove(id, secret.key + '-accounts');
        }
        vault.remove(id, secret.key);
      }
      for (const decl of manifest.network?.connections ?? []) {
        for (const connection of connections.list(id, decl.key)) connections.remove(id, decl.key, connection.id);
        vault.remove(id, decl.key + '-connections');
      }
      for (const binding of manifest.node?.secretBindings ?? []) if (!binding.oauthSecret) vault.remove(id, binding.key);
      models.remove(id);
    },
    action: async ({pluginId, action, key, accountId}) => {
      const ghost = find(pluginId); if (!ghost?.enabled || closed) throw Object.assign(Error('插件未启用或不存在'), {code: 'NOT_FOUND'});
      const decl = ghost.manifest.network?.secrets?.find(item => item.key === key && item.source === 'oauth')?.oauth;
      if (!decl) throw Object.assign(Error('插件未声明此 OAuth 凭证'), {code: 'INVALID_PARAMS'});
      switch (action) {
        case 'oauth-list': return {accounts: oauth.listAccounts(pluginId, key, decl), configured: oauth.clientConfigured(pluginId, key, decl)};
        case 'oauth-connect': return connectAccount(pluginId, key, decl, {});
        case 'oauth-disconnect': oauth.disconnectAccount(pluginId, key, accountId); changeBus.emit(pluginId, {source: 'oauth', ref: key}); return {ok: true};
        case 'oauth-default': { const ok = oauth.setDefaultAccount(pluginId, key, accountId); if (ok) changeBus.emit(pluginId, {source: 'oauth', ref: key}); return {ok}; }
        default: throw Object.assign(Error('未知账号操作'), {code: 'INVALID_PARAMS'});
      }
    },
    close() { closed = true; if(connecting)cancelActiveGhostOauthFlow();for (const handle of [...authorizations]) handle.dispose(); },
  };
}
