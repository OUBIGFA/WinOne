// WinOne seam for Cindy shared/ghost.ts as automations and routines use it
// (2d8d3c3): the control-point catalog behind "pre-run check: plugin". The
// catalog below is Cindy's, unchanged. WinOne has no Cindy plugin (ghost)
// host, so no plugin can declare a control point: the candidate list the
// pre-run fields read stays empty (electronAPI.ghosts) and a saved plugin is
// shown as unavailable, as upstream shows an uninstalled one.
export const GHOST_CONTROL_POINT_FAILURE_ACTIONS = ['allow', 'skip', 'block'] as const;
export type GhostControlPointFailureAction = (typeof GHOST_CONTROL_POINT_FAILURE_ACTIONS)[number];
export type GhostControlPointAction = 'allow' | 'skip' | 'block' | 'route';

export interface GhostControlPointSpec {
  actions: readonly GhostControlPointAction[];
  failureActions: readonly GhostControlPointFailureAction[];
  defaultOnFailure: GhostControlPointFailureAction;
  timeoutMs: number;
  permissionKey: string;
  labelKey: string;
  detailKey: string;
}

export const GHOST_CONTROL_POINTS = {
  'will-schedule-run': {
    actions: ['allow', 'skip', 'block'],
    failureActions: ['allow', 'skip', 'block'],
    defaultOnFailure: 'block',
    timeoutMs: 30_000,
    permissionKey: 'subscribe:hooks:schedule-run',
    labelKey: 'subscribeScheduleRun',
    detailKey: 'subscribeScheduleRunDetail',
  },
  'will-routine-run': {
    actions: ['allow', 'skip', 'block'],
    failureActions: ['allow', 'skip', 'block'],
    defaultOnFailure: 'block',
    timeoutMs: 30_000,
    permissionKey: 'subscribe:hooks:routine-run',
    labelKey: 'subscribeRoutineRun',
    detailKey: 'subscribeRoutineRunDetail',
  },
  'will-start-task': {
    actions: ['allow', 'route'],
    failureActions: ['allow'],
    defaultOnFailure: 'allow',
    timeoutMs: 8_000,
    permissionKey: 'subscribe:hooks:start-task',
    labelKey: 'subscribeStartTask',
    detailKey: 'subscribeStartTaskDetail',
  },
} as const satisfies Record<string, GhostControlPointSpec>;
export type GhostControlPointId = keyof typeof GHOST_CONTROL_POINTS;

export interface GhostControlPointCandidate {
  pluginId: string;
  name: string;
}
