// WinOne seam for Cindy provider hooks (b7e46bc): hooks/useProviders,
// useDeviceProviders, useModelRouteDevices and useAvailableAgents. Agents run
// on this computer only; other devices and shared providers were removed with
// Cindy cloud, so the device variants always describe the local catalog.
import {useMemo} from 'react';
import {providerViews, type AgentKind, type ProviderView} from './modelProviders';
import {useCindyHost} from './store';

const EMPTY: ProviderView[] = [];
export function useProviders(): {providers: ProviderView[]; providerOrder: string[]; ownerGeneration: null; loading: boolean} {
  const {providers} = useCindyHost();
  const views = providers ? providerViews(providers) : EMPTY;
  const providerOrder = useMemo(() => views.map((view) => view.id), [views]);
  return {providers: views, providerOrder, ownerGeneration: null, loading: providers === null};
}
export function useDeviceProviders(_deviceId?: string) {
  return useProviders();
}
export interface RouteDevice {deviceId: string; name?: string}
export function useModelRouteDevices(_options: {controlledDeviceId?: string | null; leadAgentDeviceId?: string | null}): {devices: RouteDevice[] | undefined; loading: boolean} {
  return {devices: undefined, loading: false};
}
export function useModelPickerAgents(agentKind: AgentKind, _agentDeviceId?: string | null): AgentKind[] {
  return useMemo(() => [agentKind === 'pi' ? 'pi' : 'pi'], [agentKind]);
}

// hooks/useAvailableAgents: Pi is the only harness WinOne runs, always available.
const PI_ONLY: ReadonlySet<'cc' | 'codex' | 'pi'> = new Set(['pi']);
export function useAvailableAgents(_deviceId?: string | null): {availableVendors: ReadonlySet<'cc' | 'codex' | 'pi'>; loaded: boolean} {
  return {availableVendors: PI_ONLY, loaded: true};
}
export function getCachedAvailableVendors(): ReadonlySet<'cc' | 'codex' | 'pi'> {
  return PI_ONLY;
}
