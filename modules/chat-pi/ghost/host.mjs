// Genuine local Cindy Ghost pre-run host. The reused manager owns package
// validation/receipts; the reused gateway owns timeout, failure policy and
// verdict identity. Only the operating-system and WinOne authority are seams.
import fs from 'node:fs';
import path from 'node:path';
import {Readable} from 'node:stream';
import {createHash, randomUUID} from 'node:crypto';
import {GhostManager} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/GhostManager.ts';
import {GhostSubscriptionGateway} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/subscriptionGateway.ts';
import {GhostRuntime} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/runtime/GhostRuntime.ts';
import {GhostNetworkSlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/networkSlot.ts';
import {createGhostKvStore} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostKvStore.ts';
import {handleGhostKvRequest,readBoundedBodyText} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/runtime/ghostKvEndpoint.ts';
import {GHOST_CONTROL_POINTS, GHOST_CONTROL_POINT_IDS, declaredGhostControlPoints, ghostControlPointActive, ghostInstallApprovalToken, GHOST_SECRET_VALUE_MAX_CHARS, ghostPermissionItems} from '../cindy/ghost/apps/desktop/src/shared/ghost.ts';
import {findInstalledGhostByInstanceId, installedGhostStoragePart, installedGhostPhysicalKeys} from '../cindy/ghost/apps/desktop/src/shared/pluginIdentity.ts';
import {createOneBotPaths} from '../onebot/content-service.mjs';
import {createIMSecrets} from '../onebot/im-secrets.mjs';
import {getDirDepositVault, getSaveDepositVault} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/dirDeposit.ts';
import {guardedOutboundFetch, guardedResponseFetch} from './seams/outbound-fetch.ts';
import {createSandboxAdapter} from './sandbox.mjs';
import {createGhostPlatform} from './platform.mjs';
import {pluginHasToolUi} from '../cindy/ghost/packages/plugin-protocol/src/index.ts';
import {setGhostSnapshotWorker} from './snapshot.mjs';

const failure = (code, message) => Object.assign(Error(message), {code});
const digest = value => createHash('sha256').update(value).digest('hex');

export function createGhostHost({directory, executable, sdkFile, mailSDKFile, snapshotWorker, masterKey, runtimeExecutable, nodeWorker, browserExtensionOptions, event = () => {}, host = async () => {throw Error('Plugin Host capability is unavailable');}}) {
  if (![directory, executable, sdkFile, mailSDKFile].every(value => typeof value === 'string' && path.isAbsolute(value))) throw Error('Explicit plugin host paths are required');
  const paths = createOneBotPaths(directory);
  const contentRoot = path.join(paths.root, 'plugins'), stateRoot = path.join(paths.root, 'install-state');
  fs.mkdirSync(contentRoot, {recursive: true, mode: 0o700}); fs.mkdirSync(stateRoot, {recursive: true, mode: 0o700});
  if (snapshotWorker) setGhostSnapshotWorker(snapshotWorker);
  let closed = false, locale = 'zh-CN';
  const owner = randomUUID(), pending = new Set(), inspections = new Map();
  const context = () => ({region: 'global', locale, uiLocale: locale});
  const log = {debug: () => {}, info: () => {}, warn: message => event({type: 'diagnostic', message})};
  const manager = new GhostManager({getRootDir: () => contentRoot, getStateDir: () => stateRoot, getOwnerContextKey: () => owner, getLocale: () => locale, onChanged: () => event({type: 'changed'}), log});
  const kv = createGhostKvStore({getRootDir: () => path.join(paths.root, 'kv'), log});
  const secrets = createIMSecrets(paths, masterKey, log);
  const find = id => findInstalledGhostByInstanceId(manager.list(), id);
  const secretName = (id, key) => 'ghost-' + digest(id) + '-' + key;
  const current = ghost => {
    if (closed) return false;
    const now = find(installedGhostStoragePart(ghost));
    return !!now?.enabled && ghostInstallApprovalToken(now.approval) === ghostInstallApprovalToken(ghost.approval);
  };
  function requireCurrent(id) {
    const ghost = find(id);
    if (!ghost?.enabled || ghost.approval?.state !== 'approved') throw failure('PLUGIN_UNAVAILABLE', '插件未启用或安装批准已失效，请重新启用或安装');
    return ghost;
  }
  let platform;
  const network = new GhostNetworkSlot({
    getGhost: find,
    captureRequestGuard: id => {
      const captured = requireCurrent(id);
      return () => { if (!current(captured)) throw failure('PLUGIN_CHANGED', '插件在执行期间发生变化，请重试'); };
    },
    oauthTokens:{
      getFreshAccessToken:(...args)=>platform.oauthTokens.getFreshAccessToken(...args),
      invalidateAccessToken:(...args)=>platform.oauthTokens.invalidateAccessToken(...args),
    },
    connections:{hostsFor:id=>platform.networkConnections.hostsFor(id),tokenFor:(id,hostname)=>platform.networkConnections.tokenFor(id,hostname)},
    readSecret: (id, key) => secrets.read(secretName(id, key)),
    readGhCliToken: () => host('gh-token', {}),
    getLoginEmail: () => null, // Cindy accounts/organization services are outside WinOne.
    // Declared hosts are an authorization boundary, not an SSRF exemption.
    // Every plugin socket still uses Cindy's pinned single-hop transport.
    fetchImpl: (url, init) => guardedResponseFetch(url, init),
    inFlightCallInfo: callId => platform.cards.inFlightCallInfoOf(callId),
    // Only an Agent's in-flight call may reach an address the manifest did not
    // declare; the slot checks that, and this guard checks each hop's address.
    fetchPublicImpl: (url, init, beforeDispatch) => guardedOutboundFetch(url, init, beforeDispatch),
    readHandoffFile:(...args)=>platform.readHandoffFile(...args),touchHandoffFiles:(...args)=>platform.touchHandoffFiles(...args),
    readGhostMedia: (id, hash) => platform.readGhostMedia(id, hash),
    readLibraryFile: (id, relative, signal) => platform.libraryTransport.readFile(id, relative, signal),
    saveLibraryStream: (id, input) => platform.libraryTransport.saveStream(id, input),
    uploadFileSize: (id, value, library) => library ? platform.libraryTransport.fileSize(id, value) : undefined,
    takeDirDeposit: (id, token) => getDirDepositVault().take(id, token),
    writeSaveDeposit: (id, token, name, bytes) => getSaveDepositVault().write(id, token, name, bytes),
    saveGhostMedia: params => platform.saveGhostMedia(params), isSupportedMediaMime: mime => platform.isSupportedMediaMime(mime),
    log,
  });
  let gateway, runtime;
  const adapter = createSandboxAdapter({directory: path.join(paths.root, 'sandboxes'), executable, sdkFile, mailSDKFile, current, log,
    receive: async (ghost, request) => {
      if (!current(ghost)) throw failure('PLUGIN_CHANGED', '插件在执行期间发生变化');
      const id = installedGhostStoragePart(ghost);
      if (request.channel === 'ghost-pipe:ping') return {ok: true, id};
      const message = request.payload;
      if (!message || typeof message !== 'object') return {ok: false, errorCode: 'INVALID_PARAMS', message: '插件请求格式无效'};
      if (message.type === 'event-verdict') { gateway.handleVerdict(id, message); return {ok: true}; }
      if (message.type === 'host-request' && message.kind === 'control-points') return {ok: true, points: GHOST_CONTROL_POINT_IDS.map(point => ({id: point, actions: GHOST_CONTROL_POINTS[point].actions, timeoutMs: GHOST_CONTROL_POINTS[point].timeoutMs, declared: declaredGhostControlPoints(ghost.manifest).includes(point), active: ghostControlPointActive(ghost, point)}))};
      if (message.type === 'host-request' && message.kind === 'app-context') return {ok: true, context: context()};
      if (message.type === 'host-request') return {ok: false, errorCode: 'INVALID_PARAMS', message: '未知宿主查询'};
      return platform.receive(ghost, message);
    },
    endpoint: async (ghost, route, req, res) => {
      const id = installedGhostStoragePart(ghost);
      if (route === '/app-context') {
        res.setHeader('Content-Type', 'application/json; charset=utf-8');
        res.writeHead(req.method === 'GET' ? 200 : 405);
        res.end(req.method === 'GET' ? JSON.stringify({ok:true, context:context()}) : undefined);
        return true;
      }
      if (route !== '/kv') return platform.endpoint(ghost, route, req, res);
      res.setHeader('Content-Type', 'application/json; charset=utf-8');
      const outcome=await handleGhostKvRequest({method:req.method,ghostId:id,isCurrent:()=>current(ghost),log,
        readBodyText:()=>readBoundedBodyText({headers:new Headers(req.headers),body:Readable.toWeb(req)}),
        store:{read:kv.read,write:(key,value)=>{kv.write(key,value);platform.configurationChanged(key,{parameters:value});}}});
      res.writeHead(outcome.status);res.end(outcome.body);return true;
    },
  });
  runtime = new GhostRuntime({adapter, log, onFused: id => { event({type: 'fused', pluginId: id}); }, onStateChanged: (id, state) => { platform?.onRuntimeState(id, state); event({type: 'state', pluginId: id, state}); }});
  gateway = new GhostSubscriptionGateway({listGhosts: () => manager.list(), listGhostsAsync: () => manager.listAsync(), findGhostForDelivery: find, isRunning: id => runtime.stateOf(id) === 'running', wake: async ghost => {
    const result = await runtime.spawn(ghost);
    if (!result.ok || !await runtime.waitUntilRunning(installedGhostStoragePart(ghost))) throw Error('插件沙箱启动失败');
  }, sendToGhost: (id, message) => adapter.send(id, message), now: Date.now, ownerScope: {capture: () => owner, isCurrent: value => value === owner && !closed, isStable: () => !closed}, log});
  platform = createGhostPlatform({root: paths.root, manager, runtime, adapter, gateway, network, kv, secrets, secretName, find, current, host, event, log, runtimeExecutable, nodeWorker, browserExtensionOptions, ownerScope: {capture: () => owner, isCurrent: value => value === owner && !closed, isStable: () => !closed}});
  // Resident Node declarations are independent of the browser's launch mode.
  // Approved on-demand plugins still create no worker until their first RPC.
  queueMicrotask(() => { if (!closed) for (const ghost of manager.list()) void platform.start(installedGhostStoragePart(ghost)); });
  const present = ghost => ({pluginId: installedGhostStoragePart(ghost), unread:platform.unread(installedGhostStoragePart(ghost)), name: ghost.manifest.name, version: ghost.manifest.version, enabled: ghost.enabled, points: declaredGhostControlPoints(ghost.manifest), approvedPoints: ghost.approvedControlPoints ?? [], approval: ghostInstallApprovalToken(ghost.approval), state: runtime.stateOf(installedGhostStoragePart(ghost)), hasSettings: !!ghost.manifest.settingsHtml, pages: [...(ghost.manifest.panel ? [{page: 'panel', title: ghost.manifest.panel.title ?? ghost.manifest.name}] : []), ...(ghost.manifest.mainView ? [{page: 'main', title: ghost.manifest.mainView.title ?? ghost.manifest.name}] : [])]});
  async function stop(id) { gateway.dropGhost(id); runtime.stop(id); await platform.stop(id); }
  async function call(method, params = {}) {
    if (closed) throw Error('Plugin host is closing');
    if (params.language) {
      const nextLocale = params.language === 'en-US' || params.language === 'en' ? 'en' : 'zh-CN';
      if (nextLocale !== locale) {
        locale = nextLocale;
        for (const ghost of manager.list()) if (runtime.stateOf(installedGhostStoragePart(ghost)) === 'running') {
          try { adapter.send(installedGhostStoragePart(ghost), {type:'host-context-changed',ok:true,context:context()}); } catch { /* A stopped page rereads on its next load. */ }
        }
      }
    }
    switch (method) {
      case 'list': return (await manager.listAsync()).map(present);
      case 'candidates': {
        if (!['will-schedule-run', 'will-routine-run'].includes(params.point)) throw failure('INVALID_PARAMS', '未知运行前控制节点');
        return manager.list().filter(ghost => ghostControlPointActive(ghost, params.point)).map(ghost => ({pluginId: installedGhostStoragePart(ghost), name: ghost.manifest.name}));
      }
      case 'resolve': {
        const ghost = requireCurrent(params.pluginId);
        if (!ghostControlPointActive(ghost, params.point)) throw failure('PLUGIN_UNAVAILABLE', '插件没有获准处理此运行前检查');
        return {pluginId: installedGhostStoragePart(ghost)};
      }
      case 'inspect': {
        const inspected = await manager.inspect(params.path);
        if (inspected.rejection) throw failure('PLUGIN_INVALID', inspected.rejection.reason);
        const token = randomUUID();
        while (inspections.size >= 16) inspections.delete(inspections.keys().next().value);
        const existing = manager.list().find(ghost => ghost.manifest.id === inspected.manifest.id && ghost.namespace === null);
        inspections.set(token, {path: params.path, hash: inspected.packageSha256, manifest: inspected.manifest, existing, expires: Date.now() + 10 * 60_000});
        return {token, name: inspected.manifest.name, version: inspected.manifest.version, packageSha256: inspected.packageSha256, points: declaredGhostControlPoints(inspected.manifest), permissions: ghostPermissionItems(inspected.manifest), update: !!existing};
      }
      case 'install': {
        const inspected = inspections.get(params.token); inspections.delete(params.token);
        if (!inspected || inspected.expires < Date.now() || params.confirmed !== true) throw failure('CONFIRM_REQUIRED', '请先核对插件能力并确认安装');
        // The native install confirmation displays these exact declarations;
        // bind only those capabilities to the same inspected package receipt.
        const agent = inspected.manifest.agent;
        const options = {namespace: null, expectedPackageSha256: inspected.hash, approvedControlPoints: declaredGhostControlPoints(inspected.manifest),
          ...(pluginHasToolUi(inspected.manifest)?{uiCapabilityApproved:true}:{}),
          ...(agent?.tasks ? {taskCapabilityApproved: true} : {}),
          ...(agent?.tasks && agent.targets ? {targetCapabilityApproved: true} : {}),
          ...(agent?.tasks && agent.targets && agent.delegation ? {delegationCapabilityApproved: true} : {}),
          ...(agent?.observe ? {observeCapabilityApproved: true} : {}),
          ...(agent?.companions ? {companionsCapabilityApproved: true} : {})};
        if (inspected.existing) { options.expectedInstalledApproval = ghostInstallApprovalToken(inspected.existing.approval); await stop(installedGhostStoragePart(inspected.existing)); }
        const result = inspected.existing ? await manager.update(inspected.path, options) : await manager.install(inspected.path, options);
        if (result.rejection) throw failure('PLUGIN_INSTALL_FAILED', result.rejection.reason);
        await platform.start(installedGhostStoragePart(result.ghost));
        event({type: 'changed'}); return present(result.ghost);
      }
      case 'enabled': {
        const ghost = find(params.pluginId); if (!ghost) throw failure('NOT_FOUND', '插件不存在');
        await stop(installedGhostStoragePart(ghost));
        const result = await manager.setEnabled(installedGhostPhysicalKeys(ghost).relId, params.enabled === true, ghostInstallApprovalToken(ghost.approval));
        if (params.enabled) runtime.resetFuse(installedGhostStoragePart(ghost));
        if (result?.rejection) throw failure('PLUGIN_UPDATE_FAILED', result.rejection.reason);
        if (params.enabled) await platform.start(installedGhostStoragePart(ghost));
        return {ok: true};
      }
      case 'remove': {
        const ghost = find(params.pluginId); if (!ghost) throw failure('NOT_FOUND', '插件不存在');
        if (params.confirmed !== true) throw failure('CONFIRM_REQUIRED', '请确认移除插件');
        const id = installedGhostStoragePart(ghost); await stop(id);
        const result = await platform.uninstall(id, () => manager.uninstall(installedGhostPhysicalKeys(ghost).relId));
        if (result.rejection) throw failure('PLUGIN_REMOVE_FAILED', result.rejection.reason);
        kv.remove(id);
        for (const item of ghost.manifest.network?.secrets ?? []) secrets.remove(secretName(id, item.key));
        return {ok: true};
      }
      case 'configuration': {
        const ghost = find(params.pluginId); if (!ghost) throw failure('NOT_FOUND', '插件不存在');
        const id = installedGhostStoragePart(ghost);
        return {parameters: kv.read(id), ...platform.preferenceDetails(id), secrets: (ghost.manifest.network?.secrets ?? []).map(item => ({key: item.key, label: item.label, hint: item.hint, source: item.source ?? 'user', saved: secrets.read(secretName(id, item.key)) !== null}))};
      }
      case 'configure': {
        const ghost = find(params.pluginId);
        if (!ghost || ghost.approval?.state !== 'approved') throw failure('PLUGIN_UNAVAILABLE', '请先修复插件安装批准');
        const id = installedGhostStoragePart(ghost);
        // Configuration is a Host-owned operation: a disabled plugin may be
        // prepared without executing it. Validate all fields before writing.
        if (params.taskPreferences !== undefined) await platform.validatePreferences(id, params.taskPreferences);
        const stillInstalled = find(id);
        if (!stillInstalled || ghostInstallApprovalToken(stillInstalled.approval) !== ghostInstallApprovalToken(ghost.approval)) throw failure('PLUGIN_CHANGED', '插件安装已变化，请重新打开配置');
        for (const [key, value] of Object.entries(params.secrets ?? {})) {
          const item = ghost.manifest.network?.secrets?.find(item => item.key === key && (!item.source || item.source === 'user' || item.source === 'gh-cli')) ?? ghost.manifest.node?.secretBindings?.find(item => item.key === key && !item.oauthSecret);
          if (!item || value !== null && (typeof value !== 'string' || !value.trim() || value.length > GHOST_SECRET_VALUE_MAX_CHARS)) throw failure('INVALID_PARAMS', '不能修改此插件凭证');
        }
        if (params.taskPreferences !== undefined) platform.writePreferences(id, params.taskPreferences);
        if (params.parameters !== undefined) kv.write(id, params.parameters);
        for (const [key, value] of Object.entries(params.secrets ?? {})) {
          if (value === null) secrets.remove(secretName(id, key));
          else if (!secrets.write(secretName(id, key), value.trim())) throw failure('STORE_FAILED', '插件凭证保存失败，请重试');
        }
        await stop(id);
        platform.configurationChanged(id,params);
        await platform.start(id); // Resident Node workers must resume with fresh credentials.
        return {ok: true};
      }
      case 'screen': {
        if (!['will-schedule-run', 'will-routine-run'].includes(params.point) || !params.target || !['allow', 'skip', 'block'].includes(params.target.onFailure)) throw failure('INVALID_PARAMS', '运行前检查参数无效');
        const work = gateway.screenControlPoint(params.point, params.data, params.target);
        pending.add(work);
        try { return await work; } finally { pending.delete(work); }
      }
      default: return platform.call(method, params);
    }
  }
  return {call, async close() { if (closed) return; closed = true; for (const ghost of manager.list()) await stop(installedGhostStoragePart(ghost)); await platform.close(); adapter.close(); await Promise.allSettled([...pending]); secrets.close(); }};
}
