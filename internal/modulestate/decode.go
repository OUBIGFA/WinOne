package modulestate

import (
	"errors"
	"winone/internal/catalog"
	"winone/internal/configjson"
)

// DecodeSnapshot validates maintenance input without migrating or writing it.
// Legacy three-module state remains readable so the target Host can check an
// older installation without performing its normal selection migration.
func DecodeSnapshot(data []byte) (Snapshot, error) {
	var result Snapshot
	if len(data) > 1<<20 {
		return result, errors.New("模块状态文件过大")
	}
	if err := configjson.DecodeObject(data, &result); err != nil {
		return result, err
	}
	if result.Modules == nil || result.SchemaVersion < 1 || result.SchemaVersion > 4 {
		return result, errors.New("模块状态版本无效")
	}
	required := []string{"capture", "always-on-top", "wintray"}
	if result.SchemaVersion >= 2 {
		required = append(required, "selection")
	}
	if result.SchemaVersion >= 3 {
		required = append(required, "quick-assistant")
	}
	if result.SchemaVersion >= 4 {
		required = append(required, "stroke")
	}
	for _, id := range required {
		if _, ok := result.Modules[id]; !ok {
			return result, errors.New("模块状态缺少基础模块")
		}
	}
	for id, record := range result.Modules {
		if !catalog.ValidID(id) || record.Enabled && record.PendingStop {
			return result, errors.New("模块状态无效")
		}
		if record.PackageSHA256 != "" && (!catalog.ValidOptionalID(id) || !validPackageDigest(record.PackageSHA256)) {
			return result, errors.New("模块包绑定无效")
		}
	}
	return result, nil
}
