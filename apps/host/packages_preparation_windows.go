//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"winone/internal/host"
	"winone/internal/modulepackage"
)

// Every install/update exit hands preparation ownership to durable inventory
// when possible. Failed handoff is reported without undoing a successful install
// or hiding the original error; the existing cleanup entry can retry it.
func (p *installedPackages) finishPackagePreparation(registry *modulepackage.Registry, stage *modulepackage.StagedPackage, reply *host.Response) {
	if stage == nil {
		return
	}
	if err := registry.FinishPreparation(p.ctx, filepath.Base(stage.Directory())); err != nil {
		warning := "安装准备记录仍待处理：" + err.Error()
		if reply.Error != nil {
			reply.Error.Message += "；" + warning
			return
		}
		var value map[string]any
		if decodeErr := json.Unmarshal(reply.Result, &value); decodeErr != nil || value == nil {
			reply.Error = &host.RPCError{Code: "package_preparation", Message: fmt.Sprintf("%s；无法解析操作结果", warning)}
			return
		}
		if previous, ok := value["warning"].(string); ok && previous != "" {
			warning = previous + "；" + warning
		}
		value["warning"] = warning
		encoded, encodeErr := json.Marshal(value)
		if encodeErr != nil {
			reply.Error = &host.RPCError{Code: "package_preparation", Message: warning}
			return
		}
		reply.Result = encoded
	}
}
