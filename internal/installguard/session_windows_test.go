//go:build windows

package installguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHoldRejectsAHelperInsideTheInstallationBeforeOpeningTheProfile(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(t.TempDir(), "profile")
	err = Hold(Request{Installation: filepath.Dir(executable), Profile: profile})
	if err == nil || !strings.Contains(err.Error(), "维护程序必须从安装目录外运行") {
		t.Fatalf("helper inside the payload was not rejected: %v", err)
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatalf("rejected helper opened the profile: %v", err)
	}
}
