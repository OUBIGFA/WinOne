import type { InteractionDecision } from '../../../onebot/wechat-host.mjs';

import {
  isExpiredPermissionDecision,
  type SharedPermission,
} from '../expiredPermission';

import { hasSessionPermissionUpdates } from '../../../onebot/wechat-host.mjs';


/** Receipt only: the Host may hold the choice until the task resumes. */
export const INTERACTION_CHOICE_RECEIVED_TEXT = '已收到你的选择。';


/** Text-only channels and Hook share the same final permission wording. */
export function permissionOutcomeText(decision: InteractionDecision, toolName?: string): string {
  const status =
    decision.kind === 'permission' && decision.behavior === 'allow'
      ? hasSessionPermissionUpdates(decision)
        ? '✅ 已允许（本任务）'
        : '✅ 已允许（仅本次）'
      : isExpiredPermissionDecision(decision)
        ? '⌛ 本次确认已失效'
        : '❌ 已拒绝';
  return toolName ? `${status}：${toolName}` : status;
}
export {  };
