// Package protocol implements bounded messages for local WinOne connections.
package protocol

import (
	"encoding/binary"
	"errors"
	"io"
)

const MaxFrameSize = 1024 * 1024

var ErrFrameSize = errors.New("message size must be between 1 byte and 1 MiB")

// ReadFrame checks the prefix before allocating untrusted message storage.
func ReadFrame(r io.Reader) ([]byte, error) {
	var length uint32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return nil, err
	}
	if length == 0 || length > MaxFrameSize {
		return nil, ErrFrameSize
	}
	data := make([]byte, length)
	_, err := io.ReadFull(r, data)
	return data, err
}

// WriteFrame treats a partial write as failure so peers never accept a partial message.
func WriteFrame(w io.Writer, data []byte) error {
	if len(data) == 0 || len(data) > MaxFrameSize {
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
