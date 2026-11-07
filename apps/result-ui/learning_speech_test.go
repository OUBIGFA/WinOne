package resultui

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLearningSpeechKeepsRequestedAccentAndEscapesText(t *testing.T) {
	british := speechSSMLVoice("hello <world>", "en-GB-SoniaNeural")
	if !strings.Contains(british, "en-GB-SoniaNeural") || strings.Contains(british, "<world>") {
		t.Fatal(british)
	}
	if !strings.Contains(speechSSML("hello"), edgeSpeechVoice) {
		t.Fatal("existing speech voice changed")
	}
}

func TestLearningBritishSpeechOnline(t *testing.T) {
	if os.Getenv("WINONE_TEST_SPEECH_ONLINE") != "1" {
		t.Skip("public online fixture not selected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	audio, err := edgeSpeech.synthesizeVoice(ctx, "Sustainable development.", "en-GB-SoniaNeural")
	if err != nil || len(audio) < 1000 {
		t.Fatal(len(audio), err)
	}
	t.Logf("British fallback synthesized %d bytes", len(audio))
}

func TestSpeechGECV2CapturedVector(t *testing.T) {
	// Public protocol capture from Edge at 2026-10-13 10:40 UTC; confirms
	// interoperability independently of the implementation's internal rounds.
	window := time.Date(2026, 10, 13, 10, 40, 0, 0, time.UTC)
	want := "492D7694A183A45E-56E821F95D4579C5B7C7E33DC7303FF379EC36D510213555BBBB9EB17EF15876"
	if got := speechGEC(window); got != want {
		t.Fatalf("got %s", got)
	}
	if speechGEC(window.Add(299*time.Second)) != want || speechGEC(window.Add(300*time.Second)) == want {
		t.Fatal("incorrect token window")
	}
}
