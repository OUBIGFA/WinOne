//go:build windows

package ai

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestPluginCodeAllowsSandboxReadersWithoutOpeningPrivateProfiles(t *testing.T) {
	root := t.TempDir()
	code := filepath.Join(root, "third-party", "plugins")
	profile := filepath.Join(root, "home", "browser-profile")
	for _, path := range []string{code, profile} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	private, err := windows.GetNamedSecurityInfo(profile, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	before := private.String()
	oldFile := filepath.Join(code, "existing-browser.exe")
	if err := os.WriteFile(oldFile, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := preparePluginCodeDirectory(code); err != nil {
		t.Fatal(err)
	}
	newFile := filepath.Join(code, "new-browser.exe")
	if err := os.WriteFile(newFile, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{oldFile, newFile} {
		rights := sandboxFileRights(t, path)
		if rights&0x1200a9 != 0x1200a9 || rights&(windows.FILE_WRITE_DATA|windows.FILE_APPEND_DATA|windows.WRITE_DAC|windows.WRITE_OWNER|windows.DELETE) != 0 {
			t.Fatalf("sandbox rights for %s = 0x%X; want read/execute only", filepath.Base(path), rights)
		}
	}
	if err := preparePluginCodeDirectory(code); err != nil {
		t.Fatal(err)
	}
	private, err = windows.GetNamedSecurityInfo(profile, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if private.String() != before {
		t.Fatal("plugin code preparation changed private browser profile permissions")
	}
}

// Ask Windows for effective access, independently of the preparation's ACE
// lookup. This detects missing inheritance and unintended write grants.
func sandboxFileRights(t *testing.T, path string) uint32 {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	sid, err := windows.StringToSid("S-1-15-2-1")
	if err != nil {
		t.Fatal(err)
	}
	var pin runtime.Pinner
	pin.Pin(sid)
	defer pin.Unpin()
	trustee := windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_WELL_KNOWN_GROUP, TrusteeValue: windows.TrusteeValueFromSID(sid)}
	var rights uint32
	result, _, _ := windows.NewLazySystemDLL("advapi32.dll").NewProc("GetEffectiveRightsFromAclW").Call(uintptr(unsafe.Pointer(acl)), uintptr(unsafe.Pointer(&trustee)), uintptr(unsafe.Pointer(&rights)))
	if result != 0 {
		t.Fatalf("effective sandbox rights: %v", windows.Errno(result))
	}
	return rights
}
