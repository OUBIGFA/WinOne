/** Private, instance-scoped transport for the MyGo UI. No Host RPC or arbitrary
 * filesystem operation is exposed to the browser engine. */
export interface NativeCommand {
  scope: string; action: string; words?: boolean; room?: string; private?: boolean;
  password?: string; nickname?: string; peer?: string; value?: unknown; text?: string;
  files?: unknown; magnet?: string; enabled?: boolean; id?: string;
  position?: number; volume?: number; channel?: string;
}
type Handler = (command: NativeCommand) => unknown | Promise<unknown>
declare global {
  interface Window {
    __winoneNative?: { token: string; visible?: boolean; focused?: boolean; selected?: string }
    __flashNativeInvoke?: (command: NativeCommand) => Promise<{ value?: unknown; error?: string }>
  }
}
export const nativeMode = () => Boolean(window.__winoneNative)
const handlers = new Map<string, Handler>()
const state: Record<string, unknown> = {}
let pending = false
let revision = 0

export async function nativePost(path: string, body: BodyInit, contentType: string) {
  const response = await fetch('/native/' + path, { method: 'POST', body,
    headers: { 'X-WinOne-Token': window.__winoneNative!.token, 'Content-Type': contentType } })
  if (!response.ok) throw new Error(`Native bridge: ${response.status}`)
}

export function publish(values: Record<string, unknown>) {
  if (!nativeMode()) return
  Object.assign(state, values)
  if (pending) return
  pending = true
  queueMicrotask(() => {
    pending = false
    void nativePost('state', JSON.stringify({ ...state, revision: ++revision }), 'application/json')
      .catch(error => console.error(error))
  })
}

export function register(scope: string, handler: Handler) {
  handlers.set(scope, handler)
  window.__flashNativeInvoke = async command => {
    try {
      const target = handlers.get(command.scope)
      if (!target) throw new Error('The requested conversation is not ready')
      return { value: await target(command) }
    } catch (error) { return { error: String(error) } }
  }
  return () => { if (handlers.get(scope) === handler) handlers.delete(scope) }
}

export async function pickedFiles(items: { token: string; name: string; type: string }[]) {
  return Promise.all(items.map(async item => {
    const response = await fetch('/native/file/' + encodeURIComponent(item.token), {
      headers: { 'X-WinOne-Token': window.__winoneNative!.token },
    })
    if (!response.ok) throw new Error('The selected file is no longer available')
    return new File([await response.blob()], item.name, { type: item.type })
  }))
}
