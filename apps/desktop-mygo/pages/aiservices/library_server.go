package aiservices

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
)

// Adapt the pinned Library's serversIn/jsonOf: paste a README, a Claude Code
// mcpServers map, a VS Code servers map, or a single server entry. Variable
// references remain literal; parsing never resolves secrets or contacts a server.
var mcpServerName = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func mcpJSONWithoutTrailingCommas(input string) string {
	var out strings.Builder
	quoted, escaped := false, false
	for i, c := range input {
		if !quoted && c == ',' {
			rest := strings.TrimLeft(input[i+1:], " \t\r\n")
			if strings.HasPrefix(rest, "}") || strings.HasPrefix(rest, "]") {
				continue
			}
		}
		out.WriteRune(c)
		if quoted {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
		} else if c == '"' {
			quoted = true
		}
	}
	return out.String()
}

func parseMCPConfigs(input string) ([]map[string]any, error) {
	var value map[string]any
	input = strings.TrimSpace(input)
	if json.Unmarshal([]byte(mcpJSONWithoutTrailingCommas(input)), &value) != nil {
		if json.Unmarshal([]byte("{"+strings.TrimSuffix(input, ",")+"}"), &value) != nil {
			return nil, errors.New(upstreamT("This isn't an MCP server's JSON"))
		}
	}
	entries := value
	for _, key := range []string{"mcpServers", "servers", "mcp", "context_servers"} {
		if nested := objectOf(value, key); nested != nil {
			entries = nested
			break
		}
	}
	if value["command"] != nil || value["url"] != nil || value["serverUrl"] != nil || value["httpUrl"] != nil {
		entries = map[string]any{textOf(value, "name"): value}
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	rows := []map[string]any{}
	for _, name := range names {
		entry, _ := entries[name].(map[string]any)
		if entry == nil {
			continue
		}
		row := map[string]any{"name": strings.Trim(mcpServerName.ReplaceAllString(name, "-"), "-"), "env": map[string]any{}, "headers": map[string]any{}}
		address := ""
		for _, key := range []string{"url", "serverUrl", "httpUrl", "uri"} {
			if address == "" {
				address = textOf(entry, key)
			}
		}
		if address != "" {
			transport := "http"
			if textOf(entry, "type") == "sse" || textOf(entry, "transport") == "sse" || strings.HasSuffix(strings.TrimSuffix(address, "/"), "/sse") {
				transport = "sse"
			}
			row["transport"], row["url"] = transport, address
			if h := objectOf(entry, "headers"); h != nil {
				row["headers"] = h
			} else if h := objectOf(entry, "http_headers"); h != nil {
				row["headers"] = h
			}
		} else if entry["command"] != nil {
			command := textOf(entry, "command")
			args := []string{}
			if parts, ok := entry["command"].([]any); ok && len(parts) > 0 {
				command = fmt.Sprint(parts[0])
				for _, part := range parts[1:] {
					args = append(args, fmt.Sprint(part))
				}
			}
			if command == "" {
				continue
			}
			if parts, ok := entry["args"].([]any); ok {
				for _, part := range parts {
					args = append(args, fmt.Sprint(part))
				}
			}
			row["transport"], row["command"], row["args"] = "stdio", command, args
			if env := objectOf(entry, "env"); env != nil {
				row["env"] = env
			} else if env := objectOf(entry, "environment"); env != nil {
				row["env"] = env
			}
		} else {
			continue
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, errors.New(upstreamT("This isn't an MCP server's JSON"))
	}
	return rows, nil
}

// The form's one-value-per-line display cannot encode an empty argument or a
// newline inside one value. Keep each original array/map while its own text
// has not changed, including when the user edits other fields or switches views.
func (e *libraryEditor) rememberServerFields(row map[string]any) {
	e.serverFields = map[string]any{}
	e.serverText = map[string]string{}
	for _, key := range []string{"args", "env", "headers"} {
		if value, ok := row[key]; ok {
			e.serverFields[key] = value
			e.serverText[key] = e.values[key]
		}
	}
}

func (e *libraryEditor) serverField(key string) any {
	if value, ok := e.serverFields[key]; ok && e.values[key] == e.serverText[key] {
		return value
	}
	if key == "args" {
		return listLines(e.values[key])
	}
	return mapLines(e.values[key])
}

func serverConfigJSON(e *libraryEditor) string {
	v := e.values
	config := map[string]any{"type": v["transport"]}
	if v["transport"] == "stdio" {
		config["command"], config["args"], config["env"] = v["command"], e.serverField("args"), e.serverField("env")
	} else {
		config["url"], config["headers"] = v["url"], e.serverField("headers")
	}
	data, _ := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{v["name"]: config}}, "", "  ")
	return string(data)
}

func (e *libraryEditor) readServerJSON() error {
	rows, err := parseMCPConfigs(e.serverJSON)
	if err != nil {
		return err
	}
	if len(rows) != 1 {
		return errors.New(upstreamT("Only one server here; paste several outside this box to add them"))
	}
	row := rows[0]
	if name := textOf(row, "name"); name != "" {
		e.values["name"] = name
	}
	for _, key := range []string{"transport", "command", "url"} {
		e.values[key] = textOf(row, key)
	}
	e.values["args"] = strings.Join(stringsOf(row["args"]), "\n")
	e.values["env"], e.values["headers"] = mapText(row["env"]), mapText(row["headers"])
	e.rememberServerFields(row)
	return nil
}

func (x *externalPanel) serverJSONView(c *ui.Context, e *libraryEditor) bool {
	old := e.serverView
	kit.SubTabs(c, &e.serverView, []string{upstreamT("Form"), "JSON"}, func(apply func()) {
		if e.serverError == "" {
			apply()
		}
	})
	if old == 0 && e.serverView == 1 {
		e.serverJSON = serverConfigJSON(e)
	}
	if e.serverView == 0 {
		return false
	}
	if ui.TextArea(c, &e.serverJSON).FillWidth().MinHeight(240).Font(kit.MonoFont).Label("JSON").Disabled(x.working).Changed() {
		e.serverError = ""
		if err := e.readServerJSON(); err != nil {
			e.serverError = err.Error()
		}
	}
	kit.InlineError(c, e.serverError)
	return true
}

func importedServersJSON(rows []map[string]any) string {
	entries := map[string]any{}
	for _, row := range rows {
		entry := map[string]any{"type": row["transport"]}
		keys := []string{"url", "headers"}
		if textOf(row, "transport") == "stdio" {
			keys = []string{"command", "args", "env"}
		}
		for _, key := range keys {
			entry[key] = row[key]
		}
		entries[textOf(row, "name")] = entry
	}
	data, _ := json.MarshalIndent(map[string]any{"mcpServers": entries}, "", "  ")
	return string(data)
}

func (x *externalPanel) saveServerImport(e *libraryEditor) {
	rows, err := parseMCPConfigs(e.serverJSON)
	if err != nil {
		x.err = err.Error()
		return
	}
	names := map[string]bool{}
	for _, row := range rows {
		name := textOf(row, "name")
		if name == "" {
			x.err = upstreamT("Name every server before importing.")
			return
		}
		if names[name] {
			x.err = upstreamT("Each server needs a different name: {name}", "name", name)
			return
		}
		names[name] = true
		row["agents"] = picked(e.agents)
	}
	results := []any{}
	var saveNext func(int)
	saveNext = func(index int) {
		x.request("POST", "/api/library/servers/save", map[string]any{"old": "", "server": rows[index]}, func(raw json.RawMessage) {
			x.acceptLibrary(raw)
			if x.err != "" {
				return
			}
			results = append(results, x.result)
			x.result = results
			remaining := rows[index+1:]
			if len(remaining) == 0 {
				return
			}
			// A later failure keeps only the unprocessed entries in the draft,
			// so Retry cannot overwrite the servers already saved.
			e.serverJSON = importedServersJSON(remaining)
			x.edit = e
			if len(rowsOf(x.library, "problems")) > 0 {
				return
			}
			saveNext(index + 1)
		})
	}
	saveNext(0)
}
