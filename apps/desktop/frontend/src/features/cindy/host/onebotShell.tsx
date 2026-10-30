// WinOne seam for the Cindy shell contexts used by the OneBot pages (Cindy
// b7e46bc, Apache-2.0): view history, the content header slot, the shared
// task sidebar registration and task routes. The OneBot page owns one memory
// router; Cindy's `/cc-agent/:id` task route is served inside that router by
// WinOne's Chat view, so the vendored navigation calls stay unchanged.
import {createContext, useContext, useLayoutEffect, useState, type ReactNode, type RefObject} from 'react';
import {botRouteForOwnedSession, readLoadedBotProfiles} from '../bots/botSessionOwners';

export type MainViewKey = 'cc-agent' | 'issues' | 'plugins' | 'bots';
export interface MainViewHistory {
  lastMatchedKey: MainViewKey;
  paths: Partial<Record<MainViewKey, string>>;
  /** Last local teammate, retained when visiting the index or creation page. */
  lastBotId?: string;
  /** The router entry inherited at an owner change must not seed the new history. */
  ignoredLocationKey?: string;
}
export const MainViewHistoryContext = createContext<RefObject<MainViewHistory> | null>(null);

/** One history per OneBot page mount; WinOne has a single local data owner. */
export function MainViewHistoryProvider({children}: {children: ReactNode}) {
  const [history] = useState<RefObject<MainViewHistory>>(() => ({current: {lastMatchedKey: 'bots', paths: {}}}));
  return <MainViewHistoryContext.Provider value={history}>{children}</MainViewHistoryContext.Provider>;
}

// feature-context.tsx: pages register the node shown in the window title strip.
type HeaderSlot = {setHeaderContent: (node: ReactNode) => void};
const ContentHeaderContext = createContext<HeaderSlot | null>(null);

/** Provides the header slot; `render` receives the registered node for the OneBot top bar. */
export function ContentHeaderSlot({children, render}: {children: ReactNode; render: (node: ReactNode) => ReactNode}) {
  const [header, setHeaderContent] = useState<ReactNode>(null);
  const [slot] = useState<HeaderSlot>(() => ({setHeaderContent}));
  return <ContentHeaderContext.Provider value={slot}>{render(header)}{children}</ContentHeaderContext.Provider>;
}

export function useRegisterContentHeader(node: ReactNode): void {
  const slot = useContext(ContentHeaderContext);
  if (!slot) throw new Error('useRegisterContentHeader must be used inside <ContentHeaderSlot>');
  const {setHeaderContent} = slot;
  useLayoutEffect(() => {
    setHeaderContent(node);
    return () => setHeaderContent(null);
  }, [node, setHeaderContent]);
}

// useRegisterCCAgentSidebar.tsx: the OneBot page always shows its own roster.
export function useRegisterCCAgentSidebar(): void {}

// orcaSessionIdentity.ts: WinOne has no Orca workflows, so a task route is the
// companion's chat when a loaded profile owns it, otherwise the plain task.
export async function resolveSessionRoute(sessionId: string, _session?: unknown, options?: {remote?: boolean}): Promise<string> {
  if (!options?.remote) {
    const profiles = readLoadedBotProfiles();
    const route = profiles ? botRouteForOwnedSession(profiles, sessionId)?.route : undefined;
    if (route && !route.startsWith('/cc-agent/')) return route;
  }
  return standardSessionRoute(sessionId);
}

/** Open the exact task with the ordinary UI, without resolving its parent or companion. */
export function standardSessionRoute(sessionId: string, deviceId?: string | null): string {
  return `/cc-agent/${encodeURIComponent(sessionId)}${deviceId ? `?device=${encodeURIComponent(deviceId)}` : ''}`;
}
