// Share Cindy's real advisory/security lock protocols with the already-pinned
// OneBot import. A per-process promise queue cannot protect a snapshot worker
// or a recovery journal, and it does not return the upstream `held` contract.
export {withCrossProcessLock, withSecurityBoundaryLock} from '../../cindy/bots/crossProcessLock.ts';
