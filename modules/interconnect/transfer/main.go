// Each process owns exactly one croc operation. WinOne supplies UI and lifetime;
// all transfer, crypto, reconnect, collision and store protocols remain upstream.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/schollz/croc/v11/src/codephrase"
	"github.com/schollz/croc/v11/src/comm"
	"github.com/schollz/croc/v11/src/croc"
	log "github.com/schollz/croc/v11/src/logger"
	"github.com/schollz/croc/v11/src/models"
	"github.com/schollz/croc/v11/src/publicrelay"
	"github.com/schollz/croc/v11/src/storeclient"
	"github.com/schollz/croc/v11/src/storecrypto"
	"github.com/schollz/croc/v11/src/tcp"
	"github.com/schollz/croc/v11/src/utils"
	qrcode "github.com/skip2/go-qrcode"
)

type specification struct {
	Mode            string       `json:"mode"`
	Paths           []string     `json:"paths"`
	Code            string       `json:"code"`
	Text            string       `json:"text"`
	DataDir         string       `json:"dataDir"`
	OutputDir       string       `json:"outputDir"`
	Options         croc.Options `json:"options"`
	Socks5          string       `json:"socks5"`
	HTTPProxy       string       `json:"httpProxy"`
	StoreOrigin     string       `json:"storeOrigin"`
	Downloads       int          `json:"downloads"`
	ExpirationHours float64      `json:"expirationHours"`
	UploadToken     string       `json:"uploadToken"`
}

func defaults() specification {
	return specification{StoreOrigin: "https://getcroc.com", Downloads: 1, ExpirationHours: 24, Options: croc.Options{
		Curve: "p256", HashAlgorithm: "xxhash", RelayPassword: models.DEFAULT_PASSPHRASE, MulticastAddress: "239.255.255.250", RelayPorts: []string{"9009", "9010", "9011", "9012", "9013"},
	}}
}
func decode(data []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("request contains trailing data")
	}
	return nil
}

type prompt struct {
	ID   uint64 `json:"id"`
	Text string `json:"text"`
}
type snapshot struct {
	QR            string                       `json:"qr,omitempty"`
	BrowserURL    string                       `json:"browserUrl,omitempty"`
	Status        string                       `json:"status"`
	Code          string                       `json:"code,omitempty"`
	Error         string                       `json:"error,omitempty"`
	Detail        string                       `json:"detail,omitempty"`
	Output        string                       `json:"output,omitempty"`
	Prompt        *prompt                      `json:"prompt,omitempty"`
	Progress      *croc.WinOneTransferProgress `json:"progress,omitempty"`
	StoreProgress *storeclient.Progress        `json:"storeProgress,omitempty"`
	Result        any                          `json:"result,omitempty"`
}
type task struct {
	mu             sync.Mutex
	spec           specification
	state          snapshot
	client         *croc.Client
	cancel         context.CancelFunc
	done           chan struct{}
	answers        chan string
	promptSequence uint64
	diagnostics    *diagnostics
}
type diagnostics struct {
	mu   sync.Mutex
	text string
}

var escape = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]")

func (d *diagnostics) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.text += escape.ReplaceAllString(string(p), "")
	if len(d.text) > 32768 {
		d.text = strings.ToValidUTF8(d.text[len(d.text)-32768:], "")
	}
	return len(p), nil
}
func (d *diagnostics) read() string { d.mu.Lock(); defer d.mu.Unlock(); return d.text }

func newTask(spec specification, diagnostics *diagnostics) (*task, error) {
	// Empty optional controls mean the upstream default, including after the
	// user clears a previously edited field in the advanced settings.
	base := defaults().Options
	if spec.Options.Curve == "" {
		spec.Options.Curve = base.Curve
	}
	if spec.Options.HashAlgorithm == "" {
		spec.Options.HashAlgorithm = base.HashAlgorithm
	}
	if spec.Options.RelayPassword == "" {
		spec.Options.RelayPassword = base.RelayPassword
	}
	if spec.Options.MulticastAddress == "" {
		spec.Options.MulticastAddress = base.MulticastAddress
	}
	if len(spec.Options.RelayPorts) == 0 {
		spec.Options.RelayPorts = base.RelayPorts
	}
	switch spec.Mode {
	case "send", "receive", "store_send", "store_inspect", "store_receive", "store_revoke", "prepare_share":
	default:
		return nil, errors.New("unknown transfer operation")
	}
	if !filepath.IsAbs(spec.DataDir) {
		return nil, errors.New("task directory must be absolute")
	}
	if spec.Mode == "receive" || spec.Mode == "store_receive" {
		if !filepath.IsAbs(spec.OutputDir) {
			return nil, errors.New("receive directory must be absolute")
		}
	}
	if spec.Mode == "send" || spec.Mode == "store_send" {
		if len(spec.Paths) == 0 && spec.Text == "" {
			return nil, errors.New("choose files or text")
		}
	}
	for _, p := range spec.Paths {
		if !filepath.IsAbs(p) {
			return nil, errors.New("source paths must be absolute")
		}
	}
	if spec.Mode == "store_send" && (spec.Downloads < 1 || spec.ExpirationHours <= 0 || spec.ExpirationHours > 24*365) {
		return nil, errors.New("invalid download count or expiration")
	}
	if spec.Options.ThrottleUpload != "" && !regexp.MustCompile(`^[1-9][0-9]*[kKmMgG]?$`).MatchString(spec.Options.ThrottleUpload) {
		return nil, errors.New("invalid upload speed limit")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t := &task{spec: spec, state: snapshot{Status: "running"}, cancel: cancel, done: make(chan struct{}), answers: make(chan string, 1), diagnostics: diagnostics}
	go t.run(ctx)
	return t, nil
}
func (t *task) read() snapshot {
	t.mu.Lock()
	s := t.state
	client := t.client
	t.mu.Unlock()
	s.Output = t.diagnostics.read()
	if client != nil {
		progress := client.WinOneProgress()
		s.Progress = &progress
	}
	return s
}
func (t *task) input(ctx context.Context, text string) (string, error) {
	t.mu.Lock()
	t.promptSequence++
	t.state.Prompt = &prompt{ID: t.promptSequence, Text: text}
	t.mu.Unlock()
	defer func() { t.mu.Lock(); t.state.Prompt = nil; t.mu.Unlock() }()
	select {
	case answer := <-t.answers:
		return strings.TrimSpace(answer), nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
func (t *task) answer(data []byte) error {
	var value struct {
		ID     uint64 `json:"id"`
		Answer string `json:"answer"`
	}
	if err := decode(data, &value); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state.Prompt == nil || t.state.Prompt.ID != value.ID {
		return errors.New("prompt is no longer active")
	}
	select {
	case t.answers <- value.Answer:
		t.state.Prompt = nil
		return nil
	default:
		return errors.New("prompt already answered")
	}
}
func (t *task) stop() error {
	t.cancel()
	select {
	case <-t.done:
		return nil
	case <-time.After(4 * time.Second):
		return errors.New("transfer did not stop within 4 seconds")
	}
}
func (t *task) run(ctx context.Context) {
	var result any
	var err error
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("upstream transfer panic: %v", recovered)
		}
		if err == nil && ctx.Err() != nil {
			err = ctx.Err()
		}
		t.cancel()
		t.mu.Lock()
		t.state.Result = result
		t.state.Status = "completed"
		t.state.Prompt = nil
		if err != nil {
			t.state.Status = "failed"
			t.state.Error = err.Error()
		}
		if errors.Is(err, context.Canceled) {
			t.state.Status = "cancelled"
			t.state.Error = ""
		}
		t.mu.Unlock()
		close(t.done)
	}()
	if err = os.MkdirAll(t.spec.DataDir, 0700); err != nil {
		return
	}
	directory := t.spec.DataDir
	if t.spec.Mode == "receive" || t.spec.Mode == "store_receive" {
		directory = t.spec.OutputDir
		if err = os.MkdirAll(directory, 0700); err != nil {
			return
		}
	}
	if err = os.Chdir(directory); err != nil {
		return
	}
	// Proxy globals and temporary archives are confined to this one process.
	comm.Socks5Proxy = t.spec.Socks5
	comm.HttpProxy = t.spec.HTTPProxy
	utils.InputProvider = t.input
	// Text received by croc is streamed to a task-owned file, so neither a long
	// message nor terminal diagnostics can truncate the received content.
	var textOutput *os.File
	if t.spec.Mode == "receive" {
		textOutput, err = os.Create(filepath.Join(t.spec.DataDir, "received-text.txt"))
		if err != nil {
			return
		}
		os.Stdout = textOutput
		defer textOutput.Close()
	}
	if t.spec.Mode == "prepare_share" {
		result, err = t.prepareShare(ctx)
	} else if strings.HasPrefix(t.spec.Mode, "store_") {
		result, err = t.stored(ctx)
	} else {
		err = t.direct(ctx)
		if textOutput != nil && err == nil {
			if stat, statErr := textOutput.Stat(); statErr != nil {
				err = statErr
			} else if stat.Size() > 0 {
				result = map[string]any{"textPath": textOutput.Name(), "textBytes": stat.Size()}
			}
		}
	}
}
func (t *task) direct(ctx context.Context) error {
	options := t.spec.Options
	options.IsSender = t.spec.Mode == "send"
	options.SharedSecret = strings.TrimSpace(t.spec.Code)
	if !options.IsSender {
		var err error
		options.SharedSecret, err = receiveCode(options.SharedSecret)
		if err != nil {
			return err
		}
	}
	options.IgnoreStdin = true
	options.DisableClipboard = true
	options.ShowQrCode = false
	options.Debug = false
	options.Quiet = false
	options.Stdout = false
	if !options.IsSender && options.SharedSecret == "" {
		return errors.New("enter a receive code")
	}
	public := options.RelayAddress == "" && options.RelayAddress6 == "" && !options.OnlyLocal && options.IP == ""
	if public {
		relays := publicrelay.Relays()
		var index int
		var err error
		if options.SharedSecret == "" {
			index, _, err = publicrelay.SelectFirst(ctx, relays, publicrelay.ProbeTimeout, tcp.MeasureServerLatencyContext)
			if err == nil {
				options.SharedSecret, err = codephrase.GenerateForRelay(index, len(relays))
			}
		} else {
			index, err = codephrase.RelayIndex(options.SharedSecret, len(relays))
		}
		if err != nil {
			return err
		}
		options.RelayAddress = relays[index]
		options.PublicRelay = true
	} else if options.SharedSecret == "" {
		var err error
		options.SharedSecret, err = codephrase.Generate()
		if err != nil {
			return err
		}
	}
	t.mu.Lock()
	t.state.Code = options.SharedSecret
	t.state.BrowserURL = browserReceiveURL(options.SharedSecret, public, options)
	qrValue := t.state.BrowserURL
	if qrValue == "" {
		qrValue = options.SharedSecret
	}
	t.state.QR = qrData(qrValue)
	t.mu.Unlock()
	client, err := croc.NewCtx(ctx, options)
	if err != nil {
		return err
	}
	t.mu.Lock()
	t.client = client
	t.mu.Unlock()
	if !options.IsSender {
		return client.Receive()
	}
	paths := append([]string(nil), t.spec.Paths...)
	if t.spec.Text != "" {
		file, err := os.CreateTemp(t.spec.DataDir, "croc-stdin-*.txt")
		if err != nil {
			return err
		}
		name := file.Name()
		defer os.Remove(name)
		_, writeErr := io.WriteString(file, t.spec.Text)
		closeErr := file.Close()
		if err = errors.Join(writeErr, closeErr); err != nil {
			return err
		}
		paths = []string{name}
		client.Options.SendingText = true
	}
	files, folders, count, err := croc.GetFilesInfoWithExactExclusionsContext(ctx, paths, options.ZipFolder, options.GitIgnore, options.Exclude, options.ExcludeFile)
	if err != nil {
		return err
	}
	defer utils.RemoveMarkedFiles()
	return client.Send(files, folders, count)
}
func (t *task) stored(ctx context.Context) (any, error) {
	client := &storeclient.Client{}
	callbacks := storeclient.Callbacks{
		Status:   func(detail string) { t.mu.Lock(); t.state.Detail = detail; t.mu.Unlock() },
		Progress: func(p storeclient.Progress) { t.mu.Lock(); t.state.StoreProgress = &p; t.mu.Unlock() },
	}
	if t.spec.Mode == "store_send" {
		uploaded, err := client.UploadWithOptions(ctx, t.spec.StoreOrigin, t.spec.Paths, storeclient.UploadOptions{Downloads: t.spec.Downloads, Expiration: time.Duration(t.spec.ExpirationHours * float64(time.Hour))}, callbacks)
		if err != nil {
			return nil, err
		}
		code, err := uploaded.Share.CLIToken()
		if err != nil {
			return nil, err
		}
		link, err := uploaded.Share.BrowserURL()
		if err != nil {
			return nil, err
		}
		return map[string]any{"id": uploaded.Share.ID, "code": code, "link": link, "qr": qrData(link), "expiresAt": uploaded.ExpiresAt, "downloads": uploaded.Downloads, "uploadToken": uploaded.UploadToken}, nil
	}
	share, err := storecrypto.ParseShare(t.spec.Code)
	if err != nil {
		return nil, err
	}
	if t.spec.Mode == "store_revoke" {
		return nil, client.Revoke(ctx, share, t.spec.UploadToken)
	}
	manifest, expires, err := client.Inspect(ctx, share)
	if err != nil {
		return nil, err
	}
	if t.spec.Mode == "store_inspect" {
		return map[string]any{"manifest": manifest, "expiresAt": expires}, nil
	}
	return nil, client.Receive(ctx, share, manifest, t.spec.OutputDir, callbacks)
}

// Follow croc's --qr contract: the public web bridge cannot reach a custom
// relay or speak strict DERP. Never advertise those codes as browser links.
func browserReceiveURL(code string, public bool, options croc.Options) string {
	if !public || options.Transport == croc.TransportDERP || options.RelayPassword != models.DEFAULT_PASSPHRASE {
		return ""
	}
	return "https://getcroc.com/?code=" + url.QueryEscape(code)
}

func receiveCode(value string) (string, error) {
	if !strings.Contains(value, "://") {
		return value, nil
	}
	link, err := url.Parse(value)
	if err != nil || link.Scheme != "https" || link.Host != "getcroc.com" || link.User != nil || (link.Path != "" && link.Path != "/") || strings.TrimSpace(link.Query().Get("code")) == "" {
		return "", errors.New("请输入 croc 口令或 https://getcroc.com/?code=… 接收链接")
	}
	return strings.TrimSpace(link.Query().Get("code")), nil
}

func qrData(value string) string {
	data, err := qrcode.Encode(value, qrcode.Medium, 256)
	if err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
}

func main() {
	protocolInput, protocolOutput := os.Stdin, os.Stdout
	log.SetOutput(io.Discard)
	diagnostics := &diagnostics{}
	reader, writer, err := os.Pipe()
	if err != nil {
		return
	}
	os.Stdout = writer
	os.Stderr = writer
	defer writer.Close()
	defer reader.Close()
	go io.Copy(diagnostics, reader)
	var current *task
	defer func() {
		if current != nil {
			_ = current.stop()
		}
	}()
	scanner := bufio.NewScanner(protocolInput)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	encoder := json.NewEncoder(protocolOutput)
	for scanner.Scan() {
		var request struct {
			ID     string          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		err := decode(scanner.Bytes(), &request)
		var result any
		if err == nil {
			switch request.Method {
			case "start":
				if current != nil {
					err = errors.New("process already owns a task")
					break
				}
				spec := defaults()
				err = decode(request.Params, &spec)
				if err == nil {
					current, err = newTask(spec, diagnostics)
				}
				result = map[string]bool{"started": err == nil}
			case "snapshot":
				if current == nil {
					err = errors.New("no active task")
				} else {
					result = current.read()
				}
			case "answer":
				if current == nil {
					err = errors.New("no active task")
				} else {
					err = current.answer(request.Params)
					result = map[string]bool{"accepted": err == nil}
				}
			case "stop":
				if current != nil {
					err = current.stop()
				}
				result = map[string]bool{"stopped": err == nil}
			default:
				err = errors.New("unsupported transfer command")
			}
		}
		response := map[string]any{"id": request.ID, "result": result}
		if err != nil {
			response = map[string]any{"id": request.ID, "error": err.Error()}
		}
		if err := encoder.Encode(response); err != nil {
			_ = encoder.Encode(map[string]any{"id": request.ID, "error": "encode transfer response: " + err.Error()})
			return
		}
		if request.Method == "stop" {
			return
		}
	}
}
