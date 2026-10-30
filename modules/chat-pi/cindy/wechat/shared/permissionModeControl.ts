

export function permissionModeCommandContext(
  sessionId: string,
  currentMode: PermissionMode,
  modes: PermissionModeDescriptor[],
): PermissionModeCommandContext {
  return {
    sessionId,
    currentMode,
    modes,
  };
}


export function resolvePermissionMode(
  modes: readonly PermissionModeDescriptor[],
  rawMode: string,
): PermissionModeDescriptor | null {
  const normalized = rawMode.trim().toLowerCase();
  const alias =
    normalized === 'bypass' || normalized === 'full' || normalized === 'full-access'
      ? 'bypasspermissions'
      : normalized;
  return modes.find((mode) => mode.id.toLowerCase() === alias) ?? null;
}


export function renderTextPermissionModePicker(
  ui: ImUiTextPack,
  context: PermissionModeCommandContext,
): string {
  const current =
    context.modes.find((mode) => mode.id === context.currentMode) ??
    ({
      id: context.currentMode,
      displayName: context.currentMode,
      description: '',
    } satisfies PermissionModeDescriptor);
  const options = context.modes
    .map((mode) => {
      const description = mode.description ? ` — ${mode.description}` : '';
      return `/permission ${mode.id} — ${mode.displayName}${description}`;
    })
    .join('\n');
  return [
    ui.cards.permissionMode.title,
    ui.cards.permissionMode.currentLine(current.displayName, current.description ?? ''),
    ui.cards.permissionMode.hint,
    options,
  ].join('\n\n');
}


export function renderTextPermissionModeResult(
  ui: ImUiTextPack,
  result: PermissionModeChangeResult,
): string {
  if (result.kind === 'changed') return ui.cards.permissionMode.resolved(result.label);
  if (result.kind === 'confirmation-required') {
    return [
      ui.cards.permissionMode.fullAccessConfirmTitle,
      ui.cards.permissionMode.fullAccessConfirmBody,
      `${ui.cards.permissionMode.btnConfirmFullAccess}: /permission ${result.mode} confirm`,
      `${ui.cards.permissionMode.btnCancelFullAccess}: ${ui.cards.permissionMode.fullAccessCancelled}`,
    ].join('\n\n');
  }
  return ui.cards.permissionMode.failed(result.reason);
}
export {  };
