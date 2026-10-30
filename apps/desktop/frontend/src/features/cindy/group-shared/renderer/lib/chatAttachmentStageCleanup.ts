import { electronAPI } from '../../../host/electronAPI';
import type { AttachedFile } from '../../../lib/fileTypes';
import { createLogger } from '../../../host/logger';

const log = createLogger('ChatAttachmentStageCleanup');

function stagedPaths(
  files: readonly Pick<AttachedFile, 'path' | 'stagedPathShared'>[],
): string[] {
  return [
    ...new Set(
      files
        .filter((file) => file.stagedPathShared !== true)
        .map((file) => file.path)
        .filter((filePath): filePath is string =>
          typeof filePath === 'string' && filePath.toLowerCase().endsWith('.bin'),
        ),
    ),
  ];
}

/**
 * Best-effort cleanup for physical `.bin` copies owned by an unsent draft.
 * Main re-checks the controlled cache root, so legacy/original paths are safe
 * to pass through and are simply ignored there.
 */
export function cleanupStagedChatAttachmentFiles(
  files: readonly Pick<AttachedFile, 'path' | 'stagedPathShared'>[],
): void {
  const paths = stagedPaths(files);
  if (paths.length === 0) return;
  try {
    void electronAPI.cleanupStagedChatAttachments(paths).catch((error: unknown) => {
      log.warn('staged attachment cleanup failed', error);
    });
  } catch (error) {
    log.warn('staged attachment cleanup failed', error);
  }
}
