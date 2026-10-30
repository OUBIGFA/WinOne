// WinOne host for Cindy's local group chats (botGroupChatService, Cindy b7e46bc,
// Apache-2.0). Cindy's service owns the timeline, rounds, 分工 plans and every
// group rule; this module supplies what Cindy's register.ts wires into it:
// each member's hidden lane in the OneBot Chat store, the native dispatch and
// its terminal receipt, the auxiliary-model 分工 decision, 分工 work
// directories and local attachments. There is no Chat Server, phone or cloud
// companion here, so those optional dependencies stay absent.
import fs from 'node:fs';
import fsp from 'node:fs/promises';
import path from 'node:path';
import {randomUUID} from 'node:crypto';
import {execFile} from 'node:child_process';
import {and, eq, notInArray} from 'drizzle-orm';
import {createBotGroupChatService} from '../cindy/tasks/botGroupChatService.ts';
import {createBotGroupPlanDecider} from '../cindy/tasks/botGroupPlanDecider.ts';
import {createBotGroupWorkDir, botGroupAttachmentsPath} from '../cindy/tasks/botGroupWorkDir.ts';
import {botGroupRuntimeFailureCode} from '../cindy/tasks/botGroupRuntimeFailure.ts';
import {BOT_GROUP_ATTACHMENTS_MAX, isBotGroupClientId, parseBotGroupIdFromRouteKey} from '../cindy/tasks/shared/botGroupChat.ts';
import {botGroupMembers, botGroups} from '../cindy/tasks/db/schema.ts';
import {visibleMessageTextForConversationSearch} from '../cindy/bots/search/conversationSearch.pure.ts';
import {requestUtilityText} from './learning-host.mjs';
import {readBranch} from './history.mjs';
import {oneBotError} from './content-service.mjs';
import {decodeGroupImage, storeAvatarImage} from './bots-service.mjs';
import {createLogger} from './logger.ts';
import {prepareGroupWorktree} from './worktree.mjs';
import {createGroupFiles} from './group-files.mjs';

const log = createLogger('bot-group');
const ATTACHMENT_CATEGORIES = new Set(['image', 'pdf', 'text', 'office', 'file']);
const MANAGED_IMAGE = /^cindy-media:\/\/blobs\/([0-9a-f]{64})\.(png|jpe?g|webp|gif)$/;
/** A staged group file (a dropped browser File without a path): Cindy's 20 MB file limit. */
const STAGED_FILE_MAX_BYTES = 20 << 20;

/** Cindy botGroupAttachments safeAttachmentFileName: one safe path segment. */
function safeAttachmentFileName(name) {
  const base = name.split(/[\\/]/).pop() ?? '';
  // eslint-disable-next-line no-control-regex
  const cleaned = base.replace(/[\u0000-\u001f<>:"|?*]/g, '_').replace(/^[.\s]+|[.\s]+$/g, '');
  return (cleaned || 'attachment').slice(0, 120);
}
async function regularFileSize(file) {
  try { const info = await fsp.stat(file); return info.isFile() ? info.size : null; } catch { return null; }
}
/** Cindy botGroupAttachments readEntry: one composer attachment as sent. */
function readEntry(value) {
  if (!value || typeof value !== 'object') return null;
  const name = typeof value.originalName === 'string' && value.originalName.trim() ? value.originalName : value.name;
  if (typeof name !== 'string' || !name.trim() || name.length > 255) return null;
  if (typeof value.path !== 'string' || typeof value.mimeType !== 'string') return null;
  if (typeof value.category !== 'string' || !ATTACHMENT_CATEGORIES.has(value.category)) return null;
  if (value.url !== undefined && typeof value.url !== 'string') return null;
  const id = typeof value.id === 'string' && value.id.length > 0 && value.id.length <= 128 ? value.id : randomUUID();
  return {id, name: name.trim(), path: value.path, category: value.category, mimeType: value.mimeType,
    url: typeof value.url === 'string' ? value.url : null, annotated: value.annotated === true};
}
// WinOne's git read environment (internal/chat gitReadEnvironment): a
// terminal's repository, ceiling, config injection and locale never apply.
const GIT_DROPPED = new Set(['GIT_DIR', 'GIT_WORK_TREE', 'GIT_COMMON_DIR', 'GIT_INDEX_FILE', 'GIT_PREFIX', 'GIT_CEILING_DIRECTORIES',
  'GIT_CONFIG_PARAMETERS', 'GIT_CONFIG_COUNT', 'GIT_OPTIONAL_LOCKS', 'LC_ALL', 'LANG']);
/** Read-only git for 分工 snapshots (status, rev-parse, diff names). */
function git(args, cwd) {
  const env = {...Object.fromEntries(Object.entries(process.env).filter(([name]) => {
    const key = name.toUpperCase();
    return !GIT_DROPPED.has(key) && !key.startsWith('GIT_CONFIG_KEY_') && !key.startsWith('GIT_CONFIG_VALUE_');
  })), GIT_OPTIONAL_LOCKS: '0', LC_ALL: 'C', LANG: 'C'};
  return new Promise((resolve, reject) => execFile('git', args, {cwd, env, timeout: 10000, windowsHide: true, maxBuffer: 16 << 20},
    (error, stdout) => error ? reject(error) : resolve(stdout)));
}

/**
 * The group service of one OneBot owner, created inside the task service's
 * scope (its task store, native task client and profile mirror).
 */
export function createOneBotGroups({scope, native, refreshProfile}) {
  const {profiles, paths, host} = scope;
  const taskStore = scope.taskStore;
  const live = () => !scope.closed;
  let service;

  /** Cindy readMembers joins the profile projection; it must be current first. */
  function syncProfiles() {
    const ids = profiles.list().map(profile => profile.id);
    for (const id of ids) refreshProfile(id);
    // A deleted teammate leaves its groups (Cindy's membership cascade).
    taskStore.drizzle.delete(botGroupMembers).where(ids.length ? notInArray(botGroupMembers.botId, ids) : undefined).run();
  }

  /** A lane whose member left or whose group is gone is archived and closed (Cindy archiveLanes). */
  async function reconcileLanes() {
    const stale = profiles.groupLanes().filter(lane => {
      const groupId = parseBotGroupIdFromRouteKey(lane.routeKey);
      return !groupId || !taskStore.drizzle.select({botId: botGroupMembers.botId}).from(botGroupMembers)
        .where(and(eq(botGroupMembers.groupId, groupId), eq(botGroupMembers.botId, lane.botId))).get();
    }).map(lane => lane.sessionId);
    if (!stale.length) return;
    archive(stale, Date.now());
    await closeLanes(stale).catch(error => log.warn('Group lanes were not closed', {error: String(error?.message ?? error)}));
  }
  /** Archive in the profile store and its task-store projection together (store hook and reconcile). */
  function archive(sessionIds, at) {
    for (const id of profiles.archiveSessions(sessionIds, at)) {
      const link = profiles.sessionOwnerOrNull(id);
      if (link) refreshProfile(link.botId);
    }
  }
  const closeLanes = sessionIds => host.closeGroupLanes(sessionIds);

  /** Cindy ensureBotGroupLaneSession: the member's lane, or its 分工 session for one plan. */
  async function ensureLane(input) {
    const routeKey = input.plan ? `group:${input.groupId}:plan:${input.plan.planId}` : `group:${input.groupId}`;
    let profile;
    try { profile = profiles.get(input.botId); } catch { return {ok: false, errorCode: 'MEMBER_UNAVAILABLE', message: '伙伴当前不可用'}; }
    if (profile.status !== 'active') return {ok: false, errorCode: 'MEMBER_UNAVAILABLE', message: '伙伴当前不可用'};
    try {
      // The Host answers NO_MODEL or MEMBER_UNAVAILABLE as Cindy does; other failures are its errors.
      const lane = await host.groupLane({botId: input.botId, routeKey, title: input.title, ...(input.plan ? {workingDir: input.plan.workDir, sessionId: input.plan.sessionId} : {})});
      if (lane.ok) refreshProfile(input.botId);
      return lane;
    } catch (error) {
      return {ok: false, errorCode: 'LANE_UNAVAILABLE', message: String(error?.message ?? error)};
    }
  }

  /** Cindy botGroupAttachments.prepare for this computer: images by managed address, files in place. */
  async function prepareAttachments(input) {
    if (input.attachments.length > BOT_GROUP_ATTACHMENTS_MAX) return {ok: false, errorCode: 'INVALID_PARAMS', message: '附件无效'};
    const attachments = [];
    for (const value of input.attachments) {
      const entry = readEntry(value);
      let attachment = null;
      if (entry?.category === 'image' && entry.url?.startsWith('cindy-media://')) {
        const match = MANAGED_IMAGE.exec(entry.url);
        const size = match ? await regularFileSize(path.join(paths.root, 'media', 'blobs', `${match[1]}.${match[2]}`)) : null;
        if (size !== null) attachment = {id: entry.id, name: entry.name, category: 'image', mimeType: entry.mimeType, size, url: entry.url, path: null, ...(entry.annotated ? {annotated: true} : {})};
      } else if (entry && path.isAbsolute(entry.path)) {
        const size = await regularFileSize(entry.path);
        // An image the media store could not take goes to the members as a plain file.
        if (size !== null) attachment = {id: entry.id, name: entry.name, category: entry.category === 'image' ? 'file' : entry.category, mimeType: entry.mimeType, size, url: null, path: entry.path};
      }
      if (!attachment) return {ok: false, errorCode: 'INVALID_PARAMS', message: '附件无效'};
      attachments.push(attachment);
    }
    // Managed images are content-addressed and kept like avatars; local files
    // stay where they are. Nothing was staged, so there is nothing to undo.
    return {ok: true, attachments, commit: () => undefined, discard: async () => undefined};
  }

  const workDir = createBotGroupWorkDir({
    ownerRoot: () => paths.root,
    detectRepo: dir => host.worktreeState(dir),
    prepareWorktree: prepareGroupWorktree,
    git,
    trashItem: fullPath => host.recycleGroupFolder(fullPath),
  });

  service = createBotGroupChatService({
    ensureLane,
    dispatch: async params => {
      const result = await native.dispatch({targetSessionId: params.targetSessionId, clientId: params.clientId, message: params.message,
        persistedContent: params.persistedContent, ...(params.toolsDisabled ? {toolsDisabled: true} : {}),
        ...(params.attachments?.length ? {attachments: params.attachments.map(item => ({name: item.name, ...(item.url ? {url: item.url} : {path: item.path})}))} : {}),
        onAccepted: params.onAccepted});
      // The native receipt of this exact input settles the member's turn.
      if (result.ok) native.watchTurn(params.targetSessionId, params.clientId, result.execution, terminal => service.settleLaneTurn({
        sessionId: params.targetSessionId, activeInputClientId: params.clientId,
        outcome: terminal.status === 'completed' ? 'done' : 'error', resultText: terminal.resultText ?? '',
        ...(terminal.status === 'completed' ? {} : {failureCode: botGroupRuntimeFailureCode({message: terminal.error})}),
      }));
      return result;
    },
    prepareAttachments,
    // Stop the lane's turn and drop the group inputs still queued in it.
    abortLane: sessionId => native.abortGroupLane(sessionId, isBotGroupClientId),
    closeLanes,
    // The Host applies the teammate's current permission whenever a lane is
    // configured, and a profile change refreshes every non-history lane.
    syncLanePermission: async () => undefined,
    hasPendingInteraction: sessionId => native.hasPendingInteraction(sessionId),
    readReplyText: async (sessionId, messageClientId) => {
      const entry = (await readBranch(await host.conversation(sessionId))).find(item => item.id === messageClientId && item.message?.role === 'assistant');
      const content = entry?.message?.content;
      const text = typeof content === 'string' ? content : (content ?? []).filter(part => part.type === 'text').map(part => part.text).join('\n');
      return visibleMessageTextForConversationSearch('assistant', JSON.stringify(text)).trim() || null;
    },
    decidePlan: createBotGroupPlanDecider((prompt, options) => requestUtilityText(null, prompt, options)),
    workDir,
    validateProjectDir: dir => host.groupProjectDir(dir),
    captureOwnerScope: () => scope,
    isOwnerScopeCurrent: value => value === scope && live(),
    onChanged: payload => scope.event({type: 'group-changed', payload}),
    log,
  });

  async function stageFile(params) {
    // A dropped browser File has no path on this computer: it is kept in the
    // group's folder (Cindy botGroupAttachmentsPath), trashed with the group.
    const groupId = typeof params.groupId === 'string' && /^[^\\/:\s]{1,128}$/.test(params.groupId) ? params.groupId : null;
    if (!groupId || !taskStore.drizzle.select({id: botGroups.id}).from(botGroups).where(eq(botGroups.id, groupId)).get()) throw oneBotError('NOT_FOUND', '群聊不存在');
    if (typeof params.name !== 'string' || !params.name.trim() || params.name.length > 255 || typeof params.base64 !== 'string') throw oneBotError('INVALID_PARAMS', '附件无效');
    const bytes = Buffer.from(params.base64, 'base64');
    if (!bytes.length || bytes.length > STAGED_FILE_MAX_BYTES) throw oneBotError('ATTACHMENT_TOO_LARGE', '附件不得超过 20 MB');
    const folder = path.join(botGroupAttachmentsPath(paths.root, groupId), randomUUID()), file = path.join(folder, safeAttachmentFileName(params.name));
    paths.assertPath(file);
    fs.mkdirSync(folder, {recursive: true});
    fs.writeFileSync(file, bytes, {flag: 'wx'});
    return {path: file, size: bytes.length};
  }

  const methods = {
    ...Object.fromEntries(Object.entries(createGroupFiles(scope)).map(([name, run]) => ['files.' + name, run])),
    list: () => service.listGroups(),
    get: params => service.getGroup(params.groupId, params.options),
    create: params => service.createGroup(params.input),
    update: params => service.updateGroup(params.input),
    setMembers: params => service.setMembers(params.input),
    delete: params => service.deleteGroup(params.groupId),
    send: params => service.sendMessage(params.input),
    continue: params => service.continueRound(params.groupId),
    stop: params => service.stopRound(params.groupId, params.scope),
    'plan.start': params => service.startPlan(params.input),
    'plan.dismiss': params => service.dismissPlan(params.input),
    'plan.continue': params => service.continuePlan(params.input),
    'plan.retry': params => service.retryPlan(params.input),
    'plan.edit': params => service.editPlanStep(params.input),
    'attachments.stage': stageFile,
    // A pasted or picked image becomes a managed, content-addressed image
    // (Cindy's media store) that the group message and member turns refer to.
    'attachments.image': params => {
      const image = decodeGroupImage(params.base64);
      return {url: storeAvatarImage(paths, image), mimeType: image.mimeType, size: image.bytes.length};
    },
  };
  let reconciled;
  return {
    /** A stored lane archive, from the task store hook: same tick as the group change. */
    archive,
    async call(method, params) {
      const run = methods[method];
      if (!run) throw oneBotError('UNKNOWN_METHOD', '不支持的群聊操作');
      syncProfiles();
      reconciled ??= reconcileLanes().catch(error => log.warn('Group lanes could not be reconciled', {error: String(error?.message ?? error)}));
      await reconciled;
      return run(params);
    },
    dispose: () => service.dispose(),
  };
}
