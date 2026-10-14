//go:build windows

package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
	"winone/internal/ai"
	"winone/internal/configjson"
	"winone/internal/host"
)

type avatarUpload struct {
	owner   uint32
	size    int
	data    []byte
	expires time.Time
}

// Uploads are small, short-lived and bound to the transport's process identity.
// No temporary file survives a failed pick and no store/IPC lock spans Magpie.
func (s *aiService) appendAvatar(peer uint32, upload string, offset, size int, data []byte) ([]byte, string, error) {
	s.avatarMu.Lock()
	defer s.avatarMu.Unlock()
	fail := func() ([]byte, string, error) {
		return nil, "", errors.New("图片传输已失效，请重新选择图片")
	}
	if size <= 0 || size > ai.MaxAvatarBytes || len(data) == 0 || len(data) > ai.AvatarChunkBytes || offset < 0 || offset+len(data) > size {
		return fail()
	}
	if s.avatarUploads == nil {
		s.avatarUploads = map[string]*avatarUpload{}
	}
	for token, item := range s.avatarUploads {
		if time.Now().After(item.expires) {
			delete(s.avatarUploads, token)
		}
	}
	if upload == "" {
		if offset != 0 {
			return fail()
		}
		for token, item := range s.avatarUploads {
			if item.owner == peer {
				delete(s.avatarUploads, token)
			}
		}
		if len(s.avatarUploads) >= 4 {
			return nil, "", errors.New("正在处理其他图片，请稍后重试")
		}
		var token [16]byte
		if _, err := rand.Read(token[:]); err != nil {
			return nil, "", err
		}
		upload = hex.EncodeToString(token[:])
		s.avatarUploads[upload] = &avatarUpload{owner: peer, size: size, expires: time.Now().Add(time.Minute)}
	}
	item := s.avatarUploads[upload]
	if item == nil || item.owner != peer || item.size != size || offset != len(item.data) {
		return fail()
	}
	item.data = append(item.data, data...)
	if len(item.data) < size {
		return nil, upload, nil
	}
	delete(s.avatarUploads, upload)
	return item.data, "", nil
}

func (s *aiService) handleAvatar(r host.Request) host.Response {
	response := host.Response{ID: r.ID}
	fail := func(err error) host.Response {
		response.Error = &host.RPCError{Code: "ai", Message: err.Error()}
		return response
	}
	result := func(value any) host.Response { response.Result, _ = json.Marshal(value); return response }
	switch r.Method {
	case "icons.store":
		var input struct {
			Data   []byte `json:"data"`
			Size   int    `json:"size"`
			Offset int    `json:"offset"`
			Upload string `json:"upload"`
		}
		if err := configjson.DecodeObject(r.Params, &input); err != nil {
			return fail(err)
		}
		data, upload, err := s.appendAvatar(r.PeerPID, input.Upload, input.Offset, input.Size, input.Data)
		if err != nil {
			return fail(err)
		}
		if upload != "" {
			return result(map[string]any{"upload": upload, "offset": input.Offset + len(input.Data)})
		}
		icon, err := s.translator.ImportAvatar(s.ctx, r.Method, ai.AvatarInput{Data: data})
		if err != nil {
			return fail(err)
		}
		return result(map[string]string{"icon": icon})
	case "icons.cancel":
		var input struct {
			Upload string `json:"upload"`
		}
		if err := configjson.DecodeObject(r.Params, &input); err != nil {
			return fail(err)
		}
		s.avatarMu.Lock()
		if item := s.avatarUploads[input.Upload]; item != nil && item.owner == r.PeerPID {
			delete(s.avatarUploads, input.Upload)
		}
		s.avatarMu.Unlock()
		return result(struct{}{})
	case "icons.fetch", "icons.website":
		var input ai.AvatarInput
		if err := configjson.DecodeObject(r.Params, &input); err != nil {
			return fail(err)
		}
		if len(input.Data) != 0 {
			return fail(errors.New("头像链接参数无效"))
		}
		icon, err := s.translator.ImportAvatar(s.ctx, r.Method, input)
		if err != nil {
			return fail(err)
		}
		return result(map[string]string{"icon": icon})
	case "icons.read":
		var input struct {
			Icon   string `json:"icon"`
			Offset int    `json:"offset"`
		}
		if err := configjson.DecodeObject(r.Params, &input); err != nil {
			return fail(err)
		}
		chunk, err := s.translator.ReadAvatar(input.Icon, input.Offset)
		if err != nil {
			return fail(err)
		}
		return result(chunk)
	default:
		return fail(errors.New("不支持的头像操作"))
	}
}
