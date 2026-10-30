// Renderer fixture only: Cindy's automation page with in-memory Host data.
import {createRoot} from 'react-dom/client';
import {setApplicationLanguage} from '../src/i18n';
import {SchedulerHost} from '../src/features/cindy/host/SchedulerHost';
import {publishCindyHost} from '../src/features/cindy/host/store';
import type {AIProvider} from '../src/features/ai-configuration';
import type {ChatCatalog} from '../src/features/chat-state';
import '../src/styles/shadcn.css';
import '../src/style.css';

const query = new URLSearchParams(location.search);
const empty = query.has('empty');
const providers: AIProvider[] = [{id: 'service', name: 'Local AI', authMode: 'api-key', hasKey: true, models: ['first', 'second'], modelNames: {first: 'Example Model', second: 'Another Model'}}];
const day = 24 * 3600 * 1000, now = Date.UTC(2026, 9, 25, 2);
const schedules = empty ? [] : [
  {id: 'daily', name: '每日项目巡检', prompt: '检查项目状态并汇总变化', kind: 'cron', cronExpr: '0 9 * * 1-5', timezone: 'Asia/Shanghai', recurring: true, manual: false, agentKind: 'pi', model: 'first', providerId: 'service', workspaceKind: 'dialogue', useWorktree: false, notify: {desktop: true, feishu: false}, status: 'active', createdAt: now - 9 * day, updatedAt: now - day, lastFiredAt: now - day, nextFireAt: now + day, unreadRunCount: 1},
  {id: 'project', name: 'Nightly dependency audit for a project with a long name', prompt: 'Audit', kind: 'cron', cronExpr: '30 2 * * *', timezone: 'Asia/Shanghai', recurring: true, manual: false, agentKind: 'pi', model: 'second', providerId: 'service', workspaceKind: 'project', workingDir: 'C:\\fixture\\Alpha', useWorktree: true, source: 'project', projectConfigId: 'auto-audit', notify: {desktop: true, feishu: false}, status: 'paused', createdAt: now - 20 * day, updatedAt: now - 2 * day},
];
const runs = [
  {id: 'r1', scheduleId: 'daily', sessionId: 'c1', firedAt: now - day, finishedAt: now - day + 92000, status: 'success', resultText: '一切正常'},
  {id: 'r2', scheduleId: 'daily', sessionId: 'c2', firedAt: now - 2 * day, finishedAt: now - 2 * day + 40000, status: 'failed', errorMsg: 'Model request failed', readAt: now},
  {id: 'r3', scheduleId: 'daily', firedAt: now - 3 * day, finishedAt: now - 3 * day + 2000, status: 'skipped', preRunHookResult: {checkSucceeded: true, status: 'skipped', decision: 'skip', exitCode: 2, durationMs: 1200, stdout: 'No changes since last run', stderr: '', stdoutTruncated: false, stderrTruncated: false, timedOut: false, aborted: false}, readAt: now},
];
const catalog: ChatCatalog = {projects: [{id: 'alpha', name: 'Alpha', path: 'C:\\fixture\\Alpha'}], conversations: [{id: 'c1', title: '每日项目巡检', directory: '', sessionDirectory: '', model: {provider: 'service', model: 'first'}, permission: 'ask' as const, createdAt: '2026-10-24T01:00:00Z', updatedAt: '2026-10-24T01:02:00Z'}, {id: 'c2', title: '每日项目巡检', directory: '', sessionDirectory: '', model: {provider: 'service', model: 'first'}, permission: 'ask' as const, createdAt: '2026-10-23T01:00:00Z', updatedAt: '2026-10-23T01:02:00Z'}]};
const calls: Array<{method: string; params: unknown}> = [];
const reply = (method: string, params: Record<string, unknown>): unknown => {
  switch (method) {
    case 'automations.list': return schedules;
    case 'automations.runtime': return {schedulerInstanceId: 'fixture', inFlight: 0, slotsInUse: 0, maxConcurrentRuns: 2, inFlightRuns: [], waitingSchedules: []};
    case 'automations.events': return params.cursor === Number.MAX_SAFE_INTEGER ? {cursor: 1, events: []} : new Promise(() => {});
    case 'automations.runs': return runs.filter((run) => run.scheduleId === params.id);
    case 'automations.sidebarIndexRuns': return {runs: [], inflightRunIds: [], inflightPolicies: []};
    case 'automations.templates': return {templates: [], categories: []};
    case 'automations.channels': return {feishu: {status: 'disconnected'}, wecomGroup: {configured: false, enabled: false}};
    case 'automations.capabilityStatus': return {statuses: []};
    case 'automations.detectCwd': return {gitInstalled: true, isGitRepo: true, isInsideWorktree: false, currentBranch: 'main'};
    case 'automations.inflightCount': return 0;
    case 'automations.preview': return {nextFireAt: now + day, description: ''};
    case 'catalog': return catalog;
    default: return {};
  }
};
Object.assign(window, {cindyAutomationCalls: calls, go: {main: {App: {
  SetFileDropTarget: async () => {},
  Chat: async (method: string, raw: string) => { const params = JSON.parse(raw); calls.push({method, params}); return JSON.stringify(await reply(method, params)); },
}}}});
await setApplicationLanguage(query.get('language') ?? 'zh-CN');
document.documentElement.dataset.theme = query.get('theme') ?? 'light';
publishCindyHost({catalog});
createRoot(document.getElementById('root')!).render(<div style={{height: '100dvh'}} className="chat-panel">
  <SchedulerHost providers={providers} defaultModel={{provider: 'service', model: 'first'}} onOpenConversation={(id) => calls.push({method: 'open-conversation', params: id})} onOpenAIServices={() => {}} onOpenIMSettings={() => {}}/>
</div>);
