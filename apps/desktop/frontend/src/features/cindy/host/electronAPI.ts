// WinOne seam for Cindy's preload `window.electronAPI` (Apache-2.0, Cindy
// b7e46bc). Only the members used by vendored Cindy features exist. Scheduler
// calls go to the Host `automations.*` methods and OneBot calls to `onebot.*`
// (onebotPreload.ts); events come from Host long polls instead of Electron IPC.
// Cindy-cloud and other-device features are removed by product decision, so
// `deviceLink` only takes upstream's unavailable path.
import {chatCall, pickDirectory, pickFile} from '../../../bridge';
import {oneBotPreload} from './onebotPreload';
import {oneBotGroupFilePreload} from './onebotGroupFiles';
import type {
  CreateScheduleInput,
  PreRunHookRunResult,
  Schedule,
  ScheduleRun,
  ScheduleTemplate,
  SchedulerEvent,
  SchedulerRuntimeSnapshot,
  UpdateScheduleInput,
} from '../../../../../../../third_party/cindy/scheduler/types';
import type {OpenPathResult} from '../shared/openPathResult';
import type {AgentKind} from './modelProviders';
import type {UtilityTextFailure} from '../shared/utilityTextResult';
import type {Session} from './sessionTypes';

export type ProjectAutomationReconciledEvent = {
  type: 'reconciled';
  workingDir: string;
  inserted: number;
  updated: number;
  deleted: number;
  isFirstTime: boolean;
  hashChanged: boolean;
};
type HookTestResult = Omit<PreRunHookRunResult, 'checkSucceeded'>;
/** The fields of Cindy's AuthStateChangePayload that vendored stores read. */
interface AuthStateChangePayload {isAuthenticated: boolean; dataOwnerId: string | null; ownerGeneration: number; ownerBoundaryPending?: boolean}
type Unsubscribe = () => void;
type OwnerStamp = unknown;
type ScheduleListener = (event: SchedulerEvent, ownerStamp?: OwnerStamp) => void;
type ProjectListener = (event: ProjectAutomationReconciledEvent, ownerStamp?: OwnerStamp) => void;

const call = <T>(method: string, params: unknown = {}) => chatCall<T>('automations.' + method, params);
const SCHEDULER_EVENTS = new Set(['fired', 'completed', 'failed', 'silenced', 'notified', 'deferred', 'skipped', 'session-bound', 'changed', 'runtime-state', 'read', 'all-read', 'ready']);

// One Host long-poll serves every subscriber. A subscriber added while the loop
// runs joins it; the loop ends after the last one leaves and its pending poll
// returns (the Host holds a poll for at most 25 seconds).
const scheduleListeners = new Set<ScheduleListener>();
const projectListeners = new Set<ProjectListener>();
let polling = false;
const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));
function hasListeners() { return scheduleListeners.size + projectListeners.size > 0; }
function deliver(raw: unknown) {
  if (!raw || typeof raw !== 'object') return;
  const event = raw as {type?: unknown};
  if (event.type === 'reconciled') {
    for (const listener of [...projectListeners]) listener(raw as ProjectAutomationReconciledEvent);
  } else if (typeof event.type === 'string' && SCHEDULER_EVENTS.has(event.type)) {
    for (const listener of [...scheduleListeners]) listener(raw as SchedulerEvent);
  }
}
async function pollEvents() {
  if (polling) return;
  polling = true;
  let cursor: number | undefined;
  let failures = 0;
  try {
    while (hasListeners()) {
      try {
        if (cursor === undefined) {
          // A cursor beyond the Host sequence returns at once with the current
          // position, so subscribing never replays events already handled.
          const start = (await call<{cursor?: unknown}>('events', {cursor: Number.MAX_SAFE_INTEGER}))?.cursor;
          if (typeof start !== 'number' || !Number.isSafeInteger(start) || start < 0) throw new Error('Invalid automation event cursor');
          cursor = start;
          if (failures) deliver({type: 'ready'});
          failures = 0;
          continue;
        }
        const started = Date.now();
        const page = await call<{cursor?: unknown; events?: unknown}>('events', {cursor});
        if (!page || typeof page.cursor !== 'number' || !Number.isSafeInteger(page.cursor) || !Array.isArray(page.events)) throw new Error('Invalid automation event page');
        // The Host holds an empty poll; an instant empty reply must not spin.
        if (!page.events.length && page.cursor === cursor && Date.now() - started < 200) await wait(1000);
        if (page.cursor < cursor) {
          // The Host process restarted: its sequence began again. Stores reload.
          cursor = page.cursor;
          deliver({type: 'ready'});
          continue;
        }
        cursor = page.cursor;
        failures = 0;
        for (const event of page.events as unknown[]) deliver(event);
      } catch {
        failures++;
        cursor = undefined;
        await wait(Math.min(10_000, 500 * 2 ** Math.min(failures, 5)));
      }
    }
  } finally {
    polling = false;
    if (hasListeners()) void pollEvents();
  }
}
function subscribe<T>(set: Set<T>, listener: T): Unsubscribe {
  set.add(listener);
  void pollEvents();
  return () => { set.delete(listener); };
}

// Session list changes inside WinOne Chat. ChatPage and the session seam
// announce them; Cindy's push channels are mapped onto this event.
export const SESSIONS_CHANGED_EVENT = 'winone:cindy-sessions-changed';
export function announceSessionsChanged(sessionIds: readonly string[] = []) {
  window.dispatchEvent(new CustomEvent(SESSIONS_CHANGED_EVENT, {detail: {sessionIds}}));
}
// The event names the changed conversations only; the patch is empty, so
// consumers that react to particular fields re-read through their own channel.
function sessionsPush(listener: (payload: {sessionId: string; patch: Partial<Session>}, ownerStamp: OwnerStamp) => void): Unsubscribe {
  const receive = (event: Event) => {
    const ids = (event as CustomEvent<{sessionIds?: string[]}>).detail?.sessionIds ?? [];
    for (const sessionId of ids) listener({sessionId, patch: {}}, undefined);
  };
  window.addEventListener(SESSIONS_CHANGED_EVENT, receive);
  return () => window.removeEventListener(SESSIONS_CHANGED_EVENT, receive);
}

const automationsAPI = {
  platform: 'win32' as string,
  maker: {
    schedule: {
      list: (filter?: {status?: 'active' | 'paused' | 'expired'}) => call<Schedule[]>('list', filter ? {filter} : {}),
      listTemplates: async () => (await call<{templates: ScheduleTemplate[]}>('templates')).templates,
      get: (id: string) => call<Schedule | null>('get', {id}),
      create: (input: CreateScheduleInput) => call<Schedule>('create', {input}),
      update: (id: string, patch: UpdateScheduleInput) => call<Schedule>('update', {id, patch}),
      delete: async (id: string): Promise<void> => { await call('delete', {id}); },
      pause: (id: string) => call<Schedule>('pause', {id}),
      resume: (id: string) => call<Schedule>('resume', {id}),
      runNow: (id: string) => call<{runId: string}>('run', {id}),
      testPreRunHook: (params: {command: string; timeoutMs?: number; workingDir?: string; targetSessionId?: string; scheduleName?: string}) =>
        call<HookTestResult>('hook.test', params),
      generatePreRunHook: (params: {description: string; scheduleName?: string; workingDir?: string; providerId?: string; agentKind?: AgentKind; model?: string; targetSessionId?: string; resolveBoundSessionRoute?: boolean; currentCommand?: string}) =>
        call<{ok: true; command: string; filePath: string; content: string; test: HookTestResult} | UtilityTextFailure>('hook.generate', params),
      listRuns: (id: string, limit?: number) => call<ScheduleRun[]>('runs', {id, limit: limit ?? 50}),
      listSidebarIndexRuns: () => call<unknown>('sidebarIndexRuns'),
      deleteRun: async (runId: string): Promise<void> => { await call('deleteRun', {runId}); },
      getInflightCount: (id: string) => call<number>('inflightCount', {id}),
      getRuntimeState: () => call<SchedulerRuntimeSnapshot>('runtime'),
      markRunRead: async (runId: string): Promise<void> => { await call('markRead', {runId}); },
      markAllRunsRead: () => call<number>('markAllRead'),
      /** Script capability readiness: a connector method mapped in Chat MCP settings counts as installed. */
      scriptCapabilityStatus: () => call<{statuses: Array<{capability: string; state: 'ok' | 'ghost-missing' | 'ghost-asleep'; ghostName?: string}>}>('capabilityStatus'),
      onEvent: (listener: ScheduleListener) => subscribe(scheduleListeners, listener),
    },
    projectAutomation: {
      reconcile: (params: {workingDir: string}) => call<unknown>('project.reconcile', params),
      upsertSchedule: (params: {workingDir: string; config: unknown}) => call<unknown>('project.upsertSchedule', params),
      removeSchedule: (params: {workingDir: string; id: string}) => call<unknown>('project.removeSchedule', params),
      onEvent: (listener: ProjectListener) => subscribe(projectListeners, listener),
    },
    // WinOne model catalogs refresh from AI services, not on picker open.
    requestProviderModelsAutoRefresh: async (_reason: string): Promise<void> => {},
  },
  localDb: {
    sessionsPush: {onPatched: sessionsPush, onCreated: sessionsPush},
  },
  /** Opens an automation working directory or its project configuration. */
  openPath: async (target: string): Promise<OpenPathResult> => {
    try {
      await call('open', {path: target});
      return {success: true};
    } catch (error) {
      return {success: false, error: error instanceof Error ? error.message : String(error)};
    }
  },
  showOpenDirectoryDialog: async (): Promise<{canceled: boolean; path?: string}> => {
    const path = await pickDirectory();
    return path ? {canceled: false, path} : {canceled: true};
  },
  dialog: {
    showOpenDirectory: async (_options?: unknown): Promise<{success: boolean; path: string|null}> => {const path = await pickDirectory(); return {success:!!path,path:path||null};},
    showOpenFile: async (_options: {defaultPath?: string; filters?: Array<{name: string; extensions: string[]}>}): Promise<{canceled: boolean; path?: string}> => {
      const path = await pickFile();
      return path ? {canceled: false, path} : {canceled: true};
    },
  },
  /** WebView2 File objects carry no local path; the Browse button supplies one. */
  getFilePath: (_file: File): string => {
    throw new Error('dropped-file-path-unavailable');
  },
  // Conversation deletion removes the whole session directory in the Host.
  cleanupSessionImages: async (_sessionId: string): Promise<void> => {},
  // WinOne has one local data owner and no account sign-in: the auth state
  // never changes, so a subscription never fires. Per-turn cost pushes are not emitted.
  onAuthStateChange: (_listener: (state: AuthStateChangePayload) => void): Unsubscribe => () => {},
  onUsageMessageTurnCost: undefined as undefined | ((listener: (payload: unknown, ownerStamp: OwnerStamp) => void) => Unsubscribe),
  // Other-device control was removed with Cindy cloud. Upstream only calls
  // this for a remote device id, which WinOne never produces; the rejection
  // takes Cindy's existing unavailable/fallback path, and no push ever arrives.
  deviceLink: {
    invoke: async (_deviceId: string, _channel: string, _args: unknown[]): Promise<unknown> => {
      throw new Error('remote-devices-unavailable');
    },
    onRemotePush: (_cb: (payload: {deviceId: string; channel: string; payload: unknown; ownerStamp?: unknown}, localOwnerStamp?: unknown) => void): Unsubscribe => () => {},
    onStatusChanged: (_cb: (payload: {status: 'stopped' | 'connecting' | 'online'}) => void): Unsubscribe => () => {},
  },
};

export const electronAPI = {
  ...automationsAPI,
  ...oneBotPreload,
  ...oneBotGroupFilePreload,
  maker: {...automationsAPI.maker, ...oneBotPreload.maker},
  localDb: {...automationsAPI.localDb, ...oneBotPreload.localDb},
};
