// Extracted from Cindy useComposerSendShortcutPreference.ts (779b5237),
// Copyright 2026 XD Inc., Apache-2.0. WinOne supplies its Windows key preference;
// this pure resolver keeps IME, repeat, newline, queue and steer semantics intact.
export type ComposerSendShortcutPreference = 'enter' | 'modifier-enter';
export type ComposerEnterIntent = 'queue' | 'steer' | 'native' | 'ignore' | null;
export type ComposerEnterEvent = Pick<KeyboardEvent,
 'key' | 'shiftKey' | 'altKey' | 'metaKey' | 'ctrlKey' | 'repeat' | 'isComposing'>;

export function resolveComposerEnterIntent(
 event: ComposerEnterEvent,
 preference: ComposerSendShortcutPreference,
 options: {turnRunning: boolean; platform: string | undefined},
): ComposerEnterIntent {
 if (event.key !== 'Enter' || event.shiftKey || event.altKey) return null;
 if (event.isComposing) return 'native';
 const hasModifier = options.platform === 'darwin' ? event.metaKey : event.ctrlKey;
 if (event.repeat) {
  const isSendShortcut = preference === 'enter' || hasModifier;
  return isSendShortcut ? 'ignore' : 'native';
 }
 if (preference === 'modifier-enter') return hasModifier ? 'queue' : 'native';
 if (!hasModifier) return 'queue';
 return options.turnRunning ? 'steer' : 'queue';
}
