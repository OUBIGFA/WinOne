
import { isSyntheticTriggerText } from './search-shared/syntheticTrigger';

import {
  stripInstructionBlocks,
  type WorkbenchTranscript,
  type WorkbenchTranscriptItem,
} from './shared/botWorkbench.js';


export const WORKBENCH_TRANSCRIPT_MAX_CHARS = 4_000;

const PER_MESSAGE_MAX_CHARS = 1_200;


function cleanText(raw: string): string {
  return stripInstructionBlocks(raw)
    .replace(/[ \t]+\n/g, '\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim();
}


/** 保留最近的消息,单条与总量都有界;顺序仍按时间正序。 */
export function boundTranscript(
  items: readonly WorkbenchTranscriptItem[],
  maxChars = WORKBENCH_TRANSCRIPT_MAX_CHARS,
): WorkbenchTranscript {
  const out: WorkbenchTranscriptItem[] = [];
  let used = 0;
  let truncated = false;
  for (let index = items.length - 1; index >= 0; index -= 1) {
    const cleaned = cleanText(items[index].text);
    if (!cleaned) continue;
    let text = cleaned.length > PER_MESSAGE_MAX_CHARS ? `${cleaned.slice(0, PER_MESSAGE_MAX_CHARS - 1)}…` : cleaned;
    if (used + text.length > maxChars) {
      const room = maxChars - used;
      truncated = true;
      if (room < 80) break;
      text = `…${text.slice(text.length - room + 1)}`;
    }
    out.push({ ...items[index], text });
    used += text.length;
    if (used >= maxChars) {
      truncated = truncated || index > 0;
      break;
    }
  }
  return { items: out.reverse(), truncated };
}


/** 最后一条用户消息与最后一条助手文字(各取清洗后的全文,由调用方再截断)。 */
export function lastExchange(items: readonly Pick<WorkbenchTranscriptItem, 'role' | 'text'>[]): {
  lastUserAsk: string | null;
  lastAssistantLine: string | null;
} {
  let lastUserAsk: string | null = null;
  let lastAssistantLine: string | null = null;
  for (let index = items.length - 1; index >= 0 && (!lastUserAsk || !lastAssistantLine); index -= 1) {
    const text = cleanText(items[index].text);
    if (!text) continue;
    if (items[index].role === 'user') {
      // Shared with remote history: coordination and recovery inputs carry the
      // same durable marker and must not replace the user's actual request.
      if (!isSyntheticTriggerText(text)) lastUserAsk ??= text;
    } else lastAssistantLine ??= text;
  }
  return { lastUserAsk, lastAssistantLine };
}
export {  };
