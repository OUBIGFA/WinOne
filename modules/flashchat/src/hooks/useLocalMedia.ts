import { useCallback, useEffect, useRef, useState } from 'react'

/** Own local microphone/camera capture independently of peer-list rerenders.
 * Late permission responses are stopped, and a failed device switch keeps the
 * current working capture. Publishing to peers remains in the upstream hooks.
 */
export function useLocalMedia(kind: 'audio' | 'video', onError: (error: unknown) => void) {
  const [enabled, setEnabled] = useState(false)
  const [stream, setStream] = useState<MediaStream | null>(null)
  const [devices, setDevices] = useState<MediaDeviceInfo[]>([])
  const active = useRef<MediaStream | null>(null)
  const generation = useRef(0)
  const device = useRef<string>()
  const report = useRef(onError)
  report.current = onError

  const acquire = useCallback(async (deviceId?: string) => {
    const request = ++generation.current
    try {
      const captured = await navigator.mediaDevices.getUserMedia({
        audio: kind === 'audio' ? (deviceId ? { deviceId: { exact: deviceId } } : true) : false,
        video: kind === 'video' ? (deviceId ? { deviceId: { exact: deviceId } } : true) : false,
      })
      if (request !== generation.current) {
        captured.getTracks().forEach(track => track.stop())
        return false
      }
      const previous = active.current
      active.current = captured
      device.current = deviceId
      setStream(captured)
      previous?.getTracks().forEach(track => track.stop())
      captured.getTracks().forEach(track => track.addEventListener('ended', () => {
        if (active.current === captured) setEnabled(false)
      }, { once: true }))
      try {
        const list = await navigator.mediaDevices.enumerateDevices()
        if (request === generation.current) setDevices([...new Map(list.filter(d => d.kind === `${kind}input`).map(d => [d.deviceId, d])).values()])
      } catch (error) {
        if (request === generation.current) report.current(error)
      }
      return request === generation.current
    } catch (error) {
      if (request !== generation.current) return false
      if (!active.current) setEnabled(false)
      report.current(error)
      return false
    }
  }, [kind])

  useEffect(() => {
    if (enabled) void acquire(device.current)
    else setStream(null)
    return () => {
      generation.current++
      active.current?.getTracks().forEach(track => track.stop())
      active.current = null
    }
  }, [enabled, acquire])

  const selectDevice = useCallback(async (id: string) => {
    if (active.current) return acquire(id)
    device.current = id
    return true
  }, [acquire])
  return { enabled, setEnabled, stream, devices, selectDevice }
}
