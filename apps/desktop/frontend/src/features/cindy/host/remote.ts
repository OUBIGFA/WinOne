// WinOne seam for Cindy device-link stores (b7e46bc). Other computers and
// shared providers were removed with Cindy cloud: no session is ever remote.
import type {ScheduleBinding} from '../scheduler/lib/scheduleBindingIndex';
import type {Session} from './sessionTypes';

const NONE: readonly ScheduleBinding[] = [];
export const remoteProjectsStore = {
  getSessionScheduleBindings(_deviceId: string, _sessionId: string): readonly ScheduleBinding[] {
    return NONE;
  },
  /** No session is held for another computer. */
  getSessionDeviceId(_sessionId: string): string | null {
    return null;
  },
  getDeviceSessions(_deviceId: string): readonly Session[] {
    return NO_SESSIONS;
  },
  pinSessionOrigin(_sessionId: string, _deviceId: string | null): void {},
};
const NO_SESSIONS: readonly Session[] = [];
export function useRemoteSessionScheduleBindings(_deviceId: string | null | undefined, _sessionId: string): readonly ScheduleBinding[] {
  return NONE;
}
export async function refreshRemoteDeviceSessions(_deviceId: string, _sessionIds?: readonly string[], _options?: {scope?: string}): Promise<void> {}
