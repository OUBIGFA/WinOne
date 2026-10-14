package resultui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var errSpeechPlayback = errors.New("speech playback failed")
var speechMCI = windows.NewLazySystemDLL("winmm.dll").NewProc("mciSendStringW")

func speechMCICommand(command string) (string, error) {
	input, err := windows.UTF16PtrFromString(command)
	if err != nil {
		return "", fmt.Errorf("%w: invalid command", errSpeechPlayback)
	}
	var output [256]uint16
	code, _, _ := speechMCI.Call(uintptr(unsafe.Pointer(input)), uintptr(unsafe.Pointer(&output[0])), uintptr(len(output)), 0)
	if code != 0 {
		return "", fmt.Errorf("%w: MCI %d", errSpeechPlayback, code)
	}
	return windows.UTF16ToString(output[:]), nil
}

// Windows' built-in MPEG decoder owns playback; no browser, helper process or
// persistent audio cache is needed. Close the device before deleting its file.
func playSpeech(ctx context.Context, audio []byte) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	file, err := os.CreateTemp("", "winone-speech-*.mp3")
	if err != nil {
		return fmt.Errorf("%w: temporary audio", errSpeechPlayback)
	}
	path := file.Name()
	defer func() {
		if removeErr := os.Remove(path); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("%w: audio cleanup", errSpeechPlayback))
		}
	}()
	_, writeErr := file.Write(audio)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return fmt.Errorf("%w: temporary audio write", errSpeechPlayback)
	}
	// Keep all MCI calls on the owning thread. The shared lane prevents devices
	// from overlapping while an older request is releasing its native resources.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	id, err := speechRequestID()
	if err != nil {
		return err
	}
	alias := "winone_speech_" + id
	if _, err = speechMCICommand(`open "` + path + `" type mpegvideo alias ` + alias); err != nil {
		return err
	}
	defer func() {
		_, closeErr := speechMCICommand("close " + alias)
		err = errors.Join(err, closeErr)
	}()
	if err = ctx.Err(); err != nil {
		return err
	}
	if _, err = speechMCICommand("play " + alias); err != nil {
		return err
	}
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err() // Deferred close stops playback before releasing the lane.
		case <-ticker.C:
			mode, modeErr := speechMCICommand("status " + alias + " mode")
			if modeErr != nil {
				return modeErr
			}
			switch strings.TrimSpace(mode) {
			case "stopped":
				return nil
			case "playing", "seeking":
			default:
				return fmt.Errorf("%w: unexpected device state", errSpeechPlayback)
			}
		}
	}
}
