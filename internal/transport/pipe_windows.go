//go:build windows

// Package transport owns local authenticated pipe connections; it has no UI dependencies.
package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/user"
	"strings"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"winone/internal/host"
	"winone/internal/protocol"
)

// PipeName separates interactive sessions and users without a shared TCP port.
func PipeName() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	var session uint32
	if err = windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &session); err != nil {
		return "", err
	}
	return fmt.Sprintf(`\\.\pipe\WinOne-%s-%d`, u.Uid, session), nil
}

// Listen uses go-winio's first-instance pipe protection and limits access to this user.
func Listen(name string) (net.Listener, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	return winio.ListenPipe(name, &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;" + u.Uid + ")", InputBufferSize: 65536, OutputBufferSize: 65536,
	})
}

// Serve drains accepted work on shutdown so acknowledged settings writes are not interrupted.
type Handler interface {
	Handle(host.Request) host.Response
}

func Serve(ctx context.Context, listener net.Listener, service Handler) error {
	var active sync.WaitGroup
	defer active.Wait()
	go func() { <-ctx.Done(); _ = listener.Close() }()
	// Bound idle clients so a local process cannot exhaust the Host's goroutines.
	slots := make(chan struct{}, 32)
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case slots <- struct{}{}:
		default:
			conn.Close()
			continue
		}
		active.Add(1)
		go func() { defer active.Done(); defer func() { <-slots }(); serveOne(conn, service) }()
	}
}

func serveOne(conn net.Conn, service Handler) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	data, err := protocol.ReadFrame(conn)
	if err != nil {
		return
	}
	var request host.Request
	if json.Unmarshal(data, &request) != nil {
		return
	}
	if file, ok := conn.(interface{ Fd() uintptr }); ok {
		if err := windows.GetNamedPipeClientProcessId(windows.Handle(file.Fd()), &request.PeerPID); err != nil {
			return
		}
	}
	if strings.HasPrefix(request.Method, "capture.") {
		_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	}
	response := service.Handle(request)
	data, err = json.Marshal(response)
	if err != nil {
		return
	}
	_ = protocol.WriteFrame(conn, data)
}

// Call uses one bounded request per connection; mutations are never automatically retried.
func Call(ctx context.Context, name string, request host.Request) (host.Response, error) {
	return CallWithTimeout(ctx, name, request, 5*time.Second)
}

// CallWithTimeout allows the desktop's first on-demand worker startup to finish.
func CallWithTimeout(ctx context.Context, name string, request host.Request, timeout time.Duration) (host.Response, error) {
	return CallWithVerifiedServer(ctx, name, request, timeout, nil)
}

// CallWithVerifiedServer inspects the OS-reported peer before sending a request.
// The privilege handoff uses this instead of trusting a claimed token level in JSON.
func CallWithVerifiedServer(ctx context.Context, name string, request host.Request, timeout time.Duration, verify func(uint32) error) (host.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var response host.Response
	conn, err := winio.DialPipeContext(ctx, name)
	if err != nil {
		return response, err
	}
	defer conn.Close()
	stopCancellation := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancellation()
	if verify != nil {
		file, ok := conn.(interface{ Fd() uintptr })
		if !ok {
			return response, fmt.Errorf("pipe handle unavailable")
		}
		var pid uint32
		if err := windows.GetNamedPipeServerProcessId(windows.Handle(file.Fd()), &pid); err != nil {
			return response, err
		}
		if err := verify(pid); err != nil {
			return response, err
		}
	}
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	data, err := json.Marshal(request)
	if err != nil {
		return response, err
	}
	if err = protocol.WriteFrame(conn, data); err != nil {
		return response, err
	}
	data, err = protocol.ReadFrame(conn)
	if err != nil {
		return response, err
	}
	if err = json.Unmarshal(data, &response); err != nil {
		return response, err
	}
	if response.ID != request.ID {
		return response, fmt.Errorf("response ID mismatch")
	}
	return response, nil
}
