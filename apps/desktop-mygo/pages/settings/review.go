package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// value is a JSON value that keeps its objects' members in the order the
// archive wrote them, as the React review listed them.
type value struct {
	kind    valueKind
	members []member
	items   []value
	// scalar is a string, json.Number, bool or nil.
	scalar any
}

type member struct {
	key   string
	value value
}

type valueKind int

const (
	scalarValue valueKind = iota
	objectValue
	arrayValue
)

func parseValue(raw json.RawMessage) (value, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return value{}, errors.New("empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decodeValue(decoder)
}

func decodeValue(decoder *json.Decoder) (value, error) {
	token, err := decoder.Token()
	if err != nil {
		return value{}, err
	}
	switch delim := token.(type) {
	case json.Delim:
		if delim == '{' {
			v := value{kind: objectValue}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return value{}, err
				}
				item, err := decodeValue(decoder)
				if err != nil {
					return value{}, err
				}
				v.members = append(v.members, member{key.(string), item})
			}
			_, err := decoder.Token()
			return v, err
		}
		v := value{kind: arrayValue}
		for decoder.More() {
			item, err := decodeValue(decoder)
			if err != nil {
				return value{}, err
			}
			v.items = append(v.items, item)
		}
		_, err := decoder.Token()
		return v, err
	}
	return value{kind: scalarValue, scalar: token}, nil
}

// field returns an object's member.
func (v value) field(key string) (value, bool) {
	for _, m := range v.members {
		if m.key == key {
			return m.value, true
		}
	}
	return value{}, false
}

// text is the scalar as the React review printed it: String(value ?? "").
func (v value) text() string {
	switch s := v.scalar.(type) {
	case string:
		return s
	case json.Number:
		return s.String()
	case bool:
		if s {
			return "true"
		}
		return "false"
	}
	return ""
}

// reviewSettings shows a module's settings from an archive: each field
// under its localized name (or its key), booleans as on/off, lists,
// nested settings, local path references resolved to the chosen location,
// and action scripts as written, never interpreted. Identifiers and
// revisions are left out. nested lays the fields out one under another.
func reviewSettings(c *ui.Context, values value, paths []profilePath, nested bool) {
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		for i, m := range values.members {
			if slices.Contains([]string{"id", "version", "revision", "schemaVersion"}, m.key) {
				continue
			}
			name := m.key
			if key := "profileFields." + m.key; i18n.Has(key) {
				name = i18n.T(key)
			}
			entry := ui.Row(c.Key(i)).FillWidth().GapX(16).GapY(4).Wrap().AlignItems(ui.Start)
			if nested {
				entry = ui.Column(c.Key(i)).FillWidth().Gap(4)
			}
			entry.Children(func() {
				label := kit.Help(c, name)
				if !nested {
					label.Basis(130).Grow(1).Shrink(1).MinWidth(0)
				}
				body := ui.Column(c).MinWidth(0).Gap(4)
				if !nested {
					body.Basis(260).Grow(2).Shrink(1)
				}
				body.Children(func() { showField(c, m.key, m.value, paths) })
			})
		}
	})
}

func showField(c *ui.Context, key string, v value, paths []profilePath) {
	t := c.Theme()
	plain := func(s string) { kit.Body(c, s).Selectable() }
	switch {
	case key == "openEditor" && v.kind == scalarValue && isBool(v):
		if v.scalar.(bool) {
			plain(i18n.T("capture.editFirst"))
		} else {
			plain(i18n.T("capture.quickSave"))
		}
	case key == "fileNameTemplate" && v.kind == scalarValue && v.scalar == "":
		plain(i18n.T("profiles.defaultFilename"))
	case key == "toolbarTools":
		switch {
		case v.kind == scalarValue && v.scalar == nil:
			plain(i18n.T("capture.defaultToolbar"))
		case v.kind == arrayValue && len(v.items) > 0:
			names := make([]string, len(v.items))
			for i, tool := range v.items {
				names[i] = i18n.T("capture." + tool.text())
			}
			plain(strings.Join(names, "、"))
		default:
			plain(i18n.T("profiles.none"))
		}
	case slices.Contains([]string{"outputDirectoryRef", "programRef", "pathRef", "processPathRef"}, key):
		location := ""
		if id, ok := v.scalar.(string); ok {
			for _, path := range paths {
				if path.ID == id {
					location = path.Value
				}
			}
		}
		if location == "" {
			location = i18n.T("profiles.pendingLocation")
		}
		kit.Mono(c, location).Selectable()
	case key == "code":
		// An action script is shown as text, never run or rendered.
		ui.Scroll(c).FillWidth().MaxHeight(240).Padding(8, 10).Radius(t.Radius).Background(t.Surface).Children(func() {
			kit.Mono(c, v.text()).Selectable()
		})
	case v.kind == scalarValue && isBool(v):
		if v.scalar.(bool) {
			plain(i18n.T("profiles.on"))
		} else {
			plain(i18n.T("profiles.off"))
		}
	case v.kind == arrayValue:
		if len(v.items) == 0 {
			plain(i18n.T("profiles.none"))
			return
		}
		for i, item := range v.items {
			ui.Row(c.Key(i)).FillWidth().Gap(8).AlignItems(ui.Start).Children(func() {
				kit.Help(c, "•").Shrink(0)
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() { showField(c, "", item, paths) })
			})
		}
	case v.kind == objectValue:
		reviewSettings(c, v, paths, true)
	default:
		text := v.text()
		if text == "" {
			text = i18n.T("profiles.unset")
		}
		plain(text)
	}
}

func isBool(v value) bool {
	_, ok := v.scalar.(bool)
	return ok
}
