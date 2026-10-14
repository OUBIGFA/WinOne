//go:build windows

package startupprobe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"winone/internal/catalog"
)

func TestProbeOutputBoundIncludesStreamingCopy(t *testing.T) {
	output := &limitedOutput{limit: 16}
	_, err := io.Copy(output, io.LimitReader(strings.NewReader(strings.Repeat("x", 64)), 64))
	if err == nil {
		t.Fatal("streaming copy bypassed output bound")
	}
}

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--browser-probe-fixture" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	if len(os.Args) > 2 && os.Args[1] == "--pipe" {
		os.Exit(runWorkerProbeFixture(os.Args[2]))
	}
	if len(os.Args) == 3 && (os.Args[1] == "--check-startup" || os.Args[1] == "--check-configuration") || len(os.Args) == 5 && (os.Args[1] == "--check-ui" && os.Args[3] == "--probe-root" || os.Args[1] == "--check-profile" && os.Args[3] == "--profile-root") {
		executable, _ := os.Executable()
		r := Report{Protocol: 1, Nonce: os.Args[2], PID: os.Getpid(), Executable: executable, Modules: catalog.Builtins()}
		if os.Args[1] == "--check-ui" {
			r.UIReady = true
			pid, _ := strconv.ParseUint(os.Getenv("WINONE_PROBE_BROWSER_PID"), 10, 32)
			r.BrowserProcessID = uint32(pid)
			r.BrowserCreated, _ = strconv.ParseUint(os.Getenv("WINONE_PROBE_BROWSER_CREATED"), 10, 64)
		}
		if os.Args[1] == "--check-configuration" || os.Args[1] == "--check-profile" {
			data, _ := io.ReadAll(os.Stdin)
			sum := sha256.Sum256(data)
			r.ConfigurationSHA256 = hex.EncodeToString(sum[:])
		}
		if os.Args[1] == "--check-profile" {
			snapshot, err := ReadProfileSnapshot(os.Args[4])
			if err != nil {
				os.Exit(8)
			}
			r.ProfileSHA256 = snapshot.Digest()
		}
		switch os.Getenv("WINONE_PROBE_TEST") {
		case "profile-hash":
			r.ProfileSHA256 = strings.Repeat("0", 64)
		case "profile-change":
			_ = os.WriteFile(filepath.Join(os.Args[4], "module-state.json"), []byte("{}"), 0600)
		case "configuration-hash":
			r.ConfigurationSHA256 = ""
		case "nonce":
			r.Nonce = "other"
		case "pid":
			r.PID++
		case "path":
			r.Executable += ".other"
		case "modules":
			r.Modules = nil
		case "exit":
			os.Exit(7)
		case "hang":
			time.Sleep(30 * time.Second)
		case "overflow":
			os.Stdout.WriteString(strings.Repeat("x", 65536))
			os.Exit(0)
		case "trailing":
			json.NewEncoder(os.Stdout).Encode(r)
		}
		json.NewEncoder(os.Stdout).Encode(r)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestProfileProbeReceiptRejectsWrongOrChangedControlState(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"ok", "profile-hash", "profile-change"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("WINONE_PROBE_TEST", mode)
			directory := t.TempDir()
			data := []byte("{}")
			if err := os.WriteFile(filepath.Join(directory, "configuration.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			_, err := CheckProfileConfiguration(context.Background(), executable, directory, data)
			if (err == nil) != (mode == "ok") {
				t.Fatal(mode, err)
			}
		})
	}
}

func TestConfigurationProbeRequiresMatchingInputReceipt(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"ok", "configuration-hash"} {
		t.Setenv("WINONE_PROBE_TEST", mode)
		err := CheckConfiguration(context.Background(), executable, []byte(`{"test":true}`))
		if (err == nil) != (mode == "ok") {
			t.Fatal(mode, err)
		}
	}
}

func TestProbeWaitsForBoundedVerifiedProcess(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"ok", "nonce", "pid", "path", "modules", "exit", "overflow", "trailing", "hang"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("WINONE_PROBE_TEST", mode)
			timeout := 5 * time.Second
			if mode == "hang" {
				timeout = 500 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			err := Check(ctx, executable)
			if (err == nil) != (mode == "ok") {
				t.Fatalf("%s: %v", mode, err)
			}
		})
	}
}
