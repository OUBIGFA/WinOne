package flashchat

import (
	"errors"
	"sync"
	"time"

	"winone/desktopmygo/core"
	"winone/desktopmygo/flashchat"
)

// The file side of the flash chat, as the React page's filesAdapter and
// its frame did: an offer is a live share (interconnect transfer
// share_server) whose WinOne address is the offer's "magnet URI", and
// receiving one opens a share_client listening on this computer. The page
// owns the tasks it starts: it stops only those, and all of them when it
// closes.

// record is a transfer task as interconnect.transfer_list returns it.
type record struct {
	ID   string `json:"id"`
	Spec struct {
		Mode string `json:"mode"`
	} `json:"spec"`
	State struct {
		Status     string `json:"status"`
		Ready      bool   `json:"ready"`
		RelayReady bool   `json:"relayReady"`
		RelayError string `json:"relayError"`
		Error      string `json:"error"`
		Code       string `json:"code"`
		Address    string `json:"address"`
	} `json:"state"`
}

// tasks is the set of transfer tasks the page started.
type tasks struct {
	env *core.Env
	mu  sync.Mutex
	ids map[string]bool
}

func newTasks(env *core.Env) *tasks { return &tasks{env: env, ids: map[string]bool{}} }

func (t *tasks) start(spec map[string]any) (string, error) {
	var started struct {
		ID string `json:"id"`
	}
	if err := t.env.Host.CallCompletingInto("interconnect.transfer_start", spec, &started); err != nil {
		return "", err
	}
	t.mu.Lock()
	t.ids[started.ID] = true
	t.mu.Unlock()
	return started.ID, nil
}

// stop ends a task the page started; others are not its to stop.
func (t *tasks) stop(id string) error {
	t.mu.Lock()
	owned := t.ids[id]
	t.mu.Unlock()
	if !owned {
		return nil
	}
	if err := t.env.Host.CallCompletingInto("interconnect.transfer_stop", map[string]any{"id": id}, nil); err != nil {
		return err
	}
	t.mu.Lock()
	delete(t.ids, id)
	t.mu.Unlock()
	return nil
}

// stopAll ends every task the page started.
func (t *tasks) stopAll() {
	t.mu.Lock()
	var ids []string
	for id := range t.ids {
		ids = append(ids, id)
	}
	t.mu.Unlock()
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func() { defer wg.Done(); _ = t.stop(id) }()
	}
	wg.Wait()
}

// ready waits until a task serves: a share also waits for its relay, so
// its address works across networks before it is offered.
func (t *tasks) ready(id string, cancelled func() bool) (record, error) {
	for {
		if cancelled() {
			return record{}, errors.New(own("cancelled"))
		}
		var list []record
		if err := t.env.Host.CallCompletingInto("interconnect.transfer_list", nil, &list); err != nil {
			return record{}, err
		}
		var found *record
		for i := range list {
			if list[i].ID == id {
				found = &list[i]
			}
		}
		switch {
		case found == nil:
			return record{}, errors.New(own("shareMissing"))
		case found.State.Status != "running":
			if found.State.Error != "" {
				return record{}, errors.New(found.State.Error)
			}
			return record{}, errors.New(own("shareStopped"))
		case found.State.RelayError != "":
			return record{}, errors.New(found.State.RelayError)
		case found.State.Ready && (found.Spec.Mode != "share_server" || found.State.RelayReady):
			return *found, nil
		}
		time.Sleep(pollInterval)
	}
}

// pollInterval is how often a starting task is checked, as React did.
var pollInterval = 500 * time.Millisecond

// shares is a session's flashchat.FileTransfer: each offer a share of
// the Host's.
type shares struct {
	tasks    *tasks
	mu       sync.Mutex
	offers   map[string]string // address → task
	starting map[string]bool
	closed   bool
}

func newShares(t *tasks) *shares {
	return &shares{tasks: t, offers: map[string]string{}, starting: map[string]bool{}}
}

func (s *shares) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *shares) Offer(files []flashchat.FileHandle, _ string) (string, error) {
	if s.isClosed() {
		return "", errors.New(own("roomClosed"))
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		if f.Path == "" {
			return "", errors.New(own("localFilesOnly"))
		}
		paths = append(paths, f.Path)
	}
	id, err := s.tasks.start(map[string]any{"mode": "share_server", "paths": paths})
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.starting[id] = true
	s.mu.Unlock()
	r, err := s.tasks.ready(id, s.isClosed)
	s.mu.Lock()
	delete(s.starting, id)
	if err == nil {
		s.offers[r.State.Code] = id
	}
	s.mu.Unlock()
	if err != nil {
		_ = s.tasks.stop(id)
		return "", err
	}
	return r.State.Code, nil
}

func (s *shares) Rescind(code string) error {
	s.mu.Lock()
	id, ok := s.offers[code]
	s.mu.Unlock()
	if !ok {
		return nil
	}
	if err := s.tasks.stop(id); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.offers, code)
	s.mu.Unlock()
	return nil
}

// RescindAll stops every offer, those still starting included, and takes
// no more: the room was left.
func (s *shares) RescindAll() error {
	s.mu.Lock()
	s.closed = true
	ids := map[string]bool{}
	for _, id := range s.offers {
		ids[id] = true
	}
	for id := range s.starting {
		ids[id] = true
	}
	s.offers = map[string]string{}
	s.mu.Unlock()
	var first error
	for id := range ids {
		if err := s.tasks.stop(id); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (s *shares) IsOffering(code string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.offers[code]
	return ok
}
