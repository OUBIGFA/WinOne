//go:build windows

package ai

import (
	"errors"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Only Magpie's installed code tree is readable by sandboxed application
// processes. Auth, registry options and browser profiles stay outside it.
// Inheritance also covers browsers downloaded by a plugin on its first login.
func preparePluginCodeDirectory(directory string) error {
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(name, windows.READ_CONTROL|windows.WRITE_DAC|windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(handle, &info); err != nil {
		return err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("AI plugin code directory must not be a link")
	}
	sd, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	sid, err := windows.StringToSid("S-1-15-2-1") // ALL APPLICATION PACKAGES
	if err != nil {
		return err
	}
	const rights = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_EXECUTE
	const inheritance = windows.CONTAINER_INHERIT_ACE | windows.OBJECT_INHERIT_ACE
	if dacl != nil {
		for i := uint32(0); i < uint32(dacl.AceCount); i++ {
			var ace *windows.ACCESS_ALLOWED_ACE
			if err := windows.GetAce(dacl, i, &ace); err != nil {
				return err
			}
			if ace.Header.AceType == windows.ACCESS_ALLOWED_ACE_TYPE && ace.Header.AceFlags&inheritance == inheritance && ace.Header.AceFlags&windows.INHERIT_ONLY_ACE == 0 && ace.Mask&rights == rights && (*windows.SID)(unsafe.Pointer(&ace.SidStart)).Equals(sid) {
				return nil // don't reapply inheritance to every file on each RPC
			}
		}
	}
	var pin runtime.Pinner
	pin.Pin(sid)
	defer pin.Unpin()
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{AccessPermissions: rights, AccessMode: windows.GRANT_ACCESS, Inheritance: inheritance, Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_WELL_KNOWN_GROUP, TrusteeValue: windows.TrusteeValueFromSID(sid)}}}, dacl)
	if err != nil {
		return err
	}
	return windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}
