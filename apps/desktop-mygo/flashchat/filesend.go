package flashchat

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Files go straight to the room's members over the connections the chat
// already has, as Chitchatter sends them over its peers' WebRTC channels:
// no other service or network path has to reach the other side. A file
// is announced, sent in pieces and closed; each side shows its progress.
// This is WinOne's own action, so only WinOne peers take part; offers of
// live shares from older WinOne peers still arrive as before.

// fileAction is the wire name of the direct file transfer.
const fileAction = "winone.file"

// filePiece is how much of a file one message carries.
const filePiece = 256 << 10

// MaxDirectFileSize bounds one file sent or received directly.
const MaxDirectFileSize int64 = 8 << 30

// TransferState is where a direct file transfer stands.
type TransferState string

const (
	TransferSending   TransferState = "sending"
	TransferReceiving TransferState = "receiving"
	TransferDone      TransferState = "done"
	TransferFailed    TransferState = "failed"
)

// DirectFile is one file sent or received directly.
type DirectFile struct {
	ID   string
	Name string
	Size int64
	// Moved is how many bytes went or came so far.
	Moved int64
	// PeerID is the sender of a received file, or the member a sent one
	// went to ("" for everyone in the group).
	PeerID   string
	Outgoing bool
	// Direct is a private conversation's file, not the group's.
	Direct bool
	// Path is the file sent, or where a received one is saved.
	Path  string
	State TransferState
	Err   string
	Time  time.Time
}

type fileFrame struct {
	Type   string `json:"t"`
	ID     string `json:"id"`
	Name   string `json:"name,omitempty"`
	Size   int64  `json:"size,omitempty"`
	Direct bool   `json:"direct,omitempty"`
	Offset int64  `json:"offset,omitempty"`
	Data   string `json:"data,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// incomingFile is a file being received.
type incomingFile struct {
	file *os.File
	part string
}

// files carries a session's direct transfers.
type files struct {
	s       *Session
	handle  ActionHandle
	dir     func() (string, error)
	changed func(direct bool, peerID string)

	mu       sync.Mutex
	list     map[string]*DirectFile
	incoming map[string]*incomingFile
	stopped  bool
}

func newFiles(s *Session, dir func() (string, error), changed func(direct bool, peerID string)) *files {
	f := &files{s: s, dir: dir, changed: changed, list: map[string]*DirectFile{}, incoming: map[string]*incomingFile{}}
	f.handle = s.transport.MakeAction(fileAction)
	f.handle.Connect(f.receive)
	return f
}

func (f *files) notify(t *DirectFile) {
	if f.changed == nil {
		return
	}
	peer := t.PeerID
	if !t.Direct {
		peer = ""
	}
	f.changed(t.Direct, peer)
}

// update changes a transfer under the lock and tells the page.
func (f *files) update(id string, fn func(t *DirectFile)) {
	f.mu.Lock()
	t := f.list[id]
	if t == nil {
		f.mu.Unlock()
		return
	}
	fn(t)
	copy := *t
	f.mu.Unlock()
	f.notify(&copy)
}

// send streams path to peerID ("" for everyone in the group). It returns
// once the transfer is listed; the bytes go on a goroutine.
func (f *files) send(path, peerID string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("only files can be sent")
	}
	if info.Size() > MaxDirectFileSize {
		return fmt.Errorf("file is larger than %d GiB", MaxDirectFileSize>>30)
	}
	if len(f.s.transport.Peers()) == 0 {
		return errors.New("no member is connected")
	}
	t := &DirectFile{ID: NewUUID(), Name: filepath.Base(path), Size: info.Size(), PeerID: peerID, Outgoing: true,
		Direct: peerID != "", Path: path, State: TransferSending, Time: time.Now()}
	f.mu.Lock()
	if f.stopped {
		f.mu.Unlock()
		return errors.New("room left")
	}
	f.list[t.ID] = t
	f.mu.Unlock()
	f.notify(t)
	go f.stream(*t)
	return nil
}

func (f *files) stream(t DirectFile) {
	fail := func(err error) {
		_ = f.handle.Send(fileFrame{Type: "cancel", ID: t.ID, Reason: err.Error()}, t.PeerID)
		f.update(t.ID, func(x *DirectFile) { x.State, x.Err = TransferFailed, err.Error() })
	}
	in, err := os.Open(t.Path)
	if err != nil {
		fail(err)
		return
	}
	defer in.Close()
	if err := f.handle.Send(fileFrame{Type: "start", ID: t.ID, Name: t.Name, Size: t.Size, Direct: t.Direct}, t.PeerID); err != nil {
		fail(err)
		return
	}
	buffer := make([]byte, filePiece)
	var offset int64
	for {
		if f.isStopped() {
			return
		}
		n, err := io.ReadFull(in, buffer)
		if n > 0 {
			frame := fileFrame{Type: "data", ID: t.ID, Offset: offset, Data: base64.StdEncoding.EncodeToString(buffer[:n])}
			if sendErr := f.handle.Send(frame, t.PeerID); sendErr != nil {
				fail(sendErr)
				return
			}
			offset += int64(n)
			moved := offset
			f.update(t.ID, func(x *DirectFile) { x.Moved = moved })
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			fail(err)
			return
		}
	}
	if offset != t.Size {
		fail(errors.New("the file changed while it was sent"))
		return
	}
	if err := f.handle.Send(fileFrame{Type: "end", ID: t.ID, Size: offset}, t.PeerID); err != nil {
		fail(err)
		return
	}
	f.update(t.ID, func(x *DirectFile) { x.State = TransferDone })
}

func (f *files) isStopped() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stopped
}

// receive takes the frames of files peers send.
func (f *files) receive(data json.RawMessage, peerID string) {
	var frame fileFrame
	if json.Unmarshal(data, &frame) != nil || frame.ID == "" || len(frame.ID) > 64 {
		return
	}
	switch frame.Type {
	case "start":
		f.start(frame, peerID)
	case "data":
		f.write(frame, peerID)
	case "end":
		f.finish(frame, peerID)
	case "cancel":
		f.abort(frame.ID, peerID, frame.Reason)
	}
}

// safeName keeps a received file's name to one plain file name.
func safeName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	name = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.Trim(name, " .")
	if name == "" || name == "_" {
		name = "file"
	}
	if len([]rune(name)) > 160 {
		name = string([]rune(name)[:160])
	}
	return name
}

// available is a path in dir named name, or "name (2)" and on when taken.
func available(dir, name string) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		candidate := name
		if i > 1 {
			candidate = fmt.Sprintf("%s (%d)%s", stem, i, ext)
		}
		path := filepath.Join(dir, candidate)
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			if _, err := os.Lstat(path + ".part"); errors.Is(err, os.ErrNotExist) {
				return path
			}
		}
	}
}

func (f *files) start(frame fileFrame, peerID string) {
	if frame.Size < 0 || frame.Size > MaxDirectFileSize {
		return
	}
	f.mu.Lock()
	if f.stopped || f.list[frame.ID] != nil {
		f.mu.Unlock()
		return
	}
	t := &DirectFile{ID: frame.ID, Name: safeName(frame.Name), Size: frame.Size, PeerID: peerID, Direct: frame.Direct,
		State: TransferReceiving, Time: time.Now()}
	f.list[frame.ID] = t
	f.mu.Unlock()
	fail := func(err error) {
		f.update(t.ID, func(x *DirectFile) { x.State, x.Err = TransferFailed, err.Error() })
	}
	dir, err := f.dir()
	if err == nil {
		err = os.MkdirAll(dir, 0o755)
	}
	if err != nil {
		fail(err)
		return
	}
	path := available(dir, t.Name)
	file, err := os.OpenFile(path+".part", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		fail(err)
		return
	}
	f.mu.Lock()
	f.incoming[t.ID] = &incomingFile{file: file, part: path + ".part"}
	t.Path = path
	copy := *t
	f.mu.Unlock()
	f.notify(&copy)
}

// owned returns a file being received from peerID.
func (f *files) owned(id, peerID string) (*incomingFile, *DirectFile) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, in := f.list[id], f.incoming[id]
	if t == nil || in == nil || t.Outgoing || t.PeerID != peerID || t.State != TransferReceiving {
		return nil, nil
	}
	return in, t
}

func (f *files) write(frame fileFrame, peerID string) {
	in, t := f.owned(frame.ID, peerID)
	if in == nil {
		return
	}
	data, err := base64.StdEncoding.DecodeString(frame.Data)
	if err == nil && (frame.Offset != t.Moved || frame.Offset+int64(len(data)) > t.Size) {
		err = errors.New("the file arrived out of order")
	}
	if err == nil {
		_, err = in.file.Write(data)
	}
	if err != nil {
		f.discard(frame.ID, err.Error())
		return
	}
	moved := frame.Offset + int64(len(data))
	f.update(frame.ID, func(x *DirectFile) { x.Moved = moved })
}

func (f *files) finish(frame fileFrame, peerID string) {
	in, t := f.owned(frame.ID, peerID)
	if in == nil {
		return
	}
	if t.Moved != t.Size || frame.Size != t.Size {
		f.discard(frame.ID, "the file arrived incomplete")
		return
	}
	err := in.file.Close()
	if err == nil {
		err = os.Rename(in.part, t.Path)
	}
	f.mu.Lock()
	delete(f.incoming, frame.ID)
	f.mu.Unlock()
	if err != nil {
		_ = os.Remove(in.part)
		f.update(frame.ID, func(x *DirectFile) { x.State, x.Err = TransferFailed, err.Error() })
		return
	}
	f.update(frame.ID, func(x *DirectFile) { x.State = TransferDone })
}

func (f *files) abort(id, peerID, reason string) {
	if in, _ := f.owned(id, peerID); in == nil {
		return
	}
	if reason == "" {
		reason = "the sender stopped"
	}
	f.discard(id, reason)
}

// discard drops a partly received file.
func (f *files) discard(id, reason string) {
	f.mu.Lock()
	in := f.incoming[id]
	delete(f.incoming, id)
	f.mu.Unlock()
	if in != nil {
		_ = in.file.Close()
		_ = os.Remove(in.part)
	}
	f.update(id, func(x *DirectFile) { x.State, x.Err = TransferFailed, reason })
}

// peerLeft fails what was coming from a member who left.
func (f *files) peerLeft(peerID string) {
	f.mu.Lock()
	var ids []string
	for id, t := range f.list {
		if !t.Outgoing && t.PeerID == peerID && t.State == TransferReceiving {
			ids = append(ids, id)
		}
	}
	f.mu.Unlock()
	for _, id := range ids {
		f.discard(id, "the sender left")
	}
}

// stop ends every transfer, dropping files not fully received.
func (f *files) stop() {
	f.mu.Lock()
	f.stopped = true
	var ids []string
	for id := range f.incoming {
		ids = append(ids, id)
	}
	f.mu.Unlock()
	for _, id := range ids {
		f.discard(id, "room left")
	}
	f.handle.Disconnect()
}

// snapshot lists one conversation's transfers, oldest first.
func (f *files) snapshot(peerID string) []DirectFile {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []DirectFile
	for _, t := range f.list {
		if (peerID == "" && !t.Direct) || (peerID != "" && t.Direct && t.PeerID == peerID) {
			out = append(out, *t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out
}
