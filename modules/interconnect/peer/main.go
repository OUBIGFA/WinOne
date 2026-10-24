// WinOne's process/UI adapter. Address encoding, authentication, NAT traversal
// and encrypted streams are supplied by official Tailcat v0.7.0.
package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"github.com/tailscale/tailcat"
	"tailscale.com/types/key"
)

type specification struct {
	Mode        string               `json:"mode"`
	Code        string               `json:"code"`
	Target      string               `json:"target"`
	Listen      string               `json:"listen"`
	RemotePort  uint16               `json:"remotePort"`
	DERPMapURL  string               `json:"derpMapURL"`
	Key         key.NodePrivate      `json:"key"`
	PSK         tailcat.PresharedKey `json:"psk"`
	ShareURL    string               `json:"shareURL"`
	AllowWrites bool                 `json:"allowWrites"`
	DataDir     string               `json:"dataDir"`
	Paths       []string             `json:"paths"`
	Resume      bool                 `json:"resume"`
}
type snapshot struct {
	Status   string          `json:"status"`
	Ready    bool            `json:"ready"`
	Code     string          `json:"code,omitempty"`
	QR       string          `json:"qr,omitempty"`
	Address  string          `json:"address,omitempty"`
	Error    string          `json:"error,omitempty"`
	Detail   string          `json:"detail,omitempty"`
	Active   int64           `json:"active"`
	Sent     int64           `json:"sent"`
	Received int64           `json:"received"`
	Peers    []peerPath      `json:"peers,omitempty"`
	Share    json.RawMessage `json:"share,omitempty"`
	Updating bool            `json:"updating"`
}
type peerPath struct {
	Direct   bool   `json:"direct"`
	Relay    string `json:"relay,omitempty"`
	Sent     int64  `json:"sent"`
	Received int64  `json:"received"`
}
type task struct {
	mu           sync.Mutex
	state        snapshot
	spec         specification
	cancel       context.CancelFunc
	done         chan struct{}
	server       *tailcat.Server
	client       *tailcat.Client
	active       sync.Map
	connections  sync.WaitGroup
	activeCount  atomic.Int64
	sent         atomic.Int64
	received     atomic.Int64
	lan          *child
	shareMu      sync.Mutex
	ctx          context.Context
	savedAddress tailcat.Addr
}

func decode(data []byte, value any) error {
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("命令含多余数据")
	}
	return nil
}
func start(spec specification) (*task, error) {
	switch spec.Mode {
	case "tunnel_server":
		if _, _, err := net.SplitHostPort(spec.Target); err != nil {
			return nil, fmt.Errorf("目标须为主机:端口: %w", err)
		}
	case "tunnel_client", "share_client":
		if _, err := tailcat.ParseAddr(tailcat.Addr(spec.Code)); err != nil {
			return nil, fmt.Errorf("Tailcat 地址无效: %w", err)
		}
		if spec.Listen == "" {
			spec.Listen = "127.0.0.1:0"
		}
		host, _, err := net.SplitHostPort(spec.Listen)
		if err != nil {
			return nil, err
		}
		// The share browser is a local, credential-bearing gateway. Keep its
		// origin consistent with the desktop CSP and never publish it to LAN.
		if spec.Mode == "share_client" && host != "127.0.0.1" {
			return nil, errors.New("分享接收端必须监听 127.0.0.1:端口")
		}
	case "share_server":
		if !filepath.IsAbs(spec.DataDir) {
			return nil, errors.New("共享数据目录必须为绝对路径")
		}
	default:
		return nil, errors.New("不支持的互联服务")
	}
	if spec.RemotePort == 0 {
		spec.RemotePort = 1
	}
	if strings.HasPrefix(spec.Mode, "share_") {
		spec.RemotePort = 80
	}
	ctx, cancel := context.WithCancel(context.Background())
	t := &task{spec: spec, ctx: ctx, cancel: cancel, done: make(chan struct{}), state: snapshot{Status: "running"}}
	go t.run(ctx)
	return t, nil
}
func (t *task) read() snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.state
	s.Active = t.activeCount.Load()
	s.Sent = t.sent.Load()
	s.Received = t.received.Load()
	if t.server != nil && s.Ready {
		for _, p := range t.server.Status().Peer {
			s.Peers = append(s.Peers, peerPath{Direct: p.CurAddr != "", Relay: p.Relay, Sent: p.TxBytes, Received: p.RxBytes})
		}
	}
	if t.lan != nil && s.Ready {
		ctx, cancel := context.WithTimeout(t.ctx, 2*time.Second)
		raw, err := t.lan.call(ctx, "snapshot", nil)
		cancel()
		if err != nil {
			s.Error = err.Error()
			s.Ready = false
			s.Status = "failed"
			t.state = s
			t.cancel()
		} else {
			s.Share = raw
		}
	}
	return s
}
func (t *task) stop() error {
	t.cancel()
	select {
	case <-t.done:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("互联服务未在 5 秒内停止")
	}
}
func (t *task) run(ctx context.Context) {
	var err error
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("Tailcat 运行异常: %v", p)
		}
		cancelled := ctx.Err() != nil
		t.mu.Lock()
		t.state.Ready = false
		t.mu.Unlock()
		t.cancel()
		t.active.Range(func(k, v any) bool { _ = k.(net.Conn).Close(); return true })
		t.connections.Wait()
		if t.lan != nil {
			err = errors.Join(err, t.lan.close())
		}
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.server != nil {
			err = errors.Join(err, t.server.Close())
		}
		if t.client != nil {
			err = errors.Join(err, t.client.Close())
		}
		t.state.Ready = false
		if t.state.Status != "failed" {
			t.state.Status = "cancelled"
		}
		if err != nil && !cancelled {
			t.state.Status = "failed"
			t.state.Error = err.Error()
		}
		close(t.done)
	}()
	var listener net.Listener
	if t.spec.Mode == "share_server" {
		if err = t.prepareSource(ctx); err != nil {
			return
		}
	}
	if strings.HasSuffix(t.spec.Mode, "_server") {
		if err = t.identity(); err != nil {
			return
		}
		server := &tailcat.Server{Key: t.spec.Key, PresharedKey: t.spec.PSK, DERPMapURL: t.spec.DERPMapURL, Logf: func(string, ...any) {}}
		if t.savedAddress != "" {
			var info tailcat.ConnInfo
			info, err = tailcat.ParseAddr(t.savedAddress)
			if err != nil {
				return
			}
			server.RegionID = info.RegionID
			if len(info.Region) > 0 {
				server.Region = info.Region[0]
			}
		}
		t.mu.Lock()
		t.server = server
		t.mu.Unlock()
		// In v0.7.0 Listen, rather than Start, takes a cancellation context.
		startup, cancel := context.WithTimeout(ctx, 30*time.Second)
		listener, err = server.Listen(startup, "tcp", ":"+strconv.Itoa(int(t.spec.RemotePort)))
		cancel()
		if err != nil {
			return
		}
		if err = t.saveAddress(server.TailcatAddr()); err != nil {
			listener.Close()
			return
		}
		t.mu.Lock()
		t.state.Code = string(server.TailcatAddr())
		t.state.QR = qr(t.state.Code)
		t.state.Ready = true
		t.mu.Unlock()
	} else {
		client := &tailcat.Client{Server: tailcat.Addr(t.spec.Code), DERPMapURL: t.spec.DERPMapURL, Logf: func(string, ...any) {}}
		t.mu.Lock()
		t.client = client
		t.mu.Unlock()
		// Verify both authentication and the remote service before claiming ready.
		startup, cancel := context.WithTimeout(ctx, 30*time.Second)
		var remote net.Conn
		remote, err = client.DialTCPPort(startup, t.spec.RemotePort)
		cancel()
		if err != nil {
			return
		}
		remote.Close()
		listener, err = net.Listen("tcp", t.spec.Listen)
		if err != nil {
			return
		}
		t.mu.Lock()
		t.state.Address = listener.Addr().String()
		t.state.Ready = true
		t.mu.Unlock()
	}
	defer listener.Close()
	stopListener := context.AfterFunc(ctx, func() { listener.Close(); t.active.Range(func(k, v any) bool { k.(net.Conn).Close(); return true }) })
	defer stopListener()
	if t.spec.Mode == "share_server" || t.spec.Mode == "share_client" {
		var handler http.Handler
		if t.spec.Mode == "share_server" {
			handler, err = shareHandler(ctx, t.spec.ShareURL, t.spec.AllowWrites)
		} else {
			// The local browser gateway rejects DNS rebinding and cross-origin writes.
			// File bytes pass through standard HTTP, with Tailcat providing DialContext.
			transport := &http.Transport{DialContext: func(c context.Context, network, address string) (net.Conn, error) { return t.client.DialTCPPort(c, 80) }}
			defer transport.CloseIdleConnections()
			proxy := &httputil.ReverseProxy{Transport: transport, Rewrite: func(r *httputil.ProxyRequest) { r.Out.URL.Scheme = "http"; r.Out.URL.Host = "tailcat" }, ErrorHandler: func(w http.ResponseWriter, r *http.Request, e error) {
				http.Error(w, "远端分享暂不可用", http.StatusBadGateway)
			}}
			authority := listener.Addr().String()
			handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Host != authority || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+authority) {
					http.Error(w, "请从互联助手打开分享", http.StatusForbidden)
					return
				}
				proxy.ServeHTTP(w, r)
			})
		}
		if err != nil {
			return
		}
		server := &http.Server{Handler: handler, ReadHeaderTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }}
		stopHTTP := context.AfterFunc(ctx, func() { server.Close() })
		defer stopHTTP()
		defer server.Close()
		err = server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		return
	}
	// Adapted from Tailcat cmd/tailcat/forward.go: track accepted connections,
	// cancel dials, then close streams and wait before destroying the client.
	for {
		var conn net.Conn
		conn, err = listener.Accept()
		if err != nil {
			return
		}
		t.active.Store(conn, true)
		t.connections.Add(1)
		t.activeCount.Add(1)
		go t.forward(ctx, conn)
	}
}
func (t *task) forward(ctx context.Context, conn net.Conn) {
	defer t.connections.Done()
	defer t.activeCount.Add(-1)
	defer t.active.Delete(conn)
	defer conn.Close()
	dial, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var other net.Conn
	var err error
	if t.client != nil {
		other, err = t.client.DialTCPPort(dial, t.spec.RemotePort)
	} else {
		other, err = (&net.Dialer{}).DialContext(dial, "tcp", t.spec.Target)
	}
	if err != nil {
		if ctx.Err() == nil {
			t.mu.Lock()
			t.state.Detail = err.Error()
			t.mu.Unlock()
		}
		return
	}
	defer other.Close()
	t.active.Store(other, true)
	defer t.active.Delete(other)
	stopPair := context.AfterFunc(ctx, func() { conn.Close(); other.Close() })
	defer stopPair()
	tailcat.ProxyConns(&countedConn{Conn: conn, read: &t.sent, write: &t.received}, other)
}

type countedConn struct {
	net.Conn
	read, write *atomic.Int64
}

func (c *countedConn) Read(p []byte) (int, error) {
	n, e := c.Conn.Read(p)
	c.read.Add(int64(n))
	return n, e
}
func (c *countedConn) Write(p []byte) (int, error) {
	n, e := c.Conn.Write(p)
	c.write.Add(int64(n))
	return n, e
}
func (c *countedConn) CloseWrite() error {
	if h, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return h.CloseWrite()
	}
	return nil
}

func shareHandler(ctx context.Context, raw string, allowWrites bool) (http.Handler, error) {
	target, err := url.Parse(raw)
	if err != nil || target.Scheme != "http" || target.Hostname() != "127.0.0.1" || target.RawQuery == "" || target.User != nil {
		return nil, errors.New("共享目标必须是本机 FileShare 服务")
	}
	token := target.Query().Get("key")
	if token == "" {
		return nil, errors.New("缺少本机共享凭据")
	}
	target.RawQuery = ""
	target.Path = ""
	proxy := &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) {
		r.SetURL(target)
		r.Out.Host = target.Host
		r.Out.Header.Del("Cookie")
		r.Out.Header.Set("Authorization", "Bearer "+token)
		if r.In.Header.Get("Origin") != "" {
			r.Out.Header.Set("Origin", target.Scheme+"://"+target.Host)
		}
	}, ModifyResponse: func(r *http.Response) error { r.Header.Del("Set-Cookie"); return nil }, ErrorHandler: func(w http.ResponseWriter, r *http.Request, e error) {
		http.Error(w, "共享源暂不可用", http.StatusBadGateway)
	}}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
			http.Error(w, "请从共享页面操作", http.StatusForbidden)
			return
		}
		if !allowWrites && r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "此分享仅允许下载", http.StatusForbidden)
			return
		}
		proxy.ServeHTTP(w, r)
	}), nil
}
func qr(value string) string {
	data, err := qrcode.Encode(value, qrcode.Medium, 256)
	if err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
}
func main() {
	var current *task
	defer func() {
		if current != nil {
			current.stop()
		}
	}()
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	output := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var req struct {
			ID     string          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		err := decode(scanner.Bytes(), &req)
		var result any
		if err == nil {
			switch req.Method {
			case "identity":
				result = map[string]any{"key": key.NewNode(), "psk": tailcat.NewPresharedKey()}
			case "start":
				if current != nil {
					err = errors.New("进程已经运行服务")
				} else {
					var spec specification
					err = decode(req.Params, &spec)
					if err == nil {
						current, err = start(spec)
					}
				}
				result = map[string]bool{"started": err == nil}
			case "snapshot":
				if current == nil {
					err = errors.New("服务未启动")
				} else {
					result = current.read()
				}
			case "add", "remove":
				if current == nil {
					err = errors.New("服务未启动")
				} else {
					err = current.changeShare(req.Method, req.Params)
				}
				result = map[string]bool{"accepted": err == nil}
			case "stop":
				if current != nil {
					err = current.stop()
				}
				result = map[string]bool{"stopped": err == nil}
			default:
				err = errors.New("不支持的互联命令")
			}
		}
		response := map[string]any{"id": req.ID, "result": result}
		if err != nil {
			response = map[string]any{"id": req.ID, "error": err.Error()}
		}
		if output.Encode(response) != nil {
			return
		}
		if req.Method == "stop" {
			return
		}
	}
}
