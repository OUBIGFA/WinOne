
import { answerKey, describePermissionAction, encodeMultiSelectAnswer, normalizeAskQuestions } from '../../../../../../wechat/packages/maker-shared/src/interaction';

import type {
  PluginCompanion, PluginCompanionAnswer, PluginCompanionHistory, PluginCompanionInteraction, PluginCompanionInteractionDetail, PluginCompanionInteractionKind,
  PluginCompanionMessage, PluginCompanionReply,
} from '../../shared/pluginCompanions.js';

import { OWNER_ONLY_HOST_CONFIRMATION_TOOL_NAMES } from '../agent-app-update/constants.js';

import { summarizeInteraction } from '../../../../../../autonomy/phase.js';


/** Longest reply text handed to a plugin; longer replies are cut and marked truncated. */
export const PLUGIN_COMPANION_REPLY_MAX = 8000;

/** Most messages `history` returns, and the longest text of one of them. */
export const PLUGIN_COMPANION_HISTORY_MAX = 50;

export const PLUGIN_COMPANION_MESSAGE_MAX = 2000;

const MAX_TEXT = 200;

/** Longest plan body handed to a plugin; the rest stays in the conversation. */
export const PLUGIN_COMPANION_PLAN_MAX = 20000;

const PREVIEW_MAX = 4000;

/** The most text Cindy's own allow dialog shows whole; a longer plan or command is allowed in Cindy itself. */
export const PLUGIN_COMPANION_CONFIRM_MAX = 1200;

/** Tool arguments are summarized whole up to this length (maker-shared describePermissionAction cuts longer ones). */
const WHOLE_ARGUMENTS_MAX = 500;

/** Requests only the owner settles in Cindy itself, even though they ride the generic permission card. */
const OWNER_ONLY_TOOLS: ReadonlySet<string> = new Set([...OWNER_ONLY_HOST_CONFIRMATION_TOOL_NAMES, 'ghost_contribution_submit', 'ghost_cancel_plugin_review']);

const invalid = (): never => { throw Object.assign(new Error('INVALID_PARAMS'), { code: 'INVALID_PARAMS' }); };

const record = (value: unknown): Record<string, unknown> =>
  value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {};

const text = (value: unknown, max = MAX_TEXT) => typeof value === 'string' ? Array.from(value).slice(0, max).join('') : '';


/** What a pending request asks of the owner, as far as a plugin may handle it. */
export function companionInteractionKind(raw: unknown): PluginCompanionInteractionKind {
  const request = record(raw);
  if (request.kind === 'ask_user_question') return normalizeAskQuestions(request.questions).length ? 'question' : 'other';
  if (request.kind === 'plan_review') return typeof request.plan === 'string' ? 'plan' : 'other';
  if (request.kind !== 'permission') return 'other';
  // Cindy's own confirmations (plugin install, app update, task actions…) ride this card; they stay in Cindy.
  if (record(request.metadata).hostOwnedConfirmation !== undefined) return 'other';
  const tool = typeof request.toolName === 'string' ? request.toolName : '';
  return !tool || OWNER_ONLY_TOOLS.has(tool) || OWNER_ONLY_TOOLS.has(tool.split('__').pop() ?? '') ? 'other' : 'permission';
}


/**
 * Whether the summary of a permission shows all of what allowing it lets the companion do, so that Cindy's own
 * dialog can show the owner everything before a plugin's answer allows it: the whole command with its working
 * directory, or the paths and query read. A file write shows where, not what is written; a browser step, plugin or
 * tool call is shown whole only when its arguments are short enough not to be cut (the longest summary of them is
 * the indented JSON of the whole input). Anything else is allowed in Cindy itself.
 */
function permissionShownWhole(action: string, input: unknown): boolean {
  if (action === 'command' || action === 'file-read') return true;
  if (action !== 'browser' && action !== 'plugin' && action !== 'tool') return false;
  if (input !== undefined && (typeof input !== 'object' || input === null || Array.isArray(input))) return false;
  return (JSON.stringify(input ?? {}, null, 2) ?? '').length <= WHOLE_ARGUMENTS_MAX;
}


/** One pending request the way its conversation card shows it. */
export function companionInteractionDetail(raw: unknown, ref: string): PluginCompanionInteractionDetail {
  const request = record(raw);
  const kind = companionInteractionKind(raw);
  if (kind === 'question') {
    return {
      ref, kind,
      questions: normalizeAskQuestions(request.questions).slice(0, 20).map(question => ({
        question: text(question.question, 2000),
        header: question.header ? text(question.header) : null,
        options: (question.options ?? []).slice(0, 20).map(option => ({ label: text(option.label, 400), description: option.description ? text(option.description, 1000) : null })),
        multiSelect: question.multiSelect === true,
      })),
    };
  }
  if (kind === 'permission') {
    const toolName = text(request.toolName, 400);
    const action = describePermissionAction(toolName, record(request.input));
    return {
      ref, kind, action: action.kind, tool: text(request.displayName, 400) || text(request.title, 400) || toolName,
      subject: action.subject ? text(action.subject, 400) : null, preview: text(action.preview, PREVIEW_MAX),
      allowHere: permissionShownWhole(action.kind, request.input) && Array.from(action.preview).length <= PLUGIN_COMPANION_CONFIRM_MAX,
    };
  }
  if (kind === 'plan') {
    const chars = Array.from(String(request.plan));
    return {
      ref, kind, plan: chars.slice(0, PLUGIN_COMPANION_PLAN_MAX).join(''), truncated: chars.length > PLUGIN_COMPANION_PLAN_MAX,
      allowHere: chars.length <= PLUGIN_COMPANION_CONFIRM_MAX,
    };
  }
  return { ref, kind, title: summarizeInteraction(raw)?.summary ?? null };
}


/** The decision Cindy's own card would send for this answer; INVALID_PARAMS when it does not fit the request. */
export function companionInteractionDecision(raw: unknown, answer: PluginCompanionAnswer): Record<string, unknown> {
  const request = record(raw);
  const kind = companionInteractionKind(raw);
  if (answer.kind !== kind) return invalid();
  if (answer.kind === 'permission') return { kind: 'permission', behavior: answer.decision };
  if (answer.kind === 'plan') {
    const feedback = answer.decision === 'deny' ? answer.feedback?.trim() : undefined;
    return { kind: 'plan_review', behavior: answer.decision, ...(feedback ? { reason: feedback } : {}) };
  }
  const questions = normalizeAskQuestions(request.questions);
  if (answer.answers.length !== questions.length) return invalid();
  const answers: Record<string, string> = {};
  questions.forEach((question, index) => {
    const { choices, text: own = '' } = answer.answers[index]!;
    const labels = new Set((question.options ?? []).map(option => option.label));
    if (choices.some(choice => !labels.has(choice)) || (!question.multiSelect && choices.length > 1)) invalid();
    const value = question.multiSelect ? encodeMultiSelectAnswer(question.options ?? [], new Set(choices), own) : own.trim() || choices[0] || '';
    if (!value || value === '[]') invalid();
    answers[answerKey(question)] = value;
  });
  return { kind: 'ask_user_question', answers };
}
export {  };
