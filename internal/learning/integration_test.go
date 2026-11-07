package learning

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// External services are opt-in. Anki additionally verifies the disposable
// profile name before creating anything, protecting a user's normal collection.
func TestAnkiIsolatedIntegration(t *testing.T) {
	endpoint := os.Getenv("WINONE_TEST_ANKI_ENDPOINT")
	if endpoint == "" {
		t.Skip("isolated Anki profile not selected")
	}
	a := Anki{Endpoint: endpoint}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var profiles []string
	if err := a.invoke(ctx, "getProfiles", nil, &profiles); err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0] != "WinOne Test" {
		t.Fatalf("refusing non-test profiles %q", profiles)
	}
	status, err := a.Status(ctx)
	if err != nil || status.Version < 6 {
		t.Fatal(status, err)
	}
	o := Defaults()
	o.Deck = fmt.Sprintf("WinOne Test %d *_\"", time.Now().UnixNano())
	n := Note{Word: "sustainable", Phonetic: "səˈsteɪnəbl", Meaning: "a. 可持续的 <safe> & useful", Example: "We need sustainable solutions.", Etymology: "sustain + able"}
	var audio []byte
	if os.Getenv("WINONE_TEST_LEARNING_ONLINE") == "1" {
		audio, err = (Online{CacheDirectory: t.TempDir()}).Pronunciation(ctx, n.Word, "us")
		if err != nil {
			t.Fatal(err)
		}
	}
	added, err := a.Add(ctx, o, n, audio)
	if err != nil || added.Existing || added.ID <= 0 {
		t.Fatal(added, err)
	}
	duplicate, err := a.Add(ctx, o, n, audio)
	if err != nil || !duplicate.Existing || duplicate.ID != added.ID {
		t.Fatal("duplicate", duplicate, err)
	}
	var notes []struct {
		Fields map[string]struct {
			Value string `json:"value"`
		} `json:"fields"`
	}
	if err = a.invoke(ctx, "notesInfo", map[string]any{"notes": []int64{added.ID}}, &notes); err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || notes[0].Fields["Meaning"].Value != escapedField(n.Meaning) {
		t.Fatal("field mismatch", notes)
	}
	if len(audio) > 0 {
		name := strings.TrimSuffix(strings.TrimPrefix(notes[0].Fields["Audio"].Value, "[sound:"), "]")
		var stored string
		if err = a.invoke(ctx, "retrieveMediaFile", map[string]any{"filename": name}, &stored); err != nil {
			t.Fatal(err)
		}
		decoded, err := base64.StdEncoding.DecodeString(stored)
		if err != nil || !bytes.Equal(decoded, audio) {
			t.Fatal("audio roundtrip", err)
		}
	}
	o.CardMode = "typing"
	n.Word = "coordination"
	if _, err = a.Add(ctx, o, n, nil); err != nil {
		t.Fatal("typing", err)
	}
	var templates map[string]map[string]string
	if err = a.invoke(ctx, "modelTemplates", map[string]any{"modelName": "WinOne Word Typing"}, &templates); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(templates["WinOne"]["Front"], "{{type:Word}}") {
		t.Fatal("typing template missing", templates)
	}
	if err = a.Browse(ctx, o.Deck, ""); err != nil {
		t.Fatal("browse", err)
	}
	if err = a.Review(ctx, o.Deck); err != nil {
		t.Fatal("review", err)
	}
	t.Logf("Anki API %d: fields, exact duplicate, media (%d bytes), typing, browse and review passed", status.Version, len(audio))
}

func TestLearningOnlineServices(t *testing.T) {
	if os.Getenv("WINONE_TEST_LEARNING_ONLINE") != "1" {
		t.Skip("public online fixtures not selected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	o := Online{CacheDirectory: t.TempDir()}
	for _, accent := range []string{"uk", "us"} {
		audio, err := o.Pronunciation(ctx, "take care", accent)
		if err != nil || len(audio) < 1000 {
			t.Fatal(accent, len(audio), err)
		}
		t.Logf("%s phrase recording: %d bytes", accent, len(audio))
	}
	entries, err := o.Lookup(ctx, "hello")
	if err != nil || len(entries) == 0 || len(entries[0].Meanings) == 0 {
		t.Fatal(entries, err)
	}
	t.Logf("English dictionary returned %d records", len(entries))
}
