//go:build windows

package startup

import (
	"fmt"
	"github.com/go-ole/go-ole"
	"os"
	"testing"
	"time"
)

type taskException uint32

func (e taskException) Error() string { return "localized error" }
func (e taskException) SCODE() uint32 { return uint32(e) }

func TestTaskAbsenceUsesOnlyFileNotFoundHRESULT(t *testing.T) {
	for _, code := range []uint32{0x80070002, 0x80070005, 0x8004130F, 0x800706BA} {
		want := code == 0x80070002
		for _, err := range []error{ole.NewError(uintptr(code)), ole.NewErrorWithSubError(0x80020009, "", taskException(code))} {
			if taskNotFound(err) != want {
				t.Fatalf("HRESULT %#x incorrectly classified", code)
			}
		}
	}
}
func TestScheduledTaskLookupConfirmsMissingName(t *testing.T) {
	requireSchtasks(t)
	exists, err := scheduledTaskExists(fmt.Sprintf("WinOne-Missing-Lookup-%d-%d", os.Getpid(), time.Now().UnixNano()))
	if err != nil || exists {
		t.Fatalf("missing task lookup: exists=%t error=%v", exists, err)
	}
}
