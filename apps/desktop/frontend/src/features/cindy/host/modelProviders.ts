// WinOne seam for the @cindy/model-providers functions used by vendored
// automations (b7e46bc). Providers come from WinOne AI services; each enabled
// model is offered to the Pi harness, the only harness WinOne runs.
import type {AIProvider} from '../../ai-configuration';
import {modelLabel} from '../../ai-model-label';

export type AgentKind = 'claude-code' | 'codex' | 'pi';
// Vendor words are preserved here; the Pi request adapter owns its supported
// thinkingLevelMap. A display catalog must not manufacture another whitelist.
export type Effort = string;

export interface CatalogModel {
  id: string;
  name: string;
  contextWindow?: number;
  /** Provider icon key; WinOne resolves model icons from the model id. */
  icon?: string;
  efforts: readonly Effort[];
  defaultEffort: Effort | null;
  supportsFastMode?: boolean;
  /** Further Cindy catalog metadata (context window, pricing, ...). */
  [key: string]: unknown;
}
export interface ProviderView {
  id: string;
  name: string;
  source: 'builtin' | 'user';
  agents: AgentKind[];
  connected: boolean;
  models: Partial<Record<AgentKind, CatalogModel[]>>;
  routing?: unknown;
  logoKind?: unknown;
  modelDiscoveryFailure?: unknown;
  openAiAccount?: {identity?: string};
  subscriptionAccount?: {identity?: string};
  /** The WinOne provider, for WinOne-owned presentation (icons, badges). */
  winone?: AIProvider;
  /** Further Cindy provider metadata (auth, access, ...). */
  [key: string]: unknown;
}

const views = new WeakMap<AIProvider[], ProviderView[]>();
/** Stable per providers array, so hooks can return it without re-rendering. */
export function providerViews(providers: AIProvider[]): ProviderView[] {
  let value = views.get(providers);
  if (!value) {
    value = providers.filter((provider) => provider.models.length > 0).map((provider) => ({
      id: provider.id,
      name: provider.name,
      source: 'user',
      agents: ['pi'],
      connected: true,
      models: {pi: provider.models.map((id) => {
        const metadata = provider.modelMetadata?.[id];
        const efforts = metadata?.reasoning === false ? [] : metadata?.efforts?.length ? metadata.efforts : metadata?.levels ?? [];
        return {id, name: modelLabel(provider, id), icon: provider.avatar || provider.icon,
          efforts, defaultEffort: metadata?.reasoningEffort || null, contextWindow: metadata?.contextWindow ?? 0, supportsFastMode: true};
      })},
      winone: provider,
    }));
    views.set(providers, value);
  }
  return value;
}

export function getModel(provider: ProviderView, modelId: string, agent: AgentKind): CatalogModel | undefined {
  return (provider.models[agent] ?? []).find((model) => model.id === modelId);
}
/** Cindy semantics: an explicit provider only counts while it offers the model;
 * otherwise the first connected provider that offers it. */
export function effectiveSourceIdForModel(views: readonly ProviderView[], providerId: string | null | undefined, modelId: string, agent: AgentKind): string | null {
  const sources = views.filter((provider) => provider.connected && getModel(provider, modelId, agent));
  if (providerId) return sources.some((provider) => provider.id === providerId) ? providerId : null;
  return sources[0]?.id ?? null;
}
export function sessionModelSupportsFastMode(views: readonly ProviderView[], providerId: string | null | undefined, modelId: string, agent: AgentKind): boolean {
  const sourceId = effectiveSourceIdForModel(views, providerId, modelId, agent);
  const provider = sourceId ? views.find((view) => view.id === sourceId) : undefined;
  return !!provider && !!getModel(provider, modelId, agent)?.supportsFastMode;
}
// Codex gateway wire ids (openai-codex/…) exist only in Cindy's Codex harness.
export function isCodexGatewayWireModel(_model: string | null | undefined): boolean {
  return false;
}
export function stripCodexGatewayWirePrefix(model: string): string {
  return model;
}

// Local connections keep the names chosen in WinOne AI services. There is
// no special Cindy cloud account or remote-device label in this catalog.
export function providerDisplayName(provider: Pick<ProviderView, 'id' | 'name'>, _t?: unknown): string {
  return provider.name || provider.id;
}
export function providerAccountLabel(label: string, identity?: string): string {
  return !identity || label === identity || label.endsWith(' · ' + identity) ? label : `${label} · ${identity}`;
}
