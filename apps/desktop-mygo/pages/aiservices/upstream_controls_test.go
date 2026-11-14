package aiservices

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
)

// The real worker's file parsing and exports have packaged integration tests.
// These tests exercise the native controls, explicit actions and Host arguments.
func TestAIEntrypointsShowTranslatedLabels(t *testing.T) {
	defer i18n.SetLocale(i18n.Chinese)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		_, h, p, tt := setup(t)
		i18n.SetLocale(locale)
		for _, tab := range []int{tabHome, tabCustom, tabAccounts, tabGateway} {
			p.switchTab(tab)
			settle(h, tt)
			for _, text := range tt.Texts() {
				if strings.HasPrefix(text, "ai.") || strings.HasPrefix(text, "aiNative.") {
					t.Errorf("%s tab %d shows an untranslated label: %q", locale, tab, text)
				}
			}
		}
	}
}

func TestAccountFileImportKeepsSelectionOnFailureAndRetries(t *testing.T) {
	f, h, p, _ := setup(t)
	path := filepath.Join(t.TempDir(), "account-fixture.json")
	const contents = `{"access_token":"fixture-only"}`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	h.Picked[nativeT("chooseAccountFiles")] = []string{path}
	attempts := 0
	h.SetHandler(func(method string, params json.RawMessage) (any, error) {
		if method != "capture.ai.accounts.import" {
			return f.handle(method, params)
		}
		var got struct {
			Agent, Provider string
			Files           []string
			AccessOnly      bool
		}
		if err := json.Unmarshal(params, &got); err != nil {
			t.Error(err)
		}
		if got.Agent != "codex" || got.Provider != "one" || !got.AccessOnly || !reflect.DeepEqual(got.Files, []string{contents}) {
			t.Errorf("unexpected import arguments: %+v", got)
		}
		attempts++
		if attempts == 1 {
			return nil, errors.New("Fixture service unavailable")
		}
		return map[string]any{"results": []map[string]any{{"user": "fixture@example.com", "status": "added"}, {"user": "existing@example.com", "status": "exists"}, {"file": 1, "status": "failed", "error": "invalid fixture account file"}}}, nil
	})
	a := newAccountPanel(p, p.current())
	request := accountRequest{Agent: "codex", Provider: "one"}
	tt := ui.NewTester(func(c *ui.Context) {
		a.report()
		a.importView(c, request, a.importBusy)
	}, 800, 1000)
	if err := tt.Click(nativeT("chooseAccountFiles")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if attempts != 0 || len(a.importPaths) != 1 {
		t.Fatal("choosing files imported them or lost the selection")
	}
	if err := tt.Click(nativeT("accessOnly")); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(nativeT("confirmImport")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	dirty, busy := p.env.Activity.State()
	if !strings.Contains(a.err, "Fixture service unavailable") || len(a.importPaths) != 1 || !dirty || busy {
		t.Fatalf("failed import lost its retry state: error=%q dirty=%t busy=%t", a.err, dirty, busy)
	}
	if err := tt.Click(nativeT("confirmImport")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if attempts != 2 || len(a.importPaths) != 0 || len(a.importResults) != 3 || a.err != "" {
		t.Fatal("retry did not import the chosen account and clear its pending selection")
	}
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		settle(h, tt)
		if !i18n.Has("aiNative.import.exists") || !tt.HasText("existing@example.com · "+nativeT("import.exists")) {
			t.Errorf("existing account outcome is not translated in %s", locale)
		}
		if !tt.HasText("account-fixture.json · " + nativeT("import.failed")) {
			t.Errorf("failed import did not identify the source file in %s", locale)
		}
	}
	i18n.SetLocale(i18n.Chinese)
	settle(h, tt)
	shot(t, tt, "account-files-import")
}

func TestAccountFileExportFormatsKeepTheSelectedAccountsAndDestination(t *testing.T) {
	for _, format := range []string{"sub2api", "cliproxyapi"} {
		t.Run(format, func(t *testing.T) {
			f, h, p, _ := setup(t)
			destination := filepath.Join(t.TempDir(), "export.json")
			if format == "cliproxyapi" {
				destination = filepath.Dir(destination)
			}
			h.Picked[nativeT("exportAccounts")] = destination
			h.Picked[nativeT("chooseExportPath")] = destination
			calls := 0
			h.SetHandler(func(method string, params json.RawMessage) (any, error) {
				if method != "capture.ai.accounts.export" {
					return f.handle(method, params)
				}
				var got struct {
					Agent, Provider, Format, Path string
					Users                         []string
				}
				if err := json.Unmarshal(params, &got); err != nil {
					t.Error(err)
				}
				if got.Agent != "codex" || got.Provider != "one" || got.Format != format || got.Path != destination || !reflect.DeepEqual(got.Users, []string{"included@example.com"}) {
					t.Errorf("unexpected export arguments: %+v", got)
				}
				calls++
				return accountExportResult{Files: []string{destination}}, nil
			})
			a := newAccountPanel(p, p.current())
			a.exportFormat = format
			a.accounts = []account{{User: "included@example.com"}, {User: "excluded@example.com"}}
			tt := ui.NewTester(func(c *ui.Context) {
				a.exportView(c, accountRequest{Agent: "codex", Provider: "one"}, a.importBusy)
			}, 800, 1000)
			if err := tt.Click("excluded@example.com"); err != nil {
				t.Fatal(err)
			}
			if calls != 0 {
				t.Fatal("selecting accounts started an export")
			}
			if err := tt.Click(nativeT("chooseExportPath")); err != nil {
				t.Fatal(err)
			}
			settle(h, tt)
			if calls != 1 || a.err != "" || !reflect.DeepEqual(a.exportFiles, []string{destination}) {
				t.Fatal("export did not report the chosen output")
			}
			shot(t, tt, "account-files-export-"+format)
		})
	}
}

func TestServerCertificateReadNeedsExplicitTrustAndFollowsTheDraft(t *testing.T) {
	f, h, p, _ := setup(t)
	pr := p.current()
	e := newEditor(p, pr, pr.ID)
	p.editor = e
	seen := serverCertificate{Host: "relay.example", SHA256: "fixture-certificate-pin", Subject: "CN=relay.example", NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour)}
	h.SetHandler(func(method string, params json.RawMessage) (any, error) {
		if method == "capture.ai.models.certificate" {
			return map[string]any{"certificate": seen}, nil
		}
		return f.handle(method, params)
	})
	tt := ui.NewTester(func(c *ui.Context) { e.connectionOptions(c, pr, p.connectionBusy) }, 800, 1200)
	if err := tt.Click(nativeT("readCertificate")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if pr.ServerPin != "" || e.serverCertificate == nil || !tt.HasText(seen.SHA256) {
		t.Fatal("reading the certificate trusted it or lost the result")
	}
	if err := tt.Click(nativeT("trustCertificate")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if pr.ServerPin != seen.SHA256 || !p.dirty() || len(h.Called("capture.ai.configuration.save")) != 0 {
		t.Fatal("explicit trust did not remain an unsaved provider edit")
	}
	pr.BaseURL = "https://another.example/v1"
	settle(h, tt)
	if e.serverCertificate != nil || tt.HasText(nativeT("trustCertificate")) {
		t.Fatal("changing the connection kept a certificate from the old endpoint")
	}
}

func TestAccountWindowSwitchKeepsOneLimitMode(t *testing.T) {
	_, h, p, _ := setup(t)
	pr := p.current()
	pr.AccountWindowCaps = map[string]map[string]int{"fixture": {"5h": 80}}
	e := newEditor(p, pr, pr.ID)
	tt := ui.NewTester(func(c *ui.Context) { e.accountWindowCaps(c, pr, "fixture", false) }, 800, 800)
	if err := tt.Click(nativeT("windowCapMode")); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(nativeT("windowAmount")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if len(pr.AccountWindowCaps["fixture"]) != 0 || pr.AccountWindowAmounts["fixture"]["5h"] != 1 {
		t.Fatal("fixed amount did not replace the percentage limit")
	}
	if err := tt.Click(nativeT("windowCapMode")); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(nativeT("windowShare")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if len(pr.AccountWindowAmounts["fixture"]) != 0 || pr.AccountWindowCaps["fixture"]["5h"] != 100 {
		t.Fatal("percentage limit did not replace the fixed amount")
	}
	if !p.dirty() || !tt.HasText(i18n.T("ai.accountWindowCaps")) {
		t.Fatal("limit changes did not stay in the provider draft")
	}
}
