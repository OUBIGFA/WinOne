/**
 * Chitchatter's application core.
 *
 * Framework- and DOM-agnostic: everything here runs in a browser, in Node and
 * in a terminal. See ./README.md for the boundary rule.
 *
 * Transports are deliberately NOT re-exported. `TrysteroTransport` owns the
 * WebRTC/Trystero stack, so importing this barrel should not drag that in, and
 * `InMemoryTransport` is an in-process test double that has no business in a
 * host-facing surface. Import either explicitly from
 * 'core/transport/TrysteroTransport' or 'core/transport/InMemoryTransport'.
 */

export * from './adapters/noop'
export * from './adapters/types'
export * from './chat/ChatRoom'
export * from './chat/ChatRoomRegistry'
export * from './chat/events'
export * from './chat/fileOffers'
export * from './chat/identity'
export * from './chat/peers'
export * from './chat/protocol'
export * from './chat/transcript'
export * from './config/messaging'
export * from './config/roomNames'
export * from './config/turnConfig'
export * from './crypto/Encryption'
export * from './lib/Time'
export * from './lib/sleep'
export * from './lib/type-guards'
export * from './models/chat'
export * from './models/network'
export * from './models/settings'
export * from './names/peerNames'
export * from './settings/serialization'
export * from './settings/SettingsManager'
export * from './transport/types'
