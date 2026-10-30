

export function isExpiredPermissionDecision(decision: InteractionDecision): boolean {
  return (
    decision.kind === 'permission' &&
    decision.behavior === 'deny' &&
    /timeout|session_disposed|session_aborted|session_closed|interaction_route_released|turn_terminal/.test(
      decision.reason ?? '',
    )
  );
}
export {  };
