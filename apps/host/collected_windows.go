//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"fyne.io/systray"
	"winone/internal/host"
	"winone/internal/modules"
)

// Menu polling must not restart an idle process; explicit icon actions still
// pass through the module's normal enablement and configuration guards.
type observedIcons struct {
	*modules.Gate
	worker *modules.Lazy
}

func (m observedIcons) Handle(request host.Request) host.Response {
	if request.Method != "icons" {
		return m.Gate.Handle(request)
	}
	if response, called := m.worker.HandleRunning(request); called {
		return response
	}
	return host.Response{ID: request.ID, Error: &host.RPCError{Code: "stopped", Message: "组件已退出"}}
}

// syncCollectedMenu mirrors stable native icon IDs, with explicit access to each original menu.
func syncCollectedMenu(ctx context.Context, worker moduleEndpoint, language func() string, reportFailure func(error)) {
	type icon struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Image  []byte `json:"image"`
		Hosted bool   `json:"hosted"`
	}
	type item struct {
		root    *systray.MenuItem
		cancel  context.CancelFunc
		visual  *collectedIconState
		actions map[string]*systray.MenuItem
		locale  string
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
			if state := worker.Status().State; state != "ready" {
				for id, value := range items {
					value.cancel()
					value.root.Remove()
					delete(items, id)
				}
				continue
			}
			r := worker.Handle(host.Request{Version: 1, ID: "tray-icons", Method: "icons"})
			if r.Error != nil {
				continue
			}
			var icons []icon
			if json.Unmarshal(r.Result, &icons) != nil {
				continue
			}
			seen := map[string]bool{}
			locale := language()
			for _, value := range icons {
				seen[value.ID] = true
				existing, ok := items[value.ID]
				if ok {
					if existing.locale != locale {
						for _, action := range collectedActions(value.Hosted, locale) {
							if entry := existing.actions[action.ID]; entry != nil {
								entry.SetTitle(action.Title)
							}
						}
						existing.locale = locale
						items[value.ID] = existing
					}
					if err := existing.visual.apply(existing.root, value.Name, value.Image); err != nil {
						fmt.Fprintln(os.Stderr, "tray icon update:", err)
					}
					continue
				}
				root := systray.AddMenuItemFirst(value.Name, value.Name)
				visual := &collectedIconState{name: value.Name}
				if err := visual.apply(root, value.Name, value.Image); err != nil {
					fmt.Fprintln(os.Stderr, "tray icon update:", err)
				}
				itemCtx, cancel := context.WithCancel(ctx)
				actions := map[string]*systray.MenuItem{}
				for _, action := range collectedActions(value.Hosted, locale) {
					entry := root.AddSubMenuItem(action.Title, "")
					actions[action.ID] = entry
					go func(id, name string, hosted bool, action collectedAction, entry *systray.MenuItem) {
						for {
							var shift bool
							select {
							case <-itemCtx.Done():
								return
							case value, ok := <-entry.ClickedWithShiftCh:
								if !ok {
									return
								}
								shift = value
							}
							selectedAction := collectedGesture(action, hosted, shift)
							// Match WinTray: Shift selects the distinct double-click
							// gesture; never follow a single click with a guessed retry.
							if err := performCollectedAction(worker, id, hosted, selectedAction); err != nil {
								reportFailure(fmt.Errorf("%s (%s): %w", name, action.ID, err))
							}
						}
					}(value.ID, value.Name, value.Hosted, action, entry)
				}
				items[value.ID] = item{root: root, cancel: cancel, visual: visual, actions: actions, locale: locale}
			}
			for id, value := range items {
				if !seen[id] {
					value.cancel()
					value.root.Remove()
					delete(items, id)
				}
			}
		}
	}
}
