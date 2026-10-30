// Shared contracts required by the pinned task cards and rich-answer renderer.
export * from '../../../../../../../third_party/cindy/maker-shared/sessionPrRefs';
export * from '../../../../../../../third_party/cindy/maker-shared/workingStatus';
export {visibleAskOptions} from '../group-shared/packages/maker-shared/src/interaction';
export {leadingSlashInvocation} from '../group-shared/packages/maker-shared/src/composerPalette';
export {FILE_VISUAL_LABELS, getFileVisualKind, type FileVisualInput, type FileVisualKind} from '../group-shared/packages/maker-shared/src/filePresentation';
export type GitContextDirSource='telemetry'|'worktree'|'workingDir'|'remote'|null;
