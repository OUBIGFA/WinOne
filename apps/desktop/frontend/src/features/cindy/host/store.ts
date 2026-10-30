// Live WinOne data for vendored Cindy features. The Chat page owns the
// catalog, the AI service configuration and the new-conversation model; it
// publishes them here so Cindy hooks (useProviders, useCCSessions, ...) and
// plain functions (getDraft, sessionService) read the same values.
import {useSyncExternalStore} from 'react';
import type {AIProvider, AISelection} from '../../ai-configuration';
import type {ChatCatalog} from '../../chat-state';

export interface CindyHostSnapshot {
  catalog: ChatCatalog | null;
  providers: AIProvider[] | null;
  /** The model a new Chat conversation would use. */
  defaultModel: AISelection | null;
  /** Chat's default thinking level; the OneBot Host gives the default teammate route this effort. */
  defaultThinkingLevel?: string;
}

export interface CindyHostNavigation {
  openConversation(id: string): void;
  openAIServices(): void;
  openIMSettings(): void;
}

let snapshot: CindyHostSnapshot = {catalog: null, providers: null, defaultModel: null};
const listeners = new Set<() => void>();
const defaultNavigation: CindyHostNavigation = {
  openConversation: (id) => window.dispatchEvent(new CustomEvent('winone-open-chat', {detail: {id}})),
  openAIServices: () => window.dispatchEvent(new CustomEvent('winone-open-page', {detail: {id: 'ai-services'}})),
  openIMSettings: () => window.dispatchEvent(new CustomEvent('winone-open-page', {detail: {id: 'onebot', section: 'im'}})),
};
let navigation: CindyHostNavigation = defaultNavigation;

export function publishCindyHost(patch: Partial<CindyHostSnapshot>) {
  const next = {...snapshot, ...patch};
  if (next.catalog === snapshot.catalog && next.providers === snapshot.providers && next.defaultModel === snapshot.defaultModel
    && next.defaultThinkingLevel === snapshot.defaultThinkingLevel) return;
  snapshot = next;
  for (const listener of [...listeners]) listener();
}
export function getCindyHostSnapshot(): CindyHostSnapshot {
  return snapshot;
}
export function subscribeCindyHost(listener: () => void): () => void {
  listeners.add(listener);
  return () => { listeners.delete(listener); };
}
export function useCindyHost(): CindyHostSnapshot {
  return useSyncExternalStore(subscribeCindyHost, getCindyHostSnapshot, getCindyHostSnapshot);
}
/** A page that embeds Cindy UI installs its own navigation while mounted. */
export function setCindyNavigation(value: Partial<CindyHostNavigation> | null): () => void {
  const previous = navigation;
  navigation = value ? {...defaultNavigation, ...value} : defaultNavigation;
  return () => { navigation = previous; };
}
export function cindyNavigation(): CindyHostNavigation {
  return navigation;
}
