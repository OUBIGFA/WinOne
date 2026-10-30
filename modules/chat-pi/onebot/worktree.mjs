// The same preparation used by Cindy's group plans: branch naming, fresh base,
// collision handling, copied local configuration, includes, leases and registry.
import * as manager from '../cindy/worktree/main/worktree/WorktreeManager.ts';
import {prepareHandoffWorktree} from '../cindy/worktree/main/maker-ipc/handoffWorktree.ts';
import {resolveFreshSourceBranch} from '../cindy/worktree/main/worktree/freshBase.ts';
import {resolveBusinessSessionId} from './task-host.mjs';

export async function prepareGroupWorktree(projectDir) {
  const result = await prepareHandoffWorktree({
    getForSession: manager.getForSession, listAll: manager.listAll, detectCwd: manager.detectCwd,
    suggestName: manager.suggestName, listBranches: manager.listBranches, resolveCommit: manager.revParseCommit,
    createWorktree: request => manager.createWorktree(request, {trackBackgroundCheckout: true}),
    createId: () => resolveBusinessSessionId(), resolveFreshSource: resolveFreshSourceBranch,
  }, undefined, projectDir);
  if (!result.ok) return result;
  // A Pi file tool can run immediately after admission. Wait for the original
  // checkout handle before handing the directory to that first tool.
  await manager.awaitBackgroundCheckout(result.sessionId);
  return {ok: true, sessionId: result.sessionId, workingDir: result.meta.path, branch: result.meta.branch};
}
