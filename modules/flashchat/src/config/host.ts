import { useSyncExternalStore } from 'react'

export interface HostState { language: string; theme: 'light' | 'dark' }
declare global { interface Window { __winoneState?: HostState; __winoneLegacyNickname?: string } }
const fallback: HostState = { language: 'en', theme: 'light' }
const subscribe = (listener: () => void) => {
  window.addEventListener('winone-state', listener)
  return () => window.removeEventListener('winone-state', listener)
}
export const hostState = () => window.__winoneState ?? fallback
export const useHostState = () => useSyncExternalStore(subscribe, hostState, hostState)

function syncDocument() {
  if (!window.__winoneState) return
  document.documentElement.lang = hostState().language
  document.documentElement.style.colorScheme = hostState().theme
}
syncDocument()
window.addEventListener('winone-state', syncDocument)
