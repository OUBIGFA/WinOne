// WinOne seam for Cindy's cloud companions, other computers and Chat Server
// groups (Cindy b7e46bc, Apache-2.0). These features are removed by product
// decision (2026-10-25): OneBot runs only on this computer. Each export keeps
// the upstream name and shape but always reports the "feature absent" state,
// so the vendored pages never render a cloud, remote-device or server-group
// entry and never issue a device-link or Chat Server request.
import type {KeyboardEventHandler, MouseEventHandler, ReactNode} from 'react';
import type {BotProfile} from '../bots/botStore';
import type {WorkingPhase} from '../../../../../../../third_party/cindy/maker-shared/workingStatus';

export interface CompanionCloudActor {
  externalId: string;
  homeDeviceId: string | null;
  homeDevice?: {name?: string} | null;
}
export interface CompanionCloudSnapshot {
  enabled: boolean;
  selfDeviceId: string | null;
  humanActorId: string | null;
  actors: CompanionCloudActor[];
  demotedBotIds: string[];
}
const ABSENT: CompanionCloudSnapshot = Object.freeze({enabled: false, selfDeviceId: null, humanActorId: null, actors: [], demotedBotIds: []}) as CompanionCloudSnapshot;

// companionCloudStore.ts
export function companionCloudApi(): null { return null; }
export function useCompanionCloud(): CompanionCloudSnapshot { return ABSENT; }
export function getCompanionCloudSnapshot(): CompanionCloudSnapshot { return ABSENT; }
export function refreshCompanionCloud(): void {}
export function refreshCompanionCloudSettled(): Promise<void> { return Promise.resolve(); }

// remoteBotRoster.ts: no other computer ever advertises a teammate here.
export interface RemoteBot {
  id: string;
  deviceId: string;
  deviceName: string;
  name: string;
  avatar: string;
  avatarColor: string;
  description: string;
  preview: string;
  activityAt: number;
  sessionId: string | null;
  online: boolean;
  connectionKnown?: boolean;
  generation?: {phase: WorkingPhase; startedAt: number | null};
  lastReplyAt?: number;
  readAt?: number;
}
export function parseRemoteBots(_value: unknown, _deviceId: string, _deviceName: string): RemoteBot[] { return []; }
export function remoteBotKey(bot: Pick<RemoteBot, 'deviceId' | 'id'>): string { return `${bot.deviceId}:${bot.id}`; }
export function isRemoteBotUnread(_bot: RemoteBot): boolean { return false; }
export function useRemoteBots(): RemoteBot[] { return NO_REMOTE_BOTS; }
export function markRemoteBotRead(_bot: RemoteBot): void {}
const NO_REMOTE_BOTS: RemoteBot[] = [];

// companionCloudPresentation.ts: with no cloud companion every list is local as-is.
export function withoutShadowedBots<T extends {id: string}>(bots: readonly T[], _cloud: CompanionCloudSnapshot): T[] { return [...bots]; }
export function withoutCloudCompanions<T extends {id: string}>(bots: readonly T[], _cloud: CompanionCloudSnapshot): T[] { return [...bots]; }
export function cloudCompanionAt(_cloud: CompanionCloudSnapshot, _deviceId: string | undefined, _botId: string | undefined): CompanionCloudActor | null { return null; }
export function isCloudCompanionRemote(_cloud: CompanionCloudSnapshot, _bot: Pick<RemoteBot, 'id' | 'deviceId'>): boolean { return false; }
export function cloudRemoteBots(_cloud: CompanionCloudSnapshot, remoteBots: readonly RemoteBot[]): RemoteBot[] { return [...remoteBots]; }
export function companionRunsHere(_cloud: CompanionCloudSnapshot, _localBotIds: readonly string[], _botId: string): boolean { return true; }

// cindyDeviceRoster.ts: per-computer Cindy merging needs other computers.
export type CindyDeviceBot = BotProfile | RemoteBot;
export interface CindyDeviceOption {
  key: string;
  bot: CindyDeviceBot;
  label: string;
  deviceName: string;
  online: boolean;
  unread: boolean;
  unreadCount?: number;
  route: string;
}
export function isCindyDeviceBot(_bot: {id: string; templateId?: string}): boolean { return false; }
export function cindyDeviceKey(bot: {id: string; deviceId?: string}): string { return JSON.stringify([bot.deviceId ?? null, bot.id]); }
export function cindyDeviceOptions(..._args: unknown[]): CindyDeviceOption[] { return []; }
export function CindyDeviceRow(_props: {
  current: CindyDeviceOption; options: readonly CindyDeviceOption[]; selected: boolean; subtitle: ReactNode; timestamp: string; typing?: boolean;
  onOpen: () => void; onSelect: (option: CindyDeviceOption) => void; onContextMenu?: MouseEventHandler<HTMLButtonElement>;
  onKeyDown?: KeyboardEventHandler<HTMLButtonElement>; children?: ReactNode;
}): null { return null; }

// companionAvatarCache.ts: avatars are always local here, so nothing waits for
// a cached copy (`undefined` would mean "still looking" and hide the initial).
export interface CompanionAvatarSource {avatar: string; deviceId: string | null}
export function useCachedCompanionAvatar(_source: CompanionAvatarSource | null): string | null | undefined { return null; }

// BotHomeComputerSetting.tsx / CindyMergeSetting.tsx: switching a companion's
// main computer and merging Cindys across computers are cloud features.
export interface CompanionHomeBot {id: string; name: string; status?: string}
export interface CompanionHomeTarget {deviceId: string; name?: string; online: boolean; isSelf?: boolean}
export interface CompanionHomeChoices {mode: 'unavailable' | 'local' | 'remote'; targets: CompanionHomeTarget[]}
/** Every companion lives on this computer and cannot move: the upstream shape in its resting state. */
export function useCompanionHomeSwitch(_bot: CompanionHomeBot, _failures: 'inline' | 'toast') {
  return {
    actor: null as CompanionCloudActor | null, home: null as string | null, homeHere: true, homeName: '', movable: false,
    choices: null as CompanionHomeChoices | null, choicesFailed: false, pending: null as CompanionHomeTarget | null,
    setPending: (_target: CompanionHomeTarget | null) => {}, busyTarget: null as string | null, setBusyTarget: (_deviceId: string | null) => {},
    busy: false, error: null as string | null, setError: (_error: string | null) => {}, inFlight: {current: false},
    load: () => {}, confirm: async () => {}, targetName: (target: CompanionHomeTarget) => target.name ?? '',
  };
}
export type HomeSwitch = ReturnType<typeof useCompanionHomeSwitch>;
export function HomeSwitchConfirm(_props: {bot: CompanionHomeBot; state: HomeSwitch}): null { return null; }
export function BotHomeComputerSetting(_props: {bot: CompanionHomeBot; className?: string}): null { return null; }
export function CindyMergeSetting(_props: {bot: {id: string; name: string}}): null { return null; }
export function CompanionShadowBanner(_props: {botId: string; name: string}): null { return null; }
export function useCompanionArrivalNotice(_botId: string | undefined, _name: string | undefined): void {}
export function RemoteTeammateSettings(_props: Record<string, unknown>): null { return null; }
export function RemoteOfficialXCard(_props: Record<string, unknown>): null { return null; }

// device-link: this computer is the only one.
export interface DeviceLinkDevice {deviceId: string; name: string; isSelf?: boolean; online?: boolean}
// The resource protocol is vendored unchanged; upstream reads it only for another computer.
export {
  REMOTE_RESOURCE_CHANGED_CHANNEL, REMOTE_RESOURCE_GET_CHANNEL, REMOTE_RESOURCE_INVOKE_CHANNEL, parseRemoteResourceChangedPayload,
  type RemoteResourceGetRequest,
} from '../shared/deviceLinkRemoteResources';
export function useDeviceLinkDeviceList(): DeviceLinkDevice[] | null { return null; }
export function getStickySessionDeviceId(_sessionId: string): string | null { return null; }
export function useRemoteSessionActivity(_deviceId: string | null | undefined, _sessionId: string | null | undefined): null { return null; }
export function isDeviceLinkRemotePushCurrent(..._args: unknown[]): boolean { return false; }

// remote-device/ControlledBanner.tsx: no other computer controls this one.
export function ControlledBanner(_props: Record<string, unknown>): null { return null; }
const localControllers: readonly unknown[] = Object.freeze([]);
export function useControlledBy(..._args: unknown[]): readonly unknown[] { return localControllers; }
export function useComposerCollapsed(..._args: unknown[]): boolean { return false; }

// Chat Server groups (cross-account rooms, invitations, reactions, threads).
export function ChatInviteDialog(_props: {onClose: () => void; onJoined?: (groupId: string) => void}): null { return null; }
export function ChatInviteButton(_props: {groupId: string}): null { return null; }
export function ChatMessageReactions(_props: Record<string, unknown>): null { return null; }
export function ChatMessageHoverActions(_props: Record<string, unknown>): null { return null; }
export function messageReactionRevision(..._args: unknown[]): string { return ''; }
export function ChatThreadPanel(_props: Record<string, unknown>): null { return null; }
export function animateThreadPanelWidth(_panel: HTMLElement, _from: string, _to: string): Animation | null { return null; }
export function ChatGroupSettings(_props: Record<string, unknown>): null { return null; }
export type BotGroupSharedReplies = ReadonlyMap<string, {messageId: string; bySelf: boolean}>;
// Attachment integrity is a shared pure contract even without device links.
export {isValidAttachmentIntegrity} from '../group-shared/packages/device-link/src/attachmentOssRef';

// components/settings/HookConnectionsSection.tsx: official X hooks were cloud services.
export function HookConnectionsSection(_props: {companion?: {id: string; name: string}}): null { return null; }

// No Cindy plugin is installed in WinOne. Arbitrary drag MIME data does not
// grant a plugin an identity, nor does a URL create a mail-reader connection.
export function getGhostMediaHandoverFromDataTransfer(_data: DataTransfer): null { return null; }
export async function attachGhostMediaToSession(..._args: unknown[]): Promise<never> { throw new Error('Cindy plugin media is unavailable in WinOne'); }
export async function readMailPreview(..._args: unknown[]) { return {supported: false, message: null}; }
export async function openMailPreview(..._args: unknown[]): Promise<never> { throw new Error('No Cindy mail reader is installed'); }
