package trystero

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
)

// Trystero 0.26.0's data-channel framing (action-wire.ts, shared-peer.ts,
// data-limits.ts). Every byte here is a compatibility contract with the
// browser peers.

const (
	wireVersion = 2

	kindInline    = 0
	kindOffer     = 1
	kindChunk     = 2
	kindAccepted  = 3
	kindRejected  = 4
	kindCancelled = 5

	// Payload formats, in the high nibble of the kind byte.
	FormatString = 0
	FormatJSON   = 1
	FormatBinary = 2

	inlineHeaderSize  = 36
	offerHeaderSize   = 48
	controlHeaderSize = 6
	chunkHeaderSize   = 14

	maxActionFrameBytes = 16384
	maxRoomFrameBytes   = 16515
	chunkSize           = maxActionFrameBytes - chunkHeaderSize
	maxTypeBytes        = 32
	maxMetadataBytes    = 16336

	defaultMaxReceiveBytes = 256 << 20
	maxRoomTokenBytes      = 128

	roomFrameVersion         = 1
	roomPresenceFrameVersion = 2
)

// marshalJSON encodes as JSON.stringify does: no HTML escaping, so a
// payload has the bytes a browser peer would send for the same value.
func marshalJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// encodeStart builds the first frame of an action: the whole message when
// it fits one frame (inline), else the offer of a chunked transfer.
func encodeStart(typ string, format byte, payload, meta []byte, id uint32) (frame []byte, inline bool, err error) {
	if typ == "" || len(typ) > maxTypeBytes || bytes.IndexByte([]byte(typ), 0) >= 0 {
		return nil, false, errors.New("action type must contain 1–32 UTF-8 bytes and no null characters")
	}
	if len(meta) > maxMetadataBytes {
		return nil, false, errors.New("action metadata is too large")
	}
	inline = inlineHeaderSize+len(meta)+len(payload) <= maxActionFrameBytes
	header := offerHeaderSize
	kind := byte(kindOffer)
	size := len(meta)
	if inline {
		header, kind, size = inlineHeaderSize, kindInline, len(meta)+len(payload)
	}
	frame = make([]byte, header+size)
	frame[0] = wireVersion
	frame[1] = kind | format<<4
	copy(frame[2:34], typ)
	binary.BigEndian.PutUint16(frame[34:], uint16(len(meta)))
	if !inline {
		binary.BigEndian.PutUint32(frame[36:], id)
		binary.BigEndian.PutUint64(frame[40:], math.Float64bits(float64(len(payload))))
	}
	copy(frame[header:], meta)
	if inline {
		copy(frame[header+len(meta):], payload)
	}
	return frame, inline, nil
}

func controlFrame(kind byte, id uint32, reason string) []byte {
	text := []byte(reason)
	if len(text) > 200 {
		text = text[:200]
	}
	frame := make([]byte, controlHeaderSize+len(text))
	frame[0], frame[1] = wireVersion, kind
	binary.BigEndian.PutUint32(frame[2:], id)
	copy(frame[controlHeaderSize:], text)
	return frame
}

func chunkFrame(id uint32, offset int, data []byte) []byte {
	frame := make([]byte, chunkHeaderSize+len(data))
	frame[0], frame[1] = wireVersion, kindChunk
	binary.BigEndian.PutUint32(frame[2:], id)
	binary.BigEndian.PutUint64(frame[6:], math.Float64bits(float64(offset)))
	copy(frame[chunkHeaderSize:], data)
	return frame
}

// actionFrame is a decoded frame of the action wire.
type actionFrame struct {
	kind, format byte
	typ          string
	meta         json.RawMessage
	payload      []byte // inline payload or chunk data
	id           uint32
	size         int     // offered size of a chunked transfer
	offset       float64 // a chunk's offset
	reason       string  // a rejection's reason
}

var errFrame = errors.New("invalid action frame")

// decodeAction validates a frame as handleData does; an error means the
// peer broke the protocol and is disconnected.
func decodeAction(data []byte) (actionFrame, error) {
	var f actionFrame
	if len(data) < 2 || len(data) > maxActionFrameBytes || data[0] != wireVersion {
		return f, errFrame
	}
	f.kind, f.format = data[1]&15, data[1]>>4
	if f.kind > kindCancelled || f.format > FormatBinary || (f.kind > kindOffer && f.format != 0) {
		return f, errFrame
	}
	if f.kind >= kindChunk {
		if len(data) < controlHeaderSize || ((f.kind == kindAccepted || f.kind == kindCancelled) && len(data) != controlHeaderSize) ||
			(f.kind == kindRejected && len(data) > controlHeaderSize+200) {
			return f, errFrame
		}
		f.id = binary.BigEndian.Uint32(data[2:])
		switch f.kind {
		case kindRejected:
			f.reason = string(data[controlHeaderSize:])
		case kindChunk:
			f.offset = math.NaN()
			if len(data) >= chunkHeaderSize {
				f.offset = math.Float64frombits(binary.BigEndian.Uint64(data[6:]))
				f.payload = data[chunkHeaderSize:]
			}
		}
		return f, nil
	}
	header := inlineHeaderSize
	if f.kind == kindOffer {
		header = offerHeaderSize
	}
	if len(data) < header {
		return f, errFrame
	}
	f.typ = string(bytes.ReplaceAll(data[2:34], []byte{0}, nil))
	metaLength := int(binary.BigEndian.Uint16(data[34:]))
	payloadIndex := header + metaLength
	if f.typ == "" || payloadIndex > len(data) {
		return f, errFrame
	}
	if f.kind == kindInline {
		f.size = len(data) - payloadIndex
		f.payload = data[payloadIndex:]
	} else {
		size := math.Float64frombits(binary.BigEndian.Uint64(data[40:]))
		if size <= 0 || size != math.Trunc(size) || size > 1<<53 || payloadIndex != len(data) {
			return f, errFrame
		}
		f.size = int(size)
		f.id = binary.BigEndian.Uint32(data[36:])
	}
	if metaLength > 0 {
		f.meta = json.RawMessage(data[header:payloadIndex])
		if !json.Valid(f.meta) {
			return f, errFrame
		}
	}
	return f, nil
}

// wrapRoomFrame tags an action frame with its room's token.
func wrapRoomFrame(token string, data []byte) []byte {
	frame := make([]byte, 3+len(token)+len(data))
	frame[0] = roomFrameVersion
	binary.BigEndian.PutUint16(frame[1:], uint16(len(token)))
	copy(frame[3:], token)
	copy(frame[3+len(token):], data)
	return frame
}

// presenceFrame tells a peer that this side's room is (no longer) there.
func presenceFrame(token string, present bool) []byte {
	frame := make([]byte, 4+len(token))
	frame[0] = roomPresenceFrameVersion
	if present {
		frame[1] = 1
	}
	binary.BigEndian.PutUint16(frame[2:], uint16(len(token)))
	copy(frame[4:], token)
	return frame
}

// roomFrame is a decoded frame of the shared peer connection.
type roomFrame struct {
	presence bool // a presence frame, else a room's data
	present  bool
	token    string
	payload  []byte
}

func unwrapRoomFrame(data []byte) (roomFrame, bool) {
	var f roomFrame
	if len(data) < 3 || len(data) > maxRoomFrameBytes {
		return f, false
	}
	offset := 1
	switch data[0] {
	case roomFrameVersion:
	case roomPresenceFrameVersion:
		f.presence, f.present, offset = true, data[1] == 1, 2
	default:
		return f, false
	}
	if len(data) < offset+2 {
		return f, false
	}
	size := int(binary.BigEndian.Uint16(data[offset:]))
	end := offset + 2 + size
	if size <= 0 || size > maxRoomTokenBytes || len(data) < end {
		return f, false
	}
	f.token = string(data[offset+2 : end])
	if !f.presence {
		f.payload = data[end:]
	}
	return f, true
}
