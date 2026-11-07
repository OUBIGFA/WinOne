// Chat-file handoff uses Cindy's real owner/TTL/quota store. This adapter only
// supplies the current Host file authorization and the bytes it actually read.
import fs from 'node:fs/promises';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {GhostHandoffStore, GhostHandoffQuotaError} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/handoffStore.ts';
import {isDepositExcludedFileName} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/dirDeposit.ts';
import {readBoundedFileNoFollowWithStat} from '../cindy/ghost/apps/desktop/src/main/utils/readBoundedFile.ts';
import * as blobStore from '../cindy/ghost/apps/desktop/src/main/cindy-media/blobStore.ts';
import {installedGhostStoragePart} from '../cindy/ghost/apps/desktop/src/shared/pluginIdentity.ts';
import {GHOST_HANDOFF_FILES_ARG, GHOST_HANDOFF_MAX_FILES, GHOST_HANDOFF_MAX_BYTES_PER_FILE,
  GHOST_HANDOFF_MAX_BATCH_BYTES, ghostToolDeclaresHandoff} from '../cindy/ghost/apps/desktop/src/shared/ghost.ts';

const failure = (message, code = 'ATTACHMENT_INVALID') => Object.assign(Error(message), {code});
export function withoutHandoffArg(args) {
  if (!Object.hasOwn(args, GHOST_HANDOFF_FILES_ARG)) return args;
  const {[GHOST_HANDOFF_FILES_ARG]: _forged, ...rest} = args;
  return rest;
}

/** Managed media keep their existing ownership path; only non-media files move here. */
export function isHandoffCandidate(address) {
  if (/^(?:cindy-media|xdt-image|xdt-video):/i.test(address)) return false;
  const mime = blobStore.mimeForExt(path.extname(address));
  return !mime || !blobStore.supportedMime(mime);
}

export function createGhostHandoff({root, find, current, ownerScope, host, authorize, now}) {
  const store = new GhostHandoffStore({root: () => path.join(root, 'ghost-handoff'), ...(now ? {now} : {})});
  let closed = false;
  const sweep = setTimeout(() => {
    if (!closed && ownerScope.isStable()) void store.sweep().catch(() => {});
  }, 30_000);
  sweep.unref?.();

  async function prepare({ghost, tool, urls, session, signal}) {
    const declared = ghostToolDeclaresHandoff(ghost.manifest.tools?.find(value => value.name === tool));
    const unique = [...new Set(urls)];
    const media = unique.filter(value => !isHandoffCandidate(value));
    const handoff = unique.filter(isHandoffCandidate);
    const args = declared ? withoutHandoffArg : value => value;
    if (!handoff.length) return {media, args, async commit() { return {files: [], release() {}, async discard() {}}; }};
    if (!declared) throw failure('只有声明 cindy_handoff_files 的工具可接收非媒体附件；其它工具请使用 dir');
    if (unique.length > GHOST_HANDOFF_MAX_FILES) throw failure(`单次最多交接 ${GHOST_HANDOFF_MAX_FILES} 个文件`);
    if (!session?.id) throw failure('文件交接需要当前本机任务');
    const instance = installedGhostStoragePart(ghost), owner = ownerScope.capture();
    const check = () => {
      signal?.throwIfAborted();
      if (closed || !ownerScope.isCurrent(owner) || !ownerScope.isStable() || !current(ghost))
        throw failure('任务或插件授权已变化，文件未交出');
    };
    check();
    const snapshot = await host('session-snapshot', {sessionId: session.id});
    check();
    if (!snapshot || snapshot.remoteHostId != null) throw failure('文件交接需要当前本机任务');
    // The snapshot is Host-only. It is re-read before copying and dispatch, not
    // borrowed from model arguments or a previous Full Access decision.
    const authorizationKey = value => JSON.stringify([value.workingDir, value.permissionMode ?? value.permission,
      value.readOnly, value.planModeEnabled, value.permissionGeneration ?? value.authorizationGeneration, value.sessionInstanceId]);
    const initial = authorizationKey(snapshot);
    let grant;
    const assertCurrent = async () => {
      check();
      const latest = await host('session-snapshot', {sessionId: session.id});
      check();
      if (!latest || authorizationKey(latest) !== initial) throw failure('任务文件权限已变化，文件未交出');
      await grant?.assertCurrent?.();
      check();
    };
    const workingDir = snapshot.workingDir;
    const files = [], seen = new Set();
    let total = 0;
    for (const address of handoff) {
      if (!path.isAbsolute(address) && !workingDir) throw failure('附件路径需要是绝对路径');
      const originalPath = path.isAbsolute(address) ? path.resolve(address) : path.resolve(workingDir, address);
      const realPath = await fs.realpath(originalPath).catch(() => null);
      check();
      if (!realPath) throw failure('附件不可读或已被移动');
      const identity = process.platform === 'win32' ? realPath.toLowerCase() : realPath;
      if (seen.has(identity)) continue;
      seen.add(identity);
      const name = path.basename(realPath);
      if (isDepositExcludedFileName(name)) throw failure('凭证与系统文件不能交给插件：' + name);
      const stat = await fs.stat(realPath);
      check();
      if (!stat.isFile() || stat.size > GHOST_HANDOFF_MAX_BYTES_PER_FILE) throw failure('附件必须是普通文件，单个不超过 64 MiB');
      total += stat.size;
      if (total > GHOST_HANDOFF_MAX_BATCH_BYTES) throw failure('附件合计不能超过 128 MiB');
      files.push({name, originalPath, realPath, size: stat.size});
    }
    await store.assertQuota(instance, files.map(file => file.size));
    await assertCurrent();
    const approved = [];
    total = 0;
    for (const file of files) {
      const read = await readBoundedFileNoFollowWithStat(file.realPath, GHOST_HANDOFF_MAX_BYTES_PER_FILE,
        {containWithin: path.dirname(file.realPath), verifyContentStability: true, nonBlocking: true});
      await assertCurrent();
      if (!read) throw failure('附件不可安全读取：' + file.name);
      file.size = read.bytes.byteLength;
      file.sha256 = createHash('sha256').update(read.bytes).digest('hex');
      total += file.size;
      if (total > GHOST_HANDOFF_MAX_BATCH_BYTES) throw failure('附件合计不能超过 128 MiB');
      approved.push({name: file.name, bytes: read.bytes});
    }
    await store.assertQuota(instance, files.map(file => file.size));
    // Authorize the same resolved paths and bounded bytes that will be copied.
    // The Host still owns the one normal batch decision, including resolved
    // symlink targets; automatic decisions never become manual grant memory.
    const input = {ghost, session, files, signal, assertCurrent};
    if (authorize) {
      grant = await authorize(input);
      if (grant === false || grant?.ok === false) throw failure('当前任务未授权交接这些文件');
    } else {
      const receipt = await host('handoff-authorize', {sessionId: session.id, pluginId: instance,
        approved: session.approved === true, items: files.map(({name, originalPath, realPath, size, sha256}) => ({name, originalPath, path: realPath, size, sha256}))});
      if (receipt?.ok !== true) throw failure(receipt?.message ?? '当前任务未授权交接这些文件');
    }
    await assertCurrent();
    return {media, args, assertCurrent, async commit() {
      await assertCurrent();
      const release = store.hold(instance);
      if (!release) throw failure('插件正在更换来源，文件未交出');
      const capturedRoot = store.currentRoot();
      let committed = [];
      const discard = async () => {
        for (const file of committed) await store.delete(instance, file.ref, capturedRoot);
      };
      try {
        committed = await store.import(instance, approved, check, capturedRoot);
        await assertCurrent();
        return {files: committed, release, discard};
      } catch (error) {
        try { await discard(); } finally { release(); }
        if (error instanceof GhostHandoffQuotaError) throw failure('该插件的交接副本已达 512 MiB 或 200 个，请在插件详情删除旧文件');
        throw error;
      }
    }};
  }
  const assertInstance = id => { if (closed || !find(id)) throw failure('插件不存在', 'NOT_FOUND'); };
  return {
    store, prepare,
    async readHandoffFile(id, ref, signal) { signal?.throwIfAborted(); assertInstance(id); if (!current(find(id))) return null; const result = await store.read(id, ref); signal?.throwIfAborted(); return result; },
    async touchHandoffFiles(id, refs) { assertInstance(id); await store.touch(id, refs); },
    async action({pluginId, action, ref}) {
      assertInstance(pluginId);
      if (action === 'list') return store.list(pluginId);
      if (action === 'folder') return {path: await store.folder(pluginId)};
      if (action !== 'delete') throw failure('未知交接副本操作', 'INVALID_PARAMS');
      const release = store.hold(pluginId);
      if (!release) throw failure('插件正在更换来源', 'BUSY');
      try { return {existed: await store.delete(pluginId, ref)}; } finally { release(); }
    },
    remove: id => store.removeInstance(id),
    close() { closed = true; clearTimeout(sweep); },
  };
}
