package resultui

import (
	"context"
	"errors"
	"time"
	"winone/internal/ai"
)

// Avatar reads and decoding stay off the UI thread and end with the window.
func (c *Controller) loadProviderAvatar(icon string, done func(*providerAvatar, error)) {
	go func() {
		ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
		defer cancel()
		var data []byte
		size := 0
		for {
			var chunk ai.AvatarChunk
			if err := c.Client.Call(ctx, "ai.icons.read", map[string]any{"icon": icon, "offset": len(data)}, &chunk); err != nil {
				c.update(func() { done(nil, err) })
				return
			}
			if chunk.Size <= 0 || chunk.Size > ai.MaxAvatarBytes || (size != 0 && chunk.Size != size) || len(chunk.Data) == 0 || len(chunk.Data) > ai.AvatarChunkBytes || len(data)+len(chunk.Data) > chunk.Size {
				c.update(func() { done(nil, errors.New("头像数据不完整")) })
				return
			}
			size = chunk.Size
			data = append(data, chunk.Data...)
			if len(data) == size {
				break
			}
		}
		avatar, err := decodeProviderAvatar(data)
		c.update(func() { done(avatar, err) })
	}()
}
