// WinOne seam for Cindy lib/errorDiagnosisPrompt.ts (2d8d3c3). The prompt is
// built as upstream builds it. Prefilling writes WinOne's new-conversation
// draft instead of Cindy's New Maker draft store: the text waits here until
// the following navigate('/cc-agent/new') opens a new conversation with it
// (router.tsx). Chat appends it to an unsent draft instead of replacing it,
// as upstream does, and nothing is sent automatically.
import type {TFunction} from 'i18next';
import {redactSensitiveText} from '../../../../../../../third_party/cindy/maker-shared/errorRedaction';

export interface ErrorDiagnosisSource {
  message: string;
  details: string;
  reason?: string | null;
}

export function buildErrorDiagnosisPrompt(t: TFunction, sessionUrl: string | null, source: ErrorDiagnosisSource): string {
  const details = redactSensitiveText(source.details).trim();
  return t(sessionUrl ? 'chat.errorBanner.diagnosePrompt' : 'chat.errorBanner.diagnosePromptWithoutSession', {
    sessionUrl,
    message: redactSensitiveText(source.message).trim(),
    details: source.reason ? `${details}\nreason: ${redactSensitiveText(source.reason).trim()}` : details,
  });
}

let pending = '';
export function prefillErrorDiagnosisDraft(prompt: string): void {
  pending = pending ? pending + '\n\n' + prompt : prompt;
}
/** Hands the waiting prefill to the new conversation, once. */
export function takeNewChatPrefill(): string {
  const text = pending;
  pending = '';
  return text;
}
