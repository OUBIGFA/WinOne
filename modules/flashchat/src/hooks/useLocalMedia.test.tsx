import { act, renderHook, waitFor } from '@testing-library/react'
import { vi } from 'vitest'
import { useLocalMedia } from './useLocalMedia'

// jsdom has no capture devices; only the browser acquisition boundary is fake.
function media() {
  const track = Object.assign(new EventTarget(), { stop: vi.fn() })
  return { track, stream: { getTracks: () => [track] } as unknown as MediaStream }
}
describe('local capture lifetime', () => {
  const getUserMedia = vi.fn()
  beforeEach(() => {
    getUserMedia.mockReset()
    Object.defineProperty(navigator, 'mediaDevices', { configurable: true, value: {
      getUserMedia, enumerateDevices: async () => [],
    } })
  })
  test('reports permission denial and returns the control to off', async () => {
    getUserMedia.mockRejectedValue(new DOMException('denied', 'NotAllowedError'))
    const report = vi.fn()
    const { result } = renderHook(() => useLocalMedia('audio', report))
    act(() => result.current.setEnabled(true))
    await waitFor(() => expect(report).toHaveBeenCalledOnce())
    expect(result.current.enabled).toBe(false)
    expect(result.current.stream).toBeNull()
  })
  test('stops a late permission result after the user turns capture off', async () => {
    const captured = media()
    let resolve!: (stream: MediaStream) => void
    getUserMedia.mockReturnValue(new Promise(r => { resolve = r }))
    const { result } = renderHook(() => useLocalMedia('audio', vi.fn()))
    act(() => result.current.setEnabled(true))
    act(() => result.current.setEnabled(false))
    await act(async () => resolve(captured.stream))
    expect(captured.track.stop).toHaveBeenCalledOnce()
    expect(result.current.stream).toBeNull()
  })
  test('keeps the working device if its replacement fails and releases it on unmount', async () => {
    const captured = media()
    getUserMedia.mockResolvedValueOnce(captured.stream).mockRejectedValueOnce(new Error('device busy'))
    const report = vi.fn()
    const { result, unmount } = renderHook(() => useLocalMedia('video', report))
    act(() => result.current.setEnabled(true))
    await waitFor(() => expect(result.current.stream).toBe(captured.stream))
    await act(async () => { expect(await result.current.selectDevice('other')).toBe(false) })
    expect(result.current.stream).toBe(captured.stream)
    expect(captured.track.stop).not.toHaveBeenCalled()
    expect(report).toHaveBeenCalledOnce()
    unmount()
    expect(captured.track.stop).toHaveBeenCalledOnce()
  })
  test('turns off when the browser ends the captured track', async () => {
    const captured = media()
    getUserMedia.mockResolvedValue(captured.stream)
    const { result } = renderHook(() => useLocalMedia('video', vi.fn()))
    act(() => result.current.setEnabled(true))
    await waitFor(() => expect(result.current.stream).toBe(captured.stream))
    act(() => captured.track.dispatchEvent(new Event('ended')))
    expect(result.current.enabled).toBe(false)
    expect(result.current.stream).toBeNull()
  })
})
