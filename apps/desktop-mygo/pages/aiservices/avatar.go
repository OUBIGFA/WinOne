package aiservices

import (
	"encoding/base64"
	"errors"
	"os"
	"regexp"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
	"winone/internal/ai"
)

// avatarState is the picture editor's: which way of choosing is open,
// its search and address, and a pick in progress.
type avatarState struct {
	mode   string // "", "builtin" or "url"
	search string
	url    string
	busy   bool
	err    string
}

// storedAvatar is a picture the Host stored: its content hash and type.
var storedAvatar = regexp.MustCompile(`^file:[0-9a-f]{16}\.(png|jpg|gif|webp|ico|svg)$`)

// avatarChunk is how much of a picture travels in one request: the Host
// receives it in chunks, as Magpie's picker sent it.
const avatarChunk = 256 << 10

// storeAvatar uploads a picture's bytes and returns its stored reference.
// An upload that stops early is cancelled, so the Host keeps no part of it.
func storeAvatar(env *core.Env, data []byte) (string, error) {
	if len(data) == 0 || len(data) > ai.MaxAvatarBytes {
		return "", errors.New(i18n.T("ai.avatarSize"))
	}
	upload := ""
	defer func() {
		if upload != "" {
			_ = env.Host.CallCompletingInto("capture.ai.icons.cancel", map[string]string{"upload": upload}, nil)
		}
	}()
	for offset := 0; offset < len(data); offset += avatarChunk {
		end := min(offset+avatarChunk, len(data))
		var result struct {
			Upload string `json:"upload"`
			Offset int    `json:"offset"`
			Icon   string `json:"icon"`
		}
		params := map[string]any{"data": base64.StdEncoding.EncodeToString(data[offset:end]), "size": len(data), "offset": offset}
		if upload != "" {
			params["upload"] = upload
		}
		if err := env.Host.CallCompletingInto("capture.ai.icons.store", params, &result); err != nil {
			return "", err
		}
		upload = result.Upload
		if end == len(data) {
			if !storedAvatar.MatchString(result.Icon) {
				return "", errors.New(i18n.T("ai.avatarInvalidReply"))
			}
			return result.Icon, nil
		}
		if upload == "" || result.Offset != end {
			return "", errors.New(i18n.T("ai.avatarIncomplete"))
		}
	}
	return "", errors.New(i18n.T("ai.avatarIncomplete"))
}

// readAvatarFile reads a chosen or dropped picture, refusing one larger
// than the Host stores before reading it.
func readAvatarFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() || info.Size() == 0 || info.Size() > ai.MaxAvatarBytes {
		return nil, errors.New(i18n.T("ai.avatarSize"))
	}
	return os.ReadFile(path)
}

// avatarView chooses the provider's picture: a file, the service's site,
// a built-in mark or an address; reset shows the brand's again. Adapted
// from Magpie's iconPicker.
func (e *editor) avatarView(c *ui.Context, pr *provider, locked bool) {
	p, a := e.p, &e.avatar
	id := pr.ID
	disabled := locked || a.busy
	t := c.Theme()
	pick := func(work func() (string, error)) {
		a.busy, a.err = true, ""
		p.avatarBusy = true
		core.Go(p.env, work, func(icon string, err error) {
			a.busy, p.avatarBusy = false, false
			if p.editor != e || errors.Is(err, errCancel) {
				return
			}
			if err != nil {
				a.err = err.Error()
				return
			}
			widgets.RefreshProviderAvatars()
			p.change(id, func(t *provider) { t.Avatar = ptr(icon) })
			a.mode = ""
		})
	}
	group := kit.Section(c, icons.Image, i18n.T("ai.avatar"), "", func() {
		kit.Panel(c, func() {
			kit.Stack(c, "", "", func() {
				ui.Row(c).FillWidth().Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
					ui.Row(c).Size(40, 40).Radius(8).Background(t.Surface).Justify(ui.Center).AlignItems(ui.Center).Shrink(0).
						Margin(0, 8, 0, 0).Children(func() { widgets.ProviderIcon(c, p.env, pr.mark(), 32) })
					if kit.GhostButton(c, i18n.T("ai.avatarChoose"), icons.ImagePlus, false).Disabled(disabled).Clicked() {
						pick(func() (string, error) {
							path, err := p.env.Dialogs.PickFile(i18n.T("ai.avatarChoose"), core.Filter{Name: i18n.T("ai.avatarFiles"),
								Extensions: []string{"png", "jpg", "jpeg", "gif", "webp", "ico", "svg"}})
							if err != nil || path == "" {
								return "", errCancelled(err)
							}
							data, err := readAvatarFile(path)
							if err != nil {
								return "", err
							}
							return storeAvatar(p.env, data)
						})
					}
					base := strings.TrimSpace(pr.BaseURL)
					if kit.GhostButton(c, i18n.T("ai.avatarWebsite"), icons.Globe, false).Disabled(disabled || base == "").Clicked() {
						name := pr.Name
						pick(func() (string, error) {
							var result struct {
								Icon string `json:"icon"`
							}
							err := p.env.Host.CallCompletingInto("capture.ai.icons.website", map[string]string{"url": base, "name": name}, &result)
							return result.Icon, err
						})
					}
					if kit.GhostButton(c, i18n.T("ai.avatarBuiltin"), icons.Shapes, false).Checked(a.mode == "builtin").Disabled(disabled).Clicked() {
						a.mode = toggleMode(a.mode, "builtin")
					}
					if kit.GhostButton(c, i18n.T("ai.avatarURL"), icons.Link, false).Checked(a.mode == "url").Disabled(disabled).Clicked() {
						a.mode = toggleMode(a.mode, "url")
					}
					if value(pr.Avatar) != "" && kit.GhostButton(c, i18n.T("ai.avatarReset"), icons.RotateCcw, false).Disabled(disabled).Clicked() {
						p.change(id, func(t *provider) { t.Avatar = ptr("") })
						a.err = ""
					}
				})
				hint := "ai.avatarHint"
				if a.busy {
					hint = "ai.avatarLoading"
				}
				kit.Help(c, i18n.T(hint))
				if a.mode == "url" {
					ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
						ui.TextInput(c, &a.url).Grow(1).MinWidth(0).Placeholder("https://example.com/avatar.png").
							Label(i18n.T("ai.avatarURL")).Disabled(disabled)
						address := strings.TrimSpace(a.url)
						if kit.Button(c, i18n.T("ai.avatarUseURL"), nil, false).Disabled(disabled || address == "").Clicked() {
							pick(func() (string, error) {
								var result struct {
									Icon string `json:"icon"`
								}
								err := p.env.Host.CallCompletingInto("capture.ai.icons.fetch", map[string]string{"url": address}, &result)
								return result.Icon, err
							})
						}
					})
				}
				if a.mode == "builtin" {
					ui.TextInput(c, &a.search).FillWidth().Placeholder(i18n.T("ai.avatarSearch")).Label(i18n.T("ai.avatarSearch"))
					query := strings.ToLower(strings.TrimSpace(a.search))
					ui.Scroll(c).FillWidth().MaxHeight(192).Children(func() {
						ui.Row(c).FillWidth().Gap(4).Wrap().Role(ui.RoleGroup).Label(i18n.T("ai.avatarBuiltin")).Children(func() {
							for _, icon := range widgets.BuiltInProviderIcons() {
								if !strings.Contains(strings.ToLower(icon), query) {
									continue
								}
								icon := icon
								title := strings.TrimSuffix(icon, "-color")
								chosen := value(pr.Avatar) == icon
								b := ui.ButtonBase(c.Key(icon)).Size(kit.ControlHeight, kit.ControlHeight).Radius(t.Radius).
									Justify(ui.Center).AlignItems(ui.Center).Label(title).Tooltip(title).Checked(chosen).Disabled(disabled)
								switch {
								case chosen:
									b.Background(t.SurfacePressed)
								case b.Hovered():
									b.Background(t.SurfaceHover)
								}
								if b.Clicked() {
									p.change(id, func(t *provider) { t.Avatar = ptr(icon) })
									a.mode, a.err = "", ""
								}
								b.Children(func() { widgets.ProviderIcon(c, p.env, icon, 20) })
							}
						})
					})
				}
				if a.err != "" {
					kit.Notice(c, kit.Failure, "", a.err, nil)
				}
			})
		})
	})
	// A picture dropped on the section is chosen as a file is.
	widgets.DropArea(c, p.env, group, widgets.DropFiles, false, disabled, func(paths []string, err error) {
		if err != nil {
			a.err = err.Error()
			return
		}
		path := paths[0]
		pick(func() (string, error) {
			data, err := readAvatarFile(path)
			if err != nil {
				return "", err
			}
			return storeAvatar(p.env, data)
		})
	})
}

func toggleMode(current, mode string) string {
	if current == mode {
		return ""
	}
	return mode
}

// errCancel marks a picker the user closed: nothing changes, nothing is
// reported.
var errCancel = errors.New("cancelled")

func errCancelled(err error) error {
	if err != nil {
		return err
	}
	return errCancel
}
