package wintray

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/modules/wintray/applications"
)

func TestLatestStartupModeKeepsScheduleIndependent(t *testing.T) {
	f := setup(t, "running", programDocument("reader", "Reader", `C:\Apps\reader.exe`))
	if f.tt.HasText(i18n.T("wintray.startup")) {
		t.Fatal("the obsolete startup switch still overrides the selected mode")
	}
	f.click(t, i18n.T("wintray.schedule"))
	f.click(t, i18n.T("wintray.normal"))
	if f.page.active().RunOnStartup || !f.page.active().Schedule.Value.Enabled {
		t.Fatal("normal startup changed the independent schedule")
	}
	f.h.Env.Rail.Save.Run()
	f.h.Settle(f.tt)
	if f.page.active().RunOnStartup || mode(*f.page.active()) != "normal" || !f.page.active().Schedule.Value.Enabled {
		t.Fatal("startup mode or schedule did not survive save and reload")
	}
}

func TestInstalledAppsFilterSelectAndSaveCompleteLaunchData(t *testing.T) {
	f := setup(t, "running")
	icon := image.NewRGBA(image.Rect(0, 0, 2, 2))
	icon.Set(0, 0, color.RGBA{R: 200, G: 40, B: 80, A: 255})
	apps := []applications.Application{
		{Name: "Desktop reader", Path: `C:\Apps\reader.exe`, Args: `--profile "two words"`, WorkingDirectory: `C:\Reader data`, Icon: icon},
		{Name: "Store reader", Path: `shell:AppsFolder\Example.Reader_123456789abcd!App`, Icon: icon},
	}
	before := enumerateInstalledApps
	enumerateInstalledApps = func(context.Context) ([]applications.Application, error) { return apps, nil }
	t.Cleanup(func() { enumerateInstalledApps = before; f.page.Close() })
	f.click(t, i18n.T("wintray.addInstalled"))
	f.click(t, "Desktop reader")
	f.click(t, i18n.T("wintray.installedStore"))
	if f.tt.HasText("Desktop reader") || !f.tt.HasText("Store reader") {
		t.Fatal("store filter did not narrow the catalog")
	}
	f.click(t, "Store reader")
	f.click(t, i18n.T("wintray.installedAdd"))
	if len(f.page.apps()) != 2 || len(f.fake.saved) != 0 {
		t.Fatal("selection must add both filtered and hidden choices to the unsaved draft")
	}
	first := f.page.apps()[0].Value
	if first.Args != apps[0].Args || first.WorkingDirectory != apps[0].WorkingDirectory || first.RunOnStartup || f.page.meta[first.ExePath].icon == nil {
		t.Fatalf("launch data or original icon lost: %+v", first)
	}
	f.h.Env.Rail.Save.Run()
	f.h.Settle(f.tt)
	if len(f.fake.saved) != 1 || len(f.page.apps()) != 2 || f.page.apps()[1].Value.ExePath != apps[1].Path {
		t.Fatal("selected application IDs did not survive save/reload")
	}
}

func TestProgramLocationDoesNotLaunchOrSave(t *testing.T) {
	f := setup(t, "running", programDocument("reader", "Reader", `C:\Apps\reader.exe`))
	before := *f.page.active()
	previous := openProgramLocation
	var opened string
	openProgramLocation = func(path string) error { opened = path; return nil }
	t.Cleanup(func() { openProgramLocation = previous })
	f.click(t, i18n.T("wintray.openLocation"))
	if opened != before.ExePath || len(f.fake.saved) != 0 || len(f.h.Called("wintray.launch")) != 0 || f.page.dirty() {
		t.Fatal("opening the file location changed or launched the program")
	}
	if f.tt.HasText(i18n.T("settingsDesign.replaceColumn")) {
		t.Fatal("the old replace column remains in the list")
	}
}

func TestNewProgramsStayNormalUntilModeIsChosen(t *testing.T) {
	for _, path := range []string{`C:\Apps\reader.exe`, `C:\Scripts\job.cmd`, `shell:AppsFolder\Example.Reader_123456789abcd!App`} {
		app := newProgram(path, "Reader").Value
		if mode(app) != "normal" || app.RunOnStartup {
			t.Fatalf("adding %q enabled automatic startup: %+v", path, app)
		}
	}
}

// Verify the original symbols reach both the catalog and the managed list's
// actual pixels; a non-nil bitmap alone would not prove either view paints it.
func TestInstalledAppIconsPaintBesideSingleLineNames(t *testing.T) {
	for _, dark := range []bool{false, true} {
		f := setup(t, "running")
		apps := []applications.Application{
			{Name: "Desktop icon", Path: `C:\Apps\icon.exe`},
			{Name: "Store icon", Path: `shell:AppsFolder\Example.Icon_123456789abcd!App`},
		}
		colors := []color.NRGBA{{R: 229, G: 37, B: 79, A: 255}, {R: 11, G: 193, B: 137, A: 255}}
		for index := range apps {
			icon := image.NewNRGBA(image.Rect(0, 0, 64, 64))
			draw.Draw(icon, icon.Bounds(), image.NewUniform(colors[index]), image.Point{}, draw.Src)
			apps[index].Icon = icon
		}
		previous := enumerateInstalledApps
		enumerateInstalledApps = func(context.Context) ([]applications.Application, error) { return apps, nil }
		t.Cleanup(func() { enumerateInstalledApps = previous; f.page.Close() })
		// Match the real settings host's metrics; MyGo's default theme is a
		// different type/spacing scale from WinOne's shared theme.
		f.tt = ui.NewTester(func(c *ui.Context) {
			c.SetTheme(kit.Theme(c.Preferences(), dark))
			f.page.View(c)
		}, 1300, 900)
		f.h.Settle(f.tt)
		f.click(t, i18n.T("wintray.addInstalled"))
		f.h.Shots(t, "wintray-installed-picker", f.page.View, 900, nil)
		f.h.Settle(f.tt)
		for index, app := range apps {
			assertApplicationIconPixels(t, f.tt, app.Name, colors[index])
			if f.tt.HasText(app.Path) {
				t.Fatalf("application catalog exposed a path instead of its icon/name row: %s", app.Path)
			}
			f.click(t, app.Name)
		}
		f.click(t, i18n.T("wintray.installedAdd"))
		f.h.Shots(t, "wintray-installed-icons", f.page.View, 900, nil)
		f.h.Settle(f.tt)
		for index, app := range apps {
			assertApplicationIconPixels(t, f.tt, app.Name, colors[index])
		}
		enumerateInstalledApps = previous
		f.page.Close()
	}
}

func assertApplicationIconPixels(t *testing.T, tester *ui.Tester, name string, want color.NRGBA) {
	t.Helper()
	row, found := tester.Find(name)
	// Use WinOne's shared one-line row limit, rather than the standalone
	// WinTray theme's 48 DIP metric on its older MyGo version.
	if !found || row.H > kit.RowMinHeight {
		t.Fatalf("application has no single-line icon/name row: %s: %+v", name, row)
	}
	picture := tester.Image()
	pixels := 0
	for y := int(row.Y); y < int(row.Y+row.H); y++ {
		for x := int(row.X); x < int(row.X+row.W); x++ {
			if color.NRGBAModel.Convert(picture.At(x, y)) == want {
				pixels++
			}
		}
	}
	if pixels < 256 {
		t.Fatalf("%s painted %d original icon pixels, want at least 256", name, pixels)
	}
}

func TestProgramLocationDropRejectsUnsupportedFileWithoutChangingDraft(t *testing.T) {
	f := setup(t, "running", programDocument("reader", "Reader", `C:\Apps\reader.exe`))
	before := clone(*f.page.draft)
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("notes"), 0600); err != nil {
		t.Fatal(err)
	}
	button, found := f.tt.Find(i18n.T("wintray.openLocation"))
	if !found || !f.tt.DropFiles(button.X+button.W/2, button.Y+button.H/2, path) {
		t.Fatal("the location icon has no replacement drop target")
	}
	f.h.Settle(f.tt)
	if !same(before, *f.page.draft) || len(f.fake.saved) != 0 || !f.tt.HasText(i18n.T("wintray.notProgramFile")) {
		t.Fatalf("unsupported file changed the selected program or was not refused: %+v", f.page.active())
	}
}
