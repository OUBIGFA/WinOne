/** Keep discovery compatible with the pinned public Chitchatter deployment.
 * Its production bundle uses an unset VITE_NAME, resulting in this namespace.
 * A local window origin must never split the room into a different network.
 */
export const appId = 'https://chitchatter.im_undefined'

export const defaultRtcConfig: RTCConfiguration = {
  iceServers: [
    { urls: 'stun:stun.l.google.com:19302' },
    { urls: 'stun:stun.cloudflare.com:3478' },
  ],
}

/** Merge the optional relay without erasing direct connectivity or policies. */
export function mergeRtcConfig(
  base: RTCConfiguration,
  relay?: RTCIceServer
): RTCConfiguration {
  return { ...base, iceServers: [...(base.iceServers ?? []), ...(relay ? [relay] : [])] }
}
