package learning

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"sync"
	"time"
	"winone/internal/previewtransfer"
)

const RPCMaxBytes = 96 << 20
const RPCInlineBytes = 512 << 10

type RPCPart struct {
	ID     string `json:"id,omitempty"`
	Token  string `json:"token"`
	Offset int    `json:"offset"`
	Size   int    `json:"size"`
	Data   []byte `json:"data,omitempty"`
}
type rpcBuffer struct {
	timer   *time.Timer
	owner   uint32
	size    int
	data    []byte
	expires time.Time
	store   *previewtransfer.Store
}
type RPCTransfers struct {
	mu      sync.Mutex
	buffers map[string]*rpcBuffer
}

func (s *RPCTransfers) prune() {
	if s.buffers == nil {
		s.buffers = map[string]*rpcBuffer{}
	}
	for token, item := range s.buffers {
		if time.Now().After(item.expires) {
			if item.timer != nil {
				item.timer.Stop()
			}
			if item.store != nil {
				item.store.Release(token)
			}
			delete(s.buffers, token)
		}
	}
}
func (s *RPCTransfers) Append(owner uint32, part RPCPart) (json.RawMessage, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	if part.Size < 1 || part.Size > RPCMaxBytes || part.Offset < 0 || len(part.Data) == 0 || len(part.Data) > previewtransfer.ChunkBytes || part.Offset+len(part.Data) > part.Size {
		return nil, "", errors.New("学习内容分块无效")
	}
	token := part.Token
	if token == "" {
		if part.Offset != 0 || len(s.buffers) >= 4 {
			return nil, "", errors.New("请等待当前学习内容传输完成")
		}
		token = rand.Text()
		s.buffers[token] = &rpcBuffer{owner: owner, size: part.Size, expires: time.Now().Add(2 * time.Minute)}
		ownedToken := token
		s.buffers[token].timer = time.AfterFunc(2*time.Minute, func() { s.Release(owner, ownedToken) })
	}
	item := s.buffers[token]
	if item == nil || item.owner != owner || item.store != nil || item.size != part.Size || len(item.data) != part.Offset {
		return nil, "", errors.New("学习内容传输已失效")
	}
	item.data = append(item.data, part.Data...)
	if len(item.data) < item.size {
		return nil, token, nil
	}
	if item.timer != nil {
		item.timer.Stop()
	}
	delete(s.buffers, token)
	return item.data, "", nil
}
func (s *RPCTransfers) Offer(owner uint32, data json.RawMessage) (json.RawMessage, error) {
	if len(data) <= RPCInlineBytes {
		return data, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	if len(s.buffers) >= 4 {
		return nil, errors.New("请等待当前学习内容传输完成")
	}
	store := previewtransfer.NewStoreLimit(2*time.Minute, RPCMaxBytes)
	descriptor, err := store.Own(data)
	if err != nil {
		return nil, err
	}
	s.buffers[descriptor.Token] = &rpcBuffer{owner: owner, store: store, expires: time.Now().Add(2 * time.Minute)}
	return json.Marshal(map[string]any{"winoneLearningTransfer": descriptor})
}
func (s *RPCTransfers) Read(owner uint32, part RPCPart) (previewtransfer.Chunk, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	item := s.buffers[part.Token]
	if item == nil || item.owner != owner || item.store == nil {
		return previewtransfer.Chunk{}, errors.New("学习内容传输已失效")
	}
	return item.store.Read(part.Token, part.Offset)
}
func (s *RPCTransfers) Release(owner uint32, token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.buffers[token]
	if item != nil && item.owner == owner {
		if item.timer != nil {
			item.timer.Stop()
		}
		if item.store != nil {
			item.store.Release(token)
		}
		delete(s.buffers, token)
	}
}
func (s *RPCTransfers) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, item := range s.buffers {
		if item.timer != nil {
			item.timer.Stop()
		}
		if item.store != nil {
			item.store.Release(token)
		}
	}
	s.buffers = nil
}

// CallRPC keeps large learning data inside the existing authenticated pipe and
// verifies every returned chunk. The mutation is submitted only after upload.
func CallRPC(ctx context.Context, params any, call func(string, any) (json.RawMessage, error)) (json.RawMessage, error) {
	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if len(data) > RPCMaxBytes {
		return nil, errors.New("学习内容超过传输上限")
	}
	var identity struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(data, &identity)
	var raw json.RawMessage
	if len(data) <= RPCInlineBytes {
		raw, err = call("learning.call", params)
	} else {
		token := ""
		for offset := 0; offset < len(data); {
			if err = ctx.Err(); err != nil {
				break
			}
			end := min(offset+previewtransfer.ChunkBytes, len(data))
			raw, err = call("learning.upload", RPCPart{ID: identity.ID, Token: token, Offset: offset, Size: len(data), Data: data[offset:end]})
			if err != nil {
				break
			}
			offset = end
			if offset < len(data) {
				var reply struct {
					Token string `json:"token"`
				}
				if err = json.Unmarshal(raw, &reply); err != nil {
					break
				}
				if reply.Token == "" {
					err = errors.New("学习内容传输缺少标识")
					break
				}
				token = reply.Token
			}
		}
		if err != nil && token != "" {
			_, _ = call("learning.release", RPCPart{ID: identity.ID, Token: token})
		}
	}
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Transfer *previewtransfer.Descriptor `json:"winoneLearningTransfer"`
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.Transfer == nil {
		return raw, nil
	}
	descriptor := *envelope.Transfer
	defer func() { _, _ = call("learning.release", RPCPart{ID: identity.ID, Token: descriptor.Token}) }()
	return previewtransfer.ReceiveLimit(ctx, descriptor, RPCMaxBytes, func(offset int) (previewtransfer.Chunk, error) {
		var chunk previewtransfer.Chunk
		raw, err := call("learning.read", RPCPart{ID: identity.ID, Token: descriptor.Token, Offset: offset})
		if err == nil {
			err = json.Unmarshal(raw, &chunk)
		}
		return chunk, err
	})
}
