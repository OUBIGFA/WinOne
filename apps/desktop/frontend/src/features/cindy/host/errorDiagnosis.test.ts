import {describe, expect, it, vi} from 'vitest';
import {buildErrorDiagnosisPrompt, prefillErrorDiagnosisDraft} from './errorDiagnosis';
import {useNavigate} from './router';
import {setCindyNavigation} from './store';

// Cindy's error card prefills its New Maker draft, then navigates to the new
// task page. In WinOne that page is a new Chat conversation.
describe('error diagnosis prefill', () => {
  it('hands the prompt to one new conversation', () => {
    const openNewChat = vi.fn();
    const restore = setCindyNavigation({openNewChat});
    try {
      prefillErrorDiagnosisDraft('first');
      prefillErrorDiagnosisDraft('second');
      useNavigate()('/cc-agent/new');
      useNavigate()('/cc-agent/new');
      expect(openNewChat.mock.calls).toEqual([['first\n\nsecond'], ['']]);
    } finally { restore(); }
  });

  it('builds the prompt from redacted error text', () => {
    const t = vi.fn((key: string, values: Record<string, unknown>) => key + JSON.stringify(values));
    const prompt = buildErrorDiagnosisPrompt(t as never, null, {message: 'failed', details: 'Authorization: Bearer sk-abcdefghijklmnopqrstuvwxyz', reason: 'timeout'});
    expect(t.mock.calls[0]![0]).toBe('chat.errorBanner.diagnosePromptWithoutSession');
    expect(prompt).not.toContain('abcdefghijklmnopqrstuvwxyz');
    expect(prompt).toContain('reason: timeout');
  });

  it('keeps other task paths as conversations', () => {
    const openConversation = vi.fn(), openNewChat = vi.fn();
    const restore = setCindyNavigation({openConversation, openNewChat});
    try {
      useNavigate()('/cc-agent/session-1');
      expect(openConversation).toHaveBeenCalledWith('session-1');
      expect(openNewChat).not.toHaveBeenCalled();
    } finally { restore(); }
  });
});
