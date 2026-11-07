//go:build windows

package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestOneBotRiskConfirmationBindsArgumentsFactsAndOneCall(t *testing.T) {
	args := json.RawMessage(`{"id":"rule","enabled":true}`)
	prepared := json.RawMessage(`{"enabled":true,"id":"rule"}`)
	facts := json.RawMessage(`{"risk":"resume","details":{"routine":{"revision":2}}}`)
	fingerprint, err := oneBotRiskFingerprint(args, prepared, facts)
	if err != nil {
		t.Fatal(err)
	}
	reordered, err := oneBotRiskFingerprint(prepared, args, facts)
	if err != nil || fingerprint != reordered {
		t.Fatalf("object order changed authority: %v", err)
	}
	var book oneBotRiskBook
	token, err := book.issue("session", "input", "routine_save", fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct{ args, prepared, facts json.RawMessage }{
		{json.RawMessage(`{"id":"other","enabled":true}`), prepared, facts},
		{args, json.RawMessage(`{"id":"rule","enabled":false}`), facts},
		{args, prepared, json.RawMessage(`{"risk":"resume","details":{"routine":{"revision":3}}}`)},
	} {
		changed, err := oneBotRiskFingerprint(change.args, change.prepared, change.facts)
		if err != nil {
			t.Fatal(err)
		}
		if err := book.consume(token, "session", "input", "routine_save", "teammate.routines.save", changed); err == nil {
			t.Fatal("changed call reused confirmation")
		}
	}
	for _, change := range [][4]string{{"other", "input", "routine_save", "teammate.routines.save"}, {"session", "next-input", "routine_save", "teammate.routines.save"}, {"session", "input", "routine_delete", "teammate.routines.delete"}} {
		if err := book.consume(token, change[0], change[1], change[2], change[3], fingerprint); err == nil {
			t.Fatal("another scope reused confirmation")
		}
	}
	if book.confirmedFor(token, "session", "input", "teammate.routines.save") {
		t.Fatal("unconsumed token was admitted")
	}
	if err := book.consume(token, "session", "input", "routine_save", "teammate.routines.save", fingerprint); err != nil {
		t.Fatal(err)
	}
	if !book.confirmedFor(token, "session", "input", "teammate.routines.save") {
		t.Fatal("active call lost confirmation")
	}
	if err := book.consume(token, "session", "input", "routine_save", "teammate.routines.save", fingerprint); err == nil {
		t.Fatal("confirmation consumed twice")
	}
	book.finish(token)
	if book.confirmedFor(token, "session", "input", "teammate.routines.save") {
		t.Fatal("another call inherited completed authority")
	}
}

func TestOneBotRiskConfirmationExpiresAndInvalidJSONFails(t *testing.T) {
	var book oneBotRiskBook
	token, err := book.issue("session", "input", "routine_delete", "fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	value := book.pending[token]
	value.Expires = time.Now().Add(-time.Second)
	book.pending[token] = value
	if err := book.consume(token, "session", "input", "routine_delete", "teammate.routines.delete", "fingerprint"); err == nil {
		t.Fatal("expired confirmation accepted")
	}
	if _, err := oneBotRiskFingerprint(json.RawMessage(`{`), json.RawMessage(`{}`), json.RawMessage(`{}`)); err == nil {
		t.Fatal("invalid arguments admitted")
	}
}
