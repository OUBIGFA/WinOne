//go:build windows

package branding

import (
	"bytes"
	_ "embed"
	"image"
	_ "image/png"
	"sync"
	productbranding "winone/internal/branding"

	"github.com/lxn/walk"
)

// github.png is generated from assets/github.svg by build/svg2png.py.
//
//go:embed assets/github.png
var githubPNG []byte

var (
	appIconOnce sync.Once
	appIcon     *walk.Icon
	appIconErr  error

	githubIconOnce sync.Once
	githubIcon     *walk.Icon
	githubIconErr  error
)

func AppIcon() (*walk.Icon, error) {
	appIconOnce.Do(func() {
		appIcon, appIconErr = decodeIcon(productbranding.NativeLogo, 96)
	})

	return appIcon, appIconErr
}

// GitHubIcon returns the GitHub mark sized for a toolbar-style button. The
// bitmap is authored at twice the logical size, so it stays sharp on high-DPI
// displays.
func GitHubIcon() (*walk.Icon, error) {
	githubIconOnce.Do(func() {
		githubIcon, githubIconErr = decodeIcon(githubPNG, 192)
	})

	return githubIcon, githubIconErr
}

func decodeIcon(data []byte, dpi int) (*walk.Icon, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return walk.NewIconFromImageForDPI(img, dpi)
}
