//go:build windows

package transport

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"
	"winone/internal/protocol"
)

func TestChildStdinReaderHelper(t *testing.T) {
	if os.Getenv("WINONE_CHILD_STDIN_TEST") != "1" {
		return
	}
	data, err := protocol.ReadFrame(os.Stdin)
	if err != nil || string(data) != "inherited input" {
		os.Exit(2)
	}
	os.Stdout.Write(data)
	os.Exit(0)
}

func TestChildStdinWorksAsInheritedInput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	reader, writer, err := ChildStdin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	defer reader.Close()
	var output bytes.Buffer
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestChildStdinReaderHelper$")
	child.Env = append(os.Environ(), "WINONE_CHILD_STDIN_TEST=1")
	child.Stdin, child.Stdout = reader, &output
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	reader.Close()
	if err = writer.SetWriteDeadline(time.Now().Add(time.Second)); err == nil {
		err = protocol.WriteFrame(writer, []byte("inherited input"))
	}
	if err != nil {
		t.Error(err)
	}
	if err = child.Wait(); err != nil {
		t.Fatal(err)
	}
	if output.String() != "inherited input" {
		t.Fatal("child did not read the complete frame")
	}
}

func TestChildStdinDeadlineCancelsAFullNativePipe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	reader, writer, err := ChildStdin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	if err = writer.SetWriteDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	_, err = writer.Write(make([]byte, protocol.MaxFrameSize))
	if failure, ok := err.(interface{ Timeout() bool }); !ok || !failure.Timeout() {
		t.Fatal("full native pipe did not time out", err)
	}
	cancel()
	if _, _, err = ChildStdin(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled launch created another pipe", err)
	}
}
