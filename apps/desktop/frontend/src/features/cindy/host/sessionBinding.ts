// WinOne seam for Cindy hooks/useSessionBinding.ts (2d8d3c3): whether an IM
// channel has taken over a session ("taken over by [user]"). The WinOne Host
// does not report such bindings to the window, so a session shows as its own,
// the state upstream shows before a binding resolves.
interface SessionBindingState {
  attached: boolean;
  identity: {channel: string; botContextId: string; userId: string} | null;
  displayName: string | null;
}
const INITIAL: SessionBindingState = {attached: false, identity: null, displayName: null};
export function useSessionBinding(_sessionId: string | undefined): SessionBindingState {
  return INITIAL;
}
