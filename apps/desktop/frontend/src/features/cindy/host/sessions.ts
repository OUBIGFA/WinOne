// WinOne seam for Cindy session stores used by vendored automations (b7e46bc):
// hooks/useCCSessions, lib/sessionAttentionStore, lib/makerChatStore,
// lib/composerDraftStore and the cc-agent automation sidebar index.
import {useMemo} from 'react';
import type {FailedScheduleRunSnapshot} from '../../../../../../../third_party/cindy/maker-shared/scheduleModel';
import type {Schedule} from '../../../../../../../third_party/cindy/scheduler/types';
import {conversationToSession} from './sessionService';
import type {Session} from './sessionTypes';
import {useCindyHost} from './store';

export type AttentionKind = 'done' | 'awaiting' | 'error';

export function useCCSessions(options?: {includeArchived?: 'active' | 'archived' | 'all'}): {sessions: Session[]; isLoading: boolean; error: Error | null} {
  const {catalog} = useCindyHost();
  const filter = options?.includeArchived ?? 'active';
  const sessions = useMemo(
    () => (catalog?.conversations ?? [])
      .filter((conversation) => filter === 'all' || (filter === 'archived') === !!conversation.archived)
      .map(conversationToSession),
    [catalog, filter],
  );
  return {sessions, isLoading: catalog === null, error: null};
}

// WinOne derives attention from each conversation's live state in the Host;
// opening the conversation acknowledges it. There is no separate store.
export function clearSessionAttentionMany(_sessionIds: Iterable<string>): number {
  return 0;
}

// The Host stops and releases a conversation's Pi process when it is deleted
// or archived, and deletion removes its draft with the conversation folder.
export const makerChatStore = {
  closeSessionQuery(_sessionId: string): void {},
  purgeSession(_sessionId: string): void {},
};
export function discardDraft(_sessionId: string): void {}

export interface AutomationScheduleSessionInfo {
  scheduleId: string;
  scheduleName: string;
  scheduleStatus?: Schedule['status'];
  scheduleSource?: Schedule['source'];
  nextFireAt?: number;
  workingDir?: string;
  projectConfigId?: string;
  unreadRunIds: string[];
  hasUnreadRun: boolean;
  unreadFailedRunIds: string[];
  hasFailedRun?: boolean;
  latestFailedRun?: FailedScheduleRunSnapshot;
  latestUnreadFailedRunId?: string;
  hasUnreadFailedRun: boolean;
}
// Cindy's task sidebar groups automation sessions; WinOne's Chat sidebar keeps
// its own grouping, so the index owner has nothing to maintain here.
export function useAutomationScheduleSessionIndex(_sessionId?: string): void {}
