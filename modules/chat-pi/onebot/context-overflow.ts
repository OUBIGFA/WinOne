// WinOne seam for apps/desktop/src/main/maker-ipc/contextOverflowRollover.ts
// (Cindy b7e46bc, Apache-2.0). The rollover executor rebuilds Cindy sessions
// and is not part of WinOne; the provider failure policy only needs its two
// error classifiers, copied verbatim over the original maker-core helpers.
import {CONTEXT_OVERFLOW_REASON, isContextOverflowErrorMessage} from '../cindy/bots/maker-core/context-overflow-error.ts';
import {isRemoteCompactEncryptedContentError} from '../cindy/bots/maker-core/remote-compact-encrypted-error.ts';

/** packages/maker-core/src/agents/codex/rollout-sanitize.ts */
const CODEX_HISTORY_OVERSIZED_REASON = 'codex_history_oversized';

export function isContextOverflowErrorData(data: unknown): boolean {
  if (!data || typeof data !== 'object') return false;
  const rec = data as { reason?: unknown; message?: unknown; sdkError?: unknown };
  if (rec.reason === CONTEXT_OVERFLOW_REASON) return true;
  return [rec.message, rec.sdkError].some(
    (value) =>
      typeof value === 'string' &&
      (isContextOverflowErrorMessage(value) || isRemoteCompactEncryptedContentError(value)),
  );
}

export function isOversizedHistoryErrorData(data: unknown): boolean {
  if (!data || typeof data !== 'object') return false;
  return (data as { reason?: unknown }).reason === CODEX_HISTORY_OVERSIZED_REASON;
}
