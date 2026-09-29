//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"time"

	"fyne.io/systray"
	"winone/internal/host"
	"winone/internal/modules"
)

// syncCollectedMenu mirrors stable native icon IDs, with explicit access to each original menu.
func syncCollectedMenu(ctx context.Context, parent *systray.MenuItem, worker *modules.Process) {
	type icon struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Image []byte `json:"image"`
	}
	type item struct {
		root   *systray.MenuItem
		cancel context.CancelFunc
	}
	items := map[string]item{}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer func() {
		for _, value := range items {
			value.cancel()
			value.root.Remove()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r := worker.Handle(host.Request{Version: 1, ID: "tray-icons", Method: "icons"})
			if r.Error != nil {
				continue
			}
			var icons []icon
			if json.Unmarshal(r.Result, &icons) != nil {
				continue
			}
			seen := map[string]bool{}
			for _, value := range icons {
				seen[value.ID] = true
				existing, ok := items[value.ID]
				if ok {
					existing.root.SetTitle(value.Name)
					continue
				}
				root := parent.AddSubMenuItem(value.Name, value.Name)
				if len(value.Image) > 0 {
					if header, err := png.DecodeConfig(bytes.NewReader(value.Image)); err == nil && header.Width <= 256 && header.Height <= 256 {
						ico := make([]byte, 22)
						binary.LittleEndian.PutUint16(ico[2:], 1)
						binary.LittleEndian.PutUint16(ico[4:], 1)
						ico[6] = byte(header.Width)
						ico[7] = byte(header.Height)
						binary.LittleEndian.PutUint16(ico[10:], 1)
						binary.LittleEndian.PutUint16(ico[12:], 32)
						binary.LittleEndian.PutUint32(ico[14:], uint32(len(value.Image)))
						binary.LittleEndian.PutUint32(ico[18:], 22)
						root.SetIcon(append(ico, value.Image...))
					}
				}
				open := root.AddSubMenuItem("打开", "单击原图标")
				menu := root.AddSubMenuItem("原程序菜单", "右键原图标")
				itemCtx, cancel := context.WithCancel(ctx)
				items[value.ID] = item{root, cancel}
				go func(id string) {
					for {
						right := false
						select {
						case <-itemCtx.Done():
							return
						case <-open.ClickedCh:
						case <-menu.ClickedCh:
							right = true
						}
						params, _ := json.Marshal(map[string]any{"id": id, "right": right})
						worker.GrantForeground()
						response := worker.Handle(host.Request{Version: 1, ID: "tray-action", Method: "icon.activate", Params: params})
						if response.Error != nil {
							fmt.Fprintln(os.Stderr, response.Error.Message)
						}
					}
				}(value.ID)
			}
			for id, value := range items {
				if !seen[id] {
					value.cancel()
					value.root.Remove()
					delete(items, id)
				}
			}
			if len(items) == 0 {
				parent.Hide()
			} else {
				parent.Show()
			}
		}
	}
}
