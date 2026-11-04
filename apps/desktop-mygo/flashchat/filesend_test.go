package flashchat

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFilesGoDirectlyToTheGroupAndToOneMember(t *testing.T) {
	network := NewMemoryNetwork()
	dirA, dirB, dirC := t.TempDir(), t.TempDir(), t.TempDir()
	join := func(id, dir string) *Session {
		s, err := Join(JoinOptions{RoomID: "room", Password: "secret", Nickname: id, Transport: network.Join(id),
			ReceiveDir: func() (string, error) { return dir, nil }})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(s.Leave)
		return s
	}
	a, b, c := join("a", dirA), join("b", dirB), join("c", dirC)
	wait := func(label string, ok func() bool) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for !ok() {
			if time.Now().After(deadline) {
				t.Fatal("timed out waiting for " + label)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	wait("peers", func() bool { return len(a.transport.Peers()) == 2 })

	content := make([]byte, 3*filePiece+123)
	_, _ = rand.Read(content)
	source := filepath.Join(t.TempDir(), "report.bin")
	if err := os.WriteFile(source, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.SendFile(source, ""); err != nil {
		t.Fatal(err)
	}
	for _, s := range []*Session{b, c} {
		s := s
		wait("group file", func() bool { l := s.Transfers(""); return len(l) == 1 && l[0].State == TransferDone })
		got, err := os.ReadFile(s.Transfers("")[0].Path)
		if err != nil || !bytes.Equal(got, content) || filepath.Base(s.Transfers("")[0].Path) != "report.bin" {
			t.Fatalf("received %d bytes at %s: %v", len(got), s.Transfers("")[0].Path, err)
		}
	}
	wait("sent", func() bool {
		l := a.Transfers("")
		return len(l) == 1 && l[0].State == TransferDone && l[0].Moved == int64(len(content))
	})

	// A private file reaches only its member, as a second copy beside the first.
	if err := a.SendFile(source, b.transport.(*MemoryTransport).id); err != nil {
		t.Fatal(err)
	}
	wait("direct file", func() bool { l := b.Transfers("a"); return len(l) == 1 && l[0].State == TransferDone })
	if filepath.Base(b.Transfers("a")[0].Path) != "report (2).bin" {
		t.Fatalf("second copy %s", b.Transfers("a")[0].Path)
	}
	time.Sleep(50 * time.Millisecond)
	if len(c.Transfers("a")) != 0 || len(c.Transfers("")) != 1 {
		t.Fatal("a private file reached another member")
	}
}

func TestReceivedNamesStayInTheFolder(t *testing.T) {
	for in, want := range map[string]string{`..\..\evil.exe`: "evil.exe", "a/b/c.txt": "c.txt", "con:x?.txt": "con_x_.txt", "": "file", "..": "file"} {
		if got := safeName(in); got != want {
			t.Errorf("safeName(%q) = %q, want %q", in, got, want)
		}
	}
}
