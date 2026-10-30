// WinOne seam for Cindy notification hooks used by the automation form
// (b7e46bc): hooks/useFeishuBot and hooks/useWecomGroupNotificationSettings.
// The automation form only reads whether each channel is ready; setup lives in
// OneBot's message platform settings. The Host reports the channel states.
import {useEffect, useState, useSyncExternalStore} from 'react';
import {chatCall} from '../../../bridge';

export interface AutomationChannelState {
  feishu: {status: 'connected' | 'disconnected' | 'error'; errorMessage?: string};
  wecomGroup: {configured: boolean; enabled: boolean; maskedKey?: string};
}
const unknownState: AutomationChannelState = {feishu: {status: 'disconnected'}, wecomGroup: {configured: false, enabled: false}};
let state = unknownState;
let loaded = false;
let pending: Promise<void> | null = null;
const listeners = new Set<() => void>();

export function refreshAutomationChannels(): Promise<void> {
  pending ??= chatCall<AutomationChannelState>('automations.channels')
    .then((value) => { state = value; })
    .catch(() => { state = unknownState; })
    .finally(() => { loaded = true; pending = null; for (const listener of [...listeners]) listener(); });
  return pending;
}
function useChannels() {
  const value = useSyncExternalStore((listener) => { listeners.add(listener); return () => { listeners.delete(listener); }; }, () => state, () => state);
  const [requested, setRequested] = useState(false);
  useEffect(() => { if (!requested) { setRequested(true); void refreshAutomationChannels(); } }, [requested]);
  return value;
}

export function useFeishuBot() {
  const value = useChannels();
  return {status: value.feishu.status, errorMessage: value.feishu.errorMessage ?? null, hasLoadedState: loaded};
}
export function useWecomGroupNotificationSettings() {
  const value = useChannels();
  return {enabled: value.wecomGroup.enabled, configured: value.wecomGroup.configured, maskedKey: value.wecomGroup.maskedKey, busy: false};
}
