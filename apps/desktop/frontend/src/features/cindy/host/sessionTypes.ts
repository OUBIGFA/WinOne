// WinOne seam for the parts of Cindy lib/ccAgent.types `Session` (b7e46bc)
// that vendored features read. Every WinOne conversation runs on Pi.
import type {SessionSource} from '../shared/sessionSource';

export type SessionStatus = 'active' | 'archived' | 'deleted';
export type MakerVendor = 'cc' | 'codex' | 'pi';
export type WorkspaceKind = 'project' | 'dialogue';
export interface SessionRuntimeProfileProjection {
  agentKind: 'claude-code' | 'codex' | 'pi';
  model: string;
  providerId: string | null;
  effort: string | null;
  fastMode: boolean | 'ultrafast';
}
export interface SessionRuntimePendingProjection {
  generation: number;
  source: 'agent' | 'fallback';
  profile: SessionRuntimeProfileProjection;
}
export interface Session {
  id: string;
  title: string;
  agentKind: 'cc' | 'codex' | 'pi';
  status: SessionStatus;
  workingDir: string | null;
  model?: string;
  providerId?: string;
  effort?: string;
  permissionMode?: 'ask'|'auto'|'bypassPermissions';
  fastMode?: boolean;
  source?: SessionSource;
  pinnedAt: string | null;
  createdAt: string;
  updatedAt: string;
  // Optional Cindy fields read by the OneBot pages. WinOne has no Orca
  // workflows, forks, SSH hosts or other computers, so those stay absent;
  // the rest are projected from the Chat index when the Host knows them.
  workspaceKind?: WorkspaceKind;
  userSendAt?: string | null;
  parentSessionId?: string | null;
  orcaRole?: 'lead' | 'worker' | null;
  remoteHostId?: string | null;
  deviceLinkDeviceId?: string;
  runtimeEffective?: SessionRuntimeProfileProjection;
  runtimePending?: SessionRuntimePendingProjection | null;
}
