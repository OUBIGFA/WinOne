package profiles

import (
	"encoding/json"
	"errors"
	"strings"
)

func importTray(source map[string]json.RawMessage, resolve pathResolver) (map[string]any, []string, error) {
	patch, err := exactFields(source, "exitAfterManagedAppsCompleted:b closeWindowRetrySeconds:n startupIntervalSeconds:n", "managedApps")
	if err != nil {
		return nil, nil, err
	}
	for _, key := range []string{"closeWindowRetrySeconds", "startupIntervalSeconds"} {
		if err = numberRange(patch, key, 0, 120); err != nil {
			return nil, nil, err
		}
	}
	var entries []map[string]json.RawMessage
	if err = requiredArray(source["managedApps"], &entries); err != nil {
		return nil, nil, err
	}
	apps := make([]map[string]any, 0, len(entries))
	arguments := []string{}
	ids := map[string]bool{}
	for _, entry := range entries {
		app, err := importTrayApp(entry, resolve)
		if err != nil {
			return nil, nil, err
		}
		appID := app["id"].(string)
		if strings.TrimSpace(appID) == "" || ids[appID] || strings.TrimSpace(app["name"].(string)) == "" {
			return nil, nil, errors.New("程序标识或名称为空，或标识重复")
		}
		ids[appID] = true
		if app["args"].(string) != "" {
			arguments = append(arguments, appID)
		}
		apps = append(apps, app)
	}
	patch["managedApps"] = apps
	return patch, arguments, nil
}

func importTrayApp(entry map[string]json.RawMessage, resolve pathResolver) (map[string]any, error) {
	app, err := exactFields(entry, "id:s name:s programRef:s args:s runOnStartup:b collectTrayIcon:b launchViaLogonTask:b launchHiddenInBackground:b", "trayBehavior", "schedule")
	if err != nil {
		return nil, err
	}
	tray, err := nestedFields(entry["trayBehavior"], "autoMinimizeAndHideOnLaunch:b closeDelaySeconds:n")
	if err != nil {
		return nil, err
	}
	schedule, err := nestedFields(entry["schedule"], "enabled:b startDelayMinutes:n autoExitMinutes:n frequencyEnabled:b frequencyDays:n frequencyRuns:n")
	if err != nil {
		return nil, err
	}
	if err = numberRange(tray, "closeDelaySeconds", 0, 600); err != nil {
		return nil, err
	}
	for _, key := range []string{"startDelayMinutes", "autoExitMinutes"} {
		if err = numberRange(schedule, key, 0, 1440); err != nil {
			return nil, err
		}
	}
	if schedule["frequencyEnabled"].(bool) {
		if err = numberRange(schedule, "frequencyDays", 1, 365); err != nil {
			return nil, err
		}
		if err = numberRange(schedule, "frequencyRuns", 1, 1000); err != nil {
			return nil, err
		}
	}
	modes := 0
	for _, mode := range []bool{app["launchViaLogonTask"].(bool), app["launchHiddenInBackground"].(bool), tray["autoMinimizeAndHideOnLaunch"].(bool)} {
		if mode {
			modes++
		}
	}
	if modes > 1 {
		return nil, errors.New("启动方式互相冲突")
	}
	path, err := resolve(app["programRef"].(string), "program")
	if err != nil {
		return nil, err
	}
	if path != "" {
		extension := ""
		if index := strings.LastIndex(path, "."); index >= 0 {
			extension = strings.ToLower(path[index:])
		}
		switch extension {
		case ".exe", ".bat", ".cmd", ".ps1", ".py", ".pyw":
		default:
			return nil, errors.New("不支持的程序类型")
		}
		if extension != ".exe" && (app["launchViaLogonTask"].(bool) || app["collectTrayIcon"].(bool) || tray["autoMinimizeAndHideOnLaunch"].(bool)) {
			return nil, errors.New("脚本不支持开机即启动、收进托盘或托盘收纳")
		}
	}
	if strings.ContainsRune(app["args"].(string), 0) {
		return nil, errors.New("启动参数包含无效字符")
	}
	app["exePath"] = path
	delete(app, "programRef")
	app["trayBehavior"], app["schedule"] = tray, schedule
	return app, nil
}
