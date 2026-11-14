package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

//go:embed upstream_paste.js
var upstreamPaste string

// Only the pinned pure parsers run. The pasted setup is JSON on stdin and is
// never a shell command, script argument, environment setting or saved key.
func parseProviderPaste(ctx context.Context, text string) (json.RawMessage, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	bun := filepath.Join(filepath.Dir(executable), "runtime", "bun.exe")
	if err = checkPluginResource(ctx, bun, bundledPluginResources.Bun.SHA256, -1); err != nil {
		return nil, err
	}
	code := upstreamPaste + "\nconst text = JSON.parse(await Bun.stdin.text()); console.log(JSON.stringify({parsed: curlOf(text) || envOf(text)}));\n"
	command := exec.CommandContext(ctx, bun, "--eval", code)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	input, err := json.Marshal(text)
	if err != nil {
		return nil, err
	}
	command.Stdin = strings.NewReader(string(input))
	output, err := command.Output()
	if err != nil || len(output) > 1<<20 || !json.Valid(output) {
		return nil, errors.New("connection setup could not be read")
	}
	return output, nil
}
