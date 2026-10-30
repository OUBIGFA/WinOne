// Re-export the original memory implementation without importing Cindy's
// complete maker-core index (and therefore unrelated harnesses/Electron).
export {MakerMemoryStore} from '../cindy/bots/memory/store';
export {MemoryStorage, buildBotMemoryScopeKey, parseBotMemoryScopeKey} from '../cindy/bots/memory/storage';
export * from '../cindy/bots/memory/types';
// The overload classifier the group chat's failure categories read (maker-core agents/shared).
export {parseOverloadError, UPSTREAM_OVERLOAD_REASON} from '../cindy/bots/maker-core/overload-error';

/**
 * packages/maker-core/src/session.ts: the terminal text of a runtime closed
 * mid-turn. The provider failure policy recognises it; WinOne never emits it,
 * so it stays an exact constant rather than a vendored session runtime.
 */
export const SESSION_CLOSED_MID_TURN_MESSAGE = 'Session closed before the current turn delivered a terminal event';
