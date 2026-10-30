// WinOne seam for Cindy renderer data helpers used by the OneBot pages (Cindy
// b7e46bc, Apache-2.0). WinOne has one local data owner, the Pi harness and the
// AI services catalog; these functions answer from those facts instead of
// Cindy's account stores.
import {i18n} from '../../../i18n';
import type {LucideIcon} from 'lucide-react';
import {Plug, Search, Timer} from 'lucide-react';
import type {BotModelRoute} from '../shared/botModelChain';
import {conversationToSession} from './sessionService';
import type {Session} from './sessionTypes';
import {providerViews, type AgentKind, type ProviderView} from './modelProviders';
import {getCindyHostSnapshot, subscribeCindyHost} from './store';
import {subscribeOneBotChanges} from './onebotAPI';
import {readLoadedBotProfiles} from '../bots/botSessionOwners';

// lib/localePreference.ts
export type SupportedLocale = 'zh-CN' | 'en';
export function getEffectiveLocale(): SupportedLocale {
  return i18n.language === 'en-US' ? 'en' : 'zh-CN';
}

// state/modelVisibilityPrefs.ts: WinOne shows every model its AI services enable.
export function isModelEnabled(..._args: unknown[]): boolean { return true; }
export function getModelVisibilityVersion(): number { return 0; }

// lib/providersSnapshotStore.ts / lib/localCatalogSnapshot.ts: the AI services
// catalog published by the page (same source as Chat and automations).
export interface ProvidersSnapshot {dataOwnerId: string | null; ownerGeneration: number; providers: ProviderView[]; providerOrder: string[]}
let snapshotSource: unknown = undefined;
let snapshot: ProvidersSnapshot | null = null;
export function getCachedProvidersSnapshot(): ProvidersSnapshot | null {
  const {providers} = getCindyHostSnapshot();
  if (providers === null) return null;
  // Keep one object per published catalog: botStore compares snapshots by identity.
  if (snapshotSource !== providers) {
    const views = providerViews(providers);
    snapshotSource = providers;
    snapshot = {dataOwnerId: null, ownerGeneration: 0, providers: views, providerOrder: views.map(view => view.id)};
  }
  return snapshot;
}
export async function refreshLocalCatalogSnapshot(): Promise<boolean> {
  return getCachedProvidersSnapshot() !== null;
}

// The teammate default follows Chat's default model and thinking level, the
// same route the OneBot Host gives a teammate that has no chain of its own.
function defaultRoute(): BotModelRoute | null {
  const {defaultModel: model, defaultThinkingLevel} = getCindyHostSnapshot();
  return model?.model ? {harness: 'pi', providerId: model.provider || null, model: model.model, effort: defaultThinkingLevel ?? '', fastMode: false} : null;
}
// lib/modelDefinitions.ts
export function getDefaultModelForVendor(_vendor: 'cc' | 'codex' | 'pi'): {id: string} {
  return {id: defaultRoute()?.model ?? ''};
}
// shared/botDefaultModelChain.ts: never add fallback routes to an uncustomized default.
export function defaultBotModelChain(_args: unknown): BotModelRoute[] {
  const route = defaultRoute();
  return route ? [route] : [];
}

// lib/sessionsStore.ts: optional welcome personalization reads loaded Chat rows only.
export const sessionsStore = {
  getByFilter(filter: 'all' | 'archived' | 'active'): Session[] | null {
    const catalog = getCindyHostSnapshot().catalog;
    if (!catalog) return null;
    return catalog.conversations
      .filter(conversation => filter === 'all' || (filter === 'archived') === !!conversation.archived)
      .map(conversationToSession);
  },
};

// lib/sessionsBus.ts: a profile or capability change can change the catalog of
// tools, MCP servers and Skills that a teammate's next turn can use.
export function onPatch(listener: (sessionId: string, patch: Record<string, unknown>) => void): () => void {
  return subscribeOneBotChanges(event => {
    if (!event.reset && !['profile-changed', 'capabilities-changed'].includes(event.type ?? '')) return;
    for (const sessionId of canonicalSessionsFor(event.reset ? undefined : event.botId)) listener(sessionId, {runtimeGeneration: null});
  });
}
// The loaded profile projection (registered by botStore through botSessionOwners),
// read here without importing the store. A canonical chat is the single link
// whose role is `canonical` (botStore.canonicalBotSession).
function canonicalSessionsFor(botId?: string): string[] {
  return (readLoadedBotProfiles() ?? []).flatMap(bot => {
    if (botId && bot.id !== botId) return [];
    const canonical = bot.sessions.filter(session => session.role === 'canonical');
    return canonical.length === 1 ? [canonical[0]!.id] : [];
  });
}

// hooks/useAgentIslandSettings.ts: WinOne has no floating agent island.
export function isAgentIslandSupported(): boolean { return false; }

// components/sidebar/sidebarNavigationPrefs.ts: icons for the market's tool rows.
export const SIDEBAR_NAVIGATION_ITEM_ICONS: Record<'automations' | 'plugins' | 'search', LucideIcon> = {
  automations: Timer,
  plugins: Plug,
  search: Search,
};

// lib/generatedFiles.ts (type only): files a teammate's task produced.
export interface GeneratedFileRef {
  path: string;
  name: string;
  source: 'tool' | 'command';
  ready?: boolean;
  [key: string]: unknown;
}

// lib/shareConversationImage.ts: attributes marking shareable transcript rows.
export const SHARE_SESSION_ATTR = 'data-share-session-id';
export const SHARE_MESSAGE_ATTR = 'data-share-message-id';
export const SHARE_EXCLUDE_ATTR = 'data-share-exclude';

// shared/ghost.ts (type only): Cindy plugins are not part of WinOne, so the
// market's plugin column shows the package's own description without an install state.
export interface InstalledGhost {manifest: {id: string; name?: string}; namespace?: string | null; [key: string]: unknown}

// Republish provider changes to listeners of the Cindy host snapshot.
export function subscribeProvidersSnapshot(listener: (snapshot: ProvidersSnapshot | null) => void): () => void {
  return subscribeCindyHost(() => listener(getCachedProvidersSnapshot()));
}
export type {AgentKind};
