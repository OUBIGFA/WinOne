package ankiengine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"winone/internal/learning"
)

func ankiTestPath(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func TestBundledAnkiWorkerSerializesPersistsAndExits(t *testing.T) {
	python, library := os.Getenv("WINONE_TEST_ANKI_PYTHON"), os.Getenv("WINONE_TEST_ANKI_LIBRARY")
	if python == "" || library == "" {
		t.Skip("explicit offline bundled runtime")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	options := Options{Python: python, Worker: ankiTestPath("WINONE_TEST_ANKI_WORKER", filepath.Join(root, "modules", "anki", "worker.py")), Library: library, Source: ankiTestPath("WINONE_TEST_ANKI_SOURCE", filepath.Join(root, "third_party", "anki-connect", "plugin")), Profile: t.TempDir()}
	e := New(options)
	t.Cleanup(func() { _ = e.Close() })
	if err = e.Available(); err != nil || e.cmd != nil {
		t.Fatalf("availability prewarmed the worker: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var version int
	if _, err = e.Call(ctx, "legacy", map[string]any{"action": "version", "params": map[string]any{}}, &version); err != nil || version != 6 {
		t.Fatalf("version=%d: %v", version, err)
	}
	var wg sync.WaitGroup
	for _, name := range []string{"工作", "学习", "Reading", "Phrases"} {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			var result json.RawMessage
			if _, err := e.Call(ctx, "decks.create", map[string]string{"name": name}, &result); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if _, err = e.Call(cancelled, "decks.create", map[string]string{"name": "must not exist"}, nil); err == nil {
		t.Fatal("cancelled mutation accepted")
	}
	process := e.cmd
	if accepted, err := e.CancelCurrent(12345); err != nil || accepted {
		t.Fatalf("unrelated caller cancellation accepted: %v %v", accepted, err)
	}
	e.mu.Lock()
	e.lastUsed = time.Now().Add(-time.Minute)
	e.mu.Unlock()
	if !e.releaseIdle() {
		t.Fatal("unused worker was not released")
	}
	if process.ProcessState == nil || !process.ProcessState.Exited() {
		t.Fatal("idle worker remained running")
	}
	if err = e.Close(); err != nil {
		t.Fatal(err)
	}
	if process.ProcessState == nil || !process.ProcessState.Exited() {
		t.Fatal("worker did not exit")
	}
	e2 := New(options)
	defer e2.Close()
	var decks []struct{ Name string }
	if _, err = e2.Call(ctx, "decks.list", map[string]any{}, &decks); err != nil {
		t.Fatal(err)
	}
	if len(decks) != 5 {
		t.Fatalf("expected default and four persisted decks: %+v", decks)
	}
}

func TestLearningAddUsesBundledAnkiForTypingDuplicateAndAudio(t *testing.T) {
	python, library := os.Getenv("WINONE_TEST_ANKI_PYTHON"), os.Getenv("WINONE_TEST_ANKI_LIBRARY")
	if python == "" || library == "" {
		t.Skip("explicit offline runtime")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	e := New(Options{Python: python, Library: library, Worker: ankiTestPath("WINONE_TEST_ANKI_WORKER", filepath.Join(root, "modules/anki/worker.py")), Source: ankiTestPath("WINONE_TEST_ANKI_SOURCE", filepath.Join(root, "third_party/anki-connect/plugin")), Profile: t.TempDir()})
	defer e.Close()
	client := learning.Anki{Call: func(ctx context.Context, action string, params, out any) error {
		if params == nil {
			params = map[string]any{}
		}
		_, err := e.Call(ctx, "legacy", map[string]any{"action": action, "params": params}, out)
		return err
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	options := learning.Defaults()
	options.Deck = `Study "words"_*`
	options.CardMode = "typing"
	note := learning.Note{Word: "hello", Meaning: "你好 <example>", Example: "Hello & welcome", Etymology: "root"}
	first, err := client.Add(ctx, options, note, []byte("ID3-isolated-audio"))
	if err != nil || first.ID == 0 || first.Existing {
		t.Fatalf("first add: %+v %v", first, err)
	}
	second, err := client.Add(ctx, options, note, nil)
	if err != nil || second.ID != first.ID || !second.Existing {
		t.Fatalf("duplicate: %+v %v", second, err)
	}
	if err = client.Review(ctx, options.Deck); err != nil {
		t.Fatal(err)
	}
	var review struct {
		Typing   bool
		Rendered struct {
			Question string `json:"questionHtml"`
			Answer   string `json:"answerHtml"`
		}
	}
	if _, err = e.Call(ctx, "winone.review", map[string]string{"typed": "hello"}, &review); err != nil {
		t.Fatal(err)
	}
	if !review.Typing || !strings.Contains(review.Rendered.Question, "&lt;example&gt;") || !strings.Contains(review.Rendered.Answer, "audio controls") {
		t.Fatalf("typing/HTML/audio lost: %+v", review)
	}
}
