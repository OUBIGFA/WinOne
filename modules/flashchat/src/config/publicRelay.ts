/** Public configuration published by Jami:
 * https://docs.jami.net/en_US/developer/going-further/setting-up-your-own-turn-server.html
 * These are shared public credentials, not a WinOne user's account.
 * TCP allows WebRTC to reach the relay when UDP is unavailable. Availability
 * is measured by ICE; configuration alone is not proof of connectivity.
 * Users can disable this external service or supply their own configuration.
 */
export async function publicRelay(): Promise<RTCIceServer> {
  return {
    urls: [
      'turn:turn.jami.net:3478?transport=udp',
      'turn:turn.jami.net:3478?transport=tcp',
    ],
    username: 'ring',
    credential: 'ring',
  }
}
