// Cindy's provider catalog narrowed to WinOne: every plugin model route runs on
// Pi through the models the user enabled in AI services. ProviderView objects
// come from the Host's live catalog, so anything listed is connected and enabled.
export {isCodexGatewayWireModel} from '../../cindy/voice/packages/model-providers/src/classification.ts';
export type AgentKind = 'pi';
export type ProviderView = {
  id: string;
  name?: string;
  connected?: boolean;
  disabled?: boolean;
  models?: Array<{id: string; name?: string; hidden?: boolean; deprecated?: boolean; disabled?: boolean}>;
};

export function connectedProvidersForAgent<T extends ProviderView>(views: readonly T[], _agent: string): T[] {
  return views.filter(view => view.connected !== false && view.disabled !== true);
}

export function isModelVisible(model: {hidden?: boolean; disabled?: boolean}): boolean {
  return model.hidden !== true && model.disabled !== true;
}

export function isModelSelectableForNewRoute(model: {hidden?: boolean; disabled?: boolean; deprecated?: boolean}): boolean {
  return isModelVisible(model) && model.deprecated !== true;
}
