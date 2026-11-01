// Package flashchat is the flash chat's room protocol: a port of the
// Chitchatter core WinOne's React window vendored
// (apps/desktop/frontend/src/vendor/chitchatter/core, Chitchatter commit
// 4093c21, GPL-2.0-or-later, by Jeremy Kahn) to Go, over package trystero,
// so that a WinOne window and a
// browser running Chitchatter meet in one room, see each other's names and
// verification, and exchange group and direct messages, typing, history
// and file offers.
//
// The wire shapes, action names and behaviors follow the TypeScript source
// file by file (models, peers, identity, transcript, fileOffers, ChatRoom,
// ChatRoomRegistry); comments name the upstream rule where Go needed a
// choice of its own. Audio, video and screen sharing are absent, as they
// are from the core: a peer's media actions are tolerated and ignored.
package flashchat

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

// Message is a transcript entry: a text message or inline media (a file
// offer posted into the transcript). Received entries carry the time they
// arrived, and only they are handed to newcomers as history.
type Message struct {
	ID       string
	AuthorID string
	TimeSent float64
	Text     string
	// Media marks inline media, whose MagnetURI is the offer to fetch.
	Media     bool
	MagnetURI string
	// Received is set with TimeReceived once the entry was delivered (own
	// messages) or received (others').
	Received     bool
	TimeReceived float64
}

// MarshalJSON writes the entry as models/chat.ts shapes it: a message has
// text, inline media a magnetURI, and a received entry its timeReceived.
func (m Message) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	field := func(name string, v any) {
		if b.Len() > 1 {
			b.WriteByte(',')
		}
		data, _ := marshalJSON(v)
		b.WriteString(`"` + name + `":`)
		b.Write(data)
	}
	b.WriteByte('{')
	field("authorId", m.AuthorID)
	if m.Media {
		field("magnetURI", m.MagnetURI)
	} else {
		field("text", m.Text)
	}
	field("timeSent", m.TimeSent)
	field("id", m.ID)
	if m.Received {
		field("timeReceived", m.TimeReceived)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

var errMessage = errors.New("invalid message")

// UnmarshalJSON reads an entry a peer sent. TypeScript trusted the shape;
// Go refuses one whose fields it could not show.
func (m *Message) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil || raw == nil {
		return errMessage
	}
	str := func(name string, into *string) bool { return json.Unmarshal(raw[name], into) == nil }
	num := func(name string, into *float64) bool { return json.Unmarshal(raw[name], into) == nil }
	var out Message
	if !str("id", &out.ID) || !str("authorId", &out.AuthorID) || !num("timeSent", &out.TimeSent) {
		return errMessage
	}
	if _, ok := raw["magnetURI"]; ok {
		out.Media = true
		if !str("magnetURI", &out.MagnetURI) {
			return errMessage
		}
	} else if !str("text", &out.Text) {
		return errMessage
	}
	if _, ok := raw["timeReceived"]; ok {
		out.Received = num("timeReceived", &out.TimeReceived)
	}
	*m = out
	return nil
}

// marshalJSON encodes as JSON.stringify: no HTML escaping.
func marshalJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// VerificationState is PeerVerificationState, numbered as upstream.
type VerificationState int

const (
	Verifying VerificationState = iota
	Unverified
	Verified
)

// Peer is a room member known by its signed metadata.
type Peer struct {
	PeerID         string
	UserID         string
	CustomUsername string
	PublicKey      string // the SPKI it advertised, base64
	Verification   VerificationState
	OfferedFileID  string
	// Typing in the group room, or in the direct conversation with us.
	TypingGroup, TypingDirect bool
}

// FileOfferMetadata is what a peer offers: its share address, and whether
// every file is inline media (image, audio, video).
type FileOfferMetadata struct {
	MagnetURI        string `json:"magnetURI"`
	IsAllInlineMedia bool   `json:"isAllInlineMedia"`
}

// UserMetadata is PEER_METADATA: who a peer is, with the proof.
type UserMetadata struct {
	UserID                  string `json:"userId"`
	CustomUsername          string `json:"customUsername"`
	PublicKeyString         string `json:"publicKeyString"`
	IdentitySignatureBase64 string `json:"identitySignatureBase64"`
}

// TypingStatus is TYPING_STATUS_CHANGE's payload.
type TypingStatus struct {
	IsTyping bool `json:"isTyping"`
}

// PeerAction numbers the room's actions; the wire name is
// "<namespace>.<number>" (models/network.ts).
type PeerAction int

const (
	ActionMessage PeerAction = iota
	ActionMediaMessage
	ActionMessageTranscript
	ActionPeerMetadata
	ActionAudioChange
	ActionVideoChange
	ActionScreenShare
	ActionFileOffer
	ActionTypingStatusChange
)

// Namespaces let one transport carry the group room and direct messages.
const (
	NamespaceGroup  = "g"
	NamespaceDirect = "dm"
)

// Messaging limits (config/messaging.ts).
const (
	MessageCharacterSizeLimit  = 10_000
	MessageTranscriptSizeLimit = 150
	// TypingStatusExpiry is how long after the last keystroke one still
	// counts as typing, in milliseconds.
	TypingStatusExpiry = 2_000
)

// FileHandle is a file offered to the room; the share runs from Path.
type FileHandle struct {
	Name string  `json:"name"`
	Type string  `json:"type"`
	Size float64 `json:"size"`
	Path string  `json:"path,omitempty"`
}

// IsInlineMediaFile reports image, audio and video files.
func IsInlineMediaFile(f FileHandle) bool {
	kind, _, _ := strings.Cut(f.Type, "/")
	return kind == "image" || kind == "audio" || kind == "video"
}

// IsEveryFileInlineMedia is true for a non-empty list of inline media.
func IsEveryFileInlineMedia(files []FileHandle) bool {
	if len(files) == 0 {
		return false
	}
	for _, f := range files {
		if !IsInlineMediaFile(f) {
			return false
		}
	}
	return true
}

// shouldRescindOffer: an offer withdrawn or left behind stops seeding,
// unless it is all inline media, still shown in the transcript.
func shouldRescindOffer(offer *FileOfferMetadata, isOffering func(string) bool) bool {
	return offer != nil && !offer.IsAllInlineMedia && isOffering(offer.MagnetURI)
}

// FileTransfer shares and stops sharing files for a room. Offer returns
// the address peers fetch the files by.
type FileTransfer interface {
	Offer(files []FileHandle, roomID string) (string, error)
	Rescind(magnetURI string) error
	RescindAll() error
	IsOffering(magnetURI string) bool
}

// applyTranscriptLimit keeps the newest limit entries and reports the
// rest, oldest first.
func applyTranscriptLimit(messages []Message, limit int) (kept, evicted []Message) {
	if len(messages) <= limit {
		return append([]Message(nil), messages...), nil
	}
	n := len(messages) - limit
	return append([]Message(nil), messages[n:]...), append([]Message(nil), messages[:n]...)
}
