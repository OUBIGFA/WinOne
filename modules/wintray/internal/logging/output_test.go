package logging

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type rejectedOutput struct{ err error }

func (w rejectedOutput) Write([]byte) (int, error) { return 0, w.err }

func TestSharedOutputPreservesLevelsAndStopsAfterClose(t *testing.T) {
	var output bytes.Buffer
	logger := NewOutput(&output)
	logger.Info("first")
	logger.Warn("second")
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	before := output.String()
	logger.Error("after close")
	if output.String() != before || !strings.Contains(before, "[INFO] first") || !strings.Contains(before, "[WARN] second") {
		t.Fatal(before)
	}
}
func TestSharedOutputFailureRemainsObservable(t *testing.T) {
	want := errors.New("fixture output unavailable")
	logger := NewOutput(rejectedOutput{want})
	logger.Error("message")
	if !errors.Is(logger.Close(), want) {
		t.Fatal("log write failure was hidden")
	}
}
