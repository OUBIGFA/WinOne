package interconnect

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/web"
)

// React source: features/interconnect-peer.tsx and CodeTransferStatus in
// interconnect-transfer.tsx.

// peerForm is a port mapping's draft.
type peerForm struct {
	Paths       []string `json:"paths"`
	Code        string   `json:"code"`
	Target      string   `json:"target"`
	Listen      string   `json:"listen"`
	RemotePort  float64  `json:"remotePort"`
	DerpMapURL  string   `json:"derpMapURL"`
	AllowWrites bool     `json:"allowWrites"`
}

func initialPeerForm() peerForm {
	return peerForm{Paths: []string{}, Target: "127.0.0.1:8080", Listen: "127.0.0.1:0", RemotePort: 1}
}

func (f peerForm) key() string {
	data, _ := json.Marshal(f)
	return string(data)
}

// peerPanel maps ports across networks: exposing a local port, or mapping
// one exposed elsewhere here. Leaving the tab never stops a service.
type peerPanel struct {
	env       *core.Env
	kind      string
	form      peerForm
	saved     string
	direction string
	records   []taskRecord
	busy      bool
	err       string
	message   string
	polling   bool
	polled    time.Time
	sequence  int
	closed    bool
	networkOn bool
	tasks     map[string]*peerTask
}

func newPeerPanel(env *core.Env, kind string) *peerPanel {
	form := initialPeerForm()
	return &peerPanel{env: env, kind: kind, form: form, saved: form.key(), direction: "server", tasks: map[string]*peerTask{}}
}

func (p *peerPanel) Report() {
	p.env.Activity.Report("interconnect-"+p.kind, p.form.key() != p.saved, p.busy, func() {
		var form peerForm
		_ = json.Unmarshal([]byte(p.saved), &form)
		p.form = form
	})
}

func (p *peerPanel) refresh() {
	p.sequence++
	version := p.sequence
	p.polling = true
	core.Go(p.env, func() ([]taskRecord, error) {
		var items []taskRecord
		err := p.env.Host.CallCompletingInto("interconnect.transfer_list", nil, &items)
		return items, err
	}, func(items []taskRecord, err error) {
		p.polling, p.polled = false, time.Now()
		if p.closed || version != p.sequence {
			return
		}
		if err != nil {
			p.err = err.Error()
			return
		}
		p.records = items
	})
}

func (p *peerPanel) action(work func() error, done func()) {
	p.busy, p.err = true, ""
	core.Do(p.env, work, func(err error) {
		p.busy = false
		if p.closed {
			return
		}
		if err != nil {
			p.err = err.Error()
		} else if done != nil {
			done()
		}
		p.refresh()
	})
}

func (p *peerPanel) Close() {
	p.closed = true
	for _, task := range p.tasks {
		task.Close()
	}
	p.env.Activity.Report("interconnect-"+p.kind, false, false, nil)
}

func (p *peerPanel) View(c *ui.Context, active bool) {
	if active && !p.polling && time.Since(p.polled) >= time.Second {
		p.refresh()
	}
	if active {
		c.After(time.Second)
	}
	f := &p.form
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		if p.err != "" {
			kit.Notice(c, kit.Failure, "", p.err, nil)
		}
		if p.message != "" {
			kit.Status(c, p.message)
		}
		title, hint, icon := "tunnel", "tunnelHint", icons.Network
		server, client := "exposePort", "mapPort"
		if p.kind == "share" {
			title, hint, icon = "persistent", "persistentHint", icons.FolderOpen
			server, client = "publishShare", "receiveShare"
		}
		kit.Group(c, icon, tr(title), tr(hint), func() {
			kit.Row(c, tr("direction"), "", func() {
				kit.Segmented(c, &p.direction, []kit.SelectOption{{Value: "server", Label: tr(server)}, {Value: "client", Label: tr(client)}}, tr("direction"))
			})
			kit.Stack(c, "", "", func() {
				if p.direction == "server" {
					kit.Label(c, tr("targetPort"))
					ui.TextInput(c, &f.Target).FillWidth().Label(tr("targetPort")).Disabled(p.busy)
				} else {
					kit.Label(c, tr("tailcatCode"))
					ui.TextInput(c, &f.Code).FillWidth().Label(tr("tailcatCode")).Disabled(p.busy)
				}
				ui.Collapsible(c, tr("networkOptions"), &p.networkOn, func() {
					ui.Column(c).FillWidth().Gap(10).Padding(8, 0).Children(func() {
						kit.Label(c, tr("derpMap"))
						ui.TextInput(c, &f.DerpMapURL).FillWidth().Label(tr("derpMap")).Disabled(p.busy)
						if p.direction == "client" {
							kit.Label(c, tr("listenPort"))
							ui.TextInput(c, &f.Listen).FillWidth().Label(tr("listenPort")).Disabled(p.busy)
						}
						if p.kind == "tunnel" {
							kit.Label(c, tr("remotePort"))
							kit.NumberField(c, &f.RemotePort, 1, 65535, 1, "", tr("remotePort"))
						}
						if p.kind == "share" && p.direction == "server" {
							kit.Toggle(c, &f.AllowWrites, tr("allowWrites"), "")
						}
					})
				}).FontSize(kit.HelpSize(c))
				label := "startService"
				if p.direction == "client" {
					label = "connectService"
				}
				empty := p.direction == "client" && strings.TrimSpace(f.Code) == "" || p.kind == "share" && p.direction == "server" && len(f.Paths) == 0
				ui.Row(c).Children(func() {
					if kit.PrimaryButton(c, tr(label), icons.Play, p.busy).Disabled(empty).Clicked() {
						form := *f
						params := map[string]any{"mode": p.kind + "_" + p.direction, "paths": form.Paths, "code": form.Code, "target": form.Target,
							"listen": form.Listen, "remotePort": form.RemotePort, "derpMapURL": form.DerpMapURL, "allowWrites": form.AllowWrites}
						p.action(func() error {
							return p.env.Host.CallCompletingInto("interconnect.transfer_start", params, nil)
						}, func() { p.saved = form.key() })
					}
				})
			})
		})
		kit.Section(c, icons.Network, tr("services"), "", func() {
			var records []taskRecord
			for _, r := range p.records {
				if strings.HasPrefix(r.Spec.Mode, p.kind+"_") {
					records = append(records, r)
				}
			}
			if len(records) == 0 {
				kit.Help(c, tr("noServices"))
				return
			}
			kit.Panel(c, func() {
				for _, item := range records {
					task := p.tasks[item.ID]
					if task == nil {
						task = &peerTask{env: p.env}
						p.tasks[item.ID] = task
					}
					task.View(c, item, p.kind, p.busy, p.action, func(v string) {
						c.Services().WriteClipboard(v)
						p.message = tr("copied")
					}, false, nil)
				}
			})
		})
	})
}

// peerTask is one share or mapping: how to reach it, its state and
// content, and its controls. Route and dialog choices stay with the task,
// so polling never resets them.
type peerTask struct {
	env       *core.Env
	route     string
	address   string
	qrOn      bool
	text      string
	save      *shareSave
	saveOpen  bool
	nativeOn  bool
	manageOn  bool
	detailsOn bool
	answer    string
	// browse shows the share's page below its actions, as React's iframe
	// did; browseURL is the address it was made for.
	browse    *web.Surface
	browseURL string
	browsing  bool
}

// Close destroys the share's page, if it was shown.
func (t *peerTask) Close() {
	if t.browse != nil {
		t.browse.Close()
		t.browse = nil
	}
}

type shareSave struct {
	itemID  string
	path    string
	replace bool
}

func (t *peerTask) View(c *ui.Context, item taskRecord, kind string, busy bool, action func(func() error, func()), copy func(string), reveal bool, revealed func()) {
	s := item.State
	server := strings.HasSuffix(item.Spec.Mode, "server")
	published := kind == "share" && server
	var addresses []shareAddress
	phoneURL, phoneQR := "", ""
	if s.Share != nil && s.Share.Info != nil {
		addresses = s.Share.Info.Addresses
		phoneURL, phoneQR = s.Share.Info.URL, s.Share.Info.QR
	}
	var selected *shareAddress
	for i := range addresses {
		if addresses[i].IP == t.address {
			selected = &addresses[i]
		}
	}
	if selected == nil && len(addresses) > 0 {
		selected = &addresses[0]
	}
	if selected != nil {
		if selected.URL != "" {
			phoneURL = selected.URL
		}
		if selected.QR != "" {
			phoneQR = selected.QR
		}
	}
	call := func(method string, params map[string]any) {
		action(func() error { return t.env.Host.CallCompletingInto("interconnect."+method, params, nil) }, nil)
	}
	id := item.ID
	article := ui.Column(c.Key(item.ID)).FillWidth().Padding(14, 16).Gap(10)
	if reveal && (s.Ready || s.Status != "running") {
		article.ScrollIntoView()
		revealed()
	}
	article.Children(func() {
		var names []string
		if s.Share != nil {
			for _, file := range s.Share.Items {
				names = append(names, file.Title)
			}
		}
		title := strings.Join(names, " · ")
		if title == "" {
			fallback := "receiveShare"
			if server {
				fallback = "publishShare"
			}
			if item.Spec.Target != "" {
				fallback = ""
				title = item.Spec.Target
			}
			if title == "" {
				title = item.title(tr(fallback))
			}
		}
		state := "serviceStopped"
		switch {
		case s.Status == "running" && s.Ready:
			state = "serviceReady"
		case s.Status == "running":
			state = "servicePreparing"
		case s.Status == "failed":
			state = "failedTask"
		}
		ui.Row(c).FillWidth().Gap(8).Wrap().Justify(ui.SpaceBetween).Children(func() {
			kit.Strong(c, title).Grow(1).MinWidth(0)
			kit.Caption(c, tr(state))
		})
		if published && s.Ready {
			t.routes(c, id, s, addresses, selected, phoneURL, phoneQR, busy, action, copy)
		}
		if published {
			ui.Collapsible(c, tr("winoneLiveShare"), &t.nativeOn, func() {
				ui.Column(c).FillWidth().Gap(8).Padding(8, 0).Children(func() {
					kit.Help(c, tr("nativeShareSteps"))
					relay := "crossNetworkConnecting"
					switch {
					case s.Status != "running":
						relay = "serviceStopped"
					case s.RelayReady:
						relay = "serviceReady"
					case s.RelayError != "":
						relay = "crossNetworkFailed"
					}
					kit.Help(c, tr(relay))
					if s.Code != "" {
						ui.Text(c, s.Code).FontSize(kit.HelpSize(c)).Selectable().MinWidth(0)
						ui.Row(c).Children(func() {
							if kit.Button(c, tr("copyNativeAddress"), icons.Copy, false).Clicked() {
								copy(s.Code)
							}
						})
					}
					if s.RelayError != "" && s.Status == "running" {
						kit.Notice(c, kit.Failure, "", tr("relayFailureHint")+"\n"+tr("errorDetails")+"："+s.RelayError, nil)
					}
				})
			}).FontSize(kit.HelpSize(c))
		}
		if !published && s.Code != "" {
			ui.Text(c, s.Code).FontSize(kit.HelpSize(c)).Selectable().MinWidth(0)
		}
		if s.Address != "" {
			ui.Text(c, tr("localAddress")+" "+s.Address).Selectable().MinWidth(0)
		}
		for _, warning := range []string{s.Error, s.PersistenceError, s.CleanupError} {
			if warning != "" {
				kit.Notice(c, kit.Failure, "", warning, nil)
			}
		}
		if s.Detail != "" {
			kit.Help(c, s.Detail)
		}
		if kind == "tunnel" && s.Ready {
			kit.Help(c, tr("activeConnections")+" "+kit.Itoa(s.Active)+" · ↑ "+formatBytes(s.Sent)+" B · ↓ "+formatBytes(s.Received)+" B")
		}
		for _, peer := range s.Peers {
			path := "relayPath"
			if peer.Direct {
				path = "directPath"
			}
			kit.Help(c, tr(path)+" · ↑ "+formatBytes(peer.Sent)+" B · ↓ "+formatBytes(peer.Received)+" B")
		}
		if s.Share != nil {
			t.manage(c, id, s, published, phoneURL, busy, action, copy)
		}
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if !published && s.Code != "" && kit.Button(c, tr("copyAddress"), icons.Copy, false).Clicked() {
				copy(s.Code)
			}
			if !published && s.Code != "" && s.QR != "" {
				label := "qr"
				if t.qrOn {
					label = "hideQR"
				}
				if kit.Button(c, tr(label), icons.QrCode, false).Clicked() {
					t.qrOn = !t.qrOn
				}
			}
			if s.Ready && kind == "share" && !server {
				if kit.Button(c, tr("browseShare"), icons.FolderOpen, false).Checked(t.browsing).Clicked() {
					t.browsing = !t.browsing
				}
				if kit.Button(c, tr("link"), icons.Copy, false).Clicked() {
					copy("http://" + s.Address)
				}
			}
			if s.Status == "running" {
				if kit.Button(c, tr("stopService"), icons.Square, false).Disabled(busy).Clicked() {
					call("transfer_stop", map[string]any{"id": id})
				}
				return
			}
			if kit.Button(c, tr("resumeService"), icons.Play, false).Disabled(busy).Clicked() {
				call("transfer_restart", map[string]any{"id": id})
			}
			if kit.GhostButton(c, tr("forget"), icons.Trash2, false).Disabled(busy).Clicked() {
				call("transfer_remove", map[string]any{"id": id})
			}
		})
		if !published && t.qrOn && s.QR != "" {
			qrImage(c, s.QR, tr("qr"))
		}
		if t.browsing && s.Ready && s.Address != "" && kind == "share" && !server {
			address := "http://" + s.Address
			if t.browse != nil && t.browseURL != address {
				t.Close()
			}
			if t.browse == nil {
				// Its files download to the Downloads folder, as the
				// iframe's did.
				t.browse = web.New(t.env, web.Options{URL: address, Label: tr("browseShare"), Height: 480, Downloads: true})
				t.browseURL = address
			}
			t.browse.View(c, false).Radius(kit.PanelRadius).Border(1, c.Theme().Border)
		}
	})
	t.saveDialog(c, id, busy, action)
}

// routes are the ways to receive a published share: across networks by
// its code sender, or directly on the LAN.
func (t *peerTask) routes(c *ui.Context, id string, s taskState, addresses []shareAddress, selected *shareAddress, phoneURL, phoneQR string, busy bool, action func(func() error, func()), copy func(string)) {
	if t.route == "" {
		t.route = "lan"
		if s.Transfer != nil {
			t.route = "internet"
		}
	}
	var options []kit.SelectOption
	if s.Transfer != nil {
		options = append(options, kit.SelectOption{Value: "internet", Label: tr("scanToReceive")})
	}
	options = append(options, kit.SelectOption{Value: "lan", Label: tr("lanDirect")})
	if len(options) > 1 {
		kit.Segmented(c, &t.route, options, tr("receiveMethod"))
	}
	if t.route == "internet" && s.Transfer != nil {
		t.codeTransfer(c, id, *s.Transfer, busy, action, copy)
		return
	}
	ui.Column(c).FillWidth().Gap(8).Role(ui.RoleGroup).Label(tr("lanReceive")).Children(func() {
		kit.Strong(c, tr("lanReceive"))
		kit.Help(c, tr("lanReceiveHint"))
		if phoneQR != "" {
			qrImage(c, "data:image/svg+xml;charset=utf-8,"+url.PathEscape(phoneQR), tr("lanQR"))
		}
		if phoneURL != "" {
			ui.Row(c).Children(func() {
				if kit.Button(c, tr("copyPhoneLink"), icons.Copy, false).Clicked() {
					copy(phoneURL)
				}
			})
		}
		if len(addresses) > 1 {
			options := make([]kit.SelectOption, len(addresses))
			for i, address := range addresses {
				options[i] = kit.SelectOption{Value: address.IP, Label: strings.TrimSpace(address.Name + " " + address.IP)}
			}
			value := ""
			if selected != nil {
				value = selected.IP
			}
			changed, trigger := kit.Select(c, &value, options)
			trigger.Label(tr("chooseAddress")).FillWidth()
			if changed {
				t.address = value
			}
		}
		kit.Help(c, tr("accessHint"))
	})
}

// codeTransfer is a share's croc code sender: browser and native routes,
// progress, questions and controls.
func (t *peerTask) codeTransfer(c *ui.Context, id string, state codeTransfer, busy bool, action func(func() error, func()), copy func(string)) {
	theme := c.Theme()
	call := func(method string, params map[string]any) {
		action(func() error { return t.env.Host.CallCompletingInto("interconnect."+method, params, nil) }, nil)
	}
	ui.Column(c).FillWidth().Gap(8).Role(ui.RoleGroup).Label(tr("internetReceive")).Children(func() {
		kit.Strong(c, tr("internetReceive"))
		hint := "cancelledTask"
		switch state.Status {
		case "completed":
			hint = "codeDelivered"
		case "failed":
			hint = "failedTask"
		case "running":
			hint = "internetReceiveHint"
		}
		kit.Help(c, tr(hint))
		if state.Status == "running" && state.BrowserURL != "" {
			if state.QR != "" {
				qrImage(c, state.QR, tr("internetQR"))
			}
			ui.Row(c).Children(func() {
				if kit.Button(c, tr("copyInternetLink"), icons.Copy, false).Clicked() {
					copy(state.BrowserURL)
				}
			})
			kit.Help(c, tr("browserReceiveSteps"))
		}
		if state.Status == "running" && state.Code != "" && state.BrowserURL == "" {
			kit.Help(c, tr("nativeCodeOnly"))
		}
		if state.Code != "" {
			ui.Text(c, state.Code).Font(kit.MonoFont).FontSize(theme.Rem(1.25)).Selectable().MinWidth(0)
		}
		if state.Progress != nil && state.Progress.Progress != nil && state.Progress.Progress.Max > 0 {
			bar := state.Progress.Progress
			kit.Help(c, bar.Description+" · "+size(bar.CurrentNum)+" / "+size(bar.Max)+" · "+size(bar.KBsPerSecond*1024)+"/s")
			ui.Progress(c, min(1, bar.CurrentNum/bar.Max)).FillWidth().Label(tr("fileProgress"))
		}
		if state.Error != "" {
			kit.Notice(c, kit.Failure, "", state.Error, nil)
		}
		if state.Prompt != nil {
			output := state.Output
			if len(output) > 4096 {
				output = output[len(output)-4096:]
			}
			ui.Text(c, output+state.Prompt.Text).Selectable().MinWidth(0)
			promptID := state.Prompt.ID
			ui.Row(c).FillWidth().Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
				for _, value := range []string{"yes", "no"} {
					if kit.Button(c, tr(value), nil, false).Disabled(busy).Clicked() {
						call("share_code_answer", map[string]any{"id": id, "promptId": promptID, "answer": value})
					}
				}
				ui.TextInput(c, &t.answer).Grow(1).MinWidth(120).Label(tr("answer"))
				if kit.Button(c, tr("reply"), nil, false).Disabled(busy || t.answer == "").Clicked() {
					call("share_code_answer", map[string]any{"id": id, "promptId": promptID, "answer": t.answer})
				}
			})
		}
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if state.Code != "" && kit.Button(c, tr("codeCopy"), icons.Copy, false).Clicked() {
				copy(state.Code)
			}
			if state.Status == "running" {
				if kit.Button(c, tr("stopCodeTransfer"), icons.Square, false).Disabled(busy).Clicked() {
					call("share_code_stop", map[string]any{"id": id})
				}
			} else if kit.Button(c, tr("sendCodeAgain"), icons.Play, false).Disabled(busy).Clicked() {
				call("share_code_restart", map[string]any{"id": id})
			}
		})
		if state.Output != "" || state.Detail != "" {
			ui.Collapsible(c, tr("details"), &t.detailsOn, func() {
				ui.Text(c, state.Detail+state.Output).Font(kit.MonoFont).FontSize(kit.HelpSize(c)).Selectable().MinWidth(0)
			}).FontSize(kit.HelpSize(c))
		}
	})
}

// manage lists and edits a share's content.
func (t *peerTask) manage(c *ui.Context, id string, s taskState, published bool, phoneURL string, busy bool, action func(func() error, func()), copy func(string)) {
	share := s.Share
	ui.Collapsible(c, tr("manageShare")+" · "+kit.Itoa(len(share.Items)), &t.manageOn, func() {
		ui.Column(c).FillWidth().Gap(10).Padding(8, 0).Children(func() {
			if published && s.Ready {
				ui.TextArea(c, &t.text).FillWidth().MinHeight(72).Placeholder(tr("textPlaceholder")).Label(tr("text")).Disabled(busy || s.Updating)
				ui.Row(c).Children(func() {
					if kit.Button(c, tr("publish"), nil, false).Disabled(busy || s.Updating || strings.TrimSpace(t.text) == "").Clicked() {
						text := t.text
						action(func() error {
							return t.env.Host.CallCompletingInto("interconnect.share_add_text", map[string]any{"id": id, "text": text}, nil)
						}, func() { t.text = "" })
					}
				})
			}
			for _, file := range share.Items {
				file := file
				ui.Row(c.Key(file.ID)).FillWidth().Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
					if file.Content != "" {
						ui.Text(c, file.Content).Grow(1).MinWidth(0).Selectable()
						if kit.GhostButton(c, tr("copy"), icons.Copy, false).Disabled(busy).Clicked() {
							copy(file.Content)
						}
					} else {
						line := file.Title + " (" + formatBytes(file.Size) + " B"
						if !file.Exists {
							line += " " + tr("missing")
						}
						for _, download := range share.Downloads {
							if download.ItemID == file.ID && download.ActiveCount > 0 {
								line += " " + i18n.T("interconnect.receivers", "count", download.ActiveCount)
							}
						}
						ui.Text(c, line+")").Grow(1).MinWidth(0)
						if published && s.Ready && phoneURL != "" {
							if kit.GhostButton(c, tr("downloadLink"), icons.Copy, false).Disabled(busy || !file.Exists).Clicked() {
								if link, err := url.Parse(phoneURL); err == nil {
									link.Path = "/api/items/" + file.ID + "/download"
									copy(link.String())
								}
							}
						}
						if kit.GhostButton(c, tr("save"), icons.Download, false).Disabled(busy || !file.Exists).Clicked() {
							var general struct {
								DownloadDir string `json:"downloadDir"`
							}
							action(func() error {
								return t.env.Host.CallCompletingInto("interconnect.general.get", nil, &general)
							}, func() {
								path := file.Title
								if general.DownloadDir != "" {
									path = general.DownloadDir + `\` + file.Title
								}
								t.save, t.saveOpen = &shareSave{itemID: file.ID, path: path}, true
							})
						}
					}
					if kit.GhostButton(c, tr("remove"), icons.Trash2, false).Disabled(busy || s.Updating).Clicked() {
						action(func() error {
							return t.env.Host.CallCompletingInto("interconnect.share_remove", map[string]any{"id": id, "itemId": file.ID}, nil)
						}, nil)
					}
				})
			}
			if s.Ready && published {
				ui.Row(c).Gap(8).Wrap().Children(func() {
					if kit.Button(c, tr("files"), icons.FilePlus, false).Disabled(busy || s.Updating).Clicked() {
						action(func() error {
							paths, err := t.env.Dialogs.PickFiles(tr("files"), core.Filter{})
							if err != nil || len(paths) == 0 {
								return err
							}
							return t.env.Host.CallCompletingInto("interconnect.share_add", map[string]any{"id": id, "paths": paths}, nil)
						}, nil)
					}
					if kit.Button(c, tr("chooseFolder"), icons.FolderPlus, false).Disabled(busy || s.Updating).Clicked() {
						action(func() error {
							path, err := t.env.Dialogs.PickFolder(tr("chooseFolder"))
							if err != nil || path == "" {
								return err
							}
							return t.env.Host.CallCompletingInto("interconnect.share_add", map[string]any{"id": id, "paths": []string{path}}, nil)
						}, nil)
					}
				})
			}
		})
	}).FontSize(kit.HelpSize(c))
}

// saveDialog saves a shared file where the user chooses.
func (t *peerTask) saveDialog(c *ui.Context, id string, busy bool, action func(func() error, func())) {
	if t.save == nil {
		return
	}
	save := t.save
	if kit.Dialog(c, &t.saveOpen, kit.DialogOptions{
		Title: tr("save"), Cancel: i18n.T("common.close"), Confirm: tr("save"), Busy: busy, ConfirmDisabled: save.path == "",
		Body: func() {
			kit.Label(c, tr("savePath"))
			ui.TextInput(c, &save.path).FillWidth().Label(tr("savePath")).Disabled(busy)
			ui.Row(c).Children(func() {
				if kit.Button(c, tr("choose"), icons.FolderOpen, false).Disabled(busy).Clicked() {
					folder := ""
					action(func() (err error) { folder, err = t.env.Dialogs.PickFolder(tr("choose")); return err }, func() {
						if folder != "" {
							name := save.path
							if i := strings.LastIndexAny(name, `\/`); i >= 0 {
								name = name[i+1:]
							}
							save.path = folder + `\` + name
						}
					})
				}
			})
			ui.Checkbox(c, &save.replace, tr("replaceExisting")).Disabled(busy)
		},
	}) {
		params := map[string]any{"id": id, "itemId": save.itemID, "path": save.path, "replace": save.replace}
		action(func() error {
			return t.env.Host.CallCompletingInto("interconnect.share_save_as", params, nil)
		}, func() { t.save, t.saveOpen = nil, false })
	}
	if !t.saveOpen && !busy {
		t.save = nil
	}
}

// formatBytes shows a byte count as a number, as the React page did.
func formatBytes(n float64) string { return strconv.FormatFloat(n, 'f', -1, 64) }
