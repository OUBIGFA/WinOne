//go:build !windows

package mcpapp

import (
	"errors"
	"github.com/egoist/mygo"
)

func installFramePolicy(*mygo.WebView, Session, func(error)) (framePolicy, error) {
	return nil, errors.New("MCP_APP_NATIVE_POLICY_UNAVAILABLE")
}
