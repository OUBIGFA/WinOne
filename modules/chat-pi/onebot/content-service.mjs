// WinOne host seam for Cindy's file-backed companion services. No Electron,
// cloud account, alternate harness or fallback copy of the memory/Skill rules.
import fs from 'node:fs';
import path from 'node:path';
import {Database} from 'bun:sqlite';
import {MakerMemoryStore, buildBotMemoryScopeKey, parseBotMemoryScopeKey} from './memory-exports.ts';
import {createBotMemoryService} from '../cindy/bots/botMemoryService.ts';
import {botProfileDir, botProfileMemoryDir, readBotProfileFolder, writeBotProfileFolder,
  BOT_PROFILE_TEXT_MAX_BYTES} from '../cindy/bots/botProfileFolder.ts';
import {withBotProfileLocks} from '../cindy/bots/botProfileLock.ts';
import {listBotSkills, readBotSkill, saveBotSkill, deleteBotSkill, readBotSkillSource,
  saveBotSkillSource, normalizeBotSkillSlug} from '../cindy/bots/botSkillStore.ts';
import {queryBotSkillIndex} from '../cindy/bots/botSkillQueryIndex.ts';
import {projectBotSkillMounts} from '../cindy/bots/botSkillRuntimeProjection.ts';
import {iterateBotSkillRuntimeSummaries} from '../cindy/bots/botSkillRuntimeSource.ts';
import {createBotTodoStore} from '../cindy/bots/botTodoStore.ts';
import {createLogger} from './logger.ts';

export function oneBotError(code, message) {
  return Object.assign(new Error(message), {code});
}

/** IDs are host-generated and canonical, never sanitized into a colliding Home. */
export function requireBotId(id) {
  if (typeof id !== 'string' || !/^[a-z0-9][a-z0-9_-]{0,127}$/.test(id) || /^(con|prn|aux|nul|com[0-9]|lpt[0-9])$/.test(id)) {
    throw oneBotError('INVALID_PARAMS', 'OneBot 标识无效');
  }
  return id;
}

/** Anchor the host-supplied data directory; never default to a real user profile.
 * This rejects existing junctions, symlinks and hard-linked writable files.
 * Like upstream, it is not an OS sandbox against concurrent external editors.
 */
export function createOneBotPaths(directory) {
  if (typeof directory !== 'string' || !path.isAbsolute(directory)) {
    throw oneBotError('INVALID_PARAMS', 'OneBot 必须使用宿主指定的绝对数据目录');
  }
  const root = path.resolve(directory);
  // Check the existing ancestor before recursive mkdir, so even first use does
  // not follow a pre-planted junction outside the chosen data tree.
  let existing = root;
  while (!fs.existsSync(existing)) existing = path.dirname(existing);
  for (let at = existing;; at = path.dirname(at)) {
    if (fs.lstatSync(at).isSymbolicLink()) throw oneBotError('UNSAFE_PATH', 'OneBot 数据路径不能经过链接');
    if (path.dirname(at) === at) break;
  }
  fs.mkdirSync(root, {recursive: true});
  const identity = fs.statSync(root);
  const realRoot = fs.realpathSync(root);
  function assertPath(target = root) {
    const current = fs.lstatSync(root);
    if (current.isSymbolicLink() || current.dev !== identity.dev || current.ino !== identity.ino || fs.realpathSync(root) !== realRoot) {
      throw oneBotError('UNSAFE_PATH', 'OneBot 数据目录已被替换');
    }
    const relative = path.relative(root, target);
    if (relative === '..' || relative.startsWith('..' + path.sep) || path.isAbsolute(relative)) {
      throw oneBotError('UNSAFE_PATH', 'OneBot 路径超出数据目录');
    }
    let at = root;
    const segments = relative ? relative.split(path.sep) : [];
    for (let index = 0; index < segments.length; index++) {
      at = path.join(at, segments[index]);
      let stat;
      try { stat = fs.lstatSync(at); }
      catch (error) { if (error.code === 'ENOENT') return; throw error; }
      if (stat.isSymbolicLink() || stat.isFile() && stat.nlink > 1 || index < segments.length - 1 && !stat.isDirectory()) {
        throw oneBotError('UNSAFE_PATH', 'OneBot 内容路径不能经过链接或非目录文件');
      }
    }
  }
  function home(id) {
    const value = botProfileDir(root, requireBotId(id));
    assertPath(value);
    return value;
  }
  return {root, home, assertPath};
}

/**
 * The caller owns the profile registry and supplies a captured ownership guard.
 * All entry points (settings, import and future Pi tools) use these SAME stores.
 * Do not hand raw stores to callers: a lease keeps SQLite alive until work settles.
 */
export function createOneBotContentService({directory, readBot, requestRefresh = async () => {}, maxIdleStores = 8}) {
  if (typeof readBot !== 'function') throw new TypeError('readBot is required');
  if (!Number.isSafeInteger(maxIdleStores) || maxIdleStores < 0) throw new TypeError('Invalid idle store budget');
  const paths = createOneBotPaths(directory);
  const log = createLogger('content');
  const entries = new Map();
  const pending = new Set();
  const timers = new Set();
  const owners = new Map();
  let closing = false, closed = false, closePromise;

  function assertMemoryPaths(id) {
    const memory = botProfileMemoryDir(paths.root, id);
    paths.assertPath(memory);
    // The memory service reads shards and rewrites MEMORY.md/FTS/meta. Check the
    // flat shelf, not workspace or unrelated nested resources in the Bot Home.
    if (fs.existsSync(memory)) for (const name of fs.readdirSync(memory)) {
      if (/\.md$|^meta\.json$|^fts\.db(?:-wal|-shm|-journal)?$/i.test(name)) paths.assertPath(path.join(memory, name));
    }
    return memory;
  }

  function assertOwner(id, owner) {
    if (closed) throw oneBotError('NOT_READY', 'OneBot 内容服务已关闭');
    owner.assertCurrent();
    paths.home(id);
  }

  function evictIdle() {
    const idle = [...entries].filter(([, entry]) => !entry.users);
    for (const [id, entry] of idle.slice(0, Math.max(0, idle.length - maxIdleStores))) {
      entries.delete(id);
      entry.db.close(true);
      entry.closed = true;
    }
  }

  async function memoryStore(scopeKey) {
    const id = parseBotMemoryScopeKey(scopeKey);
    const owner = owners.get(id);
    if (!owner || scopeKey !== buildBotMemoryScopeKey(id)) throw oneBotError('NOT_READY', '记忆访问没有有效的 OneBot 租约');
    assertOwner(id, owner);
    const memory = assertMemoryPaths(id);
    let entry = entries.get(id);
    if (entry && entry.ownerKey !== owner.key) {
      // No operation for this Bot can still be using the old entry: the profile
      // lock surrounds the entire call, including store acquisition and release.
      entries.delete(id);
      entry.db.close(true);
      entry.closed = true;
      entry = undefined;
    }
    if (!entry) {
      fs.mkdirSync(memory, {recursive: true});
      const db = new Database(path.join(memory, 'fts.db'), {create: true});
      entry = {db, ownerKey: owner.key, users: 1, closed: false};
      const memoryIdentity = fs.statSync(memory);
      const captured = entry;
      try {
        db.exec('PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA busy_timeout=5000;');
        // Cache statements through Bun's query API so close() finalizes them.
        // Uncached prepare() statements can keep FTS/WAL handles alive on Windows
        // until GC; using the same SQL interface does not imply the same lifetime.
        const sqlite = {exec: sql => db.exec(sql), prepare: sql => db.query(sql), transaction: fn => db.transaction(fn)};
        entry.store = new MakerMemoryStore({storageDir: memory, absWorkdir: scopeKey, db: sqlite, logger: log,
          scopeCheck() {
            if (captured.closed) throw oneBotError('NOT_READY', 'OneBot 记忆租约已结束');
            assertOwner(id, owner);
            assertMemoryPaths(id);
            const current = fs.statSync(memory);
            if (current.dev !== memoryIdentity.dev || current.ino !== memoryIdentity.ino) {
              throw oneBotError('UNSAFE_PATH', 'OneBot 记忆目录已被替换，请重新打开服务');
            }
          }});
        await entry.store.init();
        assertOwner(id, owner);
        entries.set(id, entry);
      } catch (error) { captured.closed = true; db.close(true); throw error; }
    } else entry.users = 1;
    // Refresh LRU position only on access. Active stores are never evicted.
    entries.delete(id); entries.set(id, entry);
    return entry.store;
  }

  const memory = createBotMemoryService({
    getStore: memoryStore,
    async readBot(id) { return owners.get(id) ?? null; },
    requestRefresh,
    setTimer(callback, ms) {
      const timer = setTimeout(() => { timers.delete(timer); if (!closing) callback(); }, ms);
      timer.unref?.(); timers.add(timer); return timer;
    },
    clearTimer(timer) { clearTimeout(timer); timers.delete(timer); },
  });

  function assertProfilePaths(id) {
    const home = paths.home(id);
    for (const file of ['SOUL.md', 'memories/USER.md', 'system_prompt.md', 'config.json']) paths.assertPath(path.join(home, file));
    return home;
  }

  function assertSkillPaths(id, slug) {
    const home = paths.home(id);
    const assertGenerated = folder => {
      paths.assertPath(folder);
      if (!fs.existsSync(folder)) return;
      for (const item of fs.readdirSync(folder, {withFileTypes: true})) {
        const child = path.join(folder, item.name);
        paths.assertPath(child);
        if (item.isDirectory()) assertGenerated(child);
      }
    };
    for (const shelf of ['skills', 'disabled-skills', '.runtime-skills', '.claude-plugin']) {
      const folder = path.join(home, shelf);
      paths.assertPath(folder);
      if (!fs.existsSync(folder)) continue;
      if (shelf.startsWith('.')) { assertGenerated(folder); continue; }
      // Only metadata and directories consumed by the upstream service are
      // checked. Skill scripts/assets are preserved, not executed or rewritten.
      for (const item of fs.readdirSync(folder, {withFileTypes: true})) {
        if (slug && !shelf.startsWith('.') && item.name !== slug) continue;
        const child = path.join(folder, item.name);
        paths.assertPath(child);
        if (item.isDirectory()) {
          for (const name of ['SKILL.md', 'skill.md']) paths.assertPath(path.join(child, name));
        }
      }
    }
    return home;
  }

  async function refresh(owner) {
    if (!owner.canonicalSessionId) return;
    // A committed content write must not be reported as a failed write merely
    // because refreshing a running session failed. Report the distinction.
    try { await requestRefresh(owner.canonicalSessionId); }
    catch (error) { log.warn('内容已保存，但会话刷新失败；下一轮重新载入', {message: error.message}); }
  }

  async function execute(method, params, owner) {
    // Private composition only: learning retains the same Home lock and owner
    // lease as desktop edits; this method is absent from every public table.
    if (method === 'internal.memory') return params.action(await memoryStore(buildBotMemoryScopeKey(params.botId)));
    const id = params.botId;
    const home = paths.home(id);
    if (method.startsWith('memory.')) {
      switch (method) {
        case 'memory.list': return memory.list(id, params.query);
        case 'memory.read': return memory.read(id, params.filename);
        case 'memory.update': return memory.update(params);
        case 'memory.delete': await memory.delete(params); return {ok: true};
        case 'memory.importDocument': await memory.importDocument(id, params.id, params.title, params.text, params.type); return {ok: true};
      }
      const store = await memoryStore(buildBotMemoryScopeKey(id));
      switch (method) {
        case 'memory.index': return store.getIndex();
        case 'memory.search': return store.search(params.query, params.options);
        case 'memory.write': { const result = await store.write(params.input); await refresh(owner); return result; }
        case 'memory.consolidate': { const result = await store.consolidate(params.input); await refresh(owner); return result; }
        default: throw oneBotError('UNKNOWN_METHOD', '不支持的 OneBot 记忆操作');
      }
    }
    if (method.startsWith('skills.')) {
      if (params.slug !== undefined && (typeof params.slug !== 'string' || normalizeBotSkillSlug(params.slug) !== params.slug)) {
        throw oneBotError('INVALID_PARAMS', 'Skill 标识无效');
      }
      assertSkillPaths(id, params.slug);
      switch (method) {
        case 'skills.list': return listBotSkills(paths.root, id);
        case 'skills.query': return queryBotSkillIndex(home, paths.root, id, params);
        case 'skills.read': return readBotSkill(paths.root, id, params.slug);
        case 'skills.readSource': return readBotSkillSource(paths.root, id, params.slug);
        case 'skills.save': { const result = await saveBotSkill(paths.root, id, params.input); await refresh(owner); return result; }
        case 'skills.saveSource': await saveBotSkillSource(paths.root, id, params.slug, params.source, params.options); await refresh(owner); return {ok: true};
        case 'skills.delete': { const deleted = await deleteBotSkill(paths.root, id, params.slug); await refresh(owner); return {deleted}; }
        case 'skills.mounts': return projectBotSkillMounts(home, iterateBotSkillRuntimeSummaries(paths.root, id));
        default: throw oneBotError('UNKNOWN_METHOD', '不支持的 OneBot Skill 操作');
      }
    }
    if (method === 'profile.content.read') {
      assertProfilePaths(id);
      const {config: _legacyConfig, ...content} = await readBotProfileFolder(paths.root, id, {strict: true});
      return content;
    }
    if (method === 'profile.content.write') {
      assertProfilePaths(id);
      const patch = params.patch;
      if (!patch || typeof patch !== 'object' || Array.isArray(patch) || Object.keys(patch).some(key => !['identitySource', 'userContextSource', 'systemPromptOverride'].includes(key))) {
        throw oneBotError('INVALID_PARAMS', '只能更新 OneBot 身份、用户画像和提示词内容');
      }
      // Validate ALL slots before touching any, avoiding partial saves caused by
      // a later field exceeding the upstream text boundary.
      for (const value of Object.values(patch)) if (typeof value !== 'string' || Buffer.byteLength(value) > BOT_PROFILE_TEXT_MAX_BYTES) {
        throw oneBotError('INVALID_PARAMS', 'OneBot 档案内容无效或超过 64 KiB');
      }
      await writeBotProfileFolder(paths.root, id, patch);
      await refresh(owner);
      return {ok: true};
    }
    if (method.startsWith('todos.')) {
      const file = path.join(home, 'todos.json');
      paths.assertPath(file);
      // A teammate's own patch outside an owner turn is its discovery, not an
      // owner assignment (Cindy todoForCaller ownerAuthored).
      const todos = createBotTodoStore(file, async () => ({}), () => { assertOwner(id, owner); paths.assertPath(file); }, () => method !== 'todos.ingest' && params.ownerAuthored !== false);
      switch (method) {
        case 'todos.read': return todos.read();
        case 'todos.patch': return todos.patch(params.patch);
        case 'todos.ingest': return todos.ingest(params.input);
        // These record dispatch receipts; they never pretend to execute a task.
        case 'todos.prepareAction': return todos.prepareAction(params.id, params.revision, params.requestId);
        case 'todos.settleAction': return todos.settleAction(params.id, params.requestId, params.result);
        default: throw oneBotError('UNKNOWN_METHOD', '不支持的 OneBot 待办操作');
      }
    }
    throw oneBotError('UNKNOWN_METHOD', '不支持的 OneBot 内容操作');
  }

  function call(method, params = {}) {
    if (closing) return Promise.reject(oneBotError('NOT_READY', 'OneBot 内容服务正在关闭'));
    let id, lockKey;
    try { id = requireBotId(params.botId); lockKey = paths.home(id); } catch (error) { return Promise.reject(error); }
    const operation = withBotProfileLocks([lockKey], async () => {
      const owner = await readBot(id);
      if (!owner || owner.status === 'deleting') throw oneBotError('NOT_FOUND', 'OneBot 不存在或正在删除');
      if (typeof owner.assertCurrent !== 'function' || typeof owner.key !== 'string') throw new TypeError('A captured profile ownership guard is required');
      assertOwner(id, owner);
      owners.set(id, owner);
      try {
        const result = await execute(method, params, owner);
        assertOwner(id, owner);
        return result;
      } finally {
        owners.delete(id);
        const entry = entries.get(id);
        if (entry) entry.users = 0;
        evictIdle();
      }
    });
    pending.add(operation);
    operation.then(() => pending.delete(operation), () => pending.delete(operation));
    return operation;
  }

  function close() {
    if (closePromise) return closePromise;
    closing = true;
    closePromise = (async () => {
      await Promise.allSettled([...pending]);
      for (const timer of timers) clearTimeout(timer);
      timers.clear();
      closed = true;
      for (const entry of entries.values()) { entry.closed = true; entry.db.close(true); }
      entries.clear();
    })();
    return closePromise;
  }
  // Lifecycle already owns the same Home lock. Release this Bot's idle SQLite
  // handles before the original folder cleanup, without closing other Bots.
  function releaseBot(id){
    requireBotId(id);const entry=entries.get(id);if(!entry)return;
    if(entry.users)throw oneBotError('BUSY','OneBot 记忆仍在使用');
    entry.closed=true;entry.db.close(true);entries.delete(id);
  }
  return {call, close, releaseBot, withMemory: (botId, action) => call('internal.memory', {botId, action})};
}
