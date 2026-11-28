//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

func TestLocalOCRLayoutReusesPreparedTextWorker(t *testing.T) {
	s, _ := ocrWorkerFixture(t, time.Minute)
	if err := <-s.SetStandby(true); err != nil {
		t.Fatal(err)
	}
	pid := workerReply(t, s, "before")
	layout, ok := any(s).(interface {
		RecognizeLayout(context.Context, []byte) (string, error)
	})
	if !ok {
		t.Fatal("layout recognition cannot reuse the prepared OCR service")
	}
	value, err := layout.RecognizeLayout(context.Background(), []byte("geometry"))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Lines []struct {
			Text   string
			Box    []int
			Glyphs []struct {
				Text string
				Box  []int
			}
		}
	}
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) != 1 || result.Lines[0].Text != strconv.Itoa(pid)+":geometry" || len(result.Lines[0].Glyphs) != 1 || result.Lines[0].Box[2] != 90 {
		t.Fatalf("layout or worker identity changed: %s", value)
	}
	if workerReply(t, s, "after") != pid {
		t.Fatal("layout replaced the warm text worker")
	}
}

func TestLocalOCRCollectionKeepsEmptyFragmentsDistinctFromFailure(t *testing.T) {
	s, _ := ocrWorkerFixture(t, time.Minute)
	fragment, ok := any(s).(interface {
		RecognizeFragment(context.Context, []byte) (string, error)
	})
	if !ok {
		t.Fatal("collection cannot skip a successfully recognized empty fragment")
	}
	if text, err := fragment.RecognizeFragment(context.Background(), []byte("blank")); err != nil || text != "" {
		t.Fatalf("blank fragment: %q %v", text, err)
	}
	if _, err := s.Recognize(context.Background(), []byte("blank")); err == nil {
		t.Fatal("ordinary OCR lost no-text feedback")
	}
	if _, err := fragment.RecognizeFragment(context.Background(), []byte("crash")); err == nil {
		t.Fatal("worker failure was treated as an empty fragment")
	}
}
