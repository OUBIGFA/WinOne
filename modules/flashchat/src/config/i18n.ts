import zh from './zh-CN.json'
import { hostState, useHostState } from './host'

/** Translate app-owned labels only. Messages, names and received files never
 * pass through this function; their text must remain exactly what peers sent. */
export function t(text: string, values: Record<string, string> = {}): string {
  const label = hostState().language.startsWith('zh') ? (zh as Record<string, string>)[text] ?? text : text
  // Translate the template, then insert names verbatim. Never translate peer
  // text or interpret replacement-string syntax inside a chosen nickname.
  return label.replace(/\{(\w+)\}/g, (placeholder, key: string) => values[key] ?? placeholder)
}
export function useLanguage() { return useHostState().language }
