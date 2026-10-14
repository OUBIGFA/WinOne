//go:build windows

package chat

import "golang.org/x/sys/windows"

func finishConversationCopy(source, destination string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	// No REPLACE_EXISTING: unlike a hard link, this also works on removable FAT
	// and exFAT drives, while preserving an existing destination on every retry.
	return windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH)
}
