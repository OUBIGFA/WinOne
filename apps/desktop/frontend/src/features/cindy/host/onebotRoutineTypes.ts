// WinOne seam for the @cindy/maker-scheduler types the teammate routine page
// reads (Cindy b7e46bc, Apache-2.0). The routine engine itself runs in the
// OneBot worker; the page only needs its record shapes.
export type {Routine, RoutineInput, RoutineSource, RoutineTrigger} from '../../../../../../../third_party/cindy/scheduler/routines';
export type {RoutineRun} from '../../../../../../../third_party/cindy/scheduler/routine-engine';
