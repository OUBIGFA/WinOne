

export const HOST_SEND_FAILURE_CODES = [
  'WORKDIR_MISSING',
  'LAZY_CREATE_FAILED',
  'REHYDRATE_FAILED',
  'SESSION_NOT_FOUND',
  'HOST_NOT_READY',
  'SESSION_RUNNING',
  /**
   * 共享 codex 进程要切凭证形态,但有其它本地 Codex 会话在忙。
   * 与 SESSION_RUNNING 分开:后者是本会话的 µs 级竞态,协调器静默退回队首重试即可;
   * 前者可能要等别的会话跑完几十分钟,静默重试=用户看到"永远排队"(2026-07-03 实报),
   * 必须走可见错误 + Retry。
   */
  'CREDENTIAL_SWITCH_BUSY',
  'SEND_FAILED',
] as const;


export type HostSendFailureCode = (typeof HOST_SEND_FAILURE_CODES)[number];


export type HostSendOutcome = {
  kind: 'host-send';
  accepted: false;
  code: HostSendFailureCode;
  message: string;
  /** CREDENTIAL_SWITCH_BUSY 专用:挡路的本地会话 ids(coordinator 用于事件驱动唤醒 + renderer 展示)。 */
  busySessionIds?: string[];
};


export function createHostSendFailure(
  code: HostSendFailureCode,
  message: string,
  extra?: { busySessionIds?: string[] },
): HostSendOutcome {
  return {
    kind: 'host-send',
    accepted: false,
    code,
    message,
    ...(extra?.busySessionIds ? { busySessionIds: [...extra.busySessionIds] } : {}),
  };
}
export {  };
