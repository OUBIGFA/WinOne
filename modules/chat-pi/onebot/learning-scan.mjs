// Replays are harmless: Cindy's original counter claims each completed reply
// against persistent message metadata before changing the nightly state. The
// same pass attaches the turn's own memory/Skill saves to its reply and starts
// Cindy's per-reply learning review, which claims each reply once.
import {countNightlyLearningTurn} from '../cindy/bots/botNightlyLearningCounter.ts';
import {reviewTeammateLearning} from '../cindy/bots/botLearningReview.ts';
import {learningReceiptPublisher} from '../cindy/bots/botLearningFeedback.ts';
import {readBotLearningReceipts} from '../../../third_party/cindy/maker-shared/botLearning.ts';
import {rowsForSession} from './learning-data.mjs';
import {createLogger} from './logger.ts';

const log = createLogger('bot-learning');
/** Pending receipts of one turn, keyed by its accepted input (botLearningTracker). */
export const pendingLearningKey = inputId => 'learning-input:' + inputId;

export function createOneBotLearningScan(owner) {
  let current, again = false;
  function scan() {
    if (current) { again = true; return current; }
    current = (async () => {
      do {
        again = false;
        for (const bot of owner.profiles.list().filter(row => row.status === 'active')) {
          for (const link of owner.profiles.sessions(bot.id).filter(row => ['canonical', 'thread', 'channel'].includes(row.role) && row.archivedAt == null)) {
            const rows = await rowsForSession(owner, link);
            let input;
            for (const row of rows) {
              if (row.role === 'user') input = row;
              if (row.role !== 'assistant' || !row.meta.turnCompleted) continue;
              if (!row.meta.nightlyLearningCounted) await countNightlyLearningTurn(link.id, row.id);
              // Saves made during this turn, published under its finished reply.
              const pending = input ? readBotLearningReceipts(owner.profiles.messageMeta(link.id, pendingLearningKey(input.id)).botLearningPending) : [];
              if (pending.length) {
                owner.profiles.patchMessageMeta(link.id, pendingLearningKey(input.id), {botLearningPending: []});
                learningReceiptPublisher(link.id)(row.id, pending);
              }
            }
            // Cindy reviews each finished reply once, in the background and
            // serially per teammate; the ledger backfills any earlier gap.
            const latest = rows.findLast(row => row.role === 'assistant' && row.meta.turnCompleted);
            if (latest && !latest.meta.botLearningReviewed) {
              void reviewTeammateLearning(link.id, latest.id).catch(error => log.warn('learning review failed', {error: String(error?.code ?? error?.name ?? 'Error')}));
            }
          }
        }
      } while (again && !owner.closed);
      return {ok: true};
    })().finally(() => { current = undefined; });
    return current;
  }
  return {scan};
}
