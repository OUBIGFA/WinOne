import { useContext, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ShellContext } from 'contexts/ShellContext'
import { SettingsContext } from 'contexts/SettingsContext'
import { StorageContext } from 'contexts/StorageContext'
import { RoomContext } from 'contexts/RoomContext'
import { ChatRoom } from 'core/chat/ChatRoom'
import { ChatRoomEvent, ChatRoomErrorContext } from 'core/chat/events'
import { RoomNameGenerator, RoomNameType } from 'core/config/roomNames'
import { validNetworkSettings } from 'core/config/networkSettings'
import { encryption } from 'core/crypto/Encryption'
import { serialization, isSerializedUserSettings } from 'core/settings/serialization'
import { getInlineMediaFiles } from 'core/chat/fileOffers'
import { getPeerDisplayName } from 'core/names/peerNames'
import { TrysteroTransport } from 'core/transport/TrysteroTransport'
import { Room } from 'components/Room'
import { useRoomAudio } from 'components/Room/useRoomAudio'
import { useRoomVideo } from 'components/Room/useRoomVideo'
import { useRoomScreenShare } from 'components/Room/useRoomScreenShare'
import { soundOptions } from 'config/soundNames'
import { communityRoomNames } from 'config/communityRooms'
import { isEnhancedConnectivityAvailable } from 'config/enhancedConnectivity'
import { notification } from 'services/Notification'
import { NativeCommand, nativePost, pickedFiles, publish, register } from './channel'

// A portable invitation contains room coordinates, never a vendor website.
// Old links remain accepted so a UI update does not invalidate invitations.
export function roomAddress(value: string, privateRoom: boolean) {
  try {
    const url = new URL(value)
    if (!['winone-flash:', 'https:', 'http:'].includes(url.protocol)) throw new Error('Invalid invitation')
    const route = url.protocol === 'winone-flash:' ? '/' + url.host + url.pathname : url.pathname
    const match = /^\/(public|private)\/([^/]+)$/.exec(route)
    if (!match) throw new Error('Invalid invitation')
    return { roomId: decodeURIComponent(match[2]), privateRoom: match[1] === 'private', secret: new URLSearchParams(url.hash.slice(1)).get('secret') ?? '' }
  } catch (error) {
    if (value.includes('://') || value.startsWith('winone-flash:')) throw error
    if (!value.trim()) throw new Error('Enter a room name')
    return { roomId: value.trim(), privateRoom, secret: '' }
  }
}

export function NativeShellBridge({ userId, alert }: { userId: string; alert: string }) {
  const shell = useContext(ShellContext)
  const settings = useContext(SettingsContext)
  const storage = useContext(StorageContext)
  const navigate = useNavigate()
  const [, setDirect] = useState('')
  const volumes = useRef(new Map<string, number>())
  const prefs = settings.getUserSettings()
  useEffect(() => {
    const { privateKey: _private, publicKey: _public, ...publicSettings } = prefs
    publish({ ready: true, userId, settings: publicSettings, roomId: shell.roomId ?? '',
      secret: shell.password ?? '', peers: shell.peerList.map(peer => ({
        peerId: peer.peerId, userId: peer.userId, name: getPeerDisplayName(peer),
        verified: peer.verificationState, offeredFileId: peer.offeredFileId,
        typing: peer.isTypingGroupMessage, directTyping: peer.isTypingDirectMessage,
        connection: shell.peerConnectionTypes[peer.peerId] ?? '',
        audioChannels: peer.audioChannelState,
      })), connections: shell.connectionTestResults, sounds: soundOptions, community: communityRoomNames, enhancedAvailable: isEnhancedConnectivityAvailable, alert })
  }, [prefs, userId, shell.roomId, shell.password, shell.peerList, shell.peerConnectionTypes, shell.connectionTestResults, alert])
  useEffect(() => {
    for (const [peer, channels] of Object.entries(shell.peerAudioChannels)) {
      for (const [name, audio] of Object.entries(channels)) {
        const volume = volumes.current.get(peer + ':' + name) ?? 1
        audio.volume = volume; audio.muted = volume === 0
      }
    }
  }, [shell.peerAudioChannels])
  useEffect(() => register('shell', async command => {
    switch (command.action) {
      case 'generate': return RoomNameGenerator.generate(command.words ? RoomNameType.PASSPHRASE : RoomNameType.UUID)
      case 'join': {
        const address = roomAddress(String(command.room), Boolean(command.private))
        const secret = address.secret || (address.privateRoom ? await encryption.encodePassword(address.roomId, String(command.password ?? '')) : '')
        if (address.privateRoom && !address.secret && !command.password) throw new Error('Enter a password')
        await settings.updateUserSettings({ customUsername: String(command.nickname ?? '').trim() })
        setDirect('')
        navigate(`/${address.privateRoom ? 'private' : 'public'}/${encodeURIComponent(address.roomId)}${secret ? '#secret=' + encodeURIComponent(secret) : ''}`)
        return
      }
      case 'leave': setDirect(''); navigate('/'); publish({ rooms: {}, previews: {}, media: { streams: [] } }); return
      case 'select':
        if (window.__winoneNative) window.__winoneNative.selected = String(command.peer ?? '')
        setDirect(String(command.peer ?? '')); window.dispatchEvent(new Event('native-presentation')); return
      case 'settings': {
        const update = command.value as Parameters<typeof settings.updateUserSettings>[0]
        if (!validNetworkSettings(update)) throw new Error('Invalid network settings')
        if (update.showNotificationOnNewMessage) await notification.requestPermission()
        await settings.updateUserSettings(update); return
      }
      case 'export': return serialization.serializeUserSettings(settings.getUserSettings())
      case 'import': {
        if (!isSerializedUserSettings(command.value)) throw new Error('Invalid profile')
        setDirect(''); navigate('/')
        await settings.updateUserSettings(await serialization.deserializeUserSettings(command.value)); return
      }
      case 'reset': await storage.getPersistedStorage().clear(); window.location.replace('/'); return
      case 'identity': {
        const peer = shell.peerList.find(peer => peer.peerId === command.peer)
        return encryption.stringifyCryptoKey(peer ? peer.publicKey : prefs.publicKey)
      }
      case 'sound': {
        const sound = soundOptions.find(sound => sound.value === command.value)
        if (!sound) throw new Error('Unknown sound')
        await new Audio(sound.value).play(); return
      }
      case 'volume': {
        const volume = Math.max(0, Math.min(1, Number(command.value)))
        for (const [name, audio] of Object.entries(shell.peerAudioChannels[String(command.peer)] ?? {})) {
          if (!command.channel || name === command.channel) {
            volumes.current.set(String(command.peer) + ':' + name, volume)
            audio.volume = volume; audio.muted = volume === 0
          }
        }
        return
      }
      default: throw new Error('Unknown native action')
    }
  }))
  // Every peer's direct namespace must listen before the user opens its UI.
  // Otherwise the first private message would be lost in the group view.
  return shell.roomId ? <>{shell.peerList.map(peer => <Room key={peer.peerId} userId={userId} roomId={shell.roomId!} targetPeerId={peer.peerId} />)}</> : null
}

const roomSnapshots: Record<string, unknown> = {}
const previewSnapshots: Record<string, { id: string; name: string; kind: string; duration?: number; current?: number; playing?: boolean }[]> = {}
export function NativeRoomBridge({ room, transport }: { room: ChatRoom; transport: TrysteroTransport }) {
  const context = useContext(RoomContext)
  const key = room.targetPeerId ?? ''
  const previews = useRef(previewSnapshots)
  const players = useRef(new Map<string, HTMLMediaElement>())
  const cleanups = useRef<(() => void)[]>([])
  const active = useRef(true)
  const loading = useRef(new Map<string, Promise<void>>())
  useEffect(() => {
    active.current = true
    return () => {
      active.current = false
      cleanups.current.forEach(stop => stop()); players.current.clear()
      publish({ previews: { ...previews.current } })
    }
  }, [])
  useEffect(() => {
    const update = () => {
      roomSnapshots[key] = { messages: room.getMessageLog(), sending: room.getIsMessageSending(), offer: room.getSelfFileOfferMagnetUri() ?? '', unread: context.unreadMessages }
      publish({ rooms: { ...roomSnapshots } })
    }
    update()
    const off = Object.values(ChatRoomEvent).map(event => room.on(event, update))
    return () => { off.forEach(stop => stop()); delete roomSnapshots[key]; publish({ rooms: { ...roomSnapshots } }) }
  }, [room, key, context.unreadMessages])
  useEffect(() => register('room:' + key, async command => {
    switch (command.action) {
      case 'send': {
        if (room.getIsMessageSending()) throw new Error('A message is still being sent')
        // ChatRoom reports send failures as events. Propagate that outcome so
        // the native composer keeps the draft available for retry.
        let failure = false
        const off = room.on(ChatRoomEvent.ERROR, ({ context }) => { if (context === ChatRoomErrorContext.SEND_MESSAGE) failure = true })
        try { await room.sendMessage(String(command.text)) } finally { off() }
        if (failure) throw new Error('Your message could not be sent')
        return
      }
      case 'typing': room.notifyTyping(); return
      case 'share': {
        const files = await pickedFiles(command.files as Parameters<typeof pickedFiles>[0])
        const offered = await room.offerFiles(files)
        if (!offered) throw new Error('The file could not be shared')
        const inline = getInlineMediaFiles(files)
        if (inline.length) await room.sendInlineMedia(inline)
        return
      }
      case 'rescind': await room.rescindFileOffer(); return
      case 'download': {
        publish({ transfer: { busy: true, progress: 0 } })
        try {
          await context.fileTransferService.fileTransfer.download(String(command.magnet), room.roomId, {
            doSave: true, onProgress: (progress: number) => publish({ transfer: { busy: true, progress } }),
          })
        } finally { publish({ transfer: { busy: false, progress: 1 } }) }
        return
      }
      case 'preview': {
        const id = String(command.id)
        if (previews.current[id]) { publish({ previews: { ...previews.current } }); return }
        if (loading.current.has(id)) return loading.current.get(id)
        const work = (async () => {
        const release: (() => void)[] = []
        try {
        const files = await context.fileTransferService.fileTransfer.download(String(command.magnet), room.roomId)
        if (!active.current) return
        const items: typeof previews.current[string] = []
        for (const [index, file] of files.entries()) {
          const frameId = 'preview:' + id + ':' + index
          const extension = file.name.split('.').pop()?.toLowerCase() ?? ''
          const kind = /^(png|jpe?g|gif|bmp|webp)$/.test(extension) ? 'image' : /^(mp4|webm|mov|m4v)$/.test(extension) ? 'video' : /^(aac|mp3|oga|ogg|wav|flac|m4a)$/.test(extension) ? 'audio' : 'file'
          const item = { id: frameId, name: file.name, kind, duration: 0, current: 0, playing: false }
          items.push(item)
          if (kind === 'file' || file.length > 200_000_000) { item.kind = 'file'; continue }
          const url = await new Promise<string>((resolve, reject) => file.getBlobURL((error, url) => error || !url ? reject(error ?? new Error('Media unavailable')) : resolve(url)))
          release.push(() => URL.revokeObjectURL(url))
          if (!active.current) { release.forEach(stop => stop()); return }
          if (kind === 'image') {
            // HTMLImageElement.decode can wait for the hidden document's next
            // rendering update. Decode offscreen so native previews don't
            // depend on a visible browser frame.
            const image = await createImageBitmap(await (await fetch(url)).blob())
            const canvas = document.createElement('canvas'); const scale = Math.min(1, 2048 / Math.max(image.width, image.height))
            canvas.width = Math.round(image.width * scale); canvas.height = Math.round(image.height * scale)
            const drawing = canvas.getContext('2d')!; drawing.fillStyle = '#fff'; drawing.fillRect(0, 0, canvas.width, canvas.height); drawing.drawImage(image, 0, 0, canvas.width, canvas.height)
            image.close()
            const blob = await new Promise<Blob | null>(resolve => canvas.toBlob(resolve, 'image/jpeg', .92))
            if (!active.current) { release.forEach(stop => stop()); return }
            // Publish the item first, so native state reconciliation retains
            // its incoming frame while later media finish decoding.
            previews.current[id] = items
            publish({ previews: { ...previews.current } })
            if (blob) await nativePost('frame/' + encodeURIComponent(frameId), blob, 'image/jpeg')
          } else {
            const player = document.createElement(kind === 'video' ? 'video' : 'audio')
            player.preload = 'auto'; players.current.set(frameId, player)
            let stopFrames = () => {}
            release.push(() => { stopFrames(); players.current.delete(frameId); player.pause(); player.removeAttribute('src'); player.load() })
            const update = () => { item.duration = Number.isFinite(player.duration) ? player.duration : 0; item.current = player.currentTime; item.playing = !player.paused; publish({ previews: { ...previews.current } }) }
            for (const event of ['loadedmetadata', 'timeupdate', 'play', 'pause', 'ended']) player.addEventListener(event, update)
            // Browsers may defer preload in a hidden document. Publish the
            // Play control immediately; play() loads metadata when requested.
            player.addEventListener('error', () => publish({ alert: player.error?.message || 'Media could not be decoded' }))
            player.src = url; player.load()
            if (kind === 'video') stopFrames = captureFrames(player as HTMLVideoElement, frameId)
          }
        }
        previews.current[id] = items
        cleanups.current.push(() => { release.forEach(stop => stop()); delete previews.current[id] })
        publish({ previews: { ...previews.current } })
        } catch (error) {
          release.forEach(stop => stop()); delete previews.current[id]
          publish({ previews: { ...previews.current } }); throw error
        }
        })()
        loading.current.set(id, work)
        try { await work } finally { loading.current.delete(id) }
        return
      }
      case 'playback': {
        const player = players.current.get(String(command.id))
        if (!player) throw new Error('Media unavailable')
        if (command.position !== undefined) player.currentTime = Number(command.position)
        if (command.volume !== undefined) player.volume = Math.min(1, Math.max(0, Number(command.volume)))
        if (command.enabled === true) await player.play(); else if (command.enabled === false) player.pause()
        return
      }
      default: throw new Error('Unknown room action')
    }
  }))
  return room.isDirectMessageRoom ? null : <NativeMediaBridge transport={transport} />
}

// Capture only media pixels. Controls, labels, layout and interaction are drawn
// by MyGo; no HTML or screenshots of web UI enter the native presentation.
function captureFrames(video: HTMLVideoElement, id: string) {
    const canvas = document.createElement('canvas')
    const context = canvas.getContext('2d')!
    let stopped = false
    let lastTime = -1
    let timer: ReturnType<typeof setTimeout>
    const frame = async () => {
      if (stopped) return
      try {
        if (window.__winoneNative?.visible !== false && video.videoWidth && video.videoHeight && video.currentTime !== lastTime) {
          lastTime = video.currentTime
          const scale = Math.min(1, 1280 / video.videoWidth)
          canvas.width = Math.round(video.videoWidth * scale); canvas.height = Math.round(video.videoHeight * scale)
          context.drawImage(video, 0, 0, canvas.width, canvas.height)
          const blob = await new Promise<Blob | null>(resolve => canvas.toBlob(resolve, 'image/jpeg', 0.85))
          if (blob && !stopped) await nativePost('frame/' + encodeURIComponent(id), blob, 'image/jpeg')
        }
      } catch (error) { if (!stopped) publish({ alert: String(error) }) }
      if (!stopped) timer = setTimeout(frame, window.__winoneNative?.visible === false ? 250 : 50)
    }
    void frame()
    return () => { stopped = true; clearTimeout(timer) }
}

function NativeFrames({ id, stream }: { id: string; stream: MediaStream }) {
  useEffect(() => {
    const video = document.createElement('video')
    video.srcObject = stream; video.muted = true; video.autoplay = true
    const stop = captureFrames(video, id)
    void video.play().catch(error => publish({ alert: String(error) }))
    return () => { stop(); video.pause(); video.srcObject = null }
  }, [id, stream])
  return null
}

function NativeMediaBridge({ transport }: { transport: TrysteroTransport }) {
  const context = useContext(RoomContext)
  const audio = useRoomAudio({ peerRoom: transport })
  const video = useRoomVideo({ peerRoom: transport })
  const screen = useRoomScreenShare({ peerRoom: transport })
  const streams = [
    ...(context.selfVideoStream ? [{ id: 'self-camera', peer: '', kind: 'camera', stream: context.selfVideoStream }] : []),
    ...(context.selfScreenStream ? [{ id: 'self-screen', peer: '', kind: 'screen', stream: context.selfScreenStream }] : []),
    ...Object.entries(context.peerVideoStreams).map(([peer, stream]) => ({ id: peer + '-camera', peer, kind: 'camera', stream })),
    ...Object.entries(context.peerScreenStreams).map(([peer, stream]) => ({ id: peer + '-screen', peer, kind: 'screen', stream })),
  ]
  useEffect(() => { publish({ media: {
    microphone: audio.isSpeakingToRoom, camera: video.isCameraEnabled, screen: screen.isSharingScreen,
    audioDevices: audio.audioDevices.map(d => ({ id: d.deviceId, label: d.label })),
    videoDevices: video.videoDevices.map(d => ({ id: d.deviceId, label: d.label })),
    streams: streams.map(({ stream: _stream, ...details }) => details),
  } }) }, [audio.isSpeakingToRoom, video.isCameraEnabled, screen.isSharingScreen, audio.audioDevices, video.videoDevices, context.selfVideoStream, context.selfScreenStream, context.peerVideoStreams, context.peerScreenStreams])
  useEffect(() => register('media', async (command: NativeCommand) => {
    switch (command.action) {
      case 'microphone': audio.setIsSpeakingToRoom(Boolean(command.enabled)); return
      case 'camera': video.setIsCameraEnabled(Boolean(command.enabled)); return
      case 'screen': if (command.enabled) await screen.handleScreenShareStart(); else screen.handleScreenShareStop(); return
      case 'audioDevice': {
        const device = audio.audioDevices.find(d => d.deviceId === command.id)
        if (device) await audio.handleAudioDeviceSelect(device); return
      }
      case 'videoDevice': {
        const device = video.videoDevices.find(d => d.deviceId === command.id)
        if (device) await video.handleVideoDeviceSelect(device); return
      }
      default: throw new Error('Unknown media action')
    }
  }))
  return <>{streams.map(item => <NativeFrames key={item.id} id={item.id} stream={item.stream} />)}</>
}
