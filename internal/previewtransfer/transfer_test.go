package previewtransfer

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLargePreviewRoundTripAndReplacement(t *testing.T) {
	store := NewStore(time.Minute)
	data := bytes.Repeat([]byte("large-preview"), 200000)
	descriptor, err := store.Own(data)
	if err != nil {
		t.Fatal(err)
	}
	output, err := Receive(context.Background(), descriptor, func(offset int) (Chunk, error) { return store.Read(descriptor.Token, offset) })
	if err != nil || !bytes.Equal(output, data) {
		t.Fatal("large preview changed", err)
	}
	if _, err = store.Read(descriptor.Token, -1); err == nil {
		t.Fatal("negative offset accepted")
	}
	newer, err := store.Own([]byte("replacement"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Read(descriptor.Token, 0); err == nil {
		t.Fatal("old transfer survived replacement")
	}
	store.Release(newer.Token)
	if _, err = store.Read(newer.Token, 0); err == nil {
		t.Fatal("released transfer retained")
	}
}

func TestReceiveRejectsMixedAndTruncatedData(t *testing.T) {
	store := NewStore(time.Minute)
	descriptor, err := store.Own([]byte("preview"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Release(descriptor.Token)
	_, err = Receive(context.Background(), descriptor, func(offset int) (Chunk, error) {
		chunk, err := store.Read(descriptor.Token, offset)
		chunk.Token = "another"
		return chunk, err
	})
	if err == nil {
		t.Fatal("mixed transfer accepted")
	}
	_, err = Receive(context.Background(), descriptor, func(offset int) (Chunk, error) { return Chunk{Token: descriptor.Token, Offset: offset}, nil })
	if err == nil {
		t.Fatal("empty chunk accepted")
	}
	descriptor.SHA256 = strings.Repeat("0", 64)
	_, err = Receive(context.Background(), descriptor, func(offset int) (Chunk, error) { return store.Read(descriptor.Token, offset) })
	if err == nil {
		t.Fatal("invalid digest accepted")
	}
}

func TestReceiveCancellationDuringLastRead(t *testing.T) {
	store := NewStore(time.Minute)
	descriptor, err := store.Own([]byte("preview"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Release(descriptor.Token)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	data, err := Receive(ctx, descriptor, func(offset int) (Chunk, error) {
		cancel()
		return store.Read(descriptor.Token, offset)
	})
	if !errors.Is(err, context.Canceled) || data != nil {
		t.Fatalf("canceled read returned data=%q, error=%v", data, err)
	}
}

func TestExpiredPreviewCannotBeRead(t *testing.T) {
	store := NewStore(time.Millisecond)
	descriptor, err := store.Own([]byte("preview"))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, err = store.Read(descriptor.Token, 0); err == nil {
		t.Fatal("expired preview accepted")
	}
}
