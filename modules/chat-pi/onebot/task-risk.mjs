// Cindy teammate-authority (2d8d3c3, Apache-2.0): ordinary teammate work keeps
// the task's permission mode; on an autonomous turn only the explicit risks of
// taskActionRisk need a visible, one-time owner decision. This module reads the
// facts that classification needs (the saved routine) and maps WinOne's Worker
// methods back to Cindy's helper calls, so /policy (before the card) and the
// Host call (after it) classify the same action the same way.
import {taskActionRisk} from '../cindy/tasks/taskActionAuthorization.ts';

const ROUTINE_TOOLS = {save: 'routine_save', delete: 'routine_delete', runNow: 'routine_run_now', preRunHook: 'schedule_set_pre_run_hook'};

/** The Cindy helper call for a Worker method and its prepared arguments. */
export function teammateCallOf(method, args = {}) {
  if (method === 'tasks.message') {
    const input = args.input ?? {};
    return {tool: 'message_session_task', args: {task_id: args.delegationId,
      ...(input.kind === 'approve' || input.kind === 'deny' ? {decision: input.kind} : {}),
      ...(input.kind === 'resume' ? {mode: 'resume'} : {})}};
  }
  const routine = /^teammate\.routines\.(\w+)$/.exec(method);
  if (routine && ROUTINE_TOOLS[routine[1]]) return {tool: ROUTINE_TOOLS[routine[1]], args};
  return null;
}

export function createOneBotTaskRisk({routines}) {
  // Cindy captureRoutine: what the card shows and what a changed rule invalidates.
  async function routineFacts(botId, id) {
    if (typeof id !== 'string' || !id) return undefined;
    const row = (await routines.call('list', {botId})).find(item => item.id === id);
    if (!row) return undefined;
    return {enabled: row.enabled !== false, prompt: row.prompt, builtin: !!row.builtin, preRunCommand: row.preRunHook?.command,
      details: {routine: {id: row.id, name: row.name, prompt: row.prompt, enabled: row.enabled !== false, triggers: row.triggers,
        ...(row.preRunHook ? {preRunHook: row.preRunHook} : {}), revision: row.revision}}};
  }
  /** {risk, details} for one teammate tool call on an autonomous turn; {} when ordinary. */
  return async function classify({botId, tool, args}) {
    const values = args && typeof args === 'object' && !Array.isArray(args) ? args : {};
    const routine = ['routine_save', 'routine_run_now', 'routine_delete'].includes(tool) ? await routineFacts(botId, values.id) : undefined;
    const risk = taskActionRisk({server: 'cindy_helper', tool, args: values},
      {routineEnabled: routine?.enabled, routinePrompt: routine?.prompt, routineBuiltin: routine?.builtin, routinePreRunCommand: routine?.preRunCommand});
    if (!risk) return {};
    return {risk, details: {tool, args: values, ...(routine ? routine.details : {})}};
  };
}
