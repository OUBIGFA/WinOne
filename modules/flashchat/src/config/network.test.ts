import { defaultRtcConfig, mergeRtcConfig } from './network'

test('direct connections retain STUN without an optional relay', () => {
  expect(mergeRtcConfig(defaultRtcConfig).iceServers).toEqual(defaultRtcConfig.iceServers)
  expect(defaultRtcConfig.iceServers?.length).toBeGreaterThan(0)
})

test('relay merge preserves caller policies and does not mutate the base', () => {
  const base: RTCConfiguration = { ...defaultRtcConfig, iceTransportPolicy: 'relay', bundlePolicy: 'max-bundle' }
  const relay = { urls: 'turn:example.test', username: 'test', credential: 'test' }
  const result = mergeRtcConfig(base, relay)
  expect(result.iceTransportPolicy).toBe('relay')
  expect(result.bundlePolicy).toBe('max-bundle')
  expect(result.iceServers).toEqual([...defaultRtcConfig.iceServers!, relay])
  expect(base.iceServers).toHaveLength(2)
})
