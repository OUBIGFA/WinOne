
import type { GhostControlPointResult, GhostRoutineRunPointData } from '../../shared/ghost.js';


function controlPointStopMessage(gate: Extract<GhostControlPointResult, { action: 'skip' | 'block' }>): string {
  if (gate.failure) return `Plugin "${gate.ghostName}" was unavailable or did not answer; handled by this task's failure setting`;
  return gate.reason ? `Stopped by plugin "${gate.ghostName}": ${gate.reason}` : `Stopped by plugin "${gate.ghostName}"`;
}
export { controlPointStopMessage };
