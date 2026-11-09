package learning

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestLearningLargeRPCPreservesPayloadAndSubmitsOnce(t *testing.T) {
	store := &RPCTransfers{}
	defer store.Close()
	owner := uint32(7)
	submissions := 0
	input := map[string]any{"id": "result", "method": "media.store", "params": map[string]any{"data": bytes.Repeat([]byte("media"), 400000)}}
	wanted, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	call := func(method string, params any) (json.RawMessage, error) {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		if len(data) > 1<<20 {
			t.Fatalf("chunk exceeds pipe bound: %d", len(data))
		}
		var part RPCPart
		_ = json.Unmarshal(data, &part)
		switch method {
		case "learning.upload":
			payload, token, err := store.Append(owner, part)
			if err != nil {
				return nil, err
			}
			if payload == nil {
				return json.Marshal(map[string]string{"token": token})
			}
			submissions++
			if !bytes.Equal(payload, wanted) {
				t.Fatal("uploaded mutation changed")
			}
			return store.Offer(owner, payload)
		case "learning.read":
			chunk, err := store.Read(owner, part)
			if err != nil {
				return nil, err
			}
			return json.Marshal(chunk)
		case "learning.release":
			store.Release(owner, part.Token)
			return json.RawMessage(`{}`), nil
		}
		return nil, fmt.Errorf("unexpected method %s", method)
	}
	got, err := CallRPC(context.Background(), input, call)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, wanted) || submissions != 1 {
		t.Fatalf("payload mismatch or duplicate submission: %d", submissions)
	}
	if len(store.buffers) != 0 {
		t.Fatal("completed transfer retained data")
	}
}
func TestLearningRPCRejectsOtherOwnersAndSkippedChunks(t *testing.T) {
	s := &RPCTransfers{}
	defer s.Close()
	_, token, err := s.Append(1, RPCPart{Size: 10, Data: []byte("123")})
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []RPCPart{{Token: token, Offset: 4, Size: 10, Data: []byte("4")}, {Token: token, Offset: 3, Size: 11, Data: []byte("4")}} {
		if _, _, err = s.Append(1, part); err == nil {
			t.Fatal("invalid continuation accepted")
		}
	}
	if _, _, err = s.Append(2, RPCPart{Token: token, Offset: 3, Size: 10, Data: []byte("4")}); err == nil {
		t.Fatal("another peer appended")
	}
}
