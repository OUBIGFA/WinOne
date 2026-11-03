package webui

import (
	"encoding/json"
	"errors"
	"unicode/utf8"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
)

// ExportTask saves a conversation as a .cshare archive, as the Rust
// window's chat_export.rs did for both the native page and the React
// pages: only the window chooses where (its save dialog), the Host writes
// there, and its receipt must name that file. params are the request's
// id, password and excludeMedia, nothing else. It blocks on the dialog:
// call it off the UI thread. The result is the Host's receipt, or
// {"status":"canceled"}.
func ExportTask(env *core.Env, params json.RawMessage) (json.RawMessage, error) {
	invalid := errors.New(i18n.T("chat.exportInvalid"))
	var fields map[string]json.RawMessage
	if json.Unmarshal(params, &fields) != nil || fields == nil {
		return nil, invalid
	}
	var id, password string
	for key, value := range fields {
		switch key {
		case "id":
			if json.Unmarshal(value, &id) != nil || id == "" {
				return nil, invalid
			}
		case "password":
			if json.Unmarshal(value, &password) != nil || utf8.RuneCountInString(password) > 4096 {
				return nil, invalid
			}
		case "excludeMedia":
			var exclude bool
			if json.Unmarshal(value, &exclude) != nil {
				return nil, invalid
			}
		default:
			// A page never names the destination.
			return nil, invalid
		}
	}
	if id == "" {
		return nil, invalid
	}
	path, err := env.Dialogs.SaveFile(i18n.T("chat.exportTitle"), core.Filter{Name: i18n.T("chat.exportFileType"), Extensions: []string{"cshare"}}, "WinOne-task.cshare")
	if err != nil {
		return nil, err
	}
	if path == "" {
		return json.RawMessage(`{"status":"canceled"}`), nil
	}
	fields["targetPath"], _ = json.Marshal(path)
	result, err := env.Host.Call("chat.export", fields)
	if err != nil {
		return nil, err
	}
	var receipt struct {
		Status     string  `json:"status"`
		FilePath   string  `json:"filePath"`
		Fidelity   string  `json:"fidelity"`
		LimitBytes float64 `json:"limitBytes"`
		TotalBytes float64 `json:"totalBytes"`
	}
	if json.Unmarshal(result, &receipt) != nil {
		return nil, errors.New(i18n.T("chat.exportUnconfirmed"))
	}
	ok := receipt.Status == "ok" && receipt.FilePath == path &&
		(receipt.Fidelity == "full" || receipt.Fidelity == "partial" || receipt.Fidelity == "db-only")
	oversize := receipt.Status == "oversize" && receipt.LimitBytes > 0 && receipt.TotalBytes > receipt.LimitBytes
	if !ok && !oversize {
		return nil, errors.New(i18n.T("chat.exportUnconfirmed"))
	}
	return result, nil
}
