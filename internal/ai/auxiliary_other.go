//go:build !windows

package ai

import "context"

func (t Translator) auxiliaryBridge(context.Context, []RuntimeProvider, *MagpieSettings) ([]auxiliaryProvider, func(), error) {
	return nil, func() {}, nil
}
