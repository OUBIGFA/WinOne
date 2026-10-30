// WinOne seam for Cindy components/new-chat/ModelSelector and
// useModelDiscoveryPending (b7e46bc). Cindy's automation chip reuses the chat
// model picker as its single source; in WinOne that picker is the Chat
// composer's grouped provider/model list, so this renders the same list,
// icons and badges from WinOne AI services, plus Pi thinking and Fast.
import '../../ai-model-menu.css';
import {useMemo, useRef, useState} from 'react';
import {Check, ChevronDown, ChevronRight} from 'lucide-react';
import {useTranslation} from 'react-i18next';
import {AIModelBadge} from '../../ai-model-badge';
import {AIModelIcon} from '../../ai-model-icon';
import {modelLabel} from '../../ai-model-label';
import {AIProviderIcon} from '../../ai-provider-icon';
import {ScrollArea} from '../../lobe/scroll-area/ScrollArea';
import {effectiveSourceIdForModel, getModel} from './modelProviders';
import {useProviders} from './providers';

import type {RouteDevice} from './providers';
export interface RemoteAgentSelectorOptions {devices: RouteDevice[]; selectedDeviceId: string | null; deviceModelMemory: unknown}
type Engine = 'cc' | 'codex' | 'pi';
type Selection = {engine: Engine; modelId: string; providerId: string; effort?: string; fast: boolean | 'ultrafast'; agentDevice?: {deviceId: string} | null};

export function useModelDiscoveryPending() {
  // Model lists come from AI services; opening a picker starts no discovery.
  return useMemo(() => ({pending: false, begin: (_start: () => unknown) => {}, reset: () => {}}), []);
}

export function ModelIconMark({providerId}: {icon?: string; providerId: string; name?: string; routing?: unknown; logoKind?: unknown; colorClass?: string; withMargin?: boolean}) {
  const {providers} = useProviders();
  const provider = providers.find((view) => view.id === providerId)?.winone;
  return <AIProviderIcon icon={provider?.avatar || provider?.icon}/>;
}

export function ModelSelectorContent(props: {
  fastModeConfigurable?: readonly string[];
  unifiedAgents?: readonly string[];
  onUnifiedSelect?: (selection: Selection) => boolean | void;
  remoteAgent?: RemoteAgentSelectorOptions;
  modelMemory?: unknown;
  vendorKey: string;
  modelId: string;
  effort: string;
  onModelChange: (id: string) => void;
  onEffortChange: (effort: string) => void;
  fastMode?: boolean | 'ultrafast';
  onFastModeChange?: (value: boolean | 'ultrafast') => void;
  onDismiss: () => void;
  currentProviderId: string | null;
  onProviderChange?: (providerId: string | null, reconciledModelId?: string, reconciledEffort?: string, reconciledFast?: boolean | 'ultrafast') => void;
  onNavigateToProviders?: () => void;
  reselectEmitsChange?: boolean;
  selectedRowClickOpensConfiguration?: boolean;
  overlayContentClassName?: string;
  discoveringModels?: boolean;
  followSession?: {active: boolean; label: string; onFollow: () => void};
}) {
  const {t} = useTranslation();
  const {providers} = useProviders();
  const [search, setSearch] = useState('');
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set());
  const selectedRef = useRef<HTMLButtonElement | null>(null);
  const query = search.trim().toLowerCase();
  const following = !!props.followSession?.active;
  const sourceId = effectiveSourceIdForModel(providers, props.currentProviderId, props.modelId, 'pi');
  const source = providers.find((provider) => provider.id === sourceId);
  const efforts = source ? getModel(source, props.modelId, 'pi')?.efforts ?? [] : [];
  // Preserve a saved, now-unavailable value until the user selects a new route.
  const levels = [...new Set(['', ...efforts, ...(props.effort ? [props.effort] : [])])];
  const select = (providerId: string, modelId: string) => {
    const model = getModel(providers.find((provider) => provider.id === providerId)!, modelId, 'pi')!;
    const effort = model.efforts.includes(props.effort) ? props.effort : model.defaultEffort ?? undefined;
    // Cindy callers either accept the complete route or receive it through
    // the provider callback. Do not dismiss without applying the selection.
    if (props.onUnifiedSelect) {
      const accepted = props.onUnifiedSelect({engine: 'pi', modelId, providerId, effort, fast: props.fastMode ?? false, agentDevice: null});
      if (accepted === false) return;
    } else if (props.onProviderChange) {
      props.onProviderChange(providerId, modelId, effort ?? '', props.fastMode ?? false);
    } else {
      props.onModelChange(modelId);
      props.onEffortChange(effort ?? '');
    }
    props.onDismiss();
  };
  const groups = providers.flatMap((view) => view.winone ? [{view, winone: view.winone}] : []).map(({view, winone}) => ({view, winone, models: (view.models.pi ?? []).filter((model) => !query || `${view.name} ${model.id} ${model.name}`.toLowerCase().includes(query))})).filter((group) => group.models.length > 0);
  return <div className="ai-model-menu chat-model-menu">
    <input autoFocus aria-label={t('chat.searchModels')} placeholder={t('chat.searchModels')} value={search} onChange={(event) => setSearch(event.target.value)}/>
    <ScrollArea disableContentFit viewportProps={{className: 'chat-model-viewport'}}>
      <div role="listbox" aria-label={t('chat.model')}>
        {props.followSession && <button type="button" role="option" aria-selected={following} onClick={() => { props.followSession!.onFollow(); props.onDismiss(); }}>
          <span className="chat-model-label">{props.followSession.label}</span><span className="chat-model-check" aria-hidden="true">{following && <Check size={14}/>}</span>
        </button>}
        {groups.map(({view, winone, models}) => {
          const expanded = !!query || !collapsed.has(view.id);
          return <section key={view.id}>
            <h4><button type="button" className="chat-model-group" aria-label={view.name} aria-expanded={expanded} onClick={() => setCollapsed((current) => { const next = new Set(current); if (next.has(view.id)) next.delete(view.id); else next.add(view.id); return next; })}>
              <AIProviderIcon icon={winone.avatar || winone.icon}/><span className="chat-model-label">{view.name}</span><small>{models.length}</small>{expanded ? <ChevronDown size={13}/> : <ChevronRight size={13}/>}
            </button></h4>
            {expanded && models.map((model) => {
              const selected = !following && props.modelId === model.id && sourceId === view.id;
              return <button key={model.id} ref={selected ? selectedRef : undefined} type="button" role="option" aria-selected={selected} onClick={() => select(view.id, model.id)}>
                <AIModelIcon model={model.id} name={winone.modelNames?.[model.id]}/><span className="chat-model-label">{modelLabel(winone, model.id)}</span><AIModelBadge provider={winone} id={model.id}/>
                <span className="chat-model-check" aria-hidden="true">{selected && <Check size={14}/>}</span>
              </button>;
            })}
          </section>;
        })}
      </div>
    </ScrollArea>
    {!groups.length && <p>{providers.length ? t('chat.noMatchingModels', {defaultValue: t('chat.modelUnavailable')}) : t('chat.modelUnavailable')}</p>}
    {!providers.length && props.onNavigateToProviders && <button type="button" className="chat-model-group" onClick={() => { props.onDismiss(); props.onNavigateToProviders!(); }}>{t('ai.serviceTitle')}</button>}
    {!following && <div role="group" aria-label={t('chat.thinkingLevel')} className="mt-2 flex flex-wrap items-center gap-1 border-t border-[var(--border)] pt-2">
      {levels.map((level) => <button key={level || 'auto'} type="button" aria-pressed={props.effort === level} disabled={!!level && !efforts.includes(level)} className={'rounded-full border px-2 py-0.5 text-12 ' + (props.effort === level ? 'border-[var(--text-secondary)] text-[var(--text-primary)]' : 'border-[var(--border)] text-[var(--text-tertiary)]')} onClick={() => props.onEffortChange(level)}>{level || t('chat.thinkingAuto')}</button>)}
      {props.onFastModeChange && props.fastModeConfigurable?.includes('pi') && <label className="ml-auto inline-flex items-center gap-1 text-12 text-[var(--text-secondary)]"><input type="checkbox" checked={!!props.fastMode} onChange={(event) => props.onFastModeChange!(event.target.checked)}/>{t('chat.fastMode')}</label>}
    </div>}
  </div>;
}
