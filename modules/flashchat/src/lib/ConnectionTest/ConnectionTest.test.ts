import { vi } from 'vitest'

import { ConnectionTest, ConnectionTestEvents } from './ConnectionTest'

class StubRtcPeerConnection extends EventTarget {
  createOffer = vi.fn(
    (): Promise<RTCSessionDescriptionInit> =>
      Promise.resolve({ type: 'offer', sdp: '' })
  )
  setLocalDescription = vi.fn(
    (_description: RTCSessionDescriptionInit): Promise<void> =>
      Promise.resolve()
  )
  close = vi.fn()
}

describe('ConnectionTest', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  test('waits for setLocalDescription so a late failure is handled', async () => {
    const stub = new StubRtcPeerConnection()
    let failSetLocalDescription = (_error: Error) => {}

    stub.setLocalDescription.mockImplementation(
      () =>
        new Promise<void>((_resolve, reject) => {
          failSetLocalDescription = reject
        })
    )

    vi.stubGlobal('RTCPeerConnection', function () {
      return stub
    })

    const connectionTest = new ConnectionTest({ iceServers: [] })
    let isSettled = false
    const init = connectionTest.initRtcPeerConnectionTest().finally(() => {
      isSettled = true
    })

    await new Promise(resolve => setTimeout(resolve, 0))

    expect(stub.setLocalDescription).toHaveBeenCalledTimes(1)
    expect(isSettled).toBe(false)

    failSetLocalDescription(new DOMException('closed', 'InvalidStateError'))

    await expect(init).rejects.toThrow('closed')
    expect(isSettled).toBe(true)
    connectionTest.destroyRtcPeerConnectionTest()
  })

  test('distinguishes local, public and relay candidates and cancels pending checks on close', async () => {
    vi.useFakeTimers()
    const stub = new StubRtcPeerConnection()
    vi.stubGlobal('RTCPeerConnection', function () { return stub })
    const test = new ConnectionTest({ iceTransportPolicy: 'all' })
    const updates = vi.fn()
    test.addEventListener(ConnectionTestEvents.CONNECTION_TEST_RESULTS_UPDATED, updates)
    await test.initRtcPeerConnectionTest()
    for (const type of ['host', 'srflx', 'relay']) {
      const event = new Event('icecandidate')
      Object.defineProperty(event, 'candidate', { value: { candidate: `candidate:1 1 UDP 1 192.0.2.1 1234 typ ${type}` } })
      stub.dispatchEvent(event)
      expect(test.hasServerReflexive).toBe(type !== 'host')
      expect(test.hasTURNServer).toBe(type === 'relay')
    }
    expect(updates).toHaveBeenCalledTimes(3)
    test.destroyRtcPeerConnectionTest()
    expect(vi.getTimerCount()).toBe(0)
    expect(stub.close).toHaveBeenCalledOnce()
  })
})
