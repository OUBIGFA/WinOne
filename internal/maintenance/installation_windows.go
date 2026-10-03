//go:build windows

package maintenance

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
)

// Session protects both the installation and the selected profile. Separate
// copies remain independent unless they share a profile. Always acquire the
// installation first and release both on a partial failure.
type Session struct{ installation, profile *Lease }

func AcquireRuntimeForInstallation(profile, installation string) (*Session, error) {
	return acquireSession(profile, installation, false)
}
func AcquireMaintenanceForInstallation(profile, installation string) (*Session, error) {
	return acquireSession(profile, installation, true)
}
func (s *Session) Close() error { return errors.Join(s.profile.Close(), s.installation.Close()) }

func acquireSession(profile, installation string, exclusive bool) (*Session, error) {
	key, err := installationLockDirectory(installation)
	if err != nil {
		return nil, err
	}
	installed, err := acquire(key, exclusive)
	if err != nil {
		return nil, err
	}
	// Inspect under the installation lease so maintenance cannot create or clear
	// the marker between this check and runtime admission. Any existing object
	// blocks startup, including an empty marker left by an interrupted write.
	if !exclusive {
		_, markerErr := os.Lstat(filepath.Join(installation, ".winone-pending"))
		if markerErr == nil || !errors.Is(markerErr, os.ErrNotExist) {
			return nil, errors.Join(errors.New("安装或卸载尚未完成，暂时无法启动 WinOne"), markerErr, installed.Close())
		}
	}
	selected, err := acquire(profile, exclusive)
	if err != nil {
		return nil, errors.Join(err, installed.Close())
	}
	return &Session{installation: installed, profile: selected}, nil
}

// Resolve through a directory handle so case, junctions and short-path aliases
// cannot select different locks for one existing installation. Keep lock files
// outside the installation so uninstall can remove its now-empty directory.
func installationLockDirectory(directory string) (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if !filepath.IsAbs(base) {
		return "", errors.New("LOCALAPPDATA must be an absolute directory")
	}
	path, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		return "", err
	}
	handle, err := windows.CreateFile(path, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, 32768)
	n, err := windows.GetFinalPathNameByHandle(handle, &buffer[0], uint32(len(buffer)), 0)
	if err != nil {
		return "", err
	}
	if n == 0 || n >= uint32(len(buffer)) {
		return "", errors.New("installation path is too long or empty")
	}
	identity := strings.ToLower(windows.UTF16ToString(buffer[:n]))
	sum := sha256.Sum256([]byte(identity))
	return filepath.Join(base, "WinOne", "installation-locks", fmt.Sprintf("%x", sum)), nil
}
