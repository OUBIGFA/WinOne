// Ghost's only worktree seam. Cindy's WorktreeManager remains the owner of
// Git, leases, metadata, checkout and destructive safety checks; this adapter
// supplies the plugin-owned storage scope and the Orca lifecycle hooks.
import path from 'node:path';
import fs from 'node:fs';
import {withIMScope} from '../onebot/im-scope.mjs';
import {createOneBotPaths} from '../onebot/content-service.mjs';
import * as defaultManager from '../cindy/worktree/main/worktree/WorktreeManager.ts';
import {resolveFreshSourceBranch} from '../cindy/worktree/main/worktree/freshBase.ts';
import {getManagedWorktreeReadinessForSession} from '../cindy/worktree/main/worktree/restore.ts';
import {isManagedWorktreeDirectoryName} from '../cindy/bots/shared/managedWorktreePaths.ts';

const samePath = (left, right) => {
  const a = path.resolve(left), b = path.resolve(right);
  return process.platform === 'win32' ? a.toLowerCase() === b.toLowerCase() : a === b;
};
const under = (child, parent) => {
  const a = path.resolve(child), b = path.resolve(parent);
  const normalize = value => process.platform === 'win32' ? value.toLowerCase() : value;
  const na = normalize(a), nb = normalize(b);
  return na === nb || na.startsWith(nb + path.sep);
};
const absolute = value => typeof value === 'string' && value.length > 0 && path.isAbsolute(value);
const worktreeError = (code, message, details) => Object.assign(Error(message), {code, details});

function managerCapability(manager, name) {
  if (typeof manager[name] !== 'function') {
    throw worktreeError('WORKTREE_MANAGER_CAPABILITY_MISSING', `Cindy WorktreeManager lacks ${name}; worktree was retained`);
  }
  return manager[name].bind(manager);
}

/**
 * Bind the upstream Manager to a Ghost ledger. The Manager's store is shared
 * with OneBot, so every call must carry this scope or it could read another
 * owner's profile database. The ledger already contains the canonical session
 * schema, including the worktree_path projection used by the Manager.
 */
function ownerScope(root, ledger, host) {
  const paths = createOneBotPaths(root);
  // Cindy initializes this registry before its lease reader runs. This native
  // profile has no Electron records, but keeps the same on-disk contract; real
  // runtime leases and Host reference facts still determine removability.
  const registry=path.join(paths.root,'.dev-instances');paths.assertPath(registry);fs.mkdirSync(registry,{recursive:true});
  let closed = false;
  return {
    paths,
    key: `ghost-worktree:${path.resolve(root)}`,
    get closed() { return closed; },
    close() { closed = true; },
    profiles: {
      db: ledger.sqlite,
      assertOpen() { if (closed) throw worktreeError('HOST_NOT_READY', 'Ghost worktree owner scope is closed'); },
    },
    taskStore: {
      ...ledger.db,
      query:(sql,args=[])=>ledger.sqlite.query(sql).all(...args),
      queryOne:(sql,args=[])=>ledger.sqlite.query(sql).get(...args)??null,
      // The ledger contains links, not ordinary Chat/OneBot lifetime facts.
      // A failed native read stays a failed/unknown view in Cindy's guard.
      readLocalWorktreeReferences:()=>host('task',{op:'team-native',method:'references'}),
    },
  };
}

export function createGhostWorktrees({root, ledger, host, manager = defaultManager, log = {warn() {}, info() {}}}) {
  if (!absolute(root) || !ledger?.sqlite || !ledger?.db) throw TypeError('Ghost worktree root and ledger are required');
  if (typeof host !== 'function') throw TypeError('Ghost worktree Host callback is required');
  const owner = ownerScope(root, ledger, host);
  const scoped = operation => withIMScope(owner, operation);

  const all = () => managerCapability(manager, 'listAll')();
  const getForSession = sessionId => managerCapability(manager, 'getForSession')(sessionId);

  async function detectTarget(directory) {
    if (!absolute(directory)) throw worktreeError('INVALID_PARAMS', 'Worker worktree requires an absolute project directory');
    const detected = await managerCapability(manager, 'detectCwd')(directory);
    if (!detected.gitInstalled) throw worktreeError('INVALID_PARAMS', 'git is not installed; Worker worktree was not created');
    if (!detected.isGitRepo || !detected.repoRoot) throw worktreeError('INVALID_PARAMS', 'Worker worktree target is not a Git repository');
    if (!detected.isInsideWorktree) return {baseRepo: detected.repoRoot, detected};
    const registered = all().find(meta => under(directory, meta.path));
    if (!registered) throw worktreeError('INVALID_PARAMS', 'Worker directory is inside an unregistered worktree');
    return {baseRepo: registered.baseRepo, registered, detected};
  }

  async function branchAt(directory, fallback) {
    const branches = await managerCapability(manager, 'listBranches')(directory);
    if (branches.current && branches.current !== 'HEAD') return branches.current;
    const commit = await managerCapability(manager, 'revParseCommit')(directory, 'HEAD');
    return commit ?? fallback;
  }

  async function resolve(directory, sourceBranch, leadDirectory = null) {
    return scoped(async () => {
      const target = await detectTarget(directory);
      if (typeof sourceBranch === 'string' && sourceBranch.trim()) {
        return {sourceBranch: sourceBranch.trim(), source: 'explicit'};
      }
      const targetFallback = await branchAt(directory, target.detected.currentBranch ?? 'HEAD');
      if (absolute(leadDirectory)) {
        try {
          const lead = await detectTarget(leadDirectory);
          if (samePath(lead.baseRepo, target.baseRepo)) {
            return {sourceBranch: await branchAt(leadDirectory, targetFallback), source: 'current'};
          }
        } catch {
          // A different or unavailable Lead directory must not grant access to
          // this repository; the target repository's fresh-base policy applies.
        }
      }
      const fresh = await resolveFreshSourceBranch(target.baseRepo, targetFallback);
      return {
        sourceBranch: fresh.sourceBranch,
        source: 'current',
        ...(fresh.reason ? {fallbackReason: fresh.reason} : {}),
      };
    });
  }

  async function create(sessionId, directory, sourceBranch) {
    return scoped(async () => {
      if (typeof sessionId !== 'string' || !sessionId.trim()) throw worktreeError('INVALID_PARAMS', 'Worker session id is required');
      const target = await detectTarget(directory);
      const suggestName = managerCapability(manager, 'suggestName');
      const createWorktree = managerCapability(manager, 'createWorktree');
      const name = await suggestName(target.baseRepo);
      const result = await createWorktree({sessionId, baseRepo: target.baseRepo, name, sourceBranch}, {
        trackBackgroundCheckout: true,
        // A Worker may start immediately after this callback. Never expose a
        // directory while Cindy's deferred checkout is still writing files.
        awaitFullCheckout: true,
      });
      if (!result?.ok) {
        const error = result?.error;
        throw worktreeError('WORKTREE_CREATE_FAILED', error?.message ?? 'Worker worktree creation failed', error);
      }
      const stored = getForSession(sessionId);
      if (!stored || stored.sessionId !== sessionId || !samePath(stored.path, result.meta.path)) {
        throw worktreeError('WORKTREE_METADATA_UNAVAILABLE', 'Worker worktree metadata was not bound to the created Worker session');
      }
      return stored.path;
    });
  }

  async function workerRow(sessionId) {
    return scoped(() => getForSession(sessionId));
  }

  // The manager decides whether a session is still allowed to own the path.
  // An archived/missing native row is the only state in which failed bootstrap
  // compensation may remove a pre-created checkout.
  async function canRemove(sessionId) {
    try {
      const row = await host('task', {op: 'team-native', method: 'get', taskId: sessionId});
      return !row || row.status === 'archived';
    } catch (error) {
      log.warn(`Ghost worktree removal guard failed for ${sessionId}: ${error.message}`);
      return false;
    }
  }

  async function discard(sessionId) {
    return scoped(async () => {
      const meta = getForSession(sessionId);
      if (!meta) return {status: 'absent'};
      const options = {canRemove: () => canRemove(sessionId)};
      const cancel = manager.cancelPrecreatedWorktree ?? manager.discardPrecreatedWorktree;
      if (typeof cancel !== 'function') {
        throw worktreeError('WORKTREE_MANAGER_CAPABILITY_MISSING', 'Cindy WorktreeManager lacks pre-created worktree compensation; worktree was retained');
      }
      const result = manager.cancelPrecreatedWorktree
        ? await manager.cancelPrecreatedWorktree(sessionId, {path: meta.path}, options)
        : await manager.discardPrecreatedWorktree(sessionId, meta.path, options);
      if (result?.status === 'preserved') log.info(`Preserved failed Ghost Worker worktree ${meta.path}`);
      return result ?? {status: 'preserved'};
    });
  }

  async function archive(sessionId) {
    return scoped(async () => {
      const meta = getForSession(sessionId);
      if (!meta) return {status: 'absent'};
      const remove = managerCapability(manager, 'removeWorktreeForSession');
      await remove(sessionId, {
        // Cindy's normal archive path snapshots dirty content and retains the
        // checkout when keep/dirty/live-reference guards do not allow removal.
        canRemove: () => canRemove(sessionId),
      });
      const remaining = getForSession(sessionId);
      if (remaining) {
        log.info(`Preserved archived Ghost Worker worktree ${remaining.path}`);
        return {status: 'preserved', path: remaining.path};
      }
      return {status: 'removed', path: meta.path, branch: meta.branch};
    });
  }

  return {
    resolve,
    create,
    get: workerRow,
    discard,
    archive,
    async restore(sessionId,expectedWorkingDir) {
      return scoped(async()=>{
        if(!isManagedWorktreeDirectoryName(path.basename(path.dirname(expectedWorkingDir))))return {ready:true};
        const row=await host('task',{op:'team-native',method:'get',taskId:sessionId});
        const directory=row?.directory??row?.workingDir;
        if(!row||!absolute(directory)||!samePath(directory,expectedWorkingDir))throw worktreeError('STALE_REVISION','Native Worker directory changed before restore');
        // Restore reads the original schema. Project only this Host-verified
        // binding; do not mirror transcripts or accept a plugin-supplied cwd.
        ledger.created(sessionId);
        ledger.sqlite.query('UPDATE sessions SET working_dir=?,status=? WHERE id=?').run(directory,row.status,sessionId);
        const result=await getManagedWorktreeReadinessForSession(sessionId,directory);
        return {ready:result==='ready',reason:result};
      });
    },
    close() { owner.close(); },
  };
}
