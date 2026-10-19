// Cindy 77e2c32d, Copyright 2026 XD Inc., Apache-2.0.
// Only the document/attachment types are adapted: WinOne's composer is plain
// text, and browser annotations travel on ChatAttachment, not in a second store.
import type {ChatAttachment,ChatBrowserAnnotation} from '../chat-types';

export interface ComposerSendSnapshot {
  documentToken: string;
  attachments: readonly ChatAttachment[];
  browserComments: readonly ChatBrowserAnnotation[];
}

/** Captures object versions without serializing potentially large attachment bytes. */
export function captureComposerSendSnapshot(
  document: string,
  attachments: ChatAttachment[],
  browserComments: ChatBrowserAnnotation[],
): ComposerSendSnapshot {
  return {
    documentToken: JSON.stringify(document),
    attachments,
    browserComments,
  };
}

export function isComposerSendSnapshotCurrent(
  snapshot: ComposerSendSnapshot,
  document: string,
  attachments: ChatAttachment[],
  browserComments: ChatBrowserAnnotation[],
): boolean {
  return (
    snapshot.documentToken === JSON.stringify(document) &&
    snapshot.attachments.length === attachments.length &&
    snapshot.attachments.every((item, index) => item === attachments[index]) &&
    snapshot.browserComments.length === browserComments.length &&
    snapshot.browserComments.every((item, index) => item === browserComments[index])
  );
}
