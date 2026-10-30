// Native data/model seam for Cindy's unchanged nightly learning service.
// Reads and mutations retain the original Home locks, memory CAS and Pi files.
import {oneBotOwnerScope} from './im-scope.mjs';
import {parseBotMemoryScopeKey} from './memory-exports.ts';
import {oneBotError} from './content-service.mjs';
import {AsyncLocalStorage} from 'node:async_hooks';
const learningBot = new AsyncLocalStorage();
export const withLearningBot = (botId, signal, work) => learningBot.run({botId, signal}, work);

// Pi SessionManager persists its transcript entries synchronously before the
// completed-reply receipt. There is no second asynchronous message writer.
export async function drainPersistQueue() {}
export async function patchMessageAgentMetaWithResult(sessionId, clientId, patch) {
  return oneBotOwnerScope().profiles.patchMessageMeta(sessionId, clientId, patch);
}
const queues = new WeakMap();
export function enqueueDurableWrite(_label, work) {
  const owner = oneBotOwnerScope();
  const next = (queues.get(owner.profiles) ?? Promise.resolve()).then(() => {
    if (owner.closed) throw oneBotError('NOT_READY', 'OneBot 学习存储已关闭');
    return work(owner);
  });
  queues.set(owner.profiles, next.catch(() => {}));
  return next;
}

export function getMaker() {
  const owner = oneBotOwnerScope();
  return {makerMemory: {async getStore(scopeKey) {
    const botId = parseBotMemoryScopeKey(scopeKey);
    owner.profiles.get(botId);
    // Each method owns a short content-service lease; the helper model never
    // holds a Home mutex while waiting on the network.
    return Object.fromEntries(['list', 'read', 'write', 'update', 'delete'].map(method => [method,
      (...args) => owner.content.withMemory(botId, store => store[method](...args))]));
  }}};
}
export async function listInstalledSkills() {
  const host = oneBotOwnerScope().host;
  if (!host?.installedSkills) throw oneBotError('NOT_READY', '技能目录接口尚未就绪');
  return {entries: await host.installedSkills(learningBot.getStore()?.botId)};
}
// Magpie owns model selection and transport in WinOne. Do not load Cindy's
// account catalogue or create another set of provider credentials here.
export async function requestUtilityText(_maker, prompt, options) {
  const owner = oneBotOwnerScope();
  if (options.beforeDispatch && !await options.beforeDispatch()) return {ok: false, reason: 'aborted', attempts: []};
  if (!owner.host) return {ok: false, reason: 'no_candidate', attempts: []};
  // Both the caller's own stop (a group's 分工 decision) and a learning run's
  // cancellation end the helper call. WinOne's AI layer has no reasoning toggle,
  // so `disableReasoning` is left to the per-candidate timeout.
  const signals = [options.signal, learningBot.getStore()?.signal].filter(Boolean);
  const result = await owner.host.auxiliaryText({prompt, responseInstructions: options.systemPrompt,
    timeoutMs: options.timeoutMs, maxTokens: options.maxTokens, providerId: options.providerId, model: options.model,
    signal: signals.length > 1 ? AbortSignal.any(signals) : signals[0]});
  if (!result.ok) return {...result, reason: ['no_model', 'no_candidate'].includes(result.reason) ? 'no_candidate' : result.reason, attempts: []};
  if (options.validateResponse && !options.validateResponse(result.text)) return {ok: false, reason: 'all_candidates_failed', attempts: []};
  return {...result, attempts: []};
}
const content = (method, botId, params = {}) => oneBotOwnerScope().content.call(method, {botId, ...params});
export const listBotSkillsForBot = botId => content('skills.list', botId);
export const readBotSkillForBot = (botId, slug) => content('skills.read', botId, {slug});
export const readBotSkillSourceForBot = (botId, slug) => content('skills.readSource', botId, {slug});
export const deleteBotSkillForBot = (botId, slug) => content('skills.delete', botId, {slug});
export const saveBotSkillSourceForBot = (botId, slug, source, options) => content('skills.saveSource', botId, {slug, source, options});
/**
 * Cindy saveBotSkillForSession for the learning review: the teammate is the
 * caller chat's owner (or the review's verified source), the store and its
 * CAS are content-service's, and the runtime reloads at a safe boundary.
 */
export async function saveBotSkillForSession(params, deps = {}) {
  const owner = oneBotOwnerScope();
  try {
    const resolved = deps.resolveBotId ? await deps.resolveBotId(params.callerSessionId) : (() => {
      const link = owner.profiles.sessionOwnerOrNull(params.callerSessionId);
      return link && ['canonical', 'thread'].includes(link.role) ? {ok: true, botId: link.botId, canonicalSessionId: owner.profiles.get(link.botId).canonicalSessionId}
        : {ok: false, errorCode: 'NOT_A_BOT_SESSION', message: '当前任务不属于任何伙伴'};
    })();
    if (!resolved.ok) return resolved;
    const {record, created} = await content('skills.save', resolved.botId, {input: {name: params.name, description: params.description, body: params.body,
      expectedUpdatedAt: params.expectedUpdatedAt, ...(params.slug ? {slug: params.slug} : {})}});
    owner.event({type: 'capabilities-changed', botId: resolved.botId});
    await requestBotRuntimeEpochRefresh(resolved.canonicalSessionId ?? params.callerSessionId);
    return {ok: true, created, effective: 'next-turn', skill: {slug: record.slug, name: record.name, description: record.description, updatedAt: record.updatedAt}};
  } catch (error) {
    return {ok: false, errorCode: error?.code ?? 'INTERNAL', message: error instanceof Error ? error.message : String(error)};
  }
}
export async function requestBotRuntimeEpochRefresh(sessionId) {
  const owner = oneBotOwnerScope();
  owner.profiles.sessionOwner(sessionId);
  owner.event({type: 'runtime-refresh-requested', sessionId});
}
