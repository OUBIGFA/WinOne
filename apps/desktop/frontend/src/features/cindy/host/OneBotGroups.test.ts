import {beforeEach, describe, expect, it, vi} from 'vitest';
const native = vi.hoisted(() => ({call: vi.fn()}));
vi.mock('../../../bridge', () => ({chatCall: native.call}));
import {makerChatStore} from './onebotTaskActions';

describe('hidden group lane interactions', () => {
  beforeEach(() => { native.call.mockReset(); });
  it('reads the actual hidden lane question and sends ordered answers without changing its draft', async () => {
    native.call.mockImplementation(async (method: string) => method.endsWith('.poll')
      ? {cursor: 4, approvals: [{id: 'question-1', kind: 'question', tool: 'ask', args: {}, questions: [{question: 'Which file?', options: ['report.md', 'notes.md']}]}], events: [], running: true}
      : method.endsWith('.state') ? {messages: [{id: 'reply-1', role: 'assistant', content: 'Working on the report'}], streaming: true} : {ok: true});
    await makerChatStore.ensureInitialMessages('GROUP_QUESTION');
    expect(makerChatStore.getLightSnapshot('GROUP_QUESTION').pendingAskUser?.questions[0].options?.[0].label).toBe('report.md');
    expect(makerChatStore.getSnapshot('GROUP_QUESTION').streamingText).toBe('Working on the report');
    await makerChatStore.answerUserQuestion('GROUP_QUESTION', 'question-1', {'Which file?': 'notes.md'});
    expect(native.call).toHaveBeenCalledWith('onebot.chat.question', {id: 'GROUP_QUESTION', approvalId: 'question-1', answers: ['notes.md']});
    expect(native.call.mock.calls.some(([method]) => String(method).includes('draft'))).toBe(false);
  });
  it('keeps a failed permission submission pending and preserves the question viewer draft', async () => {
    native.call.mockImplementation(async (method: string) => {
      if (method.endsWith('.permission')) throw Error('connection interrupted');
      return method.endsWith('.poll') ? {cursor: 2, approvals: [{id: 'permission-1', tool: 'write', args: {path: 'report.md'}}], events: [], running: true} : {messages: []};
    });
    await makerChatStore.ensureInitialMessages('GROUP_PERMISSION');
    makerChatStore.setAskUserDraft('GROUP_PERMISSION', {requestId: 'older-question', currentIndex: 0, answers: {}, customInput: 'unsent'});
    await expect(makerChatStore.respondToPermission('GROUP_PERMISSION', {behavior: 'allow'})).rejects.toThrow('connection interrupted');
    expect(makerChatStore.getLightSnapshot('GROUP_PERMISSION').pendingPermission?.requestId).toBe('permission-1');
    expect(makerChatStore.getLightSnapshot('GROUP_PERMISSION').askUserDraft?.customInput).toBe('unsent');
  });
});
