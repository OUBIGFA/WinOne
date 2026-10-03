// Package capturehandoff carries bounded, acknowledged editor launch requests.
package capturehandoff

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// WindowInstanceID is shared by the Host's lifecycle owner and screenshot UI.
func WindowInstanceID(pipe string) string {
	return fmt.Sprintf("winone-capture-%x", sha256.Sum256([]byte(pipe)))
}

var draftPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func ValidateDraftID(id string) error {
	if !draftPattern.MatchString(id) {
		return fmt.Errorf("invalid capture draft ID")
	}
	return nil
}

type Request struct {
	ID      string `json:"id"`
	DraftID string `json:"draftId"`
}
type Queue struct {
	mu       sync.Mutex
	sequence uint64
	pending  []Request
}

func (q *Queue) Push(id string) error {
	if err := ValidateDraftID(id); err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, request := range q.pending {
		if request.DraftID == id {
			return nil
		}
	}
	if len(q.pending) >= 8 {
		return fmt.Errorf("too many pending editor requests")
	}
	q.sequence++
	q.pending = append(q.pending, Request{ID: strconv.FormatUint(q.sequence, 10), DraftID: id})
	return nil
}

// Peek does not consume the request: the frontend may not be mounted yet.
func (q *Queue) Peek() *Request {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.pending) == 0 {
		return nil
	}
	copy := q.pending[0]
	return &copy
}
func (q *Queue) Acknowledge(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.pending) == 0 || q.pending[0].ID != id {
		return false
	}
	q.pending = q.pending[1:]
	return true
}

// Second-instance payloads can include an executable and unrelated app flags.
// Only the dedicated argument is interpreted, and duplicates are rejected.
func DraftFromArgs(args []string) (string, error) {
	found := false
	id := ""
	for index := 0; index < len(args); index++ {
		value := args[index]
		if value != "--capture-draft" && !strings.HasPrefix(value, "--capture-draft=") {
			continue
		}
		if found {
			return "", fmt.Errorf("duplicate capture draft argument")
		}
		found = true
		if value == "--capture-draft" {
			index++
			if index >= len(args) {
				return "", fmt.Errorf("missing capture draft ID")
			}
			id = args[index]
		} else {
			id = strings.TrimPrefix(value, "--capture-draft=")
		}
	}
	if found {
		if err := ValidateDraftID(id); err != nil {
			return "", err
		}
	}
	return id, nil
}
