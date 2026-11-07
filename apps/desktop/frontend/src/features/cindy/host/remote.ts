// WinOne seam for Cindy device-link stores (b7e46bc). Other computers and
// shared providers were removed with Cindy cloud: no session is ever remote.
import type {ScheduleBinding} from '../scheduler/lib/scheduleBindingIndex';
import type {Session} from './sessionTypes';

const NONE: readonly ScheduleBinding[] = [];
export const remoteProjectsStore = {
  subscribe: (_listener: () => void): (() => void) => () => {},
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
// Cindy refreshRemoteSessions.ts takes (deviceId, sessionIds | deviceName, options);
// WinOne has no other device to read from.
export async function refreshRemoteDeviceSessions(_deviceId: string, _sessionsOrName?: readonly string[] | string, _options?: Record<string, unknown>): Promise<void> {}

// Cindy useRemoteSessionSync.ts / remoteBotSettingsClient.ts (2d8d3c3): a
// teammate on another computer. WinOne teammates are local, so there is no
// remote session to follow and remote settings take upstream's unavailable path.
export function useRemoteSessionSync(_sessionId: string | undefined, _deviceId: string | null | undefined): void {}
export async function invokeBotSettings<T = unknown>(_deviceId: string, _method: string, ..._args: unknown[]): Promise<T> {
  throw new Error('remote-devices-unavailable');
}

// Cindy lib/deviceName.ts (2d8d3c3). Every WinOne route is on this computer,
// so a device shows the name it was given, else its id.
export function deviceDisplayName(deviceId: string, name?: string | null, _devices: readonly unknown[] = []): string {
  return name?.trim() || deviceId;
}

// Cindy remoteProjectsStore (2d8d3c3): sessions and owners on other devices.
export function useRemoteProjectSessions(): Session[] {
  return NO_SESSIONS as Session[];
}
export function getSessionDeviceId(_sessionId: string): string | undefined {
  return undefined;
}
