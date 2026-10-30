// WinOne seam for Cindy project hooks used by automations (b7e46bc):
// hooks/useProjectPickerOptions, cc-agent projectGrouping.normalizeWorkingDir,
// hooks/useWorktreeQueries.useDetectCwd and lib/recentWorkdirsStore.
// Projects are the folders registered in WinOne Chat.
import {useEffect, useMemo, useState} from 'react';
import {chatCall} from '../../../bridge';
import type {FolderPickerOption} from '../components/new-chat/FolderPickerPopover';
import {getCindyHostSnapshot, useCindyHost} from './store';

const posix = (value: string) => value.replace(/\\/g, '/');

export function useProjectPickerOptions(): FolderPickerOption[] {
  const {catalog} = useCindyHost();
  return useMemo(() => (catalog?.projects ?? []).map((project) => ({path: project.path, name: project.name, description: posix(project.path)})), [catalog]);
}
export function getProjectPickerDisplayName(cwd: string | null | undefined, projectOptions: readonly FolderPickerOption[] | undefined, _deviceLinkDeviceId?: string | null): string | null {
  if (!cwd) return null;
  const normalized = posix(cwd);
  const selected = projectOptions?.find((project) => project.path === cwd || posix(project.path).toLowerCase() === normalized.toLowerCase());
  if (selected) return selected.name;
  const parts = normalized.split('/').filter(Boolean);
  return parts[parts.length - 1] || cwd;
}
export function getProjectPickerEmptyLabelKey(mode: 'generic' | 'dialogue'): string {
  return mode === 'generic' ? 'newChat.folderPicker.dialogueOrSelectProject' : 'newChat.folderPicker.dialogue';
}

/** Cindy projectGrouping `BotSessionOwner`: the teammate a sidebar task belongs to. */
export interface BotSessionOwner {botId: string; displayName: string; avatar: string; avatarColor: string}

/** Cindy groups by directory without trailing separators or the long-path prefix. */
export function normalizeWorkingDir(raw: string | null | undefined): string | null {
  if (!raw) return null;
  let value = raw.startsWith('\\\\?\\') ? raw.slice(4) : raw;
  while (value.length > 1 && /[\\/]$/.test(value) && !/^[A-Za-z]:[\\/]$/.test(value)) value = value.slice(0, -1);
  return value || null;
}

export interface RecentWorkdirEntry {path: string; lastUsedAt: string; exists: boolean}
// The scheduler folder picker shows registered projects and never removes
// them; Chat's project menu owns registration.
export const recentWorkdirsStore = {
  /** Registered Chat projects, last used when one of their conversations last changed. */
  get(): RecentWorkdirEntry[] | null {
    const catalog = getCindyHostSnapshot().catalog;
    if (!catalog) return null;
    return catalog.projects.flatMap((project) => {
      const used = catalog.conversations.filter((conversation) => conversation.projectId === project.id).map((conversation) => conversation.updatedAt).sort().pop();
      // The catalog does not probe the disk; a registered project is presented as present.
      return used ? [{path: project.path, lastUsedAt: used, exists: true}] : [];
    });
  },
  async remove(_path: string): Promise<void> {
    throw new Error('Remove the project from WinOne Chat.');
  },
};

export interface DetectCwdResp {isGitRepo: boolean; isInsideWorktree: boolean; gitInstalled: boolean; currentBranch?: string; repoRoot?: string}
export function useDetectCwd(cwd: string | null | undefined, _deviceLinkDeviceId?: string | null, refreshEpoch = 0): {data: DetectCwdResp | null; loading: boolean} {
  const [state, setState] = useState<{key: string | null; data: DetectCwdResp | null}>({key: null, data: null});
  const key = cwd ? `${cwd}\0${refreshEpoch}` : null;
  useEffect(() => {
    if (!cwd) return;
    let alive = true;
    void chatCall<DetectCwdResp>('automations.detectCwd', {workingDir: cwd})
      .then((data) => { if (alive) setState({key, data}); })
      .catch(() => { if (alive) setState({key, data: null}); });
    return () => { alive = false; };
  }, [cwd, key]);
  // Results for another directory never show while the new one loads.
  return key && state.key === key ? {data: state.data, loading: false} : {data: null, loading: !!key};
}
