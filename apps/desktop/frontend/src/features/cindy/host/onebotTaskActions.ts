// WinOne seam for Cindy lib/makerChatStore and lib/makerTransport (b7e46bc).
// WinOne Chat owns every transcript. This module keeps what the OneBot pages
// read from Cindy's chat store: which companion conversations are running, the
// rows of the private chat that is open, and its pending-interaction flags.
// Card actions use the same native Chat queue and OneBot ownership checks as
// the rest of the app; a note never overwrites the open composer draft.
import {chatCall} from '../../../bridge';
import {messageText, type Approval, type ChatSnapshot, type ChatPoll, type ChatActivities, type ChatActivity} from '../../chat-state';
import type {AgentIslandSessionActivity} from '../shared/agentIsland';
import type {AgentInputRecovery} from '../../../../../../../modules/chat-pi/cindy/tasks/shared/agentInputQueue';

export type MessageRole = 'user' | 'assistant' | 'system' | 'tool_use' | 'tool_result' | 'thinking';
/** The fields of Cindy's ChatMessage that the OneBot pages read. */
export interface ChatMessage {
  /** Stable message identity: the accepted input id for a user row, the Pi entry id otherwise. */
  clientId: string;
  role: MessageRole;
  /** Visible text of the row. */
  content: string;
  createdAt?: number|string;
  parentToolUseId?:string;explicitDelivery?:unknown;sourceGroup?:unknown;botPrivateReply?:boolean;
  assistantPhase?:string;ghostReplyPending?:boolean;isStreaming?:boolean;turnCompleted?:boolean;
  systemCardType?:string;systemCardData?:{record?:unknown};
  images?: Array<{originalName?: string}>;
  files?: Array<{name: string}>;
  /** The user input this assistant reply answers. */
  replyToClientId?: string;
  automationOrigin?: {kind: string};
  hookSource?: string;
  isSyntheticTrigger?: boolean;
  blockedByGhost?: {ghostId: string; ghostName: string; reason: string};
}
export interface SessionChatSnapshot {readonly messages: readonly ChatMessage[]; streamingText?: string}
/** Fields consumed by Cindy's terminal-failure projection for voice handoffs. */
export interface SessionChatState {messages:ChatMessage[];agentStatus:{isRunning:boolean};isStreaming:boolean;error:unknown;inputRecovery:AgentInputRecovery;usageLimitWait:unknown;turnStoppedByUser:boolean;lastStopWasSideTask:boolean}
// Cindy makerChatStore's complete native question/permission contracts. Pi's
// approval payload is projected at the boundary, not interpreted by the view.
export interface PendingPermission {requestId: string; toolName: string; input: Record<string, unknown>; sourceDescription?: string; title?: string; displayName?: string; description?: string; suggestions?: unknown[]; autoReviewUnavailable?: boolean; autoReviewReason?: string; submitting?: boolean; submissionFailed?: boolean}
export interface PendingAskUser {requestId: string; questions: {question: string; header?: string; options?: {label: string; description?: string}[]; multiSelect?: boolean}[]; delivery?: 'async'; persistId?: string; resumeOnSubmit?: boolean}
export interface AskUserDraft {requestId: string; currentIndex: number; answers: Record<string, string>; started?: boolean; customInput?: string; selectedLabels?: string[]; showCustomInput?: boolean}
export type AskUserViewerState = 'expanded' | 'minimized';
export function pendingPermission(request: Approval | null): PendingPermission | null {return request ? {requestId: request.id, toolName: request.tool, input: request.args, autoReviewUnavailable: request.autoReviewUnavailable, autoReviewReason: request.reason} : null;}
export function pendingQuestion(request: Approval | null): PendingAskUser | null {return request ? {requestId: request.id, questions: (request.questions ?? []).map(question => ({question: question.question, options: question.options?.map(label => ({label}))}))} : null;}
/** Pending interaction flags of an open conversation (Cindy's light chat state). */
export interface SessionChatLightState {
  pendingPermission: PendingPermission | null;
  pendingAskUser: PendingAskUser | null;
  askUserViewerState: AskUserViewerState;
  askUserDraft: AskUserDraft | null;
  pendingPlanReview: unknown;
  pendingPluginSetup: unknown;
  pendingIssueConfirm: unknown;
  pendingRenameSessionsConfirm: unknown;
  pendingGhostGrantConfirm: unknown;
  pendingRemoteDesktopConfirmation: unknown;
  queuePaused: boolean;
}
export const EMPTY_LIGHT_STATE: SessionChatLightState = Object.freeze({
  pendingPermission: null,
  pendingAskUser: null,
  askUserViewerState: 'expanded',
  askUserDraft: null,
  pendingPlanReview: null,
  pendingPluginSetup: null,
  pendingIssueConfirm: null,
  pendingRenameSessionsConfirm: null,
  pendingGhostGrantConfirm: null,
  pendingRemoteDesktopConfirmation: null,
  queuePaused: false,
});
const EMPTY_SNAPSHOT: SessionChatSnapshot = Object.freeze({messages: Object.freeze([]) as readonly ChatMessage[]});

const transcripts = new Map<string, SessionChatSnapshot>();
const foregroundSessions=new Set<string>();
const lights = new Map<string, SessionChatLightState>();
const sessionListeners = new Map<string, Set<() => void>>();
function notifySession(sessionId: string) {
  for (const listener of [...sessionListeners.get(sessionId) ?? []]) listener();
}
function subscribeSession(sessionId: string, listener: () => void) {
  let set = sessionListeners.get(sessionId);
  if (!set) sessionListeners.set(sessionId, set = new Set());
  set.add(listener);
  startPolling();
  return () => {
    set.delete(listener);
    if (!set.size) sessionListeners.delete(sessionId);
    stopPollingWhenIdle();
  };
}

/** The open OneBot conversation publishes its loaded rows; `null` when it closes. */
export function publishOneBotTranscript(sessionId: string, messages: readonly ChatMessage[] | null) {
  if (messages) {foregroundSessions.add(sessionId);transcripts.set(sessionId, {messages});}
  else {foregroundSessions.delete(sessionId);transcripts.delete(sessionId);}
  notifySession(sessionId);
}
/** The open OneBot conversation publishes its pending interactions; `null` when it closes. */
export function publishOneBotLight(sessionId: string, light: Partial<SessionChatLightState> | null) {
  if (light) lights.set(sessionId, {...lights.get(sessionId)??EMPTY_LIGHT_STATE, ...light});
  else lights.delete(sessionId);
  notifySession(sessionId);
}

// Running state follows the Host activity of the OneBot Chat store. The poll
// runs only while a OneBot surface subscribes, at the ordinary Chat cadence.
export interface RunningEntry {readonly isRunning: boolean}
let running: ReadonlyMap<string, RunningEntry> = new Map();
let activities: ChatActivities = {};
const allListeners = new Set<() => void>();
const activityListeners = new Set<(list: AgentIslandSessionActivity[]) => void>();
let timer: ReturnType<typeof setTimeout> | undefined;
let polling = false;
const subscribed = () => allListeners.size + activityListeners.size + sessionListeners.size > 0;
const nativeCursors = new Map<string, number>();
const loadingSessions = new Map<string, Promise<void>>();
async function refreshNativeSession(sessionId: string): Promise<void> {
  const existing = loadingSessions.get(sessionId);
  if (existing) return existing;
  const loading = (async () => {
    const [state, poll] = await Promise.all([chatCall<ChatSnapshot>('onebot.chat.state', {id: sessionId}), chatCall<ChatPoll>('onebot.chat.poll', {id: sessionId, cursor: nativeCursors.get(sessionId) ?? 0})]);
    nativeCursors.set(sessionId, poll.cursor);
    const rows = state.messages.filter(row => row.role === 'user' || row.role === 'assistant').map(row => ({clientId: row.id ?? row.requestId ?? String(row.timestamp), role: row.role as MessageRole, content: messageText(row), createdAt: row.timestamp}));
    const next = {messages: rows, streamingText: state.streaming ? [...rows].reverse().find(row => row.role === 'assistant')?.content ?? '' : ''};
    const previous = lights.get(sessionId) ?? EMPTY_LIGHT_STATE;
    const light = {...previous, pendingPermission: pendingPermission(poll.approvals.find(item => item.kind !== 'question') ?? null), pendingAskUser: pendingQuestion(poll.approvals.find(item => item.kind === 'question') ?? null), queuePaused: !!state.queuePaused};
    if (JSON.stringify(transcripts.get(sessionId)) !== JSON.stringify(next) || JSON.stringify(previous) !== JSON.stringify(light)) {
      transcripts.set(sessionId, next); lights.set(sessionId, light); notifySession(sessionId);
    }
  })().finally(() => loadingSessions.delete(sessionId));
  loadingSessions.set(sessionId, loading);
  return loading;
}

/** Cindy's island phase for a WinOne Chat phase; idle and interrupted turns show nothing. */
function islandActivity(sessionId: string, activity: ChatActivity): AgentIslandSessionActivity | null {
  const phase = activity.phase === 'running' || activity.phase === 'needs-interaction' || activity.phase === 'completed' || activity.phase === 'error' ? activity.phase : null;
  if (!phase) return null;
  // WinOne reports the phase only, without a public action summary or timing.
  return {sessionId, phase, attention: activity.attention, currentTurnActive: phase === 'running', compactDetail: '', currentActionSummary: null,
    startedAtMs: null, lastActivityAtMs: null, workflow: null, turnGeneration: null, gracefulStopState: 'none', source: 'live'};
}
const islandList = () => Object.entries(activities).flatMap(([id, value]) => islandActivity(id, value) ?? []);

async function pollActivity() {
  timer = undefined;
  if (polling || !subscribed()) return;
  polling = true;
  try {
    if (!document.hidden) {
      const activity = await chatCall<ChatActivities>('onebot.chat.activity') ?? {};
      if (JSON.stringify(activity) !== JSON.stringify(activities)) {
        activities = activity;
        const next = new Map(Object.entries(activity).map(([id, value]) => [id, {isRunning: value.phase === 'running'}]));
        if (next.size !== running.size || [...next].some(([id, entry]) => running.get(id)?.isRunning !== entry.isRunning)) {
          running = next;
          for (const listener of [...allListeners]) listener();
        }
        const list = islandList();
        for (const listener of [...activityListeners]) listener(list);
      }
      // Hidden group lanes have no ChatPage mounted to publish their question
      // and live text. Read only lanes with visible subscribers.
      const results = await Promise.allSettled([...sessionListeners.keys()].filter(id=>!foregroundSessions.has(id)).map(refreshNativeSession));
      for (const result of results) if (result.status === 'rejected') console.warn('OneBot lane refresh failed', result.reason);
    }
  } catch {
    // Keep the last known state; the next poll retries.
  } finally {
    polling = false;
    if (subscribed()) timer = setTimeout(() => void pollActivity(), 1500);
  }
}
function startPolling() {
  if (!timer && !polling) void pollActivity();
}
function stopPollingWhenIdle() {
  if (!subscribed() && timer) {
    clearTimeout(timer);
    timer = undefined;
  }
}

/** Cindy's agent-island activity feed, served from the OneBot Chat activity. */
export function subscribeOneBotActivity(listener: (list: AgentIslandSessionActivity[]) => void): () => void {
  activityListeners.add(listener);
  listener(islandList());
  startPolling();
  return () => {
    activityListeners.delete(listener);
    stopPollingWhenIdle();
  };
}

export const makerChatStore = {
  ensureInitialMessages: refreshNativeSession,
  setAskUserViewerState(sessionId: string, state: AskUserViewerState) {lights.set(sessionId, {...lights.get(sessionId) ?? EMPTY_LIGHT_STATE, askUserViewerState: state}); notifySession(sessionId);},
  setAskUserDraft(sessionId: string, draft: AskUserDraft | null) {lights.set(sessionId, {...lights.get(sessionId) ?? EMPTY_LIGHT_STATE, askUserDraft: draft}); notifySession(sessionId);},
  async respondToPermission(sessionId: string, result: CCAgentPermissionResult) {
    const pending = lights.get(sessionId)?.pendingPermission;
    if (!pending) throw Error('Permission is no longer pending');
    await chatCall('onebot.chat.permission', {id: sessionId, approvalId: pending.requestId, allow: result.behavior === 'allow', scope: 'once'});
    const current = lights.get(sessionId);
    if (current?.pendingPermission?.requestId === pending.requestId) {lights.set(sessionId, {...current, pendingPermission: null}); notifySession(sessionId);}
  },
  async answerUserQuestion(sessionId: string, requestId: string, answers: Record<string, string>) {
    const pending = lights.get(sessionId)?.pendingAskUser;
    if (pending?.requestId !== requestId) throw Error('Question is no longer pending');
    await chatCall('onebot.chat.question', {id: sessionId, approvalId: requestId, answers: pending.questions.map(question => answers[question.question] ?? '')});
    const current = lights.get(sessionId);
    if (current?.pendingAskUser?.requestId === requestId) {lights.set(sessionId, {...current, pendingAskUser: null, askUserDraft: null}); notifySession(sessionId);}
  },
  async sendMessage(sessionId: string, text: string, _model?: string, _effort?: string, _permission?: string, _workingDir?: string | null) {
    const receipt = await chatCall<{accepted: boolean}>('prompt', {id: sessionId, text, requestId: crypto.randomUUID(), preserveDraft: true});
    return receipt.accepted === true;
  },
  getSnapshot(sessionId: string): SessionChatSnapshot {
    return transcripts.get(sessionId) ?? EMPTY_SNAPSHOT;
  },
  subscribe: subscribeSession,
  getLightSnapshot(sessionId: string): SessionChatLightState {
    return lights.get(sessionId) ?? EMPTY_LIGHT_STATE;
  },
  subscribeLight: subscribeSession,
  getRunningSnapshot(): ReadonlyMap<string, RunningEntry> {
    return running;
  },
  subscribeAll(listener: () => void): () => void {
    allListeners.add(listener);
    startPolling();
    return () => {
      allListeners.delete(listener);
      stopPollingWhenIdle();
    };
  },
};

export function makerApiForSticky(_sessionId: string) {
  return {
    cancelBotDelegation: (callerSessionId: string, delegationId: string) =>
      chatCall<{ok: boolean; message?: string}>('onebot.tasks.stop', {callerSessionId, delegationId, mode: 'cancel'}),
  };
}
/** Other computers were removed with Cindy cloud; upstream only calls this for a device id. */
export function makerApiForDevice(_deviceId: string): never {
  throw new Error('remote-devices-unavailable');
}
