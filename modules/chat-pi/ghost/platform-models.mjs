// Plugin access to the user's models: text through WinOne's AI services, images
// through its media gateway. A plugin never sees a provider, key or endpoint.
import fs from 'node:fs/promises';
import {GhostCindySlot} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/cindySlot.ts';
import {GhostModelPicker} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostModelPicker.ts';
import {sniffMediaMime} from '../cindy/ghost/apps/desktop/src/main/cindy-media/sniffMediaMime.ts';

export function createGhostModels({find, host, log, dispatcher, mediaStore, preferences}) {
  const emptyConfig = {models: [], defaults: null};
  const imageConfig = () => cached.images ?? emptyConfig;
  const cached = {images: null, videos: null, embeds: null, routes: []};
  const refresh = async () => { try {
    const catalogs = await host('media-config', {});
    cached.images = catalogs?.images ?? emptyConfig;
    cached.videos = catalogs?.videos ?? emptyConfig;
    cached.embeds = catalogs?.embeds ?? emptyConfig;
    cached.routes = await host('models', {});
  } catch { cached.images = cached.videos = cached.embeds = emptyConfig; cached.routes = []; } };
  const generated = async (method, params) => {
    const result = await host(method, params);
    return {buffer: await fs.readFile(result.path), mimeType: result.mimeType};
  };
  const resolveOneshot = model => {
    const matches = (Array.isArray(cached.routes) ? cached.routes : []).filter(row => row.id === model || row.providerId + '/' + row.id === model);
    return matches.length === 1 ? {providerId: matches[0].providerId, agentKind: 'pi', model: matches[0].id} : null;
  };
  const slot = new GhostCindySlot({
    getGhost: find, getOwnerScopeKey: () => 'local', isOwnerBoundaryPending: () => false,
    generateImage: params => generated('image-generate', params),
    editImage: params => generated('image-generate', params),
    generateVideo: params => generated('video-generate', params),
    editVideo: params => generated('video-generate', params),
    resolveOwnedMedia: (id, hash) => mediaStore.resolve(id, hash),
    getOneshotModelSelection: id => {
      const selected = preferences?.read(id);
      return selected?.model ? {providerId: selected.providerId, agentKind: selected.agentKind === 'cc' ? 'claude-code' : selected.agentKind ?? 'pi', model: selected.model} : null;
    },
    getImageConfig: imageConfig, getVideoConfig: () => cached.videos ?? emptyConfig, getEmbedConfig: () => cached.embeds ?? emptyConfig,
    embedText: params => host('embed-text', params),
    getInflightLimit: id => preferences?.inflight(id) ?? null,
    saveGhostMedia: params => mediaStore.generated(params),
    sniffDepositMime: buffer => sniffMediaMime(buffer),
    depositMedia: params => mediaStore.deposit(params),
    depositUsageBytes: id => mediaStore.usage(id),
    releaseDeposit: params => mediaStore.release(params),
    oneshotText: async ({prompt, timeoutMs, route, maxTokens}) => {
      if (route?.agentKind && route.agentKind !== 'pi') return {ok: false, reason: 'failed', message: 'WinOne 插件文本请求仅使用 Pi'};
      if (route?.model && !route.providerId) {
        const resolved = resolveOneshot(route.model);
        if (!resolved) return {ok: false, reason: 'no_candidate', message: '所选模型来源不可用或不唯一，请重新选择'};
        route = {...route, ...resolved};
      }
      const result = await host('oneshot-text', {prompt, timeoutMs, route, maxTokens});
      if (result.ok) return {ok: true, text: result.text, model: result.model};
      const reason = ['no_candidate', 'timeout'].includes(result.reason) ? result.reason : 'failed';
      return {ok: false, reason, message: result.message ?? '快速通道失败,请稍后再试'};
    },
    resolveOneshotModel: resolveOneshot,
    holdPipeCall: (id, callId, budget) => dispatcher.holdCall(id, callId, budget),
    releasePipeCall: (id, callId) => dispatcher.releaseCall(id, callId),
    claimPipeCall: (...args) => dispatcher.claimPendingCall(...args),
    settlePipeCallClaim: (...args) => dispatcher.settlePendingCallClaim(...args),
    log,
  });
  const picker = new GhostModelPicker({getGhost: find, installation: id => find(id)?.approval?.revision ?? null,
    target: () => ({id: 1, supportsRemoteRoutes: false, send: (request, cancel) => { void host('ui', {kind: 'model-picker', request}).then(answer => picker.resolve(1, request.requestId, answer?.cancelled ? null : answer?.route), cancel); return true; }, dismiss: () => {}}),
    available: async route => { const models = await host('models', {}); return route.agentKind === 'pi' && !route.agentDeviceId && models.some(model => model.id === route.model && model.providerId === route.providerId); }});
  return {
    request: async (id, message) => { await refresh(); return slot.handleModelRequest(id, message); },
    picker: (id, message) => picker.request(id, message),
  };
}
