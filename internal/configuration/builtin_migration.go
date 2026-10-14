package configuration

import "errors"

// Catalog upgrades only add newly bundled modules to older documents. A
// missing module in an already upgraded document remains an integrity error.
func upgradeSelectionDocument(document Document, registered map[string]ModuleAdapter) (Document, bool, error) {
	target := 0
	if _, ok := registered["selection"]; ok {
		target = 2
	}
	if _, ok := registered["quick-assistant"]; ok {
		target = 3
	}
	if _, ok := registered["stroke"]; ok {
		target = 4
	}
	if document.BuiltinVersion < 0 || document.BuiltinVersion > 4 {
		return document, false, errors.New("不支持的基础模块版本")
	}
	if target == 0 || document.BuiltinVersion >= target {
		return document, false, nil
	}
	next := clone(document)
	if next.ModuleEnabled != nil {
		for _, id := range []string{"capture", "always-on-top", "wintray"} {
			if _, ok := next.ModuleEnabled[id]; !ok {
				return document, false, errors.New("旧配置缺少基础模块开关")
			}
		}
		if document.BuiltinVersion < 2 {
			if _, ok := registered["selection"]; ok {
				if _, exists := next.ModuleEnabled["selection"]; !exists {
					next.ModuleEnabled["selection"] = true
				}
			}
		}
		if document.BuiltinVersion < 3 && target >= 3 {
			if _, exists := next.ModuleEnabled["quick-assistant"]; !exists {
				if _, ok := registered["quick-assistant"]; ok {
					next.ModuleEnabled["quick-assistant"] = true
				}
			}
		}
		if target == 4 {
			if _, exists := next.ModuleEnabled["stroke"]; !exists {
				next.ModuleEnabled["stroke"] = true
			}
		}
	}
	next.BuiltinVersion = target
	return next, true, nil
}
