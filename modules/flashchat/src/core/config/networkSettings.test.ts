import { validNetworkSettings } from './networkSettings'

test('accepts empty/default profiles and custom ICE policies', () => {
  expect(validNetworkSettings({})).toBe(true)
  expect(validNetworkSettings({ rtcConfig: { iceTransportPolicy: 'relay', iceServers: [{ urls: ['turn:example.com:443?transport=tcp'], username: 'test', credential: 'test' }] }, trackerUrls: ['wss://example.com/tracker'] })).toBe(true)
})
test('rejects invalid server entries and unsupported network protocols', () => {
  for (const rtcConfig of [[], null, { iceServers: [{}] }, { iceServers: [{ urls: [] }] }, { iceServers: [{ urls: 'https://example.com' }] }, { iceTransportPolicy: 'invalid' }]) expect(validNetworkSettings({rtcConfig})).toBe(false)
  expect(validNetworkSettings({ trackerUrls: ['file:///secret'] })).toBe(false)
})
