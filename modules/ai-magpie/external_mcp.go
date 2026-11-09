package main

import (
	"fmt"
	"github.com/yetone/magpie/internal/browsermcp"
	"github.com/yetone/magpie/internal/imagemcp"
)

// Library installs point at this executable. Preserve upstream's stdio MCP
// entry points before the normal Host JSON protocol consumes their stdin.
func runExternalMCPCommand(args []string) (bool, error) {
	if len(args) == 0 || args[0] != "mcp" {
		return false, nil
	}
	if len(args) < 2 {
		return true, fmt.Errorf("choose an MCP server: image, browser, search or vision")
	}
	switch args[1] {
	case "browser":
		return true, browsermcp.Run(args[2:])
	case "search":
		return true, browsermcp.RunSearch(args[2:])
	case "vision":
		return true, browsermcp.RunVision(args[2:])
	case "image":
		return true, imagemcp.Run(args[1:])
	default:
		return true, fmt.Errorf("unknown MCP server %q", args[1])
	}
}
