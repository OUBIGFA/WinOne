/** Validate profile/network input before it reaches WebRTC or WebSocket. */
export function validNetworkSettings(value: { rtcConfig?: unknown; trackerUrls?: unknown }): boolean {
  if (value.trackerUrls !== undefined && (!Array.isArray(value.trackerUrls) || !value.trackerUrls.every(url => {
    try { const parsed = new URL(url); return typeof url === 'string' && ['ws:', 'wss:'].includes(parsed.protocol) && !parsed.username && !parsed.password } catch { return false }
  }))) return false
  if (value.rtcConfig === undefined) return true
  const config = value.rtcConfig as RTCConfiguration
  if (!config || typeof config !== 'object' || Array.isArray(config)) return false
  if (config.iceTransportPolicy && !['all', 'relay'].includes(config.iceTransportPolicy)) return false
  if (config.bundlePolicy && !['balanced', 'max-compat', 'max-bundle'].includes(config.bundlePolicy)) return false
  if (config.iceServers !== undefined && (!Array.isArray(config.iceServers) || !config.iceServers.every(server => {
    if (!server || typeof server !== 'object') return false
    const urls = Array.isArray(server.urls) ? server.urls : [server.urls]
    return urls.length > 0 && urls.every(url => typeof url === 'string' && /^(stun|stuns|turn|turns):[^\s]+$/.test(url))
      && (server.username === undefined || typeof server.username === 'string')
      && (server.credential === undefined || typeof server.credential === 'string')
  }))) return false
  return true
}
