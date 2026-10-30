// SQLite host adapter for Cindy's profile/version projection. UI and runtime
// share this authority; editable Home files never become permission settings.
import {Database} from 'bun:sqlite';
import {randomUUID} from 'node:crypto';
import path from 'node:path';
import {createOneBotPaths, requireBotId, oneBotError} from './content-service.mjs';
import {mergeBotProfileCapabilities, normalizeBotProfileModelChain, botProfileContentChanged} from '../cindy/bots/botProfileVersioning.ts';
import {buildDefaultBotIdentity} from '../cindy/bots/shared/botProfileDefaults.ts';
import {normalizeBotName} from '../cindy/bots/shared/botCreation.ts';
import {BOT_PROFILE_TEXT_MAX_BYTES} from '../cindy/bots/botProfileFolder.ts';
import {isManagedBotAvatarUrl, isSupportedBotAvatarValue, portableBotAvatarOrFallback} from '../cindy/bots/shared/botAvatarValue.ts';
import {botInvitationProgress} from '../cindy/bots/shared/botInvitation.ts';
import {inferBotTemplatePresetId} from '../cindy/bots/shared/botTemplatePreset.ts';

function object(value, label) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw oneBotError('INVALID_PARAMS', `${label} 必须是对象`);
  return value;
}
function text(value, label, max, required = false) {
  if (value === undefined && !required) return '';
  if (typeof value !== 'string' || value.length > max || required && !value.trim()) throw oneBotError('INVALID_PARAMS', `${label} 无效或过长`);
  return value.trim();
}

// SOUL/USER can be edited externally; preserving their bytes avoids repeatedly
// reconciling the same trailing whitespace and accepts the upstream Home limit.
function sourceText(value, label) {
  if (typeof value !== 'string' || Buffer.byteLength(value) > BOT_PROFILE_TEXT_MAX_BYTES) throw oneBotError('INVALID_PARAMS', `${label} 无效或超过 64 KiB`);
  return value;
}

/** Reject unsupported routes explicitly, rather than quietly substituting Pi or
 * accidentally resolving a remote account with the same local provider id.
 * Model/effort words still belong to Magpie/Pi, not a WinOne fixed whitelist.
 */
function localCapabilities(value) {
  object(value, 'capabilities');
  for (const key of ['remoteHostId', 'executionDeviceId', 'homeDeviceId', 'agentDeviceId']) {
    if (value[key] != null && value[key] !== '') throw oneBotError('UNSUPPORTED_ROUTE', 'OneBot 只在本机运行');
  }
  const route = candidate => {
    object(candidate, 'model route');
    if (candidate.harness != null && candidate.harness !== 'pi') throw oneBotError('UNSUPPORTED_HARNESS', 'OneBot 仅使用 Pi');
    if (candidate.agentDeviceId != null || typeof candidate.providerId === 'string' && candidate.providerId.startsWith('model-route:')) {
      throw oneBotError('UNSUPPORTED_ROUTE', 'OneBot 不使用远程模型路线');
    }
    return {...candidate, harness: 'pi'};
  };
  const next = route(value);
  for (const key of ['modelChain', 'modelChainOverride']) if (Array.isArray(next[key])) next[key] = next[key].map(route);
  if (next.taskModelOverride != null) next.taskModelOverride = route(next.taskModelOverride);
  if (next.permissions !== undefined && !['auto', 'ask', 'trusted', 'bypassPermissions'].includes(next.permissions)) throw oneBotError('INVALID_PARAMS', 'OneBot 权限模式无效');
  const normalized = normalizeBotProfileModelChain(next);
  if (Buffer.byteLength(JSON.stringify(normalized)) > 256 * 1024) throw oneBotError('INVALID_PARAMS', 'OneBot 配置超过 256 KiB');
  return normalized;
}

export class OneBotProfileStore {
  constructor(directory) {
    this.paths = createOneBotPaths(directory);
    this.generation = randomUUID();
    this.closed = false;
    const file = path.join(this.paths.root, 'profiles.db');
    this.paths.assertPath(file);
    this.db = new Database(file, {create: true});
    try {
      const version = this.db.query('PRAGMA user_version').get().user_version;
      if (version !== 0 && version !== 1 && version !== 2) throw oneBotError('UNSUPPORTED_VERSION', 'OneBot 档案数据库版本不兼容');
      this.db.exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA busy_timeout=5000;
        CREATE TABLE IF NOT EXISTS bot_profiles(id TEXT PRIMARY KEY, data TEXT NOT NULL);
        CREATE TABLE IF NOT EXISTS bot_profile_versions(bot_id TEXT NOT NULL, version INTEGER NOT NULL, data TEXT NOT NULL, PRIMARY KEY(bot_id,version));
        CREATE TABLE IF NOT EXISTS bot_profile_pending(bot_id TEXT PRIMARY KEY, data TEXT NOT NULL);
        CREATE TABLE IF NOT EXISTS bot_lifecycle_events(id TEXT PRIMARY KEY, data TEXT NOT NULL);
        CREATE TABLE IF NOT EXISTS bot_cleanup(bot_id TEXT PRIMARY KEY, data TEXT NOT NULL);
        CREATE TABLE IF NOT EXISTS bot_sessions(session_id TEXT PRIMARY KEY, bot_id TEXT NOT NULL, route_key TEXT NOT NULL, role TEXT NOT NULL, UNIQUE(bot_id,route_key));
        CREATE TABLE IF NOT EXISTS bot_message_metadata(session_id TEXT NOT NULL, client_id TEXT NOT NULL, data TEXT NOT NULL, PRIMARY KEY(session_id,client_id));
        CREATE TABLE IF NOT EXISTS bot_worktrees(session_id TEXT PRIMARY KEY, data TEXT NOT NULL);
        CREATE TABLE IF NOT EXISTS bot_media_draft_urls(id INTEGER PRIMARY KEY, urls TEXT NOT NULL);`);
      // v2: a link can be archived (Cindy bot_session_links.archived_at), as a
      // group lane is when its member leaves or its group is deleted.
      if (version < 2) this.db.transaction(() => {
        if (!this.db.query('PRAGMA table_info(bot_sessions)').all().some(column => column.name === 'archived_at')) this.db.exec('ALTER TABLE bot_sessions ADD COLUMN archived_at INTEGER');
        this.db.exec('PRAGMA user_version=2;');
      })();
    } catch (error) { this.db.close(true); throw error; }
  }
  assertOpen() {
    if (this.closed) throw oneBotError('NOT_READY', 'OneBot 档案库已关闭');
    this.paths.assertPath(path.join(this.paths.root, 'profiles.db'));
  }
  close() { if (!this.closed) { this.closed = true; this.db.close(true); } }
  // The Pi transcript remains authoritative. This holds only host-owned
  // learning receipts that Cindy otherwise stores in messages.agent_meta.
  messageMeta(sessionId, clientId) {
    this.assertOpen();
    return JSON.parse(this.db.query('SELECT data FROM bot_message_metadata WHERE session_id=? AND client_id=?').get(sessionId, clientId)?.data ?? '{}');
  }
  /** Every receipt row of one conversation (the page overlays learning receipts). */
  messageMetaFor(sessionId) {
    this.assertOpen();
    return this.db.query('SELECT client_id AS clientId,data FROM bot_message_metadata WHERE session_id=?').all(sessionId)
      .map(row => ({clientId: row.clientId, data: JSON.parse(row.data)}));
  }
  patchMessageMeta(sessionId, clientId, patch) {
    this.assertOpen();
    this.sessionOwner(sessionId);
    const data = {...this.messageMeta(sessionId, clientId), ...patch};
    this.db.query('INSERT OR REPLACE INTO bot_message_metadata VALUES(?,?,?)').run(sessionId, clientId, JSON.stringify(data));
    return data;
  }
  get(id) {
    this.assertOpen(); requireBotId(id);
    const row = this.db.query('SELECT data FROM bot_profiles WHERE id=?').get(id);
    if (!row) throw oneBotError('NOT_FOUND', 'OneBot 不存在');
    return JSON.parse(row.data);
  }
  list() {
    this.assertOpen();
    return this.db.query('SELECT data FROM bot_profiles').all().map(row => JSON.parse(row.data))
      .sort((a, b) => (b.pinnedAt ?? 0) - (a.pinnedAt ?? 0) || b.updatedAt - a.updatedAt || a.id.localeCompare(b.id));
  }
  assertAvailableName(name, exceptId) {
    // Match Cindy's create/rename transaction: width/case variants identify
    // the same name, and archived profiles do not reserve an active name.
    const key = normalizeBotName(name);
    if (this.list().some(profile => profile.id !== exceptId && profile.status !== 'archived' && normalizeBotName(profile.name) === key)) {
      throw oneBotError('ALREADY_EXISTS', '已存在同名 OneBot，请使用其他名称');
    }
  }
  /** Persist a new profile. `trusted.avatar` is a managed image address the
   * runtime just stored; a caller-supplied value is only an emoji or sentinel. */
  create(input, trusted = {}) {
    this.assertOpen(); object(input, 'profile');
    const id = requireBotId(input.id ?? `bot_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`);
    const name = text(input.name ?? input.displayName, 'name', 200, true);
    const description = text(input.description, 'description', 12000);
    const identitySource = text(input.identitySource, 'identitySource', 12000) || buildDefaultBotIdentity(name, description);
    const userContextSource = text(input.userContextSource, 'userContextSource', 12000);
    const capabilities = localCapabilities(mergeBotProfileCapabilities({
      previous: {permissions: 'auto', harness: 'pi', toolCapabilityVersion: 1, toolsetMode: 'inherit', mcpMode: 'inherit'},
      capabilities: {...(input.capabilities === undefined ? {} : object(input.capabilities, 'capabilities')), userContextSource},
      skills: input.skills ?? [], hasSkills: true,
    }));
    const now = Date.now();
    // Managed addresses come only from bytes ingested by the runtime (Cindy createBotProfile).
    const avatar = trusted.avatar ?? portableBotAvatarOrFallback(input.avatar === undefined ? undefined : text(input.avatar, 'avatar', 2048));
    const profile = {id, name, displayName: name, description, avatar,
      avatarColor: text(input.avatarColor, 'avatarColor', 32) || 'violet', status: 'active', hiddenAt: null, pinnedAt: null,
      attentionReason: null, attentionAt: null, currentVersion: 1, canonicalSessionId: null,
      identitySource, capabilities, createdAt: now, updatedAt: now};
    return this.db.transaction(() => {
      if (this.db.query('SELECT 1 FROM bot_profiles WHERE id=?').get(id)) throw oneBotError('ALREADY_EXISTS', 'OneBot 标识已存在');
      this.assertAvailableName(name);
      this.db.query('INSERT INTO bot_profiles VALUES(?,?)').run(id, JSON.stringify(profile));
      this.saveVersion(profile);
      this.queueContent(id, {identitySource, userContextSource});
      return profile;
    })();
  }
  saveVersion(profile) {
    this.db.query('INSERT INTO bot_profile_versions VALUES(?,?,?)').run(profile.id, profile.currentVersion,
      JSON.stringify({botId: profile.id, version: profile.currentVersion, identitySource: profile.identitySource,
        capabilities: profile.capabilities, createdAt: profile.updatedAt}));
  }
  /** Cindy updateBotProfile on this projection. `expectedVersion` is optional
   * (a settings autosave carries none); when present it is a compare-and-set. */
  update(input) {
    this.assertOpen(); object(input, 'profile'); requireBotId(input.id);
    return this.db.transaction(() => {
      const previous = this.get(input.id);
      if (input.expectedVersion !== undefined && (!Number.isSafeInteger(input.expectedVersion) || previous.currentVersion !== input.expectedVersion)) {
        throw oneBotError('PRECONDITION_FAILED', 'OneBot 配置已更新，请重新读取后重试');
      }
      const now = Math.max(Date.now(), previous.updatedAt + 1);
      const next = {...previous, updatedAt: now};
      if (input.name !== undefined || input.displayName !== undefined) next.name = next.displayName = text(input.name ?? input.displayName, 'name', 200, true);
      if (normalizeBotName(next.name) !== normalizeBotName(previous.name)) this.assertAvailableName(next.name, previous.id);
      if (input.description !== undefined) next.description = text(input.description, 'description', 12000);
      if (input.avatar !== undefined) {
        const avatar = text(input.avatar, 'avatar', 2048);
        if (!isSupportedBotAvatarValue(avatar)) throw oneBotError('INVALID_PARAMS', '头像无效');
        if (isManagedBotAvatarUrl(avatar) && avatar !== previous.avatar) throw oneBotError('INVALID_PARAMS', '请通过头像选择器上传图片');
        // A delayed full-form autosave must not roll back an avatar that won elsewhere.
        const expected = input.expectedAvatar === undefined ? previous.avatar : input.expectedAvatar;
        if (expected === previous.avatar || avatar === previous.avatar) next.avatar = avatar;
      }
      if (input.avatarColor !== undefined) next.avatarColor = text(input.avatarColor, 'avatarColor', 32, true);
      if (input.enabled !== undefined) {
        if (typeof input.enabled !== 'boolean') throw oneBotError('INVALID_PARAMS', 'enabled 必须是 boolean');
        if (!['active', 'paused'].includes(previous.status)) throw oneBotError('PRECONDITION_FAILED', 'OneBot 当前状态不能直接启用或暂停');
        next.status = input.enabled ? 'active' : 'paused';
      }
      for (const [flag, field] of [['hidden', 'hiddenAt'], ['pinned', 'pinnedAt']]) {
        if (input[flag] === undefined) continue;
        if (typeof input[flag] !== 'boolean') throw oneBotError('INVALID_PARAMS', `${flag} 必须是 boolean`);
        next[field] = input[flag] ? now : null;
      }
      if (input.identitySource !== undefined) next.identitySource = sourceText(input.identitySource, 'identitySource') || buildDefaultBotIdentity(next.name, next.description);
      const preparation = botInvitationProgress(previous.capabilities.invitation);
      const retryingPortrait = preparation?.stage === 'avatar' && previous.canonicalSessionId;
      if (preparation && preparation.stage !== 'ready' && !retryingPortrait
        && (input.name !== undefined || input.description !== undefined || input.identitySource !== undefined)) {
        throw oneBotError('PRECONDITION_FAILED', '伙伴正在准备见面，请完成准备后再编辑资料');
      }
      const requested = input.capabilities === undefined ? undefined : {...object(input.capabilities, 'capabilities')};
      if (input.userContextSource !== undefined) {
        next.capabilities = {...previous.capabilities, userContextSource: sourceText(input.userContextSource, 'userContextSource')};
      }
      const merged = mergeBotProfileCapabilities({previous: next.capabilities, capabilities: requested,
        skills: input.skills, hasSkills: Object.hasOwn(input, 'skills'), capabilityBaseline: input.capabilityBaseline});
      // Template identity and invitation progress are not caller-editable settings.
      const template = typeof previous.capabilities.templateId === 'string' ? previous.capabilities.templateId : inferBotTemplatePresetId(previous.identitySource ?? '');
      if (template) merged.templateId = template; else delete merged.templateId;
      if (previous.capabilities.invitation) merged.invitation = previous.capabilities.invitation; else delete merged.invitation;
      if (Object.hasOwn(input, 'gender')) {
        if (input.gender === 'female' || input.gender === 'male') merged.gender = input.gender; else delete merged.gender;
      }
      next.capabilities = localCapabilities(merged);
      return this.commitRevision(previous, next);
    })();
  }
  /** Store a profile revision when anything a reader can see changed; returns the current row. */
  commitRevision(previous, next) {
    const changed = botProfileContentChanged({previousCapabilities: previous.capabilities, nextCapabilities: next.capabilities,
      previousIdentitySource: previous.identitySource, nextIdentitySource: next.identitySource});
    // Metadata edits also get a revision: a stale full-form save must not
    // overwrite a concurrent rename just because runtime capabilities matched.
    if (changed || ['name', 'description', 'avatar', 'avatarColor', 'status', 'hiddenAt', 'pinnedAt'].some(key => next[key] !== previous[key])) {
      next.currentVersion = previous.currentVersion + 1;
      this.saveVersion(next);
      const content = {};
      if (next.identitySource !== previous.identitySource) content.identitySource = next.identitySource;
      if (next.capabilities.userContextSource !== previous.capabilities.userContextSource) content.userContextSource = next.capabilities.userContextSource ?? '';
      if (Object.keys(content).length) this.queueContent(next.id, content);
      this.db.query('UPDATE bot_profiles SET data=? WHERE id=?').run(JSON.stringify(next), next.id);
      return next;
    }
    return previous;
  }
  /** Cindy's `bots.updateProfile` transaction for its invitation and avatar
   * services: replace identity and capabilities under a version compare-and-set. */
  txUpdateProfile(args) {
    this.assertOpen(); object(args, 'transaction');
    return this.db.transaction(() => {
      const previous = this.get(requireBotId(args.id));
      if (args.expectedCurrentVersion !== undefined && args.expectedCurrentVersion !== previous.currentVersion) {
        throw oneBotError('PRECONDITION_FAILED', 'OneBot 配置已更新，请重新读取后重试');
      }
      if (args.expectedAvatar !== undefined && args.expectedAvatar !== previous.avatar) throw oneBotError('PRECONDITION_FAILED', 'OneBot 头像已更新');
      let capabilities;
      try { capabilities = JSON.parse(args.capabilitiesJson); } catch { throw oneBotError('INVALID_PARAMS', 'OneBot 配置格式无效'); }
      const next = {...previous, updatedAt: Math.max(args.now ?? Date.now(), previous.updatedAt + 1),
        identitySource: sourceText(args.identitySource, 'identitySource'), capabilities: localCapabilities(object(capabilities, 'capabilities'))};
      if (args.avatar !== undefined) {
        if (!isSupportedBotAvatarValue(args.avatar)) throw oneBotError('INVALID_PARAMS', '头像无效');
        next.avatar = args.avatar;
      }
      this.commitRevision(previous, next);
    })();
  }
  bindSession({botId, sessionId, routeKey = 'canonical', role = 'canonical'}) {
    this.assertOpen(); requireBotId(botId);
    if (!/^[A-Z2-7]{26}$/.test(sessionId) || typeof routeKey !== 'string' || !routeKey || routeKey.length > 512 || !['canonical','channel','history','thread','group'].includes(role)) throw oneBotError('INVALID_PARAMS', 'OneBot 会话绑定无效');
    return this.db.transaction(() => {
      const profile = this.get(botId);
      const prior = this.db.query('SELECT session_id FROM bot_sessions WHERE bot_id=? AND route_key=?').get(botId, routeKey);
      if (prior) return {sessionId: prior.session_id};
      this.db.query('INSERT INTO bot_sessions(session_id,bot_id,route_key,role) VALUES(?,?,?,?)').run(sessionId, botId, routeKey, role);
      if (role === 'canonical') {
        if (profile.canonicalSessionId && profile.canonicalSessionId !== sessionId) throw oneBotError('PRECONDITION_FAILED', 'OneBot 已有主会话');
        profile.canonicalSessionId = sessionId;
        this.db.query('UPDATE bot_profiles SET data=? WHERE id=?').run(JSON.stringify(profile), botId);
      }
      return {sessionId};
    })();
  }
  /** Cindy `bots.replaceCanonicalSession`: the new chat becomes the canonical
   * link and the previous one is kept as history, under profile-version and
   * canonical-link compare-and-sets. The Host created the conversation. */
  replaceCanonical({botId, sessionId, expectedCanonicalSessionId, expectedProfileVersion}) {
    this.assertOpen(); requireBotId(botId);
    if (!/^[A-Z2-7]{26}$/.test(sessionId) || expectedCanonicalSessionId !== null && typeof expectedCanonicalSessionId !== 'string') {
      throw oneBotError('INVALID_PARAMS', 'OneBot 主任务参数无效');
    }
    return this.db.transaction(() => {
      const profile = this.get(botId);
      if (profile.status !== 'active' && profile.status !== 'paused') throw oneBotError('PRECONDITION_FAILED', `Bot 当前状态为 ${profile.status}`);
      if (profile.currentVersion !== expectedProfileVersion) throw oneBotError('PRECONDITION_FAILED', 'Bot 配置已更新，请刷新后重试');
      const invitation = botInvitationProgress(profile.capabilities.invitation);
      if (invitation && invitation.stage !== 'ready' && invitation.stage !== 'welcome') throw oneBotError('PRECONDITION_FAILED', '伙伴正在准备见面');
      const current = this.db.query("SELECT session_id FROM bot_sessions WHERE bot_id=? AND role='canonical'").get(botId)?.session_id ?? null;
      if (current !== expectedCanonicalSessionId) throw oneBotError('PRECONDITION_FAILED', 'Bot 主任务已变化，请刷新后重试');
      if (current) this.db.query("UPDATE bot_sessions SET role='history',route_key=? WHERE session_id=?").run(`history:${current}`, current);
      this.db.query('INSERT INTO bot_sessions(session_id,bot_id,route_key,role) VALUES(?,?,?,?)').run(sessionId, botId, 'canonical', 'canonical');
      profile.canonicalSessionId = sessionId;
      this.db.query('UPDATE bot_profiles SET data=? WHERE id=?').run(JSON.stringify(profile), botId);
      return {created: true, canonicalSessionId: sessionId, archivedCanonicalSessionId: current};
    })();
  }
  findSession(botId, routeKey = 'canonical') {
    this.get(botId);
    const row = this.db.query('SELECT session_id AS sessionId,role FROM bot_sessions WHERE bot_id=? AND route_key=?').get(botId, routeKey);
    return row ?? null;
  }
  sessionOwnerOrNull(sessionId) {
    this.assertOpen();
    const row = this.db.query('SELECT bot_id,route_key,role,archived_at FROM bot_sessions WHERE session_id=?').get(sessionId);
    return row ? {botId: row.bot_id, routeKey: row.route_key, role: row.role, archivedAt: row.archived_at} : null;
  }
  sessionOwner(sessionId) {
    const owner = this.sessionOwnerOrNull(sessionId);
    if (!owner) throw oneBotError('NOT_FOUND', '该会话不属于 OneBot');
    return owner;
  }
  sessions(botId) {
    this.get(botId);
    return this.db.query('SELECT session_id AS id,route_key AS routeKey,role,archived_at AS archivedAt FROM bot_sessions WHERE bot_id=?').all(botId);
  }
  /** Active group lanes of every teammate (the group service reconciles them). */
  groupLanes() {
    this.assertOpen();
    return this.db.query("SELECT session_id AS sessionId,bot_id AS botId,route_key AS routeKey FROM bot_sessions WHERE role='group' AND archived_at IS NULL").all();
  }
  /**
   * Cindy archives a group lane when its member leaves or the group is deleted.
   * The archived link keeps its role and frees its route key, so the member's
   * next turn in that group starts a new lane, as upstream does.
   */
  archiveSessions(sessionIds, at = Date.now()) {
    this.assertOpen();
    const archive = this.db.query("UPDATE bot_sessions SET archived_at=?,route_key='archived:'||session_id WHERE session_id=? AND archived_at IS NULL");
    return this.db.transaction(() => sessionIds.filter(id => archive.run(at, id).changes === 1))();
  }
  // The SQLite transaction records a Home write intent alongside the version.
  // A disk error cannot leave a successful DB edit permanently detached from its
  // editable files; the runtime drains this journal before exposing a profile.
  queueContent(id, patch) {
    const prior = this.pendingContent(id);
    this.db.query('INSERT OR REPLACE INTO bot_profile_pending VALUES(?,?)').run(id, JSON.stringify({...prior, ...patch}));
  }
  pendingContent(id) {
    this.assertOpen(); requireBotId(id);
    const row = this.db.query('SELECT data FROM bot_profile_pending WHERE bot_id=?').get(id);
    return row ? JSON.parse(row.data) : null;
  }
  contentApplied(id) { this.assertOpen(); this.db.query('DELETE FROM bot_profile_pending WHERE bot_id=?').run(requireBotId(id)); }
  versions(id) {
    this.get(id);
    return this.db.query('SELECT data FROM bot_profile_versions WHERE bot_id=? ORDER BY version DESC').all(id).map(row => JSON.parse(row.data));
  }
  // Cindy's lifecycle transaction names, backed by this profile projection.
  // The status comparison and event commit are one SQLite transaction.
  lifecycleTransaction(name, args) {
    this.assertOpen();const id=requireBotId(args.botId);
    return this.db.transaction(()=>{
      const profile=this.get(id);
      if(name==='bots.prepareProfileDeletion'){
        profile.deletionPrepared=true;
        this.db.query('UPDATE bot_profiles SET data=? WHERE id=?').run(JSON.stringify(profile),id);return;
      }
      const transitions={'bots.pauseLifecycle':['paused','pause-requested'],'bots.resumeLifecycle':['active','resumed'],'bots.archiveLifecycle':['archived','archived']};
      const next=transitions[name];if(!next)throw oneBotError('UNSUPPORTED_OPERATION','不支持的生命周期事务');
      if(profile.status!==args.expectedProfileStatus)throw oneBotError('PRECONDITION_FAILED','OneBot 生命周期已被另一处操作更新');
      if(!Number.isSafeInteger(args.at)||typeof args.eventId!=='string'||!args.eventId)throw oneBotError('INVALID_PARAMS','生命周期事件无效');
      const sessions=this.sessions(id).length;
      profile.status=next[0];profile.updatedAt=args.at;
      if(next[0]==='archived'){
        profile.canonicalSessionId=null;
        this.db.query("UPDATE bot_sessions SET role='history' WHERE bot_id=?").run(id);
      }
      this.db.query('UPDATE bot_profiles SET data=? WHERE id=?').run(JSON.stringify(profile),id);
      this.recordLifecycleEvent({id:args.eventId,botId:id,sessionId:args.canonicalSessionId,eventType:next[1],createdAt:args.at,
        payloadJson:JSON.stringify(next[0]==='archived'?{worktreeDisposition:args.worktreeDisposition,sessions}:{})});
      return next[0]==='archived'?{sessions}:undefined;
    })();
  }
  recordLifecycleEvent(row){
    this.assertOpen();
    this.db.query('INSERT INTO bot_lifecycle_events VALUES(?,?)').run(row.id,JSON.stringify(row));
  }
  removeProfile(id){
    this.assertOpen();requireBotId(id);
    return this.db.transaction(()=>{
      const profile=this.get(id);
      if(!profile.deletionPrepared||profile.status!=='archived')throw oneBotError('PRECONDITION_FAILED','OneBot 删除前尚未停止');
      // Cleanup intent commits with profile deletion, before external IM/Home
      // cleanup. A crash cannot leave user content with no recovery record.
      this.db.query('INSERT OR REPLACE INTO bot_cleanup VALUES(?,?)').run(id,JSON.stringify({botId:id,imPending:true,homePending:true}));
      this.db.query('DELETE FROM bot_message_metadata WHERE session_id IN (SELECT session_id FROM bot_sessions WHERE bot_id=?)').run(id);
      this.db.query('DELETE FROM bot_sessions WHERE bot_id=?').run(id);
      this.db.query('DELETE FROM bot_profile_pending WHERE bot_id=?').run(id);
      this.db.query('DELETE FROM bot_profile_versions WHERE bot_id=?').run(id);
      this.db.query("DELETE FROM bot_lifecycle_events WHERE json_extract(data,'$.botId')=?").run(id);
      this.db.query('DELETE FROM bot_profiles WHERE id=?').run(id);
    })();
  }
  pendingCleanup(){
    this.assertOpen();
    return this.db.query('SELECT data FROM bot_cleanup ORDER BY bot_id').all().map(row=>JSON.parse(row.data));
  }
  cleanupCompleted(id,part){
    this.assertOpen();requireBotId(id);
    if(!['imPending','homePending'].includes(part))throw oneBotError('INVALID_PARAMS','未知清理步骤');
    const row=this.db.query('SELECT data FROM bot_cleanup WHERE bot_id=?').get(id);
    if(!row)return;
    const value=JSON.parse(row.data);value[part]=false;
    if(value.imPending||value.homePending)this.db.query('UPDATE bot_cleanup SET data=? WHERE bot_id=?').run(JSON.stringify(value),id);
    else this.db.query('DELETE FROM bot_cleanup WHERE bot_id=?').run(id);
  }
  /** A synchronous guard lets the reused storage recheck ownership immediately
   * before file writes. Lifecycle deletion must remove/mark the same registry row.
   */
  owner(id) {
    let row;
    try { row = this.get(id); } catch (error) { if (error.code === 'NOT_FOUND') return null; throw error; }
    return {key: `${this.generation}:${id}`, canonicalSessionId: row.canonicalSessionId, status: row.status,
      assertCurrent: () => { if (this.get(id).status === 'deleting') throw oneBotError('NOT_FOUND', 'OneBot 正在删除'); }};
  }
}
