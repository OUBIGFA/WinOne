// Type-only seam for Cindy's provider failure policy (b7e46bc, Apache-2.0).
// Upstream declares these beside provider-group transport and interrupted-turn
// auto-resume code that WinOne does not run; the shapes are copied verbatim.
import type {AgentInputToolLoopDetails} from '../cindy/tasks/shared/agentInputQueue.ts';

/** apps/desktop/src/shared/providerGroup.ts */
export interface ProviderGroupUsageLimit {
  /** Earliest reset of an otherwise eligible member; null when no reset is known. */
  resetAtMs: number | null;
}

/** apps/desktop/src/main/maker-ipc/interruptedTurnAutoResume.ts */
export interface InterruptedTurnErrorSignals {
  /** terminal error 的展示文案（可能已被 redact）。 */
  message?: string;
  /** SDK 侧的错误 tag（claude-code 的 `SDKAssistantMessageError`，如 `server_error`）。 */
  sdkError?: string;
  /** translator 给出的稳定 reason key（如 `empty-response` / `turn-failed`）。 */
  reason?: string;
  /** 从错误里抽出的 HTTP 状态码。 */
  errorStatus?: number;
  /** Bounded details for the live error projection; never contains raw provider data. */
  toolLoop?: AgentInputToolLoopDetails;
  /** translator 判定的账号用量 / 限流错误。 */
  usageLimit?: boolean;
  /** 上游给出的限额重置时刻(unix ms)。 */
  usageResetAt?: number;
  /** Main-only: verified group exhaustion before dispatch; never borrow the old route's quota. */
  providerGroupUsageLimit?: ProviderGroupUsageLimit;
  /** Codex app-server 的结构化错误 tag(如 `usageLimitExceeded`)。 */
  codexErrorInfo?: string;
}
