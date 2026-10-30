//go:build windows

package audiooutput

import (
 "os"
 "testing"
)

// Explicit, read-only interop check. The default suite never touches an audio
// endpoint, and this opt-in never changes the user's volume or mute state.
func TestReadDefaultOutputMute(t *testing.T) {
 if os.Getenv("WINONE_AUDIO_READ_TEST")!="1" { t.Skip("set WINONE_AUDIO_READ_TEST=1 for the read-only Core Audio check") }
 if _,err:=Mute(nil);err!=nil {t.Fatal(err)}
}
