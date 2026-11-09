import { useContext, useEffect, useState, useRef } from 'react'
import { t } from 'config/i18n'

import { isRecord } from 'core/lib/type-guards'
import { RoomContext } from 'contexts/RoomContext'
import { ShellContext } from 'contexts/ShellContext'
import { PeerAction } from 'core/models/network'
import {
  ScreenShareState,
  StreamType,
  AudioChannelName,
  AudioState,
} from 'core/models/chat'
import { TrysteroTransport } from 'core/transport/TrysteroTransport'
import {
  PeerHookType,
  PeerStreamType,
  ActionNamespace,
} from 'core/transport/types'
import { usePeerAction } from 'hooks/usePeerAction'
import { MessageContext } from 'trystero'

interface UseRoomScreenShareConfig {
  peerRoom: TrysteroTransport
}

export function useRoomScreenShare({ peerRoom }: UseRoomScreenShareConfig) {
  const shellContext = useContext(ShellContext)
  const roomContext = useContext(RoomContext)
  const [isSharingScreen, setIsSharingScreen] = useState(false)
  const active = useRef<MediaStream | null>(null)
  const pending = useRef(false)
  const mounted = useRef(true)

  const {
    updatePeer,
    setScreenState,
    setAudioChannelState,
    setPeerAudioChannels,
  } = shellContext

  const {
    peerScreenStreams,
    selfScreenStream,
    setPeerScreenStreams,
    setSelfScreenStream,
  } = roomContext

  const [sendScreenShare] = usePeerAction<ScreenShareState>({
    namespace: ActionNamespace.GROUP,
    peerAction: PeerAction.SCREEN_SHARE,
    peerRoom,
    onReceive: (screenState, { peerId }: MessageContext) => {
      updatePeer(peerId, { screenShareState: screenState })

      if (screenState === ScreenShareState.NOT_SHARING) {
        deletePeerScreen(peerId)
      }
    },
  })

  peerRoom.onPeerStream(PeerStreamType.SCREEN, (stream, peerId, metadata) => {
    const isScreenShareStream =
      isRecord(metadata) &&
      'type' in metadata &&
      metadata['type'] === StreamType.SCREEN_SHARE

    if (!isScreenShareStream) return

    setPeerScreenStreams({
      ...peerScreenStreams,
      [peerId]: stream,
    })

    const [audioStream] = stream.getAudioTracks()

    if (audioStream) {
      setAudioChannelState(prevState => ({
        ...prevState,
        [AudioChannelName.SCREEN_SHARE]: AudioState.PLAYING,
      }))

      const audioTracks = stream.getAudioTracks()

      if (audioTracks.length > 0) {
        const audio = new Audio()

        audio.srcObject = stream
        audio.autoplay = true

        setPeerAudioChannels(peerAudioChannels => {
          return {
            ...peerAudioChannels,
            [peerId]: {
              ...peerAudioChannels[peerId],
              [AudioChannelName.SCREEN_SHARE]: audio,
            },
          }
        })
      }
    }
  })

  const handleScreenShareStart = async () => {
    if (active.current || pending.current) return
    pending.current = true
    try {
    const displayMedia = await window.navigator.mediaDevices.getDisplayMedia({
      audio: true,
      video: true,
    })
    if (!mounted.current) {
      displayMedia.getTracks().forEach(track => track.stop())
      return
    }
    active.current = displayMedia
    displayMedia.getVideoTracks().forEach(track => track.addEventListener('ended', handleScreenShareStop, { once: true }))

    peerRoom.addStream(displayMedia, {
      metadata: { type: StreamType.SCREEN_SHARE },
    })

    setSelfScreenStream(displayMedia)
    sendScreenShare(ScreenShareState.SHARING)
    setScreenState(ScreenShareState.SHARING)
    setIsSharingScreen(true)
    } catch (error) {
      if (mounted.current) shellContext.showAlert(t('Screen sharing did not start') + ': ' + String(error), { severity: 'error' })
    } finally { pending.current = false }
  }

  const handleScreenShareStop = () => {
    const stream = active.current
    if (!stream) return
    active.current = null
    stream.getTracks().forEach(track => track.stop())
    peerRoom.removeStream(stream, { target: peerRoom.getPeers() })
    sendScreenShare(ScreenShareState.NOT_SHARING)
    setScreenState(ScreenShareState.NOT_SHARING)
    setSelfScreenStream(null)
    setIsSharingScreen(false)
  }

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
      const stream = active.current
      active.current = null
      stream?.getTracks().forEach(track => track.stop())
      setSelfScreenStream(null)
      setScreenState(ScreenShareState.NOT_SHARING)
    }
  }, [setSelfScreenStream, setScreenState])

  useEffect(() => {
    return () => {
      setPeerScreenStreams({})
    }
  }, [setPeerScreenStreams])

  const deletePeerScreen = (peerId: string) => {
    setPeerScreenStreams(({ [peerId]: _, ...newPeerScreens }) => {
      return newPeerScreens
    })

    setPeerAudioChannels(({ ...newPeerAudios }) => {
      if (!newPeerAudios[peerId]) {
        return newPeerAudios
      }

      const screenShareAudio =
        newPeerAudios[peerId][AudioChannelName.SCREEN_SHARE]

      screenShareAudio?.pause()

      const { [AudioChannelName.SCREEN_SHARE]: _, ...newPeerAudioChannels } =
        newPeerAudios[peerId]

      newPeerAudios[peerId] = newPeerAudioChannels

      return newPeerAudios
    })
  }

  const handleScreenForNewPeer = (peerId: string) => {
    if (selfScreenStream) {
      peerRoom.addStream(selfScreenStream, {
        target: peerId,
        metadata: { type: StreamType.SCREEN_SHARE },
      })
    }
  }

  const handleScreenForLeavingPeer = (peerId: string) => {
    if (selfScreenStream) {
      peerRoom.removeStream(selfScreenStream, { target: peerId })
    }

    deletePeerScreen(peerId)
  }

  peerRoom.onPeerJoin(PeerHookType.SCREEN, (peerId: string) => {
    handleScreenForNewPeer(peerId)
  })

  peerRoom.onPeerLeave(PeerHookType.SCREEN, (peerId: string) => {
    handleScreenForLeavingPeer(peerId)
  })

  return {
    handleScreenShareStart,
    handleScreenShareStop,
    isSharingScreen,
    setIsSharingScreen,
  }
}
