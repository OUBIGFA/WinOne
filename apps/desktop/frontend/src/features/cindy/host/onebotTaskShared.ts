// Shared contracts required by the pinned task cards and rich-answer renderer.
export * from '../../../../../../../third_party/cindy/maker-shared/sessionPrRefs';
export * from '../../../../../../../third_party/cindy/maker-shared/workingStatus';
export {visibleAskOptions} from '../group-shared/packages/maker-shared/src/interaction';
export {leadingSlashInvocation} from '../group-shared/packages/maker-shared/src/composerPalette';
export {FILE_VISUAL_LABELS, getFileVisualKind, type FileVisualInput, type FileVisualKind} from '../group-shared/packages/maker-shared/src/filePresentation';
export type GitContextDirSource='telemetry'|'worktree'|'workingDir'|'remote'|null;
// Cindy maker-shared botDmThread.ts (2d8d3c3), re-exported by the package root.
export interface BotDmThreadSummary {
  /** The topic's hidden Session. */
  sessionId: string;
  rootSessionId: string;
  rootClientId: string;
  /** Visible user and companion messages in the topic. */
  replyCount: number;
  /** Latest visible message in the topic (ms), or null while it has none. */
  lastReplyAt: number | null;
  /** Latest companion reply in the topic (ms); the timeline compares it with the read position. */
  lastAssistantAt: number | null;
  createdAt: number;
}
