// Package protocol implements bounded messages for local WinOne connections.
package protocol

import (
	"encoding/binary"
	"errors"
	"io"
)

const MaxFrameSize = 1024 * 1024

// A Host settings edit carries both the original and edited configuration
// (each bounded by its store), plus credentials. Legacy worker messages and
// chunked-preview thresholds retain their existing 1 MiB limit.
const MaxHostFrameSize = 4 * MaxFrameSize

var ErrFrameSize = errors.New("消息大小超出允许范围")

// ReadFrame checks the prefix before allocating untrusted message storage.
func ReadFrame(r io.Reader) ([]byte, error) { return ReadFrameLimit(r, MaxFrameSize) }

// ReadFrameLimit applies the receiving endpoint's explicit message budget.
func ReadFrameLimit(r io.Reader, limit uint32) ([]byte, error) {
	var length uint32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return nil, err
	}
	if length == 0 || length > limit {
		return nil, ErrFrameSize
	}
	data := make([]byte, length)
	_, err := io.ReadFull(r, data)
	return data, err
}

// WriteFrame treats a partial write as failure so peers never accept a partial message.
func WriteFrame(w io.Writer, data []byte) error { return WriteFrameLimit(w, data, MaxFrameSize) }

// WriteFrameLimit uses the same endpoint budget as its matching reader.
func WriteFrameLimit(w io.Writer, data []byte, limit int) error {
	if len(data) == 0 || len(data) > limit {
		return ErrFrameSize
	}
	frame := make([]byte, 4+len(data))
	binary.LittleEndian.PutUint32(frame, uint32(len(data)))
	copy(frame[4:], data)
	n, err := w.Write(frame)
	if err != nil {
		return err
	}
	if n != len(frame) {
		return io.ErrShortWrite
	}
	return nil
}
