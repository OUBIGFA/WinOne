// Original nightly-learning execution and undo, composed with the original
// builtin routine. Only local storage facts and the WinOne model seam differ.
import fs from 'node:fs';
import path from 'node:path';
import {readNightlyLearningState, updateNightlyLearningState} from '../cindy/bots/botNightlyLearningState.ts';
import {runNightlyLearning, isNightlyLearningRunning, revertNightlyLearningItem, resolveNightlyLearningCandidate} from '../cindy/bots/botNightlyLearning.ts';
import {buildNightlyLearningRoutineInput, builtinNightlyLearningRoutineId, NIGHTLY_LEARNING_BUILTIN_VERSION} from '../cindy/bots/nightlyLearningRoutine.ts';
import {oneBotError} from './content-service.mjs';
import {withLearningBot} from './learning-host.mjs';

export function createOneBotNightlyLearning({paths, profiles, event, getEngine}) {
  function guard(botId) {
    profiles.get(botId);
    const home = paths.home(botId), history = path.join(home, 'learning-history');
    paths.assertPath(path.join(home, 'nightly-learning.json'));
    paths.assertPath(history);
    if (fs.existsSync(history)) for (const file of fs.readdirSync(history)) paths.assertPath(path.join(history, file));
  }
  async function ensure(botId) {
    guard(botId);
    const engine = getEngine(), id = builtinNightlyLearningRoutineId(botId);
    if (profiles.get(botId).status !== 'active') return engine.list(botId).find(row => row.id === id) ?? null;
    const existing = engine.list(botId).find(row => row.id === id);
    const timezone = existing?.triggers.find(trigger => trigger.kind === 'cron')?.timezone || Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
    const frequency = (await readNightlyLearningState(paths.root, botId))?.frequency ?? 'daily';
    return engine.ensureBuiltin(botId, 'nightly-learning', buildNightlyLearningRoutineInput(frequency, timezone), id, NIGHTLY_LEARNING_BUILTIN_VERSION);
  }
  async function view(botId) {
    guard(botId);
    const state = await readNightlyLearningState(paths.root, botId);
    const routine = getEngine().list(botId).find(row => row.id === builtinNightlyLearningRoutineId(botId));
    return {enabled: routine?.enabled ?? null, frequency: state?.frequency ?? 'daily', pendingTurns: state?.pending.turns ?? 0,
      running: isNightlyLearningRunning(botId), memoryEnabled: profiles.get(botId).capabilities.memory !== false,
      lastRun: state?.runs[0] ?? null, runs: state?.runs ?? [],
      candidates: (state?.candidates ?? []).map(({draft: _draft, ...candidate}) => candidate)};
  }
  return {ensure, async execute(botId, manual, signal) { guard(botId); return withLearningBot(botId, signal, () => runNightlyLearning(botId, {manual})); }, async call(method, params) {
    const {botId} = params;
    const engine = getEngine();
    // Cindy's view only reads the existing rule. Re-ensuring a disabled rule
    // calls its pause path, which would abort an explicit manual review.
    const routine = ['setEnabled', 'setFrequency', 'runNow'].includes(method)
      ? await ensure(botId) : engine.list(botId).find(row => row.id === builtinNightlyLearningRoutineId(botId));
    if (method === 'setEnabled') {
      if (typeof params.enabled !== 'boolean') throw oneBotError('INVALID_PARAMS', '学习开关必须为布尔值');
      if (!routine) throw oneBotError('INACTIVE', '伙伴未启用');
      await engine.put(botId, {...routine, enabled: params.enabled}, routine.id, routine.revision);
    } else if (method === 'setFrequency') {
      if (!['daily', 'weekly'].includes(params.frequency)) throw oneBotError('INVALID_PARAMS', '请选择每天或每周');
      await updateNightlyLearningState(paths.root, botId, state => {
        if (params.frequency === 'daily') delete state.frequency; else state.frequency = params.frequency;
        return state;
      });
      await ensure(botId);
    } else if (method === 'runNow') {
      if (!routine) throw oneBotError('INACTIVE', '伙伴未启用');
      await engine.runNow(botId, routine.id, routine.revision);
    } else if (method === 'revert') {
      if (typeof params.redo !== 'boolean') throw oneBotError('INVALID_PARAMS', '学习恢复操作无效');
      await revertNightlyLearningItem(botId, params.runId, params.itemId, params.redo);
    } else if (method === 'candidate') {
      if (!['promote', 'decline'].includes(params.action)) throw oneBotError('INVALID_PARAMS', '学习候选操作无效');
      await resolveNightlyLearningCandidate(botId, params.candidateId, params.action);
    } else if (method !== 'get') throw oneBotError('UNKNOWN_METHOD', '不支持的学习操作');
    if (method !== 'get') event({type: 'routines-changed', botId});
    return view(botId);
  }};
}
