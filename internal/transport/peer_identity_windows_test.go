//go:build windows

package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Microsoft/go-winio"
	"os"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/protocol"
)

type peerIdentityHandler struct{}

func (peerIdentityHandler) Handle(request host.Request) host.Response {
	data, _ := json.Marshal(request.PeerPID)
	return host.Response{ID: request.ID, Result: data}
}

func TestPeerIdentityComesFromWindowsNotTheRequest(t *testing.T) {
	name := fmt.Sprintf(`\\.\pipe\WinOne-peer-%d-%d`, os.Getpid(), time.Now().UnixNano())
	listener, err := Listen(name)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go Serve(ctx, listener, peerIdentityHandler{})
	conn, err := winio.DialPipeContext(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err = protocol.WriteFrame(conn, []byte(`{"version":1,"id":"peer","method":"read","PeerPID":123,"peerPID":456}`)); err != nil {
		t.Fatal(err)
	}
	data, err := protocol.ReadFrame(conn)
	if err != nil {
		t.Fatal(err)
	}
	var response host.Response
	if err = json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
	var pid uint32
	if err = json.Unmarshal(response.Result, &pid); err != nil {
		t.Fatal(err)
	}
	if pid != uint32(os.Getpid()) {
		t.Fatalf("reported peer=%d, actual=%d", pid, os.Getpid())
	}
}
