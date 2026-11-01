// Package trystero speaks the wire protocol of Trystero 0.26.0
// (@trystero-p2p/core and @trystero-p2p/torrent, MIT, Copyright (c) 2021
// Dan Motzenbecker) in Go, on pion WebRTC, so that a native peer joins the
// same rooms as browsers running Trystero.
//
// It ports what the torrent strategy uses: room topics and info hashes,
// offers and answers encrypted with the room's password and exchanged
// through WebTorrent trackers without trickle ICE, the password
// handshake, the room frames of a shared peer connection, and the action
// wire with its chunked transfers. Media is not sent; a peer's media
// signalling is received and ignored. Timings and limits keep upstream's
// values, as peers pace each other with them.
//
// A Room keeps its state on one goroutine, as Trystero keeps it on the
// browser's event loop, and calls the app's handlers in order on another.
package trystero
