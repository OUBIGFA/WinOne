// WinOne seam for Cindy hooks/useAgentCapabilities (b7e46bc). Capabilities are
// those of WinOne's Pi harness: every enabled AI service model, Pi thinking
// levels, and WinOne's priority (Fast) request option.
import {useMemo} from 'react';
import {useTranslation} from 'react-i18next';
import {type AgentKind, type Effort} from './modelProviders';
import {useProviders} from './providers';

export interface ModelDescriptor {
  id: string;
  displayName: string;
  contextWindow: number;
  efforts: readonly Effort[];
  defaultEffort: Effort | null;
  supportsFastMode?: boolean;
}
export interface AgentCapabilities {
  availableModels: ModelDescriptor[];
  hasFastMode: boolean;
  effortLevels: Array<{id: Effort; displayName: string}>;
  permissionModes: Array<{id: 'ask' | 'auto' | 'bypassPermissions'; displayName: string; description?: string}>;
}
export function useAgentCapabilities(agentKind: AgentKind | null | undefined, _deviceId?: string): {capabilities: AgentCapabilities | null; loading: boolean; error: string | null} {
  const {providers, loading} = useProviders();
  const {t} = useTranslation();
  const capabilities = useMemo<AgentCapabilities | null>(() => {
    if (!agentKind || loading) return null;
    const seen = new Map<string, ModelDescriptor>();
    const efforts = new Set<string>();
    for (const provider of providers) {
      for (const model of provider.models.pi ?? []) {
        for (const effort of model.efforts) efforts.add(effort);
        if (!seen.has(model.id)) seen.set(model.id, {id: model.id, displayName: model.name, contextWindow: model.contextWindow ?? 0, efforts: model.efforts, defaultEffort: model.defaultEffort, supportsFastMode: model.supportsFastMode});
      }
    }
    return {
      availableModels: [...seen.values()],
      hasFastMode: true,
      effortLevels: [...efforts].map((id) => ({id, displayName: id})),
      permissionModes: (['ask', 'auto', 'bypassPermissions'] as const).map((id) => ({id, displayName: t(id === 'bypassPermissions' ? 'chat.full' : 'chat.' + id)})),
    };
  }, [agentKind, loading, providers, t]);
  return {capabilities, loading, error: null};
}
