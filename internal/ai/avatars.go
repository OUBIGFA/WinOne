package ai

import "regexp"

// Magpie's stored picture identity is content-addressed. Neither a path nor a
// remote URL is accepted as a display reference; downloads happen only on pick.
var storedAvatarPattern = regexp.MustCompile(`^file:[0-9a-f]{16}\.(png|jpg|gif|webp|ico|svg)$`)

const MaxAvatarBytes = 1 << 20     // Matches Magpie's provider.MaxIcon.
const AvatarChunkBytes = 256 << 10 // Fits base64 plus RPC metadata in a 1 MiB frame.

func IsStoredAvatar(icon string) bool { return storedAvatarPattern.MatchString(icon) }
func validProviderAvatar(icon string) bool {
	return icon == "" || providerIDPattern.MatchString(icon) || IsStoredAvatar(icon)
}

// DisplayIcon keeps the preset artwork intact so a custom picture can be reset.
func (p Provider) DisplayIcon() string {
	if p.Avatar != nil && *p.Avatar != "" {
		return *p.Avatar
	}
	if p.Icon != "" {
		return p.Icon
	}
	return providerIcon(p)
}

// AvatarChunk transfers an immutable picture without inflating streaming state
// or raising the limit shared by all local RPCs.
type AvatarChunk struct {
	Data     []byte `json:"data"`
	Size     int    `json:"size"`
	MimeType string `json:"mimeType"`
}
