// WinOne seam for Cindy new-task model memory used by automations (b7e46bc):
// state/newMakerDraft (getDraft, getPersistedVendorModel),
// shared/draftModelCalibration.resolveNewTaskDefaultRoute and
// state/agentDeviceModelMemory. The "new task" default is the model a new
// WinOne Chat conversation would use.
import {effectiveSourceIdForModel, type AgentKind, type ProviderView} from './modelProviders';
import {getCindyHostSnapshot} from './store';

type Vendor = 'cc' | 'codex' | 'pi';
interface VendorPrefs {model?: string; providerId?: string | null}
export function getDraft(): {lastByVendor: Partial<Record<Vendor, VendorPrefs>>; modelChosenByVendor: Partial<Record<Vendor, boolean>>} {
  const model = getCindyHostSnapshot().defaultModel;
  return {
    lastByVendor: model?.model ? {pi: {model: model.model, providerId: model.provider || null}} : {},
    modelChosenByVendor: model?.model ? {pi: true} : {},
  };
}
export function getPersistedVendorModel(vendor: Vendor): string {
  return vendor === 'pi' ? getCindyHostSnapshot().defaultModel?.model ?? '' : '';
}

/** Cindy contract: an explicitly chosen draft model wins while a connected
 * source offers it; otherwise the first available pair; null when none. */
export function resolveNewTaskDefaultRoute(args: {providers: readonly ProviderView[]; agent: AgentKind; draft?: {model?: string; providerId?: string | null; chosenByUser?: boolean} | null; requiredProviderId?: string | null}): {model: string; providerId: string} | null {
  const required = args.requiredProviderId?.trim() || null;
  const scoped = args.providers.filter((provider) => provider.connected && (!required || provider.id === required));
  const draftModel = args.draft?.model?.trim() ?? '';
  if (draftModel && args.draft?.chosenByUser) {
    const providerId = effectiveSourceIdForModel(scoped, required ?? args.draft.providerId ?? null, draftModel, args.agent);
    if (providerId) return {model: draftModel, providerId};
  }
  for (const provider of scoped) {
    const model = provider.models[args.agent]?.[0];
    if (model) return {model: model.id, providerId: provider.id};
  }
  return null;
}

// Per-device model memory serves other computers, removed with Cindy cloud.
export function agentDeviceModelMemoryAccessors(_deviceId: string) {
  return {getEffort: () => undefined, setEffort: () => {}, setChoice: () => {}};
}

/** Cindy contract (shared/draftModelCalibration): the preferred model when a
 * connected source offers it, otherwise the first connected model. WinOne's
 * preferred model is the default of a new Chat conversation. */
export function pickConnectedModelForAgent(providers: readonly ProviderView[], agent: AgentKind, preferredModelId: string, _isModelEnabled?: unknown): {model: string; providerId: string} | null {
  const connected = providers.filter((provider) => provider.connected);
  const preferred = getCindyHostSnapshot().defaultModel;
  const wanted = preferredModelId || preferred?.model || '';
  if (wanted) {
    const providerId = effectiveSourceIdForModel(connected, preferred?.model === wanted ? preferred.provider || null : null, wanted, agent);
    if (providerId) return {model: wanted, providerId};
  }
  for (const provider of connected) {
    const model = provider.models[agent]?.[0];
    if (model) return {model: model.id, providerId: provider.id};
  }
  return null;
}
