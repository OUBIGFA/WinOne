import { z } from 'zod';


/**
 * 伙伴工作台(2026-10-17 放弃「项目」):工作台是「一眼」——等你 / 在做两块加一行页脚,
 * 数据来自宿主的程序级观察层(不碰 AI)。伙伴跟进的范围不再按目录授予:
 *  - 总调度伙伴(主人设置里「自动跟进所有任务」的那个)可读本机全部任务与在线电脑上的任务;别的伙伴开的
 *    任务默认只看不跟,除非宿主已把跟进交给它(TASK_FOLLOWED_BY_OTHER_BOT);
 *  - 其它伙伴只碰自己开的(后台任务)或明确接手(set_workbench_task_responsibility)的任务。
 * 宿主从 callerSessionId 反查伙伴(只认本机、在用的伙伴主任务),工具层不接受 botId,
 * 越权一律由宿主确定性拒绝。
 */

export type GlanceKindWire = 'waiting' | 'working';


/** 宿主「一眼」投影的一行(与 Desktop shared/botAutonomy.ts 的 GlanceRow 同形)。 */
export interface GlanceRowWire {
  sessionId: string;
  deviceId: string | null;
  deviceName: string | null;
  online: boolean;
  title: string;
  kind: GlanceKindWire;
  /** 状态表 phase(running / usage-limit / waiting-* / errored);旧宿主可能缺省。 */
  phase?: string;
  minutes: number;
  question: string | null;
  waitingFor: 'permission' | 'answer' | 'error' | null;
  followerBotId: string | null;
}


/** 宿主「一眼」投影(与 Desktop shared/botAutonomy.ts 的 BotGlance 同形)。 */
export interface BotGlanceWire {
  botId: string;
  scope: 'all' | 'own';
  waiting: GlanceRowWire[];
  working: GlanceRowWire[];
  lastSweepAt: number;
  doneToday: number;
  offlineDevices: string[];
}


export const WORKBENCH_MESSAGE_MAX_CHARS = 4_000;


const iso = (ms: number) => (Number.isFinite(ms) && ms > 0 ? new Date(ms).toISOString() : null);


/** 给模型看的「一眼」:只留判断要用的字段,task_id 可直接交给 get_task_digest。 */
export function glanceForModel(glance: BotGlanceWire): Record<string, unknown> {
  const row = (item: GlanceRowWire) => ({
    task_id: item.sessionId,
    title: item.title,
    ...(item.phase ? { phase: item.phase } : {}),
    minutes: item.minutes,
    ...(item.deviceName ? { device: item.deviceName } : {}),
    ...(item.online ? {} : { online: false }),
    ...(item.kind === 'waiting' && item.waitingFor ? { waiting_for: item.waitingFor } : {}),
    ...(item.question ? { question: item.question } : {}),
    ...(item.followerBotId && item.followerBotId !== glance.botId ? { followed_by: item.followerBotId } : {}),
  });
  return {
    scope: glance.scope,
    waiting: glance.waiting.map(row),
    working: glance.working.map(row),
    done_today: glance.doneToday,
    last_sweep_at: iso(glance.lastSweepAt),
    ...(glance.offlineDevices.length ? { offline_devices: glance.offlineDevices } : {}),
  };
}


const deadline = z.object({
  kind: z.enum(['date', 'instant']),
  date: z.string().max(10),
  timeZone: z.string().max(100),
  at: z.string().max(80).optional(),
  sourceId: z.string().max(512).optional(),
  sourceVersion: z.number().int().nonnegative().optional(),
  quote: z.string().max(2000).optional(),
  observedAt: z.string().max(80).optional(),
});

const todoPatch = z.object({
  id: z.string().max(128).optional(),
  expectedRevision: z.number().int().nonnegative().optional(),
  key: z.string().min(1).max(512).optional(),
  origin: z.enum(['assigned', 'discovered']).optional(),
  title: z.string().min(1).max(300).optional(),
  progress: z.string().max(4000).optional(),
  outcome: z.string().min(1).max(4000).optional(),
  value: z.string().max(4000).optional(),
  next: z
    .object({
      label: z.string().min(1).max(100),
      instruction: z.string().min(1).max(4000),
      kind: z.enum(['advance', 'view', 'decide']),
    })
    .nullable()
    .optional(),
  sources: z
    .array(
      z.object({
        kind: z.enum(['conversation', 'mail', 'feishu', 'github', 'community', 'task']),
        id: z.string().min(1).max(512),
        label: z.string().min(1).max(300),
        ref: z.string().max(2000).optional(),
        project: z.string().max(4096).optional(),
        version: z.number().int().nonnegative().optional(),
        observedAt: z.string().max(80).optional(),
      }),
    )
    .max(100)
    .optional(),
  associations: z
    .array(
      z.object({
        kind: z.enum(['task', 'pr']),
        id: z.string().max(512),
        label: z.string().max(300),
      }),
    )
    .max(100)
    .optional(),
  sourceDeadline: deadline.nullable().optional(),
  deadlineOverride: z.object({ value: deadline.nullable() }).optional(),
  deadlineCandidate: z
    .object({ value: deadline, reason: z.string().max(2000) })
    .nullable()
    .optional(),
  suggestedDate: z.string().max(10).nullable().optional(),
  resolvedActionRequestId: z.string().min(1).max(128).optional(),
  operation: z
    .enum(['complete', 'reopen', 'delete', 'mute', 'later', 'restore', 'confirm-deadline'])
    .optional(),
  until: z.string().max(80).optional(),
  completion: z
    .object({
      summary: z.string().min(1).max(4000),
      ref: z.string().max(2000).optional(),
    })
    .optional(),
});
export { todoPatch };
