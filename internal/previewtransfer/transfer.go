// Package previewtransfer carries a bounded immutable profile preview in chunks.
package previewtransfer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

const MaxBytes = 64 << 20
const ChunkBytes = 128 << 10

type Descriptor struct {
	Token  string `json:"token"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
}
type Chunk struct {
	Token  string `json:"token"`
	Offset int    `json:"offset"`
	Data   []byte `json:"data"`
}
type Store struct {
	mu         sync.Mutex
	ttl        time.Duration
	timer      *time.Timer
	descriptor Descriptor
	data       []byte
	expires    time.Time
}

func NewStore(ttl time.Duration) *Store { return &Store{ttl: ttl} }

// Own takes ownership of a freshly serialized buffer. Its caller must not
// mutate it afterward; the store retains at most one preview, without a copy.
func (s *Store) Own(data []byte) (Descriptor, error) {
	if len(data) == 0 || len(data) > MaxBytes {
		return Descriptor{}, errors.New("预览内容超过传输上限")
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return Descriptor{}, err
	}
	digest := sha256.Sum256(data)
	descriptor := Descriptor{Token: hex.EncodeToString(token[:]), Size: len(data), SHA256: hex.EncodeToString(digest[:])}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.descriptor = descriptor
	s.data = data
	s.expires = time.Now().Add(s.ttl)
	// The timer captures only the token, so explicit release frees the buffer.
	id := descriptor.Token
	s.timer = time.AfterFunc(s.ttl, func() { s.Release(id) })
	return descriptor, nil
}
func (s *Store) Read(token string, offset int) (Chunk, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.descriptor.Token != token || len(s.data) == 0 || time.Now().After(s.expires) {
		return Chunk{}, errors.New("配置预览已过期，请重新预览")
	}
	if offset < 0 || offset >= len(s.data) {
		return Chunk{}, errors.New("预览读取位置无效")
	}
	end := offset + ChunkBytes
	if end > len(s.data) {
		end = len(s.data)
	}
	return Chunk{Token: token, Offset: offset, Data: s.data[offset:end]}, nil
}
func (s *Store) Release(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if token != s.descriptor.Token {
		return
	}
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.data = nil
	s.descriptor = Descriptor{}
}

func Receive(ctx context.Context, descriptor Descriptor, read func(int) (Chunk, error)) ([]byte, error) {
	if len(descriptor.Token) != 32 || descriptor.Size < 1 || descriptor.Size > MaxBytes || len(descriptor.SHA256) != 64 {
		return nil, errors.New("配置预览传输信息无效")
	}
	result := make([]byte, 0, descriptor.Size)
	for len(result) < descriptor.Size {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		chunk, err := read(len(result))
		if err != nil {
			return nil, err
		}
		// A read may finish after cancellation, including the final chunk.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if chunk.Token != descriptor.Token || chunk.Offset != len(result) || len(chunk.Data) == 0 || len(chunk.Data) > ChunkBytes || len(chunk.Data) > descriptor.Size-len(result) {
			return nil, errors.New("配置预览分块不匹配")
		}
		result = append(result, chunk.Data...)
	}
	digest := sha256.Sum256(result)
	if hex.EncodeToString(digest[:]) != descriptor.SHA256 {
		return nil, errors.New("配置预览校验失败")
	}
	return result, nil
}
