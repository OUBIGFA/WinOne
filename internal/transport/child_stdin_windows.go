//go:build windows

package transport

import (
	"context"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// ChildStdin creates a private inherited stdin with an overlapped parent end.
// Unlike an anonymous Windows pipe, the parent's writes have working deadlines
// and Close can cancel them even if the child stops reading during DLL teardown.
// The caller closes reader immediately after Start and owns writer thereafter.
func ChildStdin(ctx context.Context) (*os.File, net.Conn, error) {
	return childPipe(ctx, windows.GENERIC_READ, "child-stdin")
}

// ChildStdout gives an inherited synchronous writer to a child while the
// parent retains an overlapped reader with cancellable reads and deadlines.
func ChildStdout(ctx context.Context) (*os.File, net.Conn, error) {
	return childPipe(ctx, windows.GENERIC_WRITE, "child-stdout")
}

func childPipe(ctx context.Context, access uint32, label string) (*os.File, net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	name, err := PipeName()
	if err != nil {
		return nil, nil, err
	}
	name += "-" + label + "-" + rand.Text()
	listener, err := Listen(name)
	if err != nil {
		return nil, nil, err
	}
	defer listener.Close()
	type accepted struct {
		conn net.Conn
		err  error
	}
	connections := make(chan accepted, 1)
	go func() { conn, err := listener.Accept(); connections <- accepted{conn, err} }()
	var reader *os.File
	var writer net.Conn
	failed, consumed := true, false
	defer func() {
		if failed {
			listener.Close()
			if !consumed {
				// Closing the listener also releases its outstanding Accept.
				writer = (<-connections).conn
			}
			if reader != nil {
				reader.Close()
			}
			if writer != nil {
				writer.Close()
			}
		}
	}()
	path, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, nil, err
	}
	for {
		// Only the child end is synchronous, matching ordinary stdin in Go,
		// Rust and native readers. os/exec duplicates this handle for the child.
		handle, openErr := windows.CreateFile(path, access, 0, nil, windows.OPEN_EXISTING,
			windows.SECURITY_SQOS_PRESENT|windows.SECURITY_IDENTIFICATION, 0)
		if openErr == nil {
			reader = os.NewFile(uintptr(handle), label)
			break
		}
		if !errors.Is(openErr, windows.ERROR_FILE_NOT_FOUND) && !errors.Is(openErr, windows.ERROR_PIPE_BUSY) {
			return nil, nil, openErr
		}
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	select {
	case connection := <-connections:
		consumed = true
		writer = connection.conn
		if connection.err != nil {
			return nil, nil, connection.err
		}
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	failed = false
	return reader, writer, nil
}
