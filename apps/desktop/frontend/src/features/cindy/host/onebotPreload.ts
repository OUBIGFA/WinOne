// WinOne seam for the OneBot members of Cindy's preload `window.electronAPI`
// (Apache-2.0, Cindy b7e46bc). Signatures follow Cindy's renderer typings
// (renderer/vite-env.d.ts). Every call goes to a Host `onebot.*` method; pushes
// come from the Host OneBot long poll instead of Electron IPC. Cindy cloud,
// other computers, Chat Server rooms and the agent island are not part of
// WinOne: those members are inert, so upstream takes its existing local or
// unavailable branch.
import {chatCall, pickDirectory, pickFile} from '../../../bridge';
import type {BotAutonomySettings, BotAutonomySettingsView, BotGlance} from '../shared/botAutonomy';
import type {BotCreationDraft, BotCreationRequest} from '../shared/botCreation';
import type {BotDmThreadSummary} from '../shared/botDmThread';
import type {BotHistorySearchRequest, BotHistorySearchResponse, BotLifecycleAction, BotLifecycleActionRequest, BotLifecycleActionResult} from '../shared/botLifecycle';
import type {BotMemoryDeleteInput, BotMemoryDetail, BotMemorySummary, BotMemoryUpdateInput} from '../shared/botMemory';
import type {BotModelRoute} from '../shared/botModelChain';
import type {BotSkillSummary} from '../shared/botSkill';
import type {BotWelcomeContext} from '../shared/botWelcomeContext';
import type {BotProjectOption, BotWorkbench, WorkbenchTranscript} from '../shared/botWorkbench';
import type {VoiceConversationApi, VoiceConversationEvent} from '../shared/voiceConversation';
import type {AgentIslandSessionActivity} from '../shared/agentIsland';
import type {BotDirectMessageChangedPayload, BotDirectMessageThreadResult} from '../../../../../../../third_party/cindy/maker-shared/botDirectMessage';
import type {WorkingPhase} from '../../../../../../../third_party/cindy/maker-shared/workingStatus';
import type {Routine, RoutineInput, RoutineSource} from '../../../../../../../third_party/cindy/scheduler/routines';
import type {RoutineRun} from '../../../../../../../third_party/cindy/scheduler/routine-engine';
import type {SupportedLocale} from '../../../../../../../modules/chat-pi/cindy/tasks/shared/locale';
import type {BotDelegationChangedPayload, BotDelegationNoteTraceResult} from '../../../../../../../modules/chat-pi/cindy/tasks/core/botDelegation';
import type {
  BotGroupChangedPayload, BotGroupCreateInput, BotGroupCreateResult, BotGroupGetOptions, BotGroupGetResult, BotGroupListResult,
  BotGroupMutationResult, BotGroupPlanActionInput, BotGroupPlanEditInput, BotGroupSendInput, BotGroupSendResult, BotGroupSetMembersInput,
  BotGroupUpdateInput, ChatServerApi,
} from '../../../../../../../modules/chat-pi/cindy/tasks/core/botGroupChat';
import type {NightlyLearningFrequency, NightlyLearningView} from '../../../../../../../modules/chat-pi/cindy/bots/shared/botNightlyLearning';
import type {TeammateTodo, TodoPatch} from '../../../../../../../modules/chat-pi/cindy/bots/shared/teammate-todo';
import type {
  BotImAdoptablePersonalBotsPayload, BotImAdoptPersonalBotInput, BotImConnectionAddInput, BotImConnectionAddResult, BotImConnectionGroupsPayload,
  BotImConnectionLanesPayload, BotImConnectionListPayload, BotImConnectionRemoveResult, BotImGroupActivationMode,
} from '../../../../../../../modules/chat-pi/cindy/bots/shared/botImConnection';
import type {BotMarketApi} from '../../../../../../../modules/chat-pi/cindy/bots/shared/botMarket';
import type {CompanionImportApi} from '../../../../../../../modules/chat-pi/cindy/bots/shared/companionImport';
import {subscribeOneBotChanges, oneBotAPI, type OneBotChange} from './onebotAPI';
import {cacheManagedMedia} from './onebotMedia';
import {prepareGroupDetail} from './onebotGroupFiles';
import {voiceInputPreload} from './onebotVoiceInput';
import {getCindyHostSnapshot,subscribeCindyHost} from './store';
import {makerChatStore, subscribeOneBotActivity} from './onebotTaskActions';
import {readLoadedBotProfiles} from '../bots/botSessionOwners';
import {markdownPreviewText} from '../shared/markdownPreviewText';
import type {Session} from './sessionTypes';

const call = <T>(method: string, params: Record<string, unknown> = {}) => chatCall<T>('onebot.' + method, params);
type Unsubscribe = () => void;
/** The single WinOne data owner, shared with useAuth and the data-owner seam. */
export const LOCAL_DATA_OWNER_ID = 'winone-local';
/** Cindy shared/workingStatus `WorkingStatusRequest` (that module is otherwise served by maker-shared). */
export interface WorkingStatusRequest {sessionId: string; phase: WorkingPhase; locale: SupportedLocale}

/** Deliver Host invalidations of the given kinds; a lost poll window reaches every listener. */
function onChange(types: readonly string[], listener: (event: OneBotChange) => void): Unsubscribe {
  return subscribeOneBotChanges(event => {
    if (event.reset || types.includes(event.type ?? '')) listener(event);
  });
}
const payloadOf = <T>(event: OneBotChange, fallback: T): T => (event.payload ?? fallback) as T;

// Profile rows name managed avatar bytes; fetch them before the rows render
// so BotAvatar can resolve the address synchronously (see onebotMedia.ts).
async function withAvatars<T>(rows: T): Promise<T> {
  const list = Array.isArray(rows) ? rows : [rows];
  await Promise.all(list.map(row => cacheManagedMedia((row as {avatar?: unknown} | null)?.avatar)));
  return rows;
}

// Cindy localDb/mapper finalizePlainPreview for a teammate row: the Host
// returns the visible text of the latest message; the Markdown preview is
// made here, where the parser lives, at Cindy's 140-character limit.
function withPreviews(rows: unknown[]): unknown[] {
  return rows.map(row => {
    if (!row || typeof row !== 'object') return row;
    const {lastMessageText, ...rest} = row as {lastMessageText?: unknown};
    const preview = typeof lastMessageText === 'string' ? markdownPreviewText(lastMessageText) : '';
    return {...rest, lastMessagePreview: preview ? preview.slice(0, 140) : null};
  });
}

// One capability catalog read serves the three catalog calls a settings page
// makes together. The MCP list names the teammate by its canonical session;
// the skill list that follows has no teammate argument and uses the same read.
interface CapabilityEntry {id: string; name: string; description?: string; available: boolean; personal?: boolean}
interface CapabilityCatalog {entries: {skill?: CapabilityEntry[]; toolset?: CapabilityEntry[]; mcp?: CapabilityEntry[]}}
const catalogs = new Map<string, {at: number; value: Promise<CapabilityCatalog>}>();
let lastCatalogBotId: string | null = null;
function capabilityCatalog(botId: string, fresh: boolean): Promise<CapabilityCatalog> {
  const cached = catalogs.get(botId);
  if (cached && (!fresh || Date.now() - cached.at < 2000)) return cached.value;
  const value = call<CapabilityCatalog>('capabilities.get', {botId});
  catalogs.set(botId, {at: Date.now(), value});
  value.catch(() => { if (catalogs.get(botId)?.value === value) catalogs.delete(botId); });
  return value;
}
function botIdForSession(sessionId: string | undefined): string | null {
  if (!sessionId) return null;
  return readLoadedBotProfiles()?.find(bot => bot.sessions.some(session => session.id === sessionId))?.id ?? null;
}

// Binary voice frames cross the JSON bridge as base64.
function toBase64(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = '';
  for (let index = 0; index < bytes.length; index += 0x8000) binary += String.fromCharCode(...bytes.subarray(index, index + 0x8000));
  return btoa(binary);
}
function fromBase64(value: string): ArrayBuffer {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index++) bytes[index] = binary.charCodeAt(index);
  return bytes.buffer;
}
const pcm = (value: {pcm: string; done: boolean}) => ({pcm: fromBase64(value.pcm), done: value.done});

/** Upstream `ChooseAvatarResult`: the picked image as a data URL for the crop editor. */
async function chooseAvatarSource(): Promise<{canceled: boolean; previewDataUrl?: string}> {
  const path = await pickFile();
  if (!path) return {canceled: true};
  const image = await call<{base64: string; mimeType: string}>('avatar.source', {path});
  return {canceled: false, previewDataUrl: `data:${image.mimeType};base64,${image.base64}`};
}

const voiceConversation: VoiceConversationApi = {
  models: () => call('voice.models'),
  voices: selection => call('voice.voices', {selection}),
  preview: input => call('voice.preview', {input}),
  readPreview: async previewId => pcm(await call('voice.readPreview', {previewId})),
  stopPreview: async previewId => { await call('voice.stopPreview', {previewId}); },
  preference: botId => call('voice.preference', {botId}),
  setPreference: (botId, selection, sentencePlayback) => call('voice.setPreference', {botId, selection: selection ?? null, sentencePlayback}),
  start: input => call('voice.start', {input}),
  promoteTurn: (callId, inputId) => call('voice.promoteTurn', {callId, inputId}),
  submitTurn: (callId, inputId, text, forceHandoff) => call('voice.submitTurn', {callId, inputId, text, forceHandoff}),
  startPlayback: input => call('voice.startPlayback', {input}),
  classifyExit: (callId, utteranceId, inputGeneration) => call('voice.classifyExit', {callId, utteranceId, inputGeneration}),
  audio: input => { void call('voice.audio', {callId: input.callId, pcm: toBase64(input.pcm), inputGeneration: input.inputGeneration, trace: input.trace}).catch(() => undefined); },
  setInputMuted: (callId, muted) => call('voice.setInputMuted', {callId, muted}),
  finishUtterance: async callId => { await call('voice.finishUtterance', {callId}); },
  end: async callId => { await call('voice.end', {callId}); },
  speak: input => call('voice.speak', {input}),
  readSpeech: async (callId, requestId) => pcm(await call('voice.readSpeech', {callId, requestId})),
  interrupt: async callId => { await call('voice.interrupt', {callId}); },
  onEvent: callback => onChange(['voice'], event => { if (!event.reset) callback(payloadOf<VoiceConversationEvent>(event, undefined as never)); }),
};

const groupCall = (method: string) => <T>(input: unknown) => call<T>('groups.' + method, {input});
// WinOne groups are local; Chat Server rooms were removed with Cindy cloud.
// The status reads as disabled, so upstream never offers a server room, and
// any other call reports the room as unavailable.
const unavailable = async () => ({ok: false as const, errorCode: 'CHAT_SERVER_UNAVAILABLE'});
const chatServer: ChatServerApi = {
  manage: unavailable, ownedBots: unavailable, refreshProfile: unavailable,
  status: async () => ({enabled: false, connected: false}),
  thread: unavailable, followThread: unavailable, reply: unavailable, shareToMain: unavailable, react: unavailable, reactors: unavailable,
  createInvite: unavailable, revokeInvite: unavailable, previewInvite: unavailable, acceptInvite: unavailable,
};

export const oneBotPreload = {
  localDb: {
    sessionShare: {classifyPath: (input:{path:string}) => call<{kind:'other'|'directory'}>('groups.files.classify',input)},
    bots: {
      getModelChainSettings: () => call<{modelChain: BotModelRoute[]; isCustomized: boolean}>('modelChain.get'),
      resetModelChainSettings: () => call<{modelChain: BotModelRoute[]; isCustomized: boolean}>('modelChain.reset'),
      setModelChainSettings: (body: {modelChain: BotModelRoute[]}) => call<{modelChain: BotModelRoute[]; isCustomized: boolean}>('modelChain.set', body),
      list: async (body?: {lastReadAtByBotId?: Record<string, number>; welcomeContext?: BotWelcomeContext; locale?: SupportedLocale}): Promise<unknown[]> =>
        withAvatars(withPreviews(await call<unknown[]>('bots.list', body ?? {}))),
      get: async (botId: string): Promise<unknown> => withAvatars(await call<unknown>('profiles.get', {botId})),
      generateDraft: (body: BotCreationRequest) => call<BotCreationDraft>('bots.generateDraft', {request: body}),
      generateAvatar: (token: string) => call<{avatarImageBase64: string}>('bots.generateAvatar', {token}),
      chooseAvatarSource,
      chooseAvatar: async (body: {botId: string; avatarImageBase64?: string}): Promise<{canceled: boolean; profile?: unknown}> => {
        let avatarImageBase64 = body.avatarImageBase64;
        if (avatarImageBase64 === undefined) {
          // The native picker supplies the path; the Host reads and checks the bytes.
          const path = await pickFile();
          if (!path) return {canceled: true};
          avatarImageBase64 = (await call<{base64: string}>('avatar.source', {path})).base64;
        }
        const result = await call<{canceled: boolean; profile?: unknown}>('bots.chooseAvatar', {botId: body.botId, avatarImageBase64});
        return {...result, profile: result.profile ? await withAvatars(result.profile) : result.profile};
      },
      searchHistory: (body: BotHistorySearchRequest) => call<BotHistorySearchResponse>('history.search', {...body}),
      create: async (body: unknown): Promise<unknown> => withAvatars(await call<unknown>('profiles.create', {input: body})),
      update: async (body: unknown): Promise<unknown> => withAvatars(await call<unknown>('profiles.update', {input: body})),
      createCanonicalSession: (body: {botId: string; expectedCanonicalSessionId: string | null; expectedProfileVersion: number; recoverMissingOnly?: boolean}) =>
        call<{created: boolean; canonicalSessionId: string; session: Session}>('bots.createCanonicalSession', body),
      history: (botId: string) => call<unknown[]>('bots.history', {botId}),
      dmThread: {
        ensure: (body: {botId: string; rootSessionId: string; rootClientId: string; title?: string}) =>
          call<{ok: true; sessionId: string; created: boolean} | {ok: false; errorCode: 'BOT_UNAVAILABLE' | 'NO_MODEL' | 'ROOT_UNAVAILABLE'; message: string}>('dmThread.ensure', body),
        list: (body: {botId: string; rootSessionId: string}) => call<BotDmThreadSummary[]>('dmThread.list', body),
      },
      todos: {
        list: (botId: string) => call<{items: TeammateTodo[]; version: 1}>('todos.read', {botId}),
        update: (botId: string, patch: TodoPatch) => call<TeammateTodo>('todos.patch', {botId, patch}),
        act: (botId: string, input: {id: string; revision: number; requestId: string; locale?: string}) => call<TeammateTodo | null>('todos.act', {botId, ...input}),
        /** Without `enabled` this only reads whether the daily check is on. */
        dailyCheck: (botId: string, enabled?: boolean) => call<boolean>('todos.dailyCheck', {botId, ...(enabled === undefined ? {} : {enabled})}),
      },
      projects: {
        list: (botId: string): Promise<BotProjectOption[]> => oneBotAPI.localDb.bots.projects.list(botId),
        set: (botId: string, projectId: string, linked: boolean): Promise<BotProjectOption[]> => oneBotAPI.localDb.bots.projects.set(botId, projectId, linked),
      },
      workbench: {
        get: (botId: string) => call<BotWorkbench | null>('workbench.get', {botId}),
        readTask: (botId: string, taskId: string) =>
          call<{ok: true; taskId: string; transcript: WorkbenchTranscript} | {ok: false; errorCode: string; message: string}>('workbench.readTask', {botId, taskId}),
        followScopes: () => call<Array<{botId: string; sessionIds?: string[]; delegatedSessionIds?: string[]}>>('workbench.followScopes'),
      },
      autonomy: {
        glance: (botId: string, _refreshRemote = true) => call<BotGlance>('autonomy.glance', {botId}),
        getSettings: () => call<BotAutonomySettingsView>('autonomy.settings'),
        setSettings: (patch: BotAutonomySettings) => call<BotAutonomySettingsView>('autonomy.setSettings', {patch}),
        resetSettings: () => call<BotAutonomySettingsView>('autonomy.resetSettings'),
      },
      listSkills: (botId: string) => call<BotSkillSummary[]>('skills.list', {botId}),
      memory: {
        list: (botId: string, query?: string) => call<BotMemorySummary[]>('memory.list', {botId, query: query ?? ''}),
        read: (botId: string, filename: string) => call<BotMemoryDetail>('memory.read', {botId, filename}),
        update: (body: BotMemoryUpdateInput) => call<BotMemoryDetail>('memory.update', {...body}),
        delete: async (body: BotMemoryDeleteInput): Promise<void> => { await call('memory.delete', {...body}); },
      },
    },
    messages: {
      // The OneBot Chat store reports new transcript rows by session.
      onCreated: (callback: (payload: {sessionId: string; message?: unknown}, ownerStamp?: unknown) => void): Unsubscribe =>
        onChange(['message-created'], event => { if (!event.reset) callback(payloadOf(event, {sessionId: ''})); }),
    },
  },
  maker: {
    onProvidersChanged:(listener:()=>void)=>{let previous=getCindyHostSnapshot().providers;return subscribeCindyHost(()=>{const current=getCindyHostSnapshot().providers;if(current!==previous){previous=current;listener();}});},
    runBotLifecycleAction: (request: BotLifecycleActionRequest) => call<BotLifecycleActionResult>('lifecycle.action', {...request}),
    onBotProfileChanged: (cb: (payload: {botId: string; change: 'created' | 'updated'}) => void): Unsubscribe =>
      onChange(['profile-changed'], event => cb({botId: event.botId ?? '', change: payloadOf<{change?: 'created' | 'updated'}>(event, {}).change ?? 'updated'})),
    onBotLifecycleChanged: (cb: (payload: {botId: string; action: BotLifecycleAction}, ownerStamp?: unknown) => void): Unsubscribe =>
      onChange(['lifecycle-changed'], event => cb({botId: event.botId ?? '', action: payloadOf<{action?: BotLifecycleAction}>(event, {}).action ?? 'restart'})),
    onBotAutonomyChanged: (cb: (payload: {botIds?: string[]}) => void): Unsubscribe =>
      onChange(['autonomy-changed'], event => cb(event.reset ? {} : payloadOf(event, {}))),
    onBotDelegationChanged: (cb: (payload: BotDelegationChangedPayload, ownerStamp?: unknown) => void): Unsubscribe =>
      onChange(['task-changed'], event => cb(payloadOf(event, {delegationId: '', parentSessionId: null, childSessionId: null, status: 'running'} as BotDelegationChangedPayload))),
    onBotDirectMessageChanged: (cb: (payload: BotDirectMessageChangedPayload, ownerStamp?: unknown) => void): Unsubscribe =>
      onChange(['direct-message-changed'], event => { if (!event.reset) cb(payloadOf(event, {threadId: '', participantBotIds: ['', '']} as BotDirectMessageChangedPayload)); }),
    getBotDirectMessageThread: (threadId: string, viewerBotId: string) => call<BotDirectMessageThreadResult>('directMessage.get', {threadId, viewerBotId}),
    onBotWorkbenchChanged: (cb: (payload: {botId: string; reset?: boolean}) => void): Unsubscribe =>
      onChange(['workbench-changed'], event => cb({botId: event.botId ?? '', ...(event.reset ? {reset: true} : {})})),
    // Chat resource settings (Skills, plugins, MCP servers) changed for every teammate.
    onMcpChanged: (cb: () => void): Unsubscribe => onChange(['capabilities-changed'], event => { if (event.reset || !event.botId) cb(); }),
    listCustomMcpServers: async (context?: {agentKind: string; botSessionId?: string; modelChain?: BotModelRoute[]}) => {
      const botId = botIdForSession(context?.botSessionId);
      if (!botId) throw new Error('Missing companion catalog');
      lastCatalogBotId = botId;
      const {entries} = await capabilityCatalog(botId, true);
      return {agentKind: 'pi' as const, servers: (entries.mcp ?? []).map(entry => ({id: entry.id, name: entry.name, available: entry.available}))};
    },
    listAgentSkills: async (_agentKind: 'claude-code' | 'codex' | 'pi', _params: {workingDir?: string; remoteHostId?: string; forceReload?: boolean; sessionId?: string}) => {
      if (!lastCatalogBotId) return {success: false, error: 'Missing companion catalog'};
      const {entries} = await capabilityCatalog(lastCatalogBotId, false);
      // The teammate's own Skills come from listSkills; this lists the shared ones.
      return {success: true, skills: (entries.skill ?? []).filter(entry => !entry.personal).map((entry): {
        kind: 'agent-skill'; name: string; description?: string; source: 'user' | 'skill'; enabled?: boolean;
        runtimeStatus?: 'discovered' | 'approved' | 'loaded' | 'failed' | 'unknown';
      } => ({kind: 'agent-skill', name: entry.name, description: entry.description, source: 'skill', enabled: entry.available}))};
    },
    plugins: {
      list: async (_workingDir?: string, _includeHidden?: boolean, botContext?: {botId: string; agentKind: string; remoteHostId?: string | null}) => {
        if (!botContext) return [];
        const {entries} = await capabilityCatalog(botContext.botId, false);
        return (entries.toolset ?? []).map(entry => ({id: entry.id, name: entry.name, description: entry.description ?? '', available: entry.available}));
      },
    },
    // Cindy polishes the working caption from its live turn events with an
    // auxiliary model. WinOne reports phases without that event stream, so the
    // page keeps Cindy's own phase wording (its null path).
    polishWorkingStatus: async (_request: WorkingStatusRequest): Promise<{text: string | null}> => ({text: null}),
    // WinOne routes personal IM channels only to a teammate; there is no
    // teammate-less default route. The settings summary treats this as unset.
    imDefaultSettingsGet: async (_channel: string): Promise<{agentKind: 'claude-code' | 'codex' | 'pi'; agents: Record<'claude-code' | 'codex' | 'pi', {model: string}>}> => {
      throw new Error('IM default routes are not part of WinOne');
    },
    // Voice catalogs come from WinOne AI services, which refresh their own models.
    refreshBuiltinProviderModels: async (_providerId: string): Promise<void> => {},
    traceBotDelegationNote: (parentSessionId: string, delegationId: string, text: string): Promise<BotDelegationNoteTraceResult> =>
      oneBotAPI.maker.traceBotDelegationNote(parentSessionId, delegationId, text) as Promise<BotDelegationNoteTraceResult>,
    listBotGroups: () => call<BotGroupListResult>('groups.list'),
    getBotGroup: async (groupId: string, options?: BotGroupGetOptions) => prepareGroupDetail(await call<BotGroupGetResult>('groups.get', {groupId, options: options ?? {}}),groupId),
    createBotGroup: (input: BotGroupCreateInput) => groupCall('create')<BotGroupCreateResult>(input),
    updateBotGroup: (input: BotGroupUpdateInput) => groupCall('update')<BotGroupMutationResult>(input),
    setBotGroupMembers: (input: BotGroupSetMembersInput) => groupCall('setMembers')<BotGroupMutationResult>(input),
    deleteBotGroup: (groupId: string) => call<BotGroupMutationResult>('groups.delete', {groupId}),
    sendBotGroupMessage: (input: BotGroupSendInput) => groupCall('send')<BotGroupSendResult>(input),
    continueBotGroupRound: (groupId: string) => call<BotGroupMutationResult>('groups.continue', {groupId}),
    stopBotGroupRound: (groupId: string, scope?: 'all') => call<BotGroupMutationResult>('groups.stop', {groupId, ...(scope ? {scope} : {})}),
    startBotGroupPlan: (input: BotGroupPlanActionInput) => groupCall('plan.start')<BotGroupMutationResult>(input),
    dismissBotGroupPlan: (input: BotGroupPlanActionInput) => groupCall('plan.dismiss')<BotGroupMutationResult>(input),
    continueBotGroupPlan: (input: BotGroupPlanActionInput) => groupCall('plan.continue')<BotGroupMutationResult>(input),
    retryBotGroupPlan: (input: BotGroupPlanActionInput) => groupCall('plan.retry')<BotGroupMutationResult>(input),
    editBotGroupPlanStep: (input: BotGroupPlanEditInput) => groupCall('plan.edit')<BotGroupMutationResult>(input),
    onBotGroupChanged: (cb: (payload: BotGroupChangedPayload, ownerStamp?: unknown) => void): Unsubscribe =>
      onChange(['group-changed'], event => cb(event.reset ? {groupId: '', change: 'list'} : payloadOf(event, {groupId: '', change: 'updated'} as BotGroupChangedPayload))),
    chatServer,
  },
  routines: {
    list: (botId: string) => call<Routine[]>('routines.list', {botId}),
    save: (botId: string, input: RoutineInput, id?: string) => call<Routine>('routines.save', {botId, input, ...(id ? {id} : {})}),
    remove: async (botId: string, id: string): Promise<void> => { await call('routines.remove', {botId, id}); },
    runNow: async (botId: string, id: string): Promise<void> => { await call('routines.runNow', {botId, id}); },
    history: (botId: string, id: string) => call<RoutineRun[]>('routines.history', {botId, id}),
    sources: () => call<RoutineSource[]>('routines.sources'),
    nightlyLearning: {
      get: (botId: string) => call<NightlyLearningView>('routines.nightly.get', {botId}),
      setEnabled: (botId: string, enabled: boolean) => call<NightlyLearningView>('routines.nightly.setEnabled', {botId, enabled}),
      setFrequency: (botId: string, frequency: NightlyLearningFrequency) => call<NightlyLearningView>('routines.nightly.setFrequency', {botId, frequency}),
      runNow: (botId: string) => call<NightlyLearningView>('routines.nightly.runNow', {botId}),
      revert: (botId: string, runId: string, itemId: string, redo: boolean) => call<NightlyLearningView>('routines.nightly.revert', {botId, runId, itemId, redo}),
      candidate: (botId: string, candidateId: string, action: 'promote' | 'decline') => call<NightlyLearningView>('routines.nightly.candidate', {botId, candidateId, action}),
    },
    onChanged: (listener: () => void): Unsubscribe => onChange(['routines-changed'], () => listener()),
  },
  voiceConversation,
  voiceInput: voiceInputPreload,
  wechatBot: {
    getState: () => call<WechatBotState>('wechat.state'),
    authorize: () => call<{started: true}>('wechat.authorize'),
    cancelAuthorization: () => call<{ok: true}>('wechat.cancelAuthorization'),
    unbind: () => call<{ok: true}>('wechat.unbind'),
    getChannelSettings: () => call<WechatChannelSettingsState>('wechat.settings'),
    setCompanion: (botId: string | null) => call<WechatChannelSettingsState>('wechat.setCompanion', {botId}),
    onChannelSettingsChange: (callback: (state: WechatChannelSettingsState) => void): Unsubscribe =>
      onChange(['wechat-settings'], event => { if (!event.reset) callback(payloadOf(event, undefined as never)); }),
    chooseWorkingDirectory: async (): Promise<{canceled: boolean; state: WechatChannelSettingsState}> => {
      // The native folder picker supplies the path; the Host checks and stores it.
      const path = await pickDirectory();
      if (!path) return {canceled: true, state: await call<WechatChannelSettingsState>('wechat.settings')};
      return {canceled: false, state: await call<WechatChannelSettingsState>('wechat.setWorkingDirectory', {path})};
    },
    resetWorkingDirectory: () => call<WechatChannelSettingsState>('wechat.resetWorkingDirectory'),
    onStateChange: (callback: (state: WechatBotState) => void): Unsubscribe =>
      onChange(['wechat-state'], event => { if (!event.reset) callback(payloadOf(event, undefined as never)); }),
  },
  botImConnection: {
    list: () => call<BotImConnectionListPayload>('imConnection.list'),
    add: (payload: BotImConnectionAddInput) => call<BotImConnectionAddResult>('imConnection.add', {input: payload}),
    remove: (payload: {id: string}) => call<BotImConnectionRemoveResult>('imConnection.remove', payload),
    listGroups: (payload: {id: string}) => call<BotImConnectionGroupsPayload>('imConnection.listGroups', payload),
    setGroupActivation: (payload: {id: string; chatId: string; mode: BotImGroupActivationMode}) => call<BotImConnectionGroupsPayload>('imConnection.setGroupActivation', payload),
    listLanes: (payload: {id: string}) => call<BotImConnectionLanesPayload>('imConnection.listLanes', payload),
    resetLane: (payload: {id: string; sessionId: string}) => call<BotImConnectionLanesPayload>('imConnection.resetLane', payload),
    archiveLane: (payload: {id: string; sessionId: string}) => call<BotImConnectionLanesPayload>('imConnection.archiveLane', payload),
    listAdoptable: () => call<BotImAdoptablePersonalBotsPayload>('imConnection.listAdoptable'),
    adopt: (payload: BotImAdoptPersonalBotInput) => call<BotImConnectionAddResult>('imConnection.adopt', {input: payload}),
    onChanged: (listener: (payload: BotImConnectionListPayload) => void): Unsubscribe =>
      onChange(['im-changed'], event => {
        if (event.reset) void call<BotImConnectionListPayload>('imConnection.list').then(listener, () => undefined);
        else listener(payloadOf(event, {connections: []}));
      }),
  },
  botMarket: {
    list: () => call('market.list'),
    invite: input => call('market.invite', {input}),
    status: requestId => call('market.status', {requestId}),
  } satisfies BotMarketApi,
  companionImport: {
    sources: () => call('import.sources'),
    preview: sourceId => call('import.preview', {sourceId}),
    start: selection => call('import.start', {selection}),
    status: requestId => call('import.status', {requestId}),
  } satisfies CompanionImportApi,
  agentIsland: {
    // WinOne has no floating island. Its per-session activity mirror is fed
    // from the OneBot Chat activity, which the teammate list reads.
    setVisibleSession: async (_sessionId: string | string[] | null): Promise<{ok: true}> => ({ok: true}),
    onSessionActivity: (cb: (list: AgentIslandSessionActivity[]) => void): Unsubscribe => subscribeOneBotActivity(cb),
  },
  ghosts: {
    // Cindy plugins (ghosts) are not part of WinOne: none is installed and
    // there is no focus signal to report.
    noteSessionFocused: (_sessionId: string | null): void => {},
    listSync: (): {ghosts: unknown[]} => ({ghosts: []}),
  },
  // Without a plugin market the teammate market shows each package's own tool descriptions.
  pluginMarket: undefined as undefined | {snapshot: () => Promise<{items: Array<{ghostId: string; namespace: string | null; name?: string; installState?: string}>}>},
  openExternal: (url: string): Promise<{success: boolean}> => oneBotAPI.openExternal(url),
  // Official X hooks were removed with Cindy cloud; BotChannelsPage checks for the function.
  hookControl: undefined as undefined | {setXCompanion: (botId: string | null, expectedBotId: string | null) => Promise<unknown>},
  sidebarSettings: {
    // One local data owner holds the per-owner sidebar preferences in local storage.
    claimLegacyRendererOwner: () => ({dataOwnerId: LOCAL_DATA_OWNER_ID, ownerGeneration: 0, claimed: true, canInitialize: true, pinnedLegacyConsumed: false}),
  },
  readCachedImageAsBase64: (params: {url: string}) => call<{base64: string; mimeType: string}>('media.read', {url: params.url}),
};
export {makerChatStore};
