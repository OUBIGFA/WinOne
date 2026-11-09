/** Invitations are portable: the bundled desktop origin exists only locally. */
export function invitationUrl(raw = window.location.href): string {
  const url = new URL(raw)
  const shared = new URL(url.pathname + url.search + url.hash, 'https://chitchatter.im')
  return shared.href
}

export function parseInvitation(value: string, localOrigin = window.location.origin) {
  try {
    const url = new URL(value)
    if (url.origin !== 'https://chitchatter.im' && url.origin !== localOrigin) return null
    const match = /^\/(public|private)\/([^/]+)$/.exec(url.pathname)
    if (!match) return null
    return { roomName: decodeURIComponent(match[2]), path: url.pathname + url.hash }
  } catch { return null }
}
