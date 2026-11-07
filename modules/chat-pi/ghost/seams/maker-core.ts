// The two narrow things plugin slots read from Cindy's agent core.
export {parseLibraryAssetRef, resolveLibraryAssetPath} from '../../cindy/ghost/packages/maker-core/src/agents/shared/library-native-read.ts';
import {createAutoReviewIntentProjection} from '../../cindy/tasks/core/autoReviewIntent.ts';
// The pure upstream normalizer is shared with task admission and Auto review.
export const normalizeAutoReviewUserIntent=createAutoReviewIntentProjection().normalize;

export type ReviewableAction = {kind: 'tool'; name: string; input: Record<string, unknown>};
export type AutoReviewDecision = {verdict: 'allow' | 'block' | 'ask'; reason?: string};
export type Logger = {debug?(...args: unknown[]): void; info(...args: unknown[]): void; warn(...args: unknown[]): void; error?(...args: unknown[]): void};

// A tool action as WinOne's permission gate reviews it.
export function toolAutoReviewAction(name: string, input: Record<string, unknown>): ReviewableAction {
  return {kind: 'tool', name, input};
}
