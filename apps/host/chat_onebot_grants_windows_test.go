//go:build windows

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOneBotInputGrantsSurviveRestartWithoutChangingAuthority(t *testing.T) {
	directory := t.TempDir()
	open := func() *chatOneBot { return newChatOneBot(&chatControl{directory: directory}) }
	first := open()
	grant := oneBotInputGrant{SessionID: "fixture-session", ConnectionID: "fixture12345", Channel: "telegram", Group: true, Owner: false, Digest: strings.Repeat("a", 64)}
	if err := first.grantInput("fixture-receipt", grant); err != nil {
		t.Fatal(err)
	}
	saved, err := first.inputGrant(grant.SessionID, "fixture-receipt")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(directory, "onebot", "input-grants.bin")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(grant.SessionID)) || bytes.Contains(raw, []byte("fixture-receipt")) {
		t.Fatal("authority persisted as editable plaintext")
	}
	reopened := open()
	restored, err := reopened.inputGrant(grant.SessionID, "fixture-receipt")
	if err != nil || !restored.Expires.Equal(saved.Expires) {
		t.Fatalf("restore expiry: %+v %v", restored, err)
	}
	// JSON preserves the instant, not Go's process-local monotonic clock.
	restored.Expires = saved.Expires
	if restored != saved {
		t.Fatalf("restore authority: %+v", restored)
	}
	if _, err := reopened.inputGrant("another-session", "fixture-receipt"); err == nil {
		t.Fatal("receipt crossed session boundary")
	}
	for _, modify := range []func(*oneBotInputGrant){func(g *oneBotInputGrant) { g.Owner = true }, func(g *oneBotInputGrant) { g.Group = false }, func(g *oneBotInputGrant) { g.Digest = strings.Repeat("b", 64) }, func(g *oneBotInputGrant) { g.ConnectionID = "another12345" }} {
		changed := grant
		modify(&changed)
		if err := reopened.grantInput("fixture-receipt", changed); err == nil {
			t.Fatal("replayed receipt changed authority")
		}
	}
	if err := reopened.grantInput("fixture-receipt", grant); err != nil {
		t.Fatal(err)
	}
	unchanged, _ := reopened.inputGrant(grant.SessionID, "fixture-receipt")
	if !unchanged.Expires.Equal(saved.Expires) {
		t.Fatal("duplicate delivery extended the grant")
	}
	corrupted := append([]byte(nil), raw...)
	corrupted[len(corrupted)/2] ^= 0x80
	if err := os.WriteFile(file, corrupted, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := open().inputGrant(grant.SessionID, "fixture-receipt"); err == nil {
		t.Fatal("corrupted protected authority was accepted")
	}
}

func TestOneBotInputGrantWriteFailureAndExpiryFailClosed(t *testing.T) {
	directory := t.TempDir()
	bot := newChatOneBot(&chatControl{directory: directory})
	grant := oneBotInputGrant{SessionID: "fixture-session", Digest: strings.Repeat("a", 64)}
	if err := bot.grantInput("first", grant); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(directory, "onebot", "input-grants.bin")
	if err := os.Rename(file, file+".kept"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	if err := bot.grantInput("not-persisted", grant); err == nil {
		t.Fatal("failed write reported success")
	}
	if _, err := bot.inputGrant(grant.SessionID, "not-persisted"); err == nil {
		t.Fatal("failed receipt still authorized in memory")
	}
	bot.grantMu.Lock()
	expired := bot.grants["first"]
	expired.Expires = time.Now().Add(-time.Second)
	bot.grants["first"] = expired
	bot.grantMu.Unlock()
	if _, err := bot.inputGrant(grant.SessionID, "first"); err == nil {
		t.Fatal("expired receipt authorized an operation")
	}
}
