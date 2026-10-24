//go:build windows

package systray

import (
	"errors"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// notificationText reserves the terminating NUL without splitting a UTF-16 pair.
func notificationText(value string, limit int) ([]uint16, error) {
	units, err := windows.UTF16FromString(value)
	if err != nil {
		return nil, err
	}
	if len(units) > limit {
		units = units[:limit]
		if last := units[len(units)-1]; last >= 0xd800 && last <= 0xdbff {
			units = units[:len(units)-1]
		}
	}
	return units, nil
}

func warningNotification(base notifyIconData, title, message string) (notifyIconData, error) {
	if strings.TrimSpace(title) == "" || strings.TrimSpace(message) == "" {
		return notifyIconData{}, errors.New("notification title and message are required")
	}
	heading, err := notificationText(title, 63)
	if err != nil {
		return notifyIconData{}, err
	}
	body, err := notificationText(message, 255)
	if err != nil {
		return notifyIconData{}, err
	}
	// Do not retain NIF_INFO on the icon: later tooltip/image updates must not
	// repeat this notification. Preserve a GUID identity when one is in use.
	base.Flags = 0x10 | 0x40 | (base.Flags & 0x20) // NIF_INFO | NIF_REALTIME | optional NIF_GUID.
	base.Info = [256]uint16{}
	base.InfoTitle = [64]uint16{}
	copy(base.InfoTitle[:], heading)
	copy(base.Info[:], body)
	base.InfoFlags = 0x2 | 0x10 | 0x80 // Warning, no sound, respect Windows quiet time.
	base.Size = uint32(unsafe.Sizeof(base))
	return base, nil
}

// ShowWarning requests a one-shot Windows notification using this tray icon.
// Windows controls visibility; quiet-time or disabled notifications are respected.
func ShowWarning(title, message string) error {
	return showNotification(title, message, false)
}

// ShowMessage uses the same one-shot lifecycle for a successful operation.
func ShowMessage(title, message string) error {
	return showNotification(title, message, true)
}
func showNotification(title, message string, information bool) error {
	wt.muNID.Lock()
	defer wt.muNID.Unlock()
	if !wt.isReady() || wt.nid == nil {
		return ErrTrayNotReadyYet
	}
	data, err := warningNotification(*wt.nid, title, message)
	if err != nil {
		return err
	}
	if information {
		data.InfoFlags = 0x1 | 0x10 | 0x80
	}
	err = data.modify()
	if errors.Is(err, syscall.Errno(0)) {
		return errors.New("Windows rejected the tray notification")
	}
	return err
}
