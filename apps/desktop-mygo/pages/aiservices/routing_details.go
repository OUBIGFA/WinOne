package aiservices

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// Known trace fields have product labels. Unknown fields remain available in
// Complete evidence; they must not disappear on the next runtime upgrade.
func traceFields(c *ui.Context, raw json.RawMessage, mask func(string) string, omit ...string) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		// Arrays may contain scalar evidence (token samples, timings or flags).
		// Keep these visible instead of rendering empty rows for non-objects.
		text := string(bytes.TrimSpace(raw))
		if strings.HasPrefix(text, "\"") {
			json.Unmarshal(raw, &text)
		}
		kit.Help(c, mask(text)).Selectable()
		return
	}
	skipped := map[string]bool{}
	for _, key := range omit {
		skipped[key] = true
	}
	for _, key := range sortedKeys(fields) {
		if skipped[key] || !i18n.Has("aiTrace."+key) {
			continue
		}
		value := bytes.TrimSpace(fields[key])
		if len(value) == 0 || bytes.Equal(value, []byte("null")) {
			continue
		}
		label := i18n.T("aiTrace." + key)
		if value[0] == '{' || value[0] == '[' {
			var list []json.RawMessage
			if value[0] == '[' {
				json.Unmarshal(value, &list)
			}
			// Flat lists (models, renewals, group paths) are readable inline.
			var flat []string
			for _, item := range list {
				var s string
				if json.Unmarshal(item, &s) != nil {
					flat = nil
					break
				}
				flat = append(flat, s)
			}
			if len(flat) > 0 {
				kit.Help(c, label+": "+mask(strings.Join(flat, " · ")))
				continue
			}
			open := ui.Local(ui.Column(c.Key(key)), "details", func() bool { return false })
			ui.Collapsible(c, label, open, func() {
				if value[0] == '{' {
					traceFields(c, value, mask)
				} else {
					for index, item := range list {
						ui.Column(c.Key(index)).FillWidth().Gap(5).Padding(8, 0).Children(func() { traceFields(c, item, mask) })
					}
				}
			})
			continue
		}
		text := string(value)
		if value[0] == '"' {
			json.Unmarshal(value, &text)
		}
		if text == "true" {
			text = i18n.T("aiTrace.yes")
		} else if text == "false" {
			text = i18n.T("aiTrace.no")
		}
		kit.Help(c, label+": "+mask(text)).Selectable()
	}
}

func traceBlock(c *ui.Context, text string) {
	ui.Scroll(c).FillWidth().MaxHeight(320).Padding(10).Background(c.Theme().Surface).Radius(kit.PanelRadius).Children(func() { kit.Mono(c, text).MinWidth(0).Selectable() })
}

func (t *nativeTrace) bodyView(c *ui.Context, p *page) {
	mask := p.routing.accountMasker()
	var data map[string]json.RawMessage
	if !nativeDecode(&t.detailAPI, t.detail, &data) {
		return
	}
	switch t.detailKind {
	case "body":
		for _, kind := range []string{"request", "response"} {
			var body string
			var cut bool
			json.Unmarshal(data[kind+"Body"], &body)
			json.Unmarshal(data[kind+"Truncated"], &cut)
			ui.Column(c.Key(kind)).FillWidth().Gap(8).Children(func() {
				kit.Strong(c, i18n.T("usage.archive"+kind))
				if cut {
					kit.Help(c, i18n.T("usage.archiveTruncated"))
				}
				traceBlock(c, mask(body))
			})
		}
		if len(data["headers"]) > 0 {
			open := ui.Local(ui.Column(c), "headers", func() bool { return false })
			ui.Collapsible(c, i18n.T("usage.archiveHeaders"), open, func() { routingJSON(c, data["headers"], mask) })
		}
	case "held":
		var report string
		json.Unmarshal(data["text"], &report)
		traceBlock(c, mask(report))
	case "context":
		var value struct {
			From  string `json:"from"`
			Items []struct {
				Name   string `json:"name"`
				Tokens int    `json:"tokens"`
				Text   string `json:"text"`
				Pieces []struct {
					Role string `json:"role"`
					Name string `json:"name"`
					Src  string `json:"src"`
					Size int    `json:"size"`
					Text string `json:"text"`
				} `json:"pieces"`
			} `json:"items"`
		}
		if !nativeDecode(&t.detailAPI, t.detail, &value) {
			return
		}
		for index, item := range value.Items {
			ui.Column(c.Key(index)).FillWidth().Gap(8).Children(func() {
				kit.Strong(c, mask(fmt.Sprintf("%s · %d tokens", item.Name, item.Tokens)))
				if item.Text != "" {
					traceBlock(c, mask(item.Text))
				}
				for j, piece := range item.Pieces {
					node := ui.Column(c.Key(j)).FillWidth().Gap(4)
					node.Children(func() {
						kit.Label(c, piece.Role+" · "+mask(piece.Name))
						if piece.Text != "" {
							traceBlock(c, mask(piece.Text))
						}
						if piece.Size > 0 {
							kit.Help(c, fmt.Sprintf("%d bytes", piece.Size))
						}
						if piece.Src != "" {
							// Context returns bounded inline image data; decode once per
							// piece and never fetch external links while reading a record.
							type decoded struct {
								source ui.ImageSource
								err    error
							}
							picture := ui.Local(node, "image:"+piece.Src, func() decoded {
								header, encoded, ok := strings.Cut(piece.Src, ",")
								if !ok || !strings.HasPrefix(header, "data:image/") || !strings.HasSuffix(header, ";base64") {
									return decoded{err: fmt.Errorf("%s", nativeT("invalidResponse"))}
								}
								data, err := base64.StdEncoding.DecodeString(encoded)
								if err != nil {
									return decoded{err: err}
								}
								source, err := ui.DecodeImage(data)
								return decoded{source, err}
							})
							if picture.err != nil {
								kit.Help(c, picture.err.Error())
							} else {
								ui.Image(c, picture.source).MaxWidthPercent(100).MaxHeight(320).Fit(ui.Contain).Label(mask(piece.Name))
							}
						}
					})
				}
			})
		}
	}
}
