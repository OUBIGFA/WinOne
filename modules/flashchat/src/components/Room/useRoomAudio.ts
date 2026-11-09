import { useContext, useEffect } from 'react'
import { useLocalMedia } from 'hooks/useLocalMedia'
import { t } from 'config/i18n'

import { ShellContext } from 'contexts/ShellContext'
import { PeerAction } from 'core/models/network'
import {
  AudioState,
  AudioChannelName,
  PeerAudioChannelState,
  StreamType,
} from 'core/models/chat'
import { TrysteroTransport } from 'core/transport/TrysteroTransport'
import {
  PeerHookType,
  PeerStreamType,
  ActionNamespace,
} from 'core/transport/types'
import { usePeerAction } from 'hooks/usePeerAction'
import { MessageContext } from 'trystero'

interface UseRoomAudioConfig {
  peerRoom: TrysteroTransport
}

export function useRoomAudio({ peerRoom }: UseRoomAudioConfig) {
  const shellContext = useContext(ShellContext)
  const { peerList, updatePeer, setAudioChannelState, setPeerAudioChannels } = shellContext
  const { enabled: isSpeakingToRoom, setEnabled: setIsSpeakingToRoom, stream: audioStream, devices: audioDevices, selectDevice } = useLocalMedia('audio', error => {
    shellContext.showAlert(t('Microphone unavailable') + ': ' + String(error), { severity: 'error' })
  })

  const [sendAudioChange] = usePeerAction<Partial<PeerAudioChannelState>>({
    namespace: ActionNamespace.GROUP,
    peerAction: PeerAction.AUDIO_CHANGE,
    peerRoom,
    onReceive: (peerAudioChannelState, { peerId }: MessageContext) => {
      const microphoneAudioChannel =
        peerAudioChannelState[AudioChannelName.MICROPHONE]

      if (!microphoneAudioChannel) return

      const peer = peerList.find(({ peerId: id }) => id === peerId)

      if (!peer) return

      updatePeer(peerId, {
        audioChannelState: {
          ...peer.audioChannelState,
          ...peerAudioChannelState,
        },
      })

      if (microphoneAudioChannel === AudioState.STOPPED) {
        deletePeerAudio(peerId)
      }
    },
  })

  peerRoom.onPeerStream(PeerStreamType.AUDIO, (stream, peerId, metadata) => {
    if (
      typeof metadata === 'object' &&
      metadata !== null &&
      'type' in metadata &&
      metadata['type'] !== StreamType.MICROPHONE
    ) {
      return
    }

    const audioTracks = stream.getAudioTracks()

    if (audioTracks.length === 0) return

    const audio = new Audio()

    audio.srcObject = stream
    audio.autoplay = true

    setPeerAudioChannels(peerAudioChannels => ({
      ...peerAudioChannels,
      [peerId]: {
        ...peerAudioChannels[peerId],
        [AudioChannelName.MICROPHONE]: audio,
      },
    }))
  })

  useEffect(() => {
    if (!audioStream) return
    peerRoom.addStream(audioStream, { metadata: { type: StreamType.MICROPHONE } })
    sendAudioChange({ [AudioChannelName.MICROPHONE]: AudioState.PLAYING })
    setAudioChannelState(state => ({ ...state, [AudioChannelName.MICROPHONE]: AudioState.PLAYING }))
    return () => {
      peerRoom.removeStream(audioStream, { target: peerRoom.getPeers() })
      sendAudioChange({ [AudioChannelName.MICROPHONE]: AudioState.STOPPED })
      setAudioChannelState(state => ({ ...state, [AudioChannelName.MICROPHONE]: AudioState.STOPPED }))
    }
  }, [audioStream, peerRoom, sendAudioChange, setAudioChannelState])
  const handleAudioDeviceSelect = (device: MediaDeviceInfo) => selectDevice(device.deviceId)

  const deletePeerAudio = (peerId: string) => {
    setPeerAudioChannels(({ ...newPeerAudios }) => {
      if (!newPeerAudios[peerId]) {
        return newPeerAudios
      }

      const microphoneAudio = newPeerAudios[peerId][AudioChannelName.MICROPHONE]

      microphoneAudio?.pause()

      const { [AudioChannelName.MICROPHONE]: _, ...newPeerAudioChannels } =
        newPeerAudios[peerId]

      newPeerAudios[peerId] = newPeerAudioChannels

      return newPeerAudios
    })
  }

  const handleAudioForNewPeer = (peerId: string) => {
    if (audioStream) {
      peerRoom.addStream(audioStream, {
        target: peerId,
        metadata: { type: StreamType.MICROPHONE },
      })
    }
  }

  const handleAudioForLeavingPeer = (peerId: string) => {
    if (audioStream) {
      peerRoom.removeStream(audioStream, { target: peerId })
    }

    deletePeerAudio(peerId)
  }

  peerRoom.onPeerJoin(PeerHookType.AUDIO, (peerId: string) => {
    handleAudioForNewPeer(peerId)
  })

  peerRoom.onPeerLeave(PeerHookType.AUDIO, (peerId: string) => {
    handleAudioForLeavingPeer(peerId)
  })

  return {
    audioDevices,
    isSpeakingToRoom,
    setIsSpeakingToRoom,
    handleAudioDeviceSelect,
  }
}
