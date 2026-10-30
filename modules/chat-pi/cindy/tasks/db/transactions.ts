import { fastModeFromStorage, fastModeToStorage } from '../core/agentCapabilities';

// inproc 回滚口：仅在 XDT_DB_INPROC=true 时使用。
// 默认热路径走 file worker（dbWorker.ts + dispatcher），这里要和同名 tx handler 保持一致。

import type Database from 'better-sqlite3';




function insertBotSession(db: Database.Database, s: Record<string, unknown>): void {
  db.prepare(`INSERT INTO sessions
    (id, title, working_dir, workspace_kind, model, effort, permission_mode, status,
     sdk_session_id, total_token_usage, total_cost_usd, context_tokens, context_window,
     fast_mode, plan_mode_enabled, cleared_at, pinned_at, user_send_at, agent_kind,
     orca_role, parent_session_id, forked_at_message_id, worktree_path, extra_dirs,
     remote_host_id, provider_id, source, created_at, updated_at, agent_device_id)
    VALUES (?, ?, ?, ?, ?, ?, ?, 'active', NULL, 0, 0, 0, 0, ?, 0, NULL, NULL, NULL,
     ?, NULL, ?, NULL, NULL, ?, ?, ?, ?, ?, ?, ?)`)
    .run(expectString(s.id, 'session.id'), expectString(s.title, 'session.title'),
      nullableString(s.workingDir), expectString(s.workspaceKind, 'session.workspaceKind'),
      expectString(s.model, 'session.model'), expectString(s.effort, 'session.effort'),
      expectString(s.permissionMode, 'session.permissionMode'), fastModeToStorage(s.fastMode),
      expectString(s.agentKind, 'session.agentKind'),
      nullableString(s.parentSessionId),
      expectString(s.extraDirs, 'session.extraDirs'), nullableString(s.remoteHostId), nullableString(s.providerId),
      expectString(s.source, 'session.source'), expectNumber(s.createdAt, 'session.createdAt'),
      expectNumber(s.updatedAt, 'session.updatedAt'), nullableString(s.agentDeviceId));
}


function botsFinishDelegation(
  db: Database.Database,
  args: unknown,
): { id: string; parentSessionId: string | null; childSessionId: string | null; targetBotId: string | null; runSequence: number; status: string } | null {
  const p = asRecord(args, 'bots.finishDelegation args');
  return db.transaction(() => {
    const values: unknown[] = [
      expectString(p.status, 'status'), nullableString(p.resultSummary),
      expectString(p.outputArtifactsJson, 'outputArtifactsJson'), nullableString(p.lastError),
    ];
    const tokenSet = p.tokensUsed === undefined ? '' : ', tokens_used = ?';
    if (p.tokensUsed !== undefined) values.push(expectNumber(p.tokensUsed, 'tokensUsed'));
    const completedAt = expectNumber(p.completedAt, 'completedAt');
    values.push(completedAt, completedAt, expectString(p.delegationId, 'delegationId'));
    let receiptGuard = '';
    if (p.expectedRunSequence !== undefined) {
      receiptGuard += ' AND run_sequence = ?';
      values.push(expectNumber(p.expectedRunSequence, 'expectedRunSequence'));
    }
    if (p.expectedExecution !== undefined) {
      const receipt = asRecord(p.expectedExecution, 'expectedExecution');
      receiptGuard += " AND json_extract(permission_snapshot_json, '$.taskExecution.runSequence') = run_sequence AND json_extract(permission_snapshot_json, '$.taskExecution.instanceId') = ? AND json_extract(permission_snapshot_json, '$.taskExecution.generation') = ?";
      values.push(expectString(receipt.instanceId, 'instanceId'), expectNumber(receipt.generation, 'generation'));
    }
    const row = db.prepare(`UPDATE bot_delegations SET status = ?, result_summary = ?, output_artifacts_json = ?, last_error = ?
      ${tokenSet}, pending_interaction_json = NULL, completed_at = ?, completion_delivered_at = NULL, updated_at = ?
      WHERE id = ? AND status IN ('queued','running','waiting') ${receiptGuard}
      RETURNING id, parent_session_id AS parentSessionId, child_session_id AS childSessionId, target_bot_id AS targetBotId, run_sequence AS runSequence, status`)
      .get(...values) as
      | { id: string; parentSessionId: string | null; childSessionId: string | null; targetBotId: string | null; runSequence: number; status: string }
      | undefined;
    if (!row) return null;
    if (row.childSessionId && row.targetBotId !== null) {
      // Only legacy Bot-to-Bot execution containers have this archive lifecycle.
      // An independent Session (target_bot_id IS NULL) outlives a turn, including
      // failure/cancellation: visibility and explicit archive belong to its user.
      db.prepare(`UPDATE sessions SET status = 'archived', updated_at = ?
        WHERE id = ? AND status = 'active'`)
        .run(completedAt, row.childSessionId);
      // 保留 route_key('delegation:<id>')作为来源标记:bot_delegations 行会随发起伙伴
      // 的删除级联消失,之后只有它能证明这段 history 是"别人派给我的",不是自己的私聊
      // (botHistoryScope.listBotRecallSessionIds)。route_key 的其它读者都只认 group 键。
      db.prepare(`UPDATE bot_session_links SET role = 'history',
        archived_at = ? WHERE session_id = ?`)
        .run(completedAt, row.childSessionId);
    }
    return row;
  })();
}


function botsCreateDelegation(db: Database.Database, args: unknown): void {
  const p = asRecord(args, 'bots.createDelegation args');
  const d = asRecord(p.delegation, 'delegation');
  const requestingBotId = expectString(d.requestingBotId, 'delegation.requestingBotId');
  const maxActiveChildren = expectNumber(p.maxActiveChildren, 'maxActiveChildren');
  const createdAt = expectNumber(d.createdAt, 'delegation.createdAt');
  db.transaction(() => {
    const count = db.prepare(`SELECT COUNT(*) AS count FROM bot_delegations
      WHERE requesting_bot_id = ? AND status IN ('queued','running','waiting')`)
      .get(requestingBotId) as { count: number };
    if (count.count >= maxActiveChildren) throw new Error('BOT_DELEGATION_CONCURRENCY_LIMIT');
    // null = 委派给一条普通 Cindy 任务(非 Bot),不建 bot_session_links 行。
    const targetBotId = nullableString(d.targetBotId);
    const targetProfileVersion = d.targetProfileVersion === null || d.targetProfileVersion === undefined
      ? null
      : expectNumber(d.targetProfileVersion, 'delegation.targetProfileVersion');
    const s = asRecord(p.session, 'session');
    insertBotSession(db, s);
    const childSessionId = expectString(d.childSessionId, 'delegation.childSessionId');
    const delegationId = expectString(d.id, 'delegation.id');
    if (targetBotId) {
      db.prepare(`INSERT INTO bot_session_links
        (id, bot_id, session_id, profile_version, role, route_key, created_at, archived_at)
        VALUES (?, ?, ?, ?, 'delegation', ?, ?, NULL)`)
        .run(`${targetBotId}:${childSessionId}`, targetBotId, childSessionId,
          targetProfileVersion,
          `delegation:${delegationId}`, createdAt);
    }
    db.prepare(`INSERT INTO bot_delegations
      (id, requesting_bot_id, target_bot_id, parent_session_id, child_session_id, objective,
       context_refs_json, artifact_refs_json, permission_snapshot_json, lineage_json,
       target_profile_version, depth, budget_tokens, tokens_used, status, result_summary,
       last_error, created_at, accepted_at, completed_at, updated_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, '[]', ?, ?, ?, ?, NULL, 0, 'queued', NULL, NULL, ?, NULL, NULL, ?)`)
      .run(delegationId, requestingBotId, targetBotId,
        expectString(d.parentSessionId, 'delegation.parentSessionId'), childSessionId,
        expectString(d.objective, 'delegation.objective'), expectString(d.contextRefsJson, 'delegation.contextRefsJson'),
        expectString(d.permissionSnapshotJson, 'delegation.permissionSnapshotJson'),
        expectString(d.lineageJson, 'delegation.lineageJson'),
        targetProfileVersion,
        expectNumber(d.depth, 'delegation.depth'),
        createdAt, createdAt);
  })();
}


function botsReopenDelegation(
  db: Database.Database,
  args: unknown,
): { reopened: boolean; previousParentSessionId: string | null } {
  const p = asRecord(args, 'bots.reopenDelegation args');
  const delegationId = expectString(p.delegationId, 'delegationId');
  const requestingBotId = expectString(p.requestingBotId, 'requestingBotId');
  const expectedStatus = expectString(p.expectedStatus, 'expectedStatus');
  if (!['completed', 'failed', 'cancelled', 'timed-out'].includes(expectedStatus)) {
    throw new Error('bots.reopenDelegation expectedStatus must be terminal');
  }
  const targetBotId = nullableString(p.targetBotId);
  const targetProfileVersion = p.targetProfileVersion === null || p.targetProfileVersion === undefined
    ? null
    : expectNumber(p.targetProfileVersion, 'targetProfileVersion');
  const reopenedAt = expectNumber(p.reopenedAt, 'reopenedAt');
  const maxActiveChildren = expectNumber(p.maxActiveChildren, 'maxActiveChildren');

  return db.transaction(() => {
    const current = db.prepare(`SELECT requesting_bot_id AS requestingBotId,
      target_bot_id AS targetBotId, target_profile_version AS targetProfileVersion,
      parent_session_id AS parentSessionId, child_session_id AS childSessionId, status
      FROM bot_delegations WHERE id = ?`).get(delegationId) as
      | {
          requestingBotId: string;
          targetBotId: string | null;
          targetProfileVersion: number | null;
          parentSessionId: string | null;
          childSessionId: string | null;
          status: string;
        }
      | undefined;
    if (
      !current
      || current.requestingBotId !== requestingBotId
      || current.targetBotId !== targetBotId
      || current.targetProfileVersion !== targetProfileVersion
      || current.status !== expectedStatus
    ) {
      return { reopened: false, previousParentSessionId: current?.parentSessionId ?? null };
    }
    const count = db.prepare(`SELECT COUNT(*) AS count FROM bot_delegations
      WHERE requesting_bot_id = ? AND id <> ? AND status IN ('queued','running','waiting')`)
      .get(requestingBotId, delegationId) as { count: number };
    if (count.count >= maxActiveChildren) throw new Error('BOT_DELEGATION_CONCURRENCY_LIMIT');

    const childSessionId = expectString(p.childSessionId, 'childSessionId');
    if (targetBotId === null) {
      // Recheck in the transaction, after any asynchronous worktree lookup.
      // Never resurrect an explicitly archived/deleted task or replace history.
      const child = db.prepare('SELECT status FROM sessions WHERE id = ?').get(childSessionId) as
        { status: string } | undefined;
      if (current.childSessionId !== childSessionId || child?.status !== 'active') {
        return { reopened: false, previousParentSessionId: current.parentSessionId };
      }
      db.prepare('UPDATE sessions SET parent_session_id = ?, updated_at = ? WHERE id = ?')
        .run(expectString(p.parentSessionId, 'parentSessionId'), reopenedAt, childSessionId);
    } else {
      insertBotSession(db, asRecord(p.session, 'session'));
    }
    if (p.worktreePath != null) {
      db.prepare('UPDATE sessions SET worktree_path = ? WHERE id = ?')
        .run(expectString(p.worktreePath, 'worktreePath'), childSessionId);
    }
    if (targetBotId) {
      db.prepare(`INSERT INTO bot_session_links
        (id, bot_id, session_id, profile_version, role, route_key, created_at, archived_at)
        VALUES (?, ?, ?, ?, 'delegation', ?, ?, NULL)`)
        .run(`${targetBotId}:${childSessionId}`, targetBotId, childSessionId,
          targetProfileVersion, `delegation:${delegationId}`, reopenedAt);
    }
    const updated = db.prepare(`UPDATE bot_delegations SET
      parent_session_id = ?, child_session_id = ?, objective = ?,
      permission_snapshot_json = ?, status = 'queued', result_summary = NULL,
      output_artifacts_json = '[]', pending_interaction_json = NULL, last_error = NULL, tokens_used = 0,
      run_sequence = run_sequence + 1, accepted_at = NULL, completed_at = NULL,
      completion_delivered_at = NULL, updated_at = ?
      WHERE id = ? AND status = ?`).run(
      expectString(p.parentSessionId, 'parentSessionId'),
      childSessionId,
      expectString(p.objective, 'objective'),
      expectString(p.permissionSnapshotJson, 'permissionSnapshotJson'),
      reopenedAt,
      delegationId,
      expectedStatus,
    );
    if (updated.changes !== 1) throw new Error('BOT_DELEGATION_REOPEN_RACE');
    return { reopened: true, previousParentSessionId: current.parentSessionId };
  })();
}


function asRecord(value: unknown, label: string): Record<string, unknown> {
  if (!isRecord(value)) throw invalidArgs(`${label} must be an object`);
  return value;
}


function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}


function expectString(value: unknown, label: string): string {
  if (typeof value !== 'string') throw invalidArgs(`${label} must be a string`);
  return value;
}


function nullableString(value: unknown): string | null {
  if (value == null) return null;
  if (typeof value !== 'string') throw invalidArgs('value must be string or null');
  return value;
}


function expectNumber(value: unknown, label: string): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    throw invalidArgs(`${label} must be a finite number`);
  }
  return value;
}


function invalidArgs(message: string): Error {
  return Object.assign(new Error(message), { code: 'INVALID_ARGS' });
}
export { botsCreateDelegation, botsFinishDelegation, botsReopenDelegation };
