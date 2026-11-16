package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

// These tests catch truncated/oversized input and short writes on real byte streams.
func TestFrameRoundTrip(t *testing.T) {
	var stream bytes.Buffer
	want := []byte(`{"name":"窗口"}`)
	if err := WriteFrame(&stream, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFrame(&stream)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("got %s, %v", got, err)
	}
}

func TestRejectOversizeBeforeReadingPayload(t *testing.T) {
	var stream bytes.Buffer
	_ = binary.Write(&stream, binary.LittleEndian, uint32(MaxFrameSize+1))
	if _, err := ReadFrame(&stream); !errors.Is(err, ErrFrameSize) {
		t.Fatalf("got %v", err)
	}
}

func TestConfigurationSnapshotPairFitsFrame(t *testing.T) {
	// Two near-limit AI documents plus their request wrapper must survive the
	// actual framing path; the stores still reject oversized configurations.
	want := bytes.Repeat([]byte("x"), 2*1024*1024+4096)
	var stream bytes.Buffer
	if err := WriteFrameLimit(&stream, want, MaxHostFrameSize); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFrameLimit(&stream, MaxHostFrameSize)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("configuration edit was truncated", err)
	}
	stream.Reset()
	_ = binary.Write(&stream, binary.LittleEndian, uint32(MaxHostFrameSize+1))
	if _, err = ReadFrameLimit(&stream, MaxHostFrameSize); !errors.Is(err, ErrFrameSize) {
		t.Fatal("oversized Host frame accepted", err)
	}
}

func TestRejectTruncatedPayload(t *testing.T) {
	stream := bytes.NewReader([]byte{4, 0, 0, 0, 1})
	if _, err := ReadFrame(stream); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("got %v", err)
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return 0, nil }

func TestRejectShortWrite(t *testing.T) {
	if err := WriteFrame(shortWriter{}, []byte("x")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("got %v", err)
	}
}
