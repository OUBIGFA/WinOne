// @vitest-environment jsdom
// Cindy's automation page (b7e46bc) inside WinOne: the vendored page renders
// Host data, its controls reach the Host methods, and Cindy navigation maps to
// WinOne conversations.
import {cleanup, render, screen, waitFor, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach, beforeEach, expect, test, vi} from 'vitest';

const native = vi.hoisted(() => ({call: vi.fn(), pickDirectory: vi.fn(), pickFile: vi.fn()}));
vi.mock('../../../bridge', () => ({chatCall: native.call, pickDirectory: native.pickDirectory, pickFile: native.pickFile, setFileDropTarget: vi.fn(async () => {})}));

import {setApplicationLanguage} from '../../../i18n';
import {SchedulerHost} from './SchedulerHost';
import {publishCindyHost} from './store';

const schedule = {id: 'daily-check', name: 'Daily check', prompt: 'Inspect the project', kind: 'cron', cronExpr: '0 9 * * *', timezone: 'Asia/Shanghai', recurring: true, manual: false, agentKind: 'pi', model: 'fixture', providerId: 'service', workspaceKind: 'dialogue', useWorktree: false, notify: {desktop: true, feishu: false}, status: 'active', createdAt: 1, updatedAt: 1, nextFireAt: Date.UTC(2030, 0, 1, 1)};
const run = {id: 'run-1', scheduleId: 'daily-check', sessionId: 'conversation-1', firedAt: Date.UTC(2026, 9, 24, 1), finishedAt: Date.UTC(2026, 9, 24, 1, 2), status: 'success', resultText: 'All clear'};
const providers = [{id: 'service', name: 'Service', authMode: 'api-key' as const, models: ['fixture'], hasKey: true}];

beforeEach(async () => {
  await setApplicationLanguage('zh-CN');
  native.call.mockImplementation(async (method: string, params: Record<string, unknown> = {}) => {
    switch (method) {
      case 'automations.list': return [schedule];
      case 'automations.runtime': return {schedulerInstanceId: 'fixture', inFlight: 0, slotsInUse: 0, maxConcurrentRuns: 2, inFlightRuns: [], waitingSchedules: []};
      case 'automations.events': return params.cursor === Number.MAX_SAFE_INTEGER ? {cursor: 7, events: []} : new Promise(() => {});
      case 'automations.runs': return [run];
      case 'automations.sidebarIndexRuns': return {runs: [], inflightRunIds: [], inflightPolicies: []};
      case 'automations.templates': return {templates: [], categories: []};
      case 'automations.run': return {runId: 'run-2'};
      case 'automations.channels': return {feishu: {status: 'disconnected'}, wecomGroup: {configured: false, enabled: false}};
      case 'catalog': return {projects: [], conversations: [{id: 'conversation-1', title: 'Daily check run', directory: '', sessionDirectory: '', model: {provider: 'service', model: 'fixture'}, permission: 'ask', createdAt: '2026-10-24T01:00:00Z', updatedAt: '2026-10-24T01:02:00Z'}]};
      default: return {};
    }
  });
  publishCindyHost({catalog: {projects: [], conversations: []}});
});
afterEach(() => { cleanup(); native.call.mockReset(); });

test('renders Cindy master-detail with Host automations and runs the selected task', async () => {
  const user = userEvent.setup();
  render(<SchedulerHost providers={providers} defaultModel={{provider: 'service', model: 'fixture'}} onOpenConversation={() => {}} onOpenAIServices={() => {}} onOpenIMSettings={() => {}}/>);
  expect(await screen.findAllByText('Daily check')).not.toHaveLength(0);
  // Cindy's run card shows the run status; the result opens in its conversation.
  await waitFor(() => expect(native.call).toHaveBeenCalledWith('automations.runs', {id: 'daily-check', limit: 50}));
  expect(await screen.findAllByLabelText(/状态|Status/)).not.toHaveLength(0);
  const page = document.querySelector('.cindy-chat-ui') as HTMLElement;
  const runButtons = within(page).getAllByRole('button', {name: /立即运行|运行/});
  await user.click(runButtons[0]!);
  await waitFor(() => expect(native.call).toHaveBeenCalledWith('automations.run', {id: 'daily-check'}));
  expect(native.call).toHaveBeenCalledWith('automations.events', {cursor: Number.MAX_SAFE_INTEGER});
});
