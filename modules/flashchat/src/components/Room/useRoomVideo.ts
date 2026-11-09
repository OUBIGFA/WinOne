import { useContext, useEffect } from 'react'
import { useLocalMedia } from 'hooks/useLocalMedia'
import { t } from 'config/i18n'

import { RoomContext } from 'contexts/RoomContext'
import { ShellContext } from 'contexts/ShellContext'
import { PeerAction } from 'core/models/network'
import { VideoState, StreamType } from 'core/models/chat'
import { TrysteroTransport } from 'core/transport/TrysteroTransport'
import {
  PeerHookType,
  PeerStreamType,
  ActionNamespace,
} from 'core/transport/types'
import { isRecord } from 'core/lib/type-guards'
import { usePeerAction } from 'hooks/usePeerAction'
import { MessageContext } from 'trystero'

interface UseRoomVideoConfig {
  peerRoom: TrysteroTransport
}

export function useRoomVideo({ peerRoom }: UseRoomVideoConfig) {
  const shellContext = useContext(ShellContext)
  const roomContext = useContext(RoomContext)
  const { updatePeer, setVideoState } = shellContext
  const { peerVideoStreams, setPeerVideoStreams, setSelfVideoStream } = roomContext
  const { enabled: isCameraEnabled, setEnabled: setIsCameraEnabled, stream: selfVideoStream, devices: videoDevices, selectDevice } = useLocalMedia('video', error => {
    shellContext.showAlert(t('Camera unavailable') + ': ' + String(error), { severity: 'error' })
  })

  const [sendVideoChange] = usePeerAction<VideoState>({
    namespace: ActionNamespace.GROUP,
    peerAction: PeerAction.VIDEO_CHANGE,
    peerRoom,
    onReceive: (videoState, { peerId }: MessageContext) => {
      updatePeer(peerId, { videoState })

      if (videoState === VideoState.STOPPED) {
        deletePeerVideo(peerId)
      }
    },
  })

  peerRoom.onPeerStream(PeerStreamType.VIDEO, (stream, peerId, metadata) => {
    const isWebcamStream =
      isRecord(metadata) &&
      'type' in metadata &&
      metadata['type'] === StreamType.WEBCAM

    if (!isWebcamStream) return

    setPeerVideoStreams({
      ...peerVideoStreams,
      [peerId]: stream,
    })
  })

  useEffect(() => {
    if (!selfVideoStream) return
    peerRoom.addStream(selfVideoStream, { metadata: { type: StreamType.WEBCAM } })
    sendVideoChange(VideoState.PLAYING)
    setVideoState(VideoState.PLAYING)
    setSelfVideoStream(selfVideoStream)
    return () => {
      peerRoom.removeStream(selfVideoStream, { target: peerRoom.getPeers() })
      sendVideoChange(VideoState.STOPPED)
      setVideoState(VideoState.STOPPED)
      setSelfVideoStream(null)
    }
  }, [selfVideoStream, peerRoom, sendVideoChange, setVideoState, setSelfVideoStream])
  useEffect(() => () => setPeerVideoStreams({}), [setPeerVideoStreams])
  const handleVideoDeviceSelect = (device: MediaDeviceInfo) => selectDevice(device.deviceId)

  const deletePeerVideo = (peerId: string) => {
    const newPeerVideos = { ...peerVideoStreams }

    delete newPeerVideos[peerId]
    setPeerVideoStreams(newPeerVideos)
  }

  const handleVideoForNewPeer = (peerId: string) => {
    if (selfVideoStream) {
      peerRoom.addStream(selfVideoStream, {
        target: peerId,
        metadata: { type: StreamType.WEBCAM },
      })
    }
  }

  const handleVideoForLeavingPeer = (peerId: string) => {
    if (selfVideoStream) {
      peerRoom.removeStream(selfVideoStream, { target: peerId })
    }

    deletePeerVideo(peerId)
  }

  peerRoom.onPeerJoin(PeerHookType.VIDEO, (peerId: string) => {
    handleVideoForNewPeer(peerId)
  })

  peerRoom.onPeerLeave(PeerHookType.VIDEO, (peerId: string) => {
    handleVideoForLeavingPeer(peerId)
  })

  return {
    videoDevices,
    isCameraEnabled,
    setIsCameraEnabled,
    handleVideoDeviceSelect,
  }
}
