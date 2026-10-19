//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Once initialized, an account summary must behave like magpie's cached
// account page: reread auth without starting Bun or asking for models.
func TestPluginAccountSummaryDoesNotRequireRunningThePlugin(t *testing.T) {
	executable := os.Getenv("WINONE_AI_PLUGIN_TEST_EXE")
	if executable == "" {
		t.Skip("set WINONE_AI_PLUGIN_TEST_EXE")
	}
	bundle := filepath.Join(t.TempDir(), "shared", "ai")
	original := filepath.Dir(executable)
	paths := []string{"WinOne.AI.exe", "runtime/bun.exe"}
	if err := filepath.WalkDir(filepath.Join(original, "plugins", "cline"), func(file string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			relative, err := filepath.Rel(original, file)
			if err != nil {
				return err
			}
			paths = append(paths, relative)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, relative := range paths {
		data, err := os.ReadFile(filepath.Join(original, relative))
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(bundle, relative)
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(target, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	runtime := Translator{Executable: filepath.Join(bundle, "WinOne.AI.exe"), DataDirectory: t.TempDir(), AccountBackend: "plugin"}
	// An installed third-party extension must not force ordinary summaries
	// to execute code when the complete configuration still matches.
	extension := filepath.Join(runtime.DataDirectory, "fixture.mjs")
	if err := os.WriteFile(extension, []byte(`export default async()=>({})`), 0600); err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(runtime.DataDirectory, "third-party", "plugins.json")
	if err := os.MkdirAll(filepath.Dir(registry), 0700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"plugins": []map[string]string{{"spec": extension}}})
	if err := os.WriteFile(registry, data, 0600); err != nil {
		t.Fatal(err)
	}
	initial, err := runtime.Accounts(context.Background(), "accounts.list", "cline")
	if err != nil {
		t.Fatal(err)
	}
	// Move only this test's runtime aside. Summary reads still work; an actual
	// sign-in must fail its resource validation instead of bypassing it.
	if err = os.Rename(filepath.Join(bundle, "runtime", "bun.exe"), filepath.Join(bundle, "runtime", "bun-held.exe")); err != nil {
		t.Fatal(err)
	}
	var before struct {
		Methods []json.RawMessage `json:"methods"`
	}
	if err = json.Unmarshal(initial, &before); err != nil || len(before.Methods) == 0 {
		t.Fatal("missing initial methods", err)
	}
	auth := filepath.Join(runtime.DataDirectory, "config", "magpie", "plugin-auth.json")
	for _, signedIn := range []bool{true, false} {
		value := []byte(`{}`)
		if signedIn {
			value = []byte(`{"cline":{"type":"api","key":"fixture-only-key","email":"local@example.invalid"}}`)
		}
		if err = os.WriteFile(auth, value, 0600); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		output, err := runtime.Accounts(context.Background(), "accounts.list", "cline")
		if err != nil {
			t.Fatal("summary started the plugin instead of reading local state", err)
		}
		var listing struct {
			Accounts []struct {
				User string `json:"user"`
			} `json:"accounts"`
			Methods []json.RawMessage `json:"methods"`
		}
		if err = json.Unmarshal(output, &listing); err != nil {
			t.Fatal(err)
		}
		if signedIn && (len(listing.Accounts) != 1 || listing.Accounts[0].User != "local@example.invalid") || !signedIn && len(listing.Accounts) != 0 {
			t.Fatal("summary did not reread current auth")
		}
		if len(listing.Methods) != len(before.Methods) {
			t.Fatal("summary lost login methods")
		}
		t.Logf("local summary (signed in=%v): %s", signedIn, time.Since(start))
	}
	if err = runtime.SignIn(context.Background(), "cline", make(chan string), func(json.RawMessage) {}); err == nil {
		t.Fatal("sign-in bypassed missing-runtime validation")
	}
}

func TestPluginAccountSurvivesSwitchingInstallationDirectory(t *testing.T) {
	executable := os.Getenv("WINONE_AI_PLUGIN_TEST_EXE")
	if executable == "" {
		t.Skip("set WINONE_AI_PLUGIN_TEST_EXE")
	}
	profile := t.TempDir()
	runtime := Translator{Executable: executable, DataDirectory: profile, AccountBackend: "plugin"}
	if _, err := runtime.Accounts(context.Background(), "accounts.list", "cline"); err != nil {
		t.Fatal(err)
	}
	copyRoot := filepath.Join(t.TempDir(), "shared", "ai")
	original := filepath.Dir(executable)
	paths := []string{"WinOne.AI.exe", "runtime/bun.exe"}
	if err := filepath.WalkDir(filepath.Join(original, "plugins", "cline"), func(file string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			relative, err := filepath.Rel(original, file)
			if err != nil {
				return err
			}
			paths = append(paths, relative)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, relative := range paths {
		data, err := os.ReadFile(filepath.Join(original, relative))
		if err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(copyRoot, relative)
		if err = os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(destination, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	runtime.Executable = filepath.Join(copyRoot, "WinOne.AI.exe")
	if _, err := runtime.Accounts(context.Background(), "accounts.list", "cline"); err != nil {
		// This profile is empty and private. Keep the worker's diagnostic here
		// so a bundle/profile migration failure is actionable without exposing
		// production account data through the public settings response.
		raw, detail := runtime.invoke(context.Background(), map[string]string{"operation": "accounts.list", "agent": "cline", "agentBackend": "plugin"})
		t.Fatalf("same account profile cannot follow the verified installation: %v; worker=%s (%v)", err, raw, detail)
	}
}

// Opt-in packaged-runtime check. It lists sign-in methods in empty private
// homes, without submitting credentials or starting a browser sign-in.
func TestBundledPluginAccountMethods(t *testing.T) {
	executable := os.Getenv("WINONE_AI_PLUGIN_TEST_EXE")
	if executable == "" {
		t.Skip("set WINONE_AI_PLUGIN_TEST_EXE to a complete plugin bundle")
	}
	runtime := Translator{Executable: executable, DataDirectory: t.TempDir()}
	output, err := runtime.Accounts(context.Background(), "accounts.catalog", "")
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Agents []struct {
			ID      string `json:"id"`
			Backend string `json:"backend"`
		} `json:"agents"`
	}
	if err = json.Unmarshal(output, &catalog); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, agent := range catalog.Agents {
		if agent.Backend != "plugin" {
			continue
		}
		count++
		t.Run(agent.ID, func(t *testing.T) {
			scoped := runtime.ForProvider("fixture-" + agent.ID)
			scoped.AccountBackend = "plugin"
			data, err := scoped.Accounts(context.Background(), "accounts.list", agent.ID)
			if err != nil {
				t.Fatal(err)
			}
			var listing struct {
				Accounts []json.RawMessage `json:"accounts"`
				Methods  []struct {
					Type  string `json:"type"`
					Label string `json:"label"`
				} `json:"methods"`
			}
			if err = json.Unmarshal(data, &listing); err != nil {
				t.Fatal(err)
			}
			if len(listing.Accounts) != 0 || len(listing.Methods) == 0 {
				t.Fatal("empty isolated account did not expose its sign-in methods")
			}
			for _, method := range listing.Methods {
				if method.Label == "" || method.Type != "oauth" && method.Type != "api" {
					t.Fatal("invalid sign-in method")
				}
				t.Logf("%s: %s", method.Type, method.Label)
			}
			for index, method := range listing.Methods {
				if method.Type != "api" {
					continue
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				prompted := false
				err = scoped.SignInMethod(ctx, agent.ID, &index, make(chan string), func(raw json.RawMessage) {
					var event struct {
						SignIn *struct {
							State  string `json:"state"`
							Prompt *struct {
								Type string `json:"type"`
								Key  string `json:"key"`
							} `json:"prompt"`
						} `json:"signin"`
					}
					if json.Unmarshal(raw, &event) == nil && event.SignIn != nil && event.SignIn.Prompt != nil {
						// Some API methods first ask for a site or account scope.
						prompt := event.SignIn.Prompt
						prompted = prompt.Key != "" && (prompt.Type == "password" || prompt.Type == "text" || prompt.Type == "select")
						cancel()
					}
				})
				if !prompted || !errors.Is(err, context.Canceled) {
					t.Fatalf("canceling the API-key prompt: prompted=%v error=%v", prompted, err)
				}
			}
		})
	}
	if count == 0 {
		t.Fatal("no plugin accounts were included")
	}
}
