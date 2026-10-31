package interconnect

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// React source: features/interconnect-transfer.tsx.

// transferForm is the composer's draft, the shape the transfer services
// take.
type transferForm struct {
	DerpMapURL      string         `json:"derpMapURL"`
	AllowWrites     bool           `json:"allowWrites"`
	Listen          string         `json:"listen"`
	Paths           []string       `json:"paths"`
	Text            string         `json:"text"`
	Code            string         `json:"code"`
	OutputDir       string         `json:"outputDir"`
	Options         map[string]any `json:"options"`
	Socks5          string         `json:"socks5"`
	HTTPProxy       string         `json:"httpProxy"`
	StoreOrigin     string         `json:"storeOrigin"`
	Downloads       float64        `json:"downloads"`
	ExpirationHours float64        `json:"expirationHours"`
}

func initialForm() transferForm {
	return transferForm{Listen: "127.0.0.1:0", Paths: []string{}, Options: map[string]any{}, StoreOrigin: "https://gofile.io", Downloads: 1, ExpirationHours: 24}
}

func (f transferForm) clone() transferForm {
	f.Paths = slices.Clone(f.Paths)
	f.Options = maps.Clone(f.Options)
	if f.Options == nil {
		f.Options = map[string]any{}
	}
	return f
}

const (
	gofileOrigin = "https://gofile.io"
	crocOrigin   = "https://getcroc.com"
)

func presetOrigin(origin string) bool { return origin == gofileOrigin || origin == crocOrigin }

// comparable is the form as unsent input: choosing a cloud service is a
// preference, so a preset origin does not count; a custom one does.
func (f transferForm) comparable() string {
	if presetOrigin(f.StoreOrigin) {
		f.StoreOrigin = ""
	}
	data, _ := json.Marshal(f)
	return string(data)
}

// transferPanel sends and receives: files or text by croc code, a lasting
// share (LAN pool, relay and code sender in one worker) or a cloud copy;
// below it, the tasks with their progress and results. File bytes stay in
// the worker processes; the window only names paths.
type transferPanel struct {
	env       *core.Env
	stored    bool
	form      transferForm
	saved     transferForm
	direction string
	payload   string
	// shareTarget is the live share new files join: "" the latest,
	// "new" a new share.
	shareTarget string
	croc        string
	items       []taskRecord
	busy        bool
	err         string
	message     string
	answers     map[string]*string
	reveal      string
	polling     bool
	polled      time.Time
	generation  int
	closed      bool
	optionTexts map[string]*string
	disclosures map[string]bool
	numbers     struct{ downloads, expiration float64 }
	tasks       map[string]*peerTask
}

func newTransferPanel(env *core.Env, stored bool) *transferPanel {
	p := &transferPanel{env: env, stored: stored, form: initialForm(), saved: initialForm(), direction: "send", payload: "files",
		croc: crocOrigin, answers: map[string]*string{}, optionTexts: map[string]*string{}, disclosures: map[string]bool{}, tasks: map[string]*peerTask{}}
	return p
}

func (p *transferPanel) gofile() bool { return p.stored && p.form.StoreOrigin == gofileOrigin }

func (p *transferPanel) source() string {
	if p.stored {
		return "interconnect-store"
	}
	return "interconnect-transfer"
}

// Report records unsent input; reverting keeps the chosen cloud service.
func (p *transferPanel) Report() {
	unsent := p.form.comparable() != p.saved.comparable()
	p.env.Activity.Report(p.source(), unsent, p.busy, func() {
		origin := p.form.StoreOrigin
		p.setForm(p.saved.clone())
		if presetOrigin(origin) {
			p.form.StoreOrigin = origin
		}
	})
}

// setForm replaces the draft; option fields show its values again.
func (p *transferPanel) setForm(form transferForm) {
	p.form = form.clone()
	p.optionTexts = map[string]*string{}
}

// Apply takes a prefill: files to send, or a code to receive.
func (p *transferPanel) Apply(intent core.Intent) {
	p.direction = "send"
	if intent.Receive {
		p.direction = "receive"
	}
	p.payload = "files"
	p.add(intent.Paths)
	if intent.Code != "" {
		p.form.Code = intent.Code
	}
}

// add selects paths once each, by their case-insensitive name.
func (p *transferPanel) add(paths []string) {
	seen := map[string]int{}
	var all []string
	for _, path := range append(slices.Clone(p.form.Paths), paths...) {
		key := strings.ToLower(path)
		if i, ok := seen[key]; ok {
			all[i] = path
			continue
		}
		seen[key] = len(all)
		all = append(all, path)
	}
	if all == nil {
		all = []string{}
	}
	p.form.Paths = all
}

func (p *transferPanel) refresh() {
	p.generation++
	version := p.generation
	p.polling = true
	core.Go(p.env, func() ([]taskRecord, error) {
		var items []taskRecord
		err := p.env.Host.CallCompletingInto("interconnect.transfer_list", nil, &items)
		return items, err
	}, func(items []taskRecord, err error) {
		p.polling, p.polled = false, time.Now()
		if p.closed || version != p.generation {
			return
		}
		if err != nil {
			p.err = err.Error()
			return
		}
		p.items = items
	})
}

// action runs one operation, then reads the tasks again.
func (p *transferPanel) action(work func() error, done func()) {
	p.busy, p.err, p.message = true, "", ""
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

func (p *transferPanel) copy(c *ui.Context, value string) {
	c.Services().WriteClipboard(value)
	p.message = tr("copiedTransfer")
}

func (p *transferPanel) liveShares() []taskRecord {
	var shares []taskRecord
	for _, item := range p.items {
		if item.Spec.Mode == "share_server" && item.State.Status == "running" && item.State.Ready {
			shares = append(shares, item)
		}
	}
	return shares
}

// target is the share new files join, nil for a new one.
func (p *transferPanel) target() *taskRecord {
	shares := p.liveShares()
	switch {
	case p.shareTarget == "new" || len(shares) == 0:
		return nil
	case p.shareTarget == "":
		return &shares[0]
	}
	for i := range shares {
		if shares[i].ID == p.shareTarget {
			return &shares[i]
		}
	}
	return nil
}

// start sends or receives with the draft, as the chosen mode does.
func (p *transferPanel) start(mode string, override func(*transferForm)) {
	draft := p.form.clone()
	if override != nil {
		override(&draft)
	}
	payload, target := p.payload, p.target()
	gofile := p.gofile()
	stored := p.stored
	var started struct {
		ID string `json:"id"`
	}
	reset := true
	p.action(func() error {
		call := func(method string, params any, result any) error {
			return p.env.Host.CallCompletingInto("interconnect."+method, params, result)
		}
		if mode == "store_inspect" && gofile {
			reset = false
			return call("transfer_open_cloud", map[string]any{"url": strings.TrimSpace(draft.Code)}, nil)
		}
		sending := mode == "send" || mode == "store_send"
		paths, text := draft.Paths, ""
		if payload == "text" && sending {
			paths, text = []string{}, draft.Text
		}
		options := maps.Clone(draft.Options)
		if mode == "receive" {
			options["transport"] = "auto"
		}
		switch {
		case !stored && mode == "send" && target != nil:
			p.env.Post(func() { p.reveal = target.ID })
			if payload == "text" {
				return call("share_add_text", map[string]any{"id": target.ID, "text": text}, nil)
			}
			return call("share_add", map[string]any{"id": target.ID, "paths": paths}, nil)
		case !stored && mode == "send":
			// One worker owns the LAN pool, the lasting relay and the code
			// sender.
			err := call("transfer_start", map[string]any{"mode": "share_server", "paths": paths, "text": text, "derpMapURL": draft.DerpMapURL,
				"allowWrites": draft.AllowWrites, "transfer": map[string]any{"code": draft.Code, "options": draft.Options, "socks5": draft.Socks5, "httpProxy": draft.HTTPProxy}}, &started)
			if err == nil {
				id := started.ID
				p.env.Post(func() { p.shareTarget, p.reveal = id, id })
			}
			return err
		case !stored && mode == "receive" && strings.HasPrefix(strings.TrimSpace(draft.Code), "tc"):
			return call("transfer_start", map[string]any{"mode": "share_client", "code": strings.TrimSpace(draft.Code), "listen": draft.Listen, "derpMapURL": draft.DerpMapURL}, nil)
		}
		spec := map[string]any{"mode": mode, "paths": paths, "text": text, "code": draft.Code, "outputDir": draft.OutputDir, "options": options,
			"socks5": draft.Socks5, "httpProxy": draft.HTTPProxy, "storeOrigin": draft.StoreOrigin, "downloads": draft.Downloads, "expirationHours": draft.ExpirationHours}
		err := call("transfer_start", spec, &started)
		if err == nil && (mode == "store_send" || mode == "store_inspect") {
			id := started.ID
			p.env.Post(func() { p.reveal = id })
		}
		return err
	}, func() {
		if reset {
			next := p.form.clone()
			next.Paths, next.Text = []string{}, ""
			p.setForm(next)
			p.saved = next.clone()
		}
	})
}

func (p *transferPanel) Close() {
	p.closed = true
	for _, task := range p.tasks {
		task.Close()
	}
	p.env.Activity.Report(p.source(), false, false, nil)
}

// View shows the composer and the tasks; active polls the tasks.
func (p *transferPanel) View(c *ui.Context, active, settingsPending bool) {
	if active && !p.polling && time.Since(p.polled) >= 750*time.Millisecond {
		p.refresh()
	}
	if active {
		c.After(750 * time.Millisecond)
	}
	// A pasted share picks its service without another click.
	if p.stored {
		code := strings.TrimSpace(p.form.Code)
		switch {
		case strings.HasPrefix(code, "https://gofile.io/d/"):
			p.form.StoreOrigin = gofileOrigin
		case strings.HasPrefix(code, "croc-store-v1.") || strings.HasPrefix(code, "https://getcroc.com/s/"):
			p.form.StoreOrigin = p.croc
		}
	}
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		if p.err != "" {
			kit.Notice(c, kit.Failure, "", p.err, nil)
		}
		if p.message != "" {
			kit.Status(c, p.message)
		}
		p.composer(c, active, settingsPending)
		p.tasksView(c)
	})
}

func (p *transferPanel) composer(c *ui.Context, active, settingsPending bool) {
	f := &p.form
	gofile := p.gofile()
	target := p.target()
	label := "sendOrReceive"
	if p.stored {
		label = "store"
	}
	kit.Panel(c, func() {
		ui.Column(c).FillWidth().Padding(16).Gap(14).Role(ui.RoleGroup).Label(tr(label)).Disabled(p.busy).Children(func() {
			if p.stored {
				service := "croc"
				if gofile {
					service = "gofile"
				}
				kit.Row(c, tr("cloudService"), "", func() {
					if kit.Segmented(c, &service, []kit.SelectOption{{Value: "gofile", Label: "GoFile"}, {Value: "croc", Label: tr("crocCloud")}}, tr("cloudService")) {
						if !gofile {
							p.croc = f.StoreOrigin
						}
						if service == "gofile" {
							f.StoreOrigin = gofileOrigin
						} else {
							f.StoreOrigin = p.croc
						}
					}
				})
			}
			ui.Row(c).FillWidth().GapX(16).GapY(10).Wrap().Children(func() {
				kit.Segmented(c, &p.direction, []kit.SelectOption{{Value: "send", Label: tr("send")}, {Value: "receive", Label: tr("receive")}}, tr("direction"))
				if p.direction == "send" {
					kit.Segmented(c, &p.payload, []kit.SelectOption{{Value: "files", Label: tr("sendFiles")}, {Value: "text", Label: tr("sendText")}}, tr("sendContent"))
				}
			})
			if p.direction == "send" {
				p.sendInput(c, active, target)
			} else {
				p.receiveInput(c, gofile)
			}
			if settingsPending {
				kit.Help(c, tr("saveBeforeSending"))
			}
			p.submit(c, gofile, target, settingsPending)
			if !(gofile && p.direction == "receive") {
				p.advanced(c, gofile, target)
			}
		})
	})
}

func (p *transferPanel) sendInput(c *ui.Context, active bool, target *taskRecord) {
	f := &p.form
	if shares := p.liveShares(); !p.stored && len(shares) > 0 {
		kit.Stack(c, tr("sendToShare"), "", func() {
			options := make([]kit.SelectOption, 0, len(shares)+1)
			title := func(item taskRecord) string {
				name := tr("publishShare")
				if item.State.Share != nil && len(item.State.Share.Items) > 0 {
					name = item.State.Share.Items[0].Title
				}
				return tr("continueShare") + " · " + name
			}
			for _, item := range shares {
				options = append(options, kit.SelectOption{Value: item.ID, Label: title(item)})
			}
			options = append(options, kit.SelectOption{Value: "new", Label: tr("newShare")})
			value := "new"
			if target != nil {
				value = target.ID
			}
			if changed, trigger := kit.Select(c, &value, options); changed {
				p.shareTarget = value
			} else {
				trigger.Label(tr("sendToShare")).FillWidth()
			}
		})
	}
	if p.payload == "text" {
		ui.TextArea(c, &f.Text).FillWidth().MinHeight(144).Placeholder(tr("textPlaceholder")).Label(tr("sendText"))
		return
	}
	theme, pal := c.Theme(), kit.Colors(c)
	zone := ui.Column(c).FillWidth().Padding(20).Gap(12).AlignItems(ui.Center).Radius(kit.PanelRadius).
		Border(1, pal.Hairline).Background(theme.Surface)
	zone.Children(func() {
		ui.Icon(c, icons.Upload).FontSize(24).TextColor(theme.TextMuted)
		kit.Help(c, tr("drop"))
		ui.Row(c).Gap(8).Wrap().Justify(ui.Center).Children(func() {
			if kit.Button(c, tr("chooseFiles"), icons.FilePlus, false).Clicked() {
				var paths []string
				p.action(func() (err error) { paths, err = p.env.Dialogs.PickFiles(tr("chooseFiles"), core.Filter{}); return err }, func() { p.add(paths) })
			}
			if kit.Button(c, tr("chooseFolder"), icons.FolderPlus, false).Clicked() {
				folder := ""
				p.action(func() (err error) { folder, err = p.env.Dialogs.PickFolder(tr("chooseFolder")); return err }, func() {
					if folder != "" {
						p.add([]string{folder})
					}
				})
			}
			if kit.Button(c, tr("paste"), icons.ClipboardPaste, false).Clicked() {
				files, err := c.Services().ReadClipboardData(transfer.FileList).Files()
				if err != nil || len(files) == 0 {
					p.err = i18n.T("fileDrop.localOnly")
				} else {
					p.add(files)
				}
			}
		})
		if len(f.Paths) > 0 {
			ui.Column(c).FillWidth().Gap(6).Children(func() {
				ui.Row(c).FillWidth().Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
					kit.Label(c, tr("selectedFiles", "count", len(f.Paths)))
					if kit.GhostButton(c, tr("clear"), nil, false).Clicked() {
						f.Paths = []string{}
					}
				})
				for _, path := range slices.Clone(f.Paths) {
					ui.Row(c.Key(path)).FillWidth().Gap(8).AlignItems(ui.Start).Children(func() {
						ui.Text(c, path).Grow(1).MinWidth(0).Tooltip(path)
						if kit.IconButton(c, icons.Trash2, tr("remove")+" "+path).Clicked() {
							f.Paths = slices.DeleteFunc(slices.Clone(f.Paths), func(v string) bool { return v == path })
						}
					})
				}
			})
		}
	})
	widgets.DropArea(c, p.env, zone, widgets.DropAny, true, p.busy || !active, func(paths []string, err error) {
		if err != nil {
			p.err = err.Error()
			return
		}
		p.add(paths)
	})
}

func (p *transferPanel) receiveInput(c *ui.Context, gofile bool) {
	f := &p.form
	theme := c.Theme()
	ui.Column(c).FillWidth().Gap(10).AlignItems(ui.Center).Padding(12, 0).Children(func() {
		ui.Icon(c, icons.Download).FontSize(24).TextColor(theme.TextMuted)
		heading, placeholder := "enterReceiveCode", "receivePlaceholder"
		if gofile {
			heading, placeholder = "enterCloudLink", "cloudLinkPlaceholder"
		}
		kit.Strong(c, tr(heading))
		ui.TextInput(c, &f.Code).FillWidth().Placeholder(tr(placeholder)).Label(tr("code"))
		if kit.Button(c, tr("clipboardCode"), icons.ClipboardPaste, false).Clicked() {
			f.Code = c.Services().ReadClipboard()
		}
	})
}

func (p *transferPanel) submit(c *ui.Context, gofile bool, target *taskRecord, settingsPending bool) {
	f := &p.form
	label := "receiveNow"
	mode := "receive"
	icon := icons.Download
	switch {
	case p.stored && p.direction == "send":
		label, mode, icon = "uploadAndShare", "store_send", icons.Send
	case p.stored && gofile:
		label, mode = "openCloudDownload", "store_inspect"
	case p.stored:
		label, mode = "inspect", "store_inspect"
	case p.direction == "send" && target != nil:
		label, mode, icon = "addToShare", "send", icons.Send
	case p.direction == "send":
		label, mode, icon = "sendSelection", "send", icons.Send
	}
	empty := strings.TrimSpace(f.Code) == ""
	if p.direction == "send" {
		empty = len(f.Paths) == 0
		if p.payload == "text" {
			empty = strings.TrimSpace(f.Text) == ""
		}
	}
	updating := !p.stored && p.direction == "send" && target != nil && target.State.Updating
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		ui.Row(c).Children(func() {
			if kit.PrimaryButton(c, tr(label), icon, p.busy).Disabled(settingsPending || updating || empty).Clicked() {
				p.start(mode, nil)
			}
		})
		hint := "sendOnlineHint"
		switch {
		case p.stored && gofile:
			hint = "gofileHint"
		case p.stored:
			hint = "crocHint"
		case p.direction == "receive":
			hint = "receiveSteps"
		case target != nil:
			hint = "appendShareHint"
		}
		kit.Help(c, tr(hint))
		if gofile {
			kit.Help(c, tr("gofileRetention"))
		}
	})
}

// optionText is the text of an option field, kept while the form is.
func (p *transferPanel) optionText(key string) *string {
	if text := p.optionTexts[key]; text != nil {
		return text
	}
	value := ""
	switch v := p.form.Options[key].(type) {
	case string:
		value = v
	case []any:
		var parts []string
		for _, part := range v {
			if s, ok := part.(string); ok {
				parts = append(parts, s)
			}
		}
		value = strings.Join(parts, ",")
	case []string:
		value = strings.Join(v, ",")
	}
	p.optionTexts[key] = &value
	return &value
}

func splitList(text string) []string {
	parts := []string{}
	for _, part := range strings.Split(text, ",") {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

// disclosure is a collapsible group whose open state the panel keeps.
func (p *transferPanel) disclosure(c *ui.Context, key, title string, fn func()) {
	open := p.disclosures[key]
	ui.Collapsible(c, title, &open, func() {
		ui.Column(c).FillWidth().Gap(12).Padding(8, 0, 4).Children(fn)
	}).FontSize(kit.HelpSize(c))
	p.disclosures[key] = open
}

func (p *transferPanel) textField(c *ui.Context, label string, value *string) {
	kit.Stack(c, label, "", func() { ui.TextInput(c, value).FillWidth().Label(label) })
}

func (p *transferPanel) optionField(c *ui.Context, key, label, placeholder string, list bool) {
	text := p.optionText(key)
	kit.Stack(c, label, "", func() {
		in := ui.TextInput(c, text).FillWidth().Label(label)
		if placeholder != "" {
			in.Placeholder(placeholder)
		}
		if in.Changed() {
			if list {
				p.form.Options[key] = splitList(*text)
			} else {
				p.form.Options[key] = *text
			}
		}
	})
}

func (p *transferPanel) optionToggle(c *ui.Context, key, label string) {
	on := p.form.Options[key] == true
	if kit.Toggle(c, &on, label, "").Changed() {
		p.form.Options[key] = on
	}
}

func (p *transferPanel) optionSelect(c *ui.Context, label, value string, choices []kit.SelectOption, change func(string)) {
	kit.Stack(c, label, "", func() {
		changed, trigger := kit.Select(c, &value, choices)
		trigger.Label(label).FillWidth()
		if changed {
			change(value)
		}
	})
}

func (p *transferPanel) advanced(c *ui.Context, gofile bool, target *taskRecord) {
	f := &p.form
	title := "advancedOptions"
	if p.stored {
		title = "shareSettings"
	}
	send := p.direction == "send"
	p.disclosure(c, "advanced", tr(title), func() {
		ui.Column(c).FillWidth().Gap(12).Disabled(p.busy || !p.stored && send && target != nil).Children(func() {
			if p.stored && send {
				if !gofile {
					p.textField(c, tr("origin"), &f.StoreOrigin)
					kit.Help(c, tr("storeOriginHint"))
					ui.Row(c).FillWidth().Gap(12).Wrap().Children(func() {
						ui.Column(c).Basis(200).Grow(1).Children(func() {
							kit.Stack(c, tr("downloads"), "", func() {
								kit.NumberField(c, &f.Downloads, 0, 1e6, 1, "", tr("downloads")).FillWidth()
							})
						})
						ui.Column(c).Basis(200).Grow(1).Children(func() {
							kit.Stack(c, tr("expiration"), "", func() {
								kit.NumberField(c, &f.ExpirationHours, 0, 1e6, 1, "", tr("expiration")).FillWidth()
							})
						})
					})
				}
				limits := "storeFilesOnly"
				if gofile {
					limits = "gofileLimits"
				}
				kit.Help(c, tr(limits))
				kit.Help(c, tr("cloudPayloadHint"))
			}
			if !send && !gofile {
				kit.Stack(c, tr("output"), "", func() {
					ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
						ui.TextInput(c, &f.OutputDir).Grow(1).MinWidth(0).Label(tr("output"))
						if kit.Button(c, tr("choose"), icons.FolderPlus, false).Clicked() {
							folder := ""
							p.action(func() (err error) { folder, err = p.env.Dialogs.PickFolder(tr("output")); return err }, func() {
								if folder != "" {
									f.OutputDir = folder
								}
							})
						}
					})
				})
			}
			p.disclosure(c, "network", tr("networkOptions"), func() {
				kit.Help(c, tr("advancedHint"))
				if !p.stored {
					p.textField(c, tr("derpMap"), &f.DerpMapURL)
					if !send {
						p.textField(c, tr("listenPort"), &f.Listen)
					}
					p.optionField(c, "relayAddress", tr("relay"), tr("automaticRelay"), false)
					p.optionField(c, "relayAddress6", tr("relay6"), "", false)
					p.optionField(c, "relayPassword", tr("relayPassword"), "", false)
					p.optionField(c, "IP", tr("senderIP"), "", false)
					p.optionField(c, "multicastAddress", tr("multicast"), "", false)
					p.optionField(c, "relayPorts", tr("ports"), "", true)
				}
				p.textField(c, tr("socks5"), &f.Socks5)
				p.textField(c, tr("httpProxy"), &f.HTTPProxy)
				if !p.stored {
					p.optionToggle(c, "onlyLocal", tr("localOnly"))
					p.optionToggle(c, "disableLocal", tr("disableLocal"))
				}
			})
			if !p.stored && send {
				kit.Toggle(c, &f.AllowWrites, tr("allowWrites"), "")
			}
			if !p.stored {
				p.disclosure(c, "transfer", tr("transferOptions"), func() {
					if send {
						p.textField(c, tr("customCode"), &f.Code)
						transport, _ := f.Options["transport"].(string)
						if transport == "" {
							transport = "auto"
						}
						p.optionSelect(c, tr("transport"), transport, []kit.SelectOption{
							{Value: "auto", Label: tr("auto")}, {Value: "derp", Label: tr("derp")}, {Value: "relay", Label: tr("relayOnly")},
						}, func(v string) { f.Options["transport"] = v })
					}
					collision := "ask"
					if f.Options["rename"] == true {
						collision = "rename"
					} else if f.Options["overwrite"] == true {
						collision = "overwrite"
					}
					p.optionSelect(c, tr("collision"), collision, []kit.SelectOption{
						{Value: "ask", Label: tr("ask")}, {Value: "rename", Label: tr("rename")}, {Value: "overwrite", Label: tr("overwrite")},
					}, func(v string) { f.Options["rename"], f.Options["overwrite"] = v == "rename", v == "overwrite" })
					p.optionField(c, "curve", tr("curve"), "", false)
					if send {
						p.optionField(c, "hashAlgorithm", tr("hash"), "", false)
						p.optionField(c, "throttleUpload", tr("throttle"), "", false)
						p.optionToggle(c, "zipFolder", tr("zip"))
						p.optionToggle(c, "gitIgnore", tr("gitIgnore"))
						p.optionToggle(c, "noCompress", tr("noCompress"))
						p.optionToggle(c, "noMultiplexing", tr("noMulti"))
						p.optionToggle(c, "ask", tr("verifyPeer"))
						p.optionField(c, "exclude", tr("exclude"), "", true)
						p.optionField(c, "excludeFile", tr("excludeFile"), "", true)
					}
				})
			}
		})
	})
}

// records are the tasks this panel lists.
func (p *transferPanel) records() []taskRecord {
	var records []taskRecord
	gofile := p.gofile()
	for _, item := range p.items {
		mode := item.Spec.Mode
		if p.stored {
			if strings.HasPrefix(mode, "store_") && (item.Spec.StoreOrigin == gofileOrigin) == gofile {
				records = append(records, item)
			}
		} else if slices.Contains([]string{"send", "receive", "share_server", "share_client"}, mode) {
			records = append(records, item)
		}
	}
	return records
}

func (p *transferPanel) tasksView(c *ui.Context) {
	title := "receivingAndHistory"
	if p.stored {
		title = "tasks"
	}
	records := p.records()
	kit.Section(c, icons.RefreshCw, tr(title), "", func() {
		if len(records) == 0 {
			empty := "noTasks"
			if p.stored && p.direction == "send" {
				empty = "storeEmpty"
			} else if p.stored {
				empty = "storeReceiveEmpty"
			}
			kit.Help(c, tr(empty))
			return
		}
		kit.Panel(c, func() {
			for _, item := range records {
				if strings.HasPrefix(item.Spec.Mode, "share_") {
					task := p.tasks[item.ID]
					if task == nil {
						task = &peerTask{env: p.env}
						p.tasks[item.ID] = task
					}
					task.View(c, item, "share", p.busy, p.action, func(v string) { p.copy(c, v) }, p.reveal == item.ID, func() { p.reveal = "" })
					continue
				}
				p.taskView(c, item)
			}
		})
	})
}

func (p *transferPanel) taskView(c *ui.Context, item taskRecord) {
	theme := c.Theme()
	s := item.State
	r := s.Result
	if r == nil {
		r = &taskResult{}
	}
	expired := false
	if r.ExpiresAt != "" {
		if when, err := time.Parse(time.RFC3339Nano, r.ExpiresAt); err == nil && !when.After(time.Now()) {
			expired = true
		}
	}
	available := (s.Status == "completed" || !p.stored && s.Status == "running") && !r.Revoked && !expired
	code := r.Code
	if code == "" {
		code = s.Code
	}
	image := r.QR
	if image == "" {
		image = s.QR
	}
	var current, maximum float64
	description := ""
	if s.StoreProgress != nil {
		current, maximum, description = s.StoreProgress.TotalBytes, s.StoreProgress.TotalSize, s.StoreProgress.FileName
	} else if s.Progress != nil && s.Progress.Progress != nil {
		bar := s.Progress.Progress
		current, maximum, description = bar.CurrentNum, bar.Max, bar.Description
	}
	statuses := map[string]string{"running": "runningTask", "completed": "completedTask", "failed": "failedTask", "cancelled": "cancelledTask", "interrupted": "interruptedTask"}
	status := s.Status
	if key, ok := statuses[status]; ok {
		status = tr(key)
	}
	fallback := "receive"
	switch item.Spec.Mode {
	case "store_inspect":
		fallback = "inspect"
	case "store_revoke":
		fallback = "revoke"
	case "send":
		fallback = "sendText"
	}
	article := ui.Column(c.Key(item.ID)).FillWidth().Padding(14, 16).Gap(10)
	if p.reveal == item.ID && (s.Ready || s.Status != "running") {
		// The result shows once per send, not on every progress read.
		article.ScrollIntoView()
		p.reveal = ""
	}
	article.Children(func() {
		ui.Row(c).FillWidth().Gap(8).Wrap().Justify(ui.SpaceBetween).Children(func() {
			kit.Strong(c, item.title(tr(fallback))).MinWidth(0).Grow(1)
			kit.Caption(c, status)
		})
		if maximum > 0 {
			line := description + " · " + tr("fileProgress") + " " + size(current) + " / " + size(maximum)
			if s.Progress != nil && s.Progress.Progress != nil && s.Progress.Progress.KBsPerSecond > 0 {
				line += " · " + size(s.Progress.Progress.KBsPerSecond*1024) + "/s"
			}
			kit.Help(c, line)
			ui.Progress(c, min(1, current/maximum)).FillWidth().Label(tr("fileProgress"))
		}
		if s.Status == "running" && maximum == 0 && s.Prompt == nil {
			kit.Help(c, tr("waitTask"))
		}
		if available && (code != "" || image != "" || r.Link != "") {
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				if image != "" {
					heading := "scanToReceive"
					if p.stored {
						heading = "storeReady"
					}
					kit.Strong(c, tr(heading))
					qrImage(c, image, tr(heading))
				}
				if p.stored {
					kit.Help(c, tr("storeReceiveHint"))
				}
				if r.Link != "" {
					ui.Text(c, r.Link).Selectable().MinWidth(0)
				}
				if !p.stored && code != "" {
					ui.Text(c, code).Font(kit.MonoFont).FontSize(theme.Rem(1.25)).Selectable().MinWidth(0)
				}
				ui.Row(c).Gap(8).Wrap().Children(func() {
					if r.Link != "" && kit.PrimaryButton(c, tr("linkCopy"), icons.Copy, false).Clicked() {
						p.copy(c, r.Link)
					}
					if r.Service == "gofile" && r.Link != "" && kit.Button(c, tr("openCloudDownload"), icons.Download, false).Clicked() {
						link := r.Link
						p.action(func() error {
							return p.env.Host.CallCompletingInto("interconnect.transfer_open_cloud", map[string]any{"url": link}, nil)
						}, nil)
					}
					if !p.stored && code != "" && kit.Button(c, tr("codeCopy"), icons.Copy, false).Clicked() {
						p.copy(c, code)
					}
				})
				if r.ExpiresAt != "" {
					line := tr("expiresAt") + " " + formatTime(r.ExpiresAt)
					if r.Downloads > 0 {
						line += " · " + tr("downloads") + " " + kit.Itoa(r.Downloads)
					}
					kit.Help(c, line)
				}
				if p.stored && code != "" {
					p.disclosure(c, "native:"+item.ID, tr("storeNativeCode"), func() {
						ui.Text(c, code).Selectable().MinWidth(0)
						ui.Row(c).Children(func() {
							if kit.Button(c, tr("codeCopy"), icons.Copy, false).Clicked() {
								p.copy(c, code)
							}
						})
					})
				}
			})
		}
		if r.Revoked {
			kit.Help(c, tr("revoked"))
		} else if expired {
			kit.Help(c, tr("storeExpired"))
		}
		for _, warning := range []string{s.Error, s.PersistenceError, s.CleanupError, s.NotificationError} {
			if warning != "" {
				kit.Notice(c, kit.Failure, "", warning, nil)
			}
		}
		if s.Prompt != nil {
			p.promptView(c, item.ID, s.Prompt, s.Output, "transfer_answer")
		}
		if r.Manifest != nil {
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				for _, file := range r.Manifest.F {
					ui.Row(c.Key(file.N)).FillWidth().Gap(8).Wrap().Justify(ui.SpaceBetween).Children(func() {
						ui.Text(c, file.N).MinWidth(0)
						kit.Caption(c, size(file.S))
					})
				}
			})
			ui.Row(c).Children(func() {
				if kit.Button(c, tr("accept"), icons.Download, false).Disabled(p.busy).Clicked() {
					code, output := item.Spec.Code, item.Spec.OutputDir
					p.start("store_receive", func(f *transferForm) { f.Code, f.OutputDir = code, output })
				}
			})
		}
		if r.TextPath != "" {
			ui.Column(c).FillWidth().Gap(6).Children(func() {
				ui.Text(c, tr("textReceived")+" "+r.TextPath).Selectable().MinWidth(0)
				ui.Row(c).Gap(8).Wrap().Children(func() {
					id := item.ID
					if kit.Button(c, tr("openText"), nil, false).Clicked() {
						p.action(func() error {
							return p.env.Host.CallCompletingInto("interconnect.transfer_text_open", map[string]any{"id": id}, nil)
						}, nil)
					}
					if kit.Button(c, tr("copyReceivedText"), icons.Copy, false).Clicked() {
						text := ""
						p.action(func() error {
							return p.env.Host.CallCompletingInto("interconnect.transfer_text", map[string]any{"id": id}, &text)
						}, func() { p.copy(c, text) })
					}
				})
			})
		}
		ui.Row(c).Gap(8).Wrap().Children(func() {
			id := item.ID
			call := func(method string) {
				p.action(func() error {
					return p.env.Host.CallCompletingInto("interconnect."+method, map[string]any{"id": id}, nil)
				}, nil)
			}
			if s.Status == "running" {
				if kit.Button(c, tr("cancel"), icons.Square, false).Disabled(p.busy).Clicked() {
					call("transfer_stop")
				}
				return
			}
			if item.Spec.Mode == "store_send" && (r.Code != "" || r.ID != "") && !r.Revoked {
				if kit.Button(c, tr("revoke"), nil, false).Disabled(p.busy).Clicked() {
					call("share_revoke")
				}
			} else if kit.GhostButton(c, tr("forget"), icons.Trash2, false).Disabled(p.busy).Clicked() {
				call("transfer_remove")
			}
			if item.Spec.Mode != "store_revoke" && kit.GhostButton(c, tr("reuse"), icons.Play, false).Disabled(p.busy).Clicked() {
				p.reuse(item.Spec)
			}
		})
		if s.Output != "" || s.Detail != "" {
			p.disclosure(c, "details:"+item.ID, tr("details"), func() {
				text := s.Detail
				if text != "" && s.Output != "" {
					text += "\n"
				}
				ui.Text(c, text+s.Output).Font(kit.MonoFont).FontSize(kit.HelpSize(c)).Selectable().MinWidth(0)
			})
		}
	})
}

// reuse fills the composer with a past task's request.
func (p *transferPanel) reuse(spec taskSpec) {
	form := initialForm()
	data, _ := json.Marshal(spec.raw)
	_ = json.Unmarshal(data, &form)
	if form.Options == nil {
		form.Options = map[string]any{}
	}
	if form.Paths == nil {
		form.Paths = []string{}
	}
	p.setForm(form)
	p.direction = "receive"
	if strings.HasSuffix(spec.Mode, "send") {
		p.direction = "send"
	}
	p.payload = "files"
	if spec.Text != "" {
		p.payload = "text"
	}
	p.message = tr("restoreDraft")
}

// promptView answers a question the transfer asks: yes, no, or text.
func (p *transferPanel) promptView(c *ui.Context, id string, question *prompt, output, method string) {
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		kit.Strong(c, tr("confirmation"))
		text := output
		if len(text) > 4096 {
			text = text[len(text)-4096:]
		}
		ui.Text(c, text+question.Text).Font(kit.MonoFont).FontSize(kit.HelpSize(c)).Selectable().MinWidth(0)
		answer := func(value string) {
			promptID := question.ID
			p.action(func() error {
				return p.env.Host.CallCompletingInto("interconnect."+method, map[string]any{"id": id, "promptId": promptID, "answer": value}, nil)
			}, nil)
		}
		ui.Row(c).FillWidth().Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
			for _, value := range []string{"yes", "no"} {
				if kit.Button(c, tr(value), nil, false).Disabled(p.busy).Clicked() {
					answer(value)
				}
			}
			text := p.answers[id]
			if text == nil {
				text = new(string)
				p.answers[id] = text
			}
			ui.TextInput(c, text).Grow(1).MinWidth(120).Label(tr("answer"))
			if kit.Button(c, tr("reply"), nil, false).Disabled(p.busy || *text == "").Clicked() {
				answer(*text)
			}
		})
	})
}

func formatTime(value string) string {
	when, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	return when.Local().Format("2006/1/2 15:04:05")
}
