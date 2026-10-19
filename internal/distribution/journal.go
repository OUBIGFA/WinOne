package distribution

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"reflect"
	"strings"
	"sync"
	"winone/internal/configjson"
	"winone/internal/settings"
)

type UpgradeJournal struct {
	SchemaVersion        int               `json:"schemaVersion"`
	ID                   string            `json:"id"`
	Revision             uint64            `json:"revision"`
	Phase                string            `json:"phase"`
	Plan                 UpgradePlan       `json:"plan"`
	FileStates           []string          `json:"fileStates"`
	MetadataStates       map[string]string `json:"metadataStates"`
	MetadataFingerprints map[string]string `json:"metadataFingerprints"`
}

// UpgradeJournalStore records caller-confirmed work, not evidence of successful
// file operations. The updater holds an exclusive maintenance lease; revision
// checks detect stale callers but do not replace that cross-process lease.
type UpgradeJournalStore struct {
	mu    sync.Mutex
	path  string
	state UpgradeJournal
}

func CreateUpgradeJournal(path string, plan UpgradePlan) (*UpgradeJournalStore, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	state := UpgradeJournal{SchemaVersion: 1, ID: hex.EncodeToString(id[:]), Phase: "prepared", Plan: plan, FileStates: make([]string, len(plan.Files))}
	state.MetadataStates = map[string]string{"manifest": "pending", "uninstaller": "pending", "registration": "pending", "shortcut": "pending"}
	state.MetadataFingerprints = map[string]string{}
	state.Plan.Files = append([]UpgradeFile(nil), plan.Files...)
	for i, file := range plan.Files {
		state.FileStates[i] = "pending"
		if file.Action == "unchanged" {
			state.FileStates[i] = "unchanged"
		}
	}
	if err := validateUpgradeJournal(state); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	_, writeErr := file.Write(data)
	if err = errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
		return nil, err
	}
	return &UpgradeJournalStore{path: path, state: state}, nil
}

func OpenUpgradeJournal(path string) (*UpgradeJournalStore, error) {
	state, err := readUpgradeJournal(path)
	if err != nil {
		return nil, err
	}
	return &UpgradeJournalStore{path: path, state: state}, nil
}

func (s *UpgradeJournalStore) Snapshot() UpgradeJournal {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneUpgradeJournal(s.state)
}

func cloneUpgradeJournal(value UpgradeJournal) UpgradeJournal {
	value.Plan.Files = append([]UpgradeFile(nil), value.Plan.Files...)
	value.FileStates = append([]string(nil), value.FileStates...)
	value.MetadataStates = maps.Clone(value.MetadataStates)
	value.MetadataFingerprints = maps.Clone(value.MetadataFingerprints)
	return value
}

func (s *UpgradeJournalStore) update(revision uint64, change func(*UpgradeJournal) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := readUpgradeJournal(s.path)
	if err != nil {
		return err
	}
	if revision != s.state.Revision || !reflect.DeepEqual(current, s.state) {
		return errors.New("upgrade journal changed; reload before continuing")
	}
	if revision == ^uint64(0) {
		return errors.New("upgrade journal revision exhausted")
	}
	next := cloneUpgradeJournal(current)
	if err = change(&next); err != nil {
		return err
	}
	next.Revision++
	if err = validateUpgradeJournal(next); err != nil {
		return err
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	if err = settings.WriteAtomic(s.path, data); err != nil {
		return err
	}
	s.state = next
	return nil
}

func (s *UpgradeJournalStore) Transition(revision uint64, phase string) error {
	return s.update(revision, func(j *UpgradeJournal) error {
		allowed := false
		switch j.Phase {
		case "prepared":
			allowed = phase == "applying" || phase == "restoring"
		case "applying":
			allowed = phase == "checking" || phase == "restoring"
		case "checking":
			allowed = phase == "committed" || phase == "restoring"
		case "restoring":
			allowed = phase == "rolled-back"
		}
		if !allowed {
			return errors.New("invalid upgrade phase transition")
		}
		j.Phase = phase
		return nil
	})
}

// RecordFile must persist writing/restoring before touching a file. A reopened
// journal retaining either intent is uncertain work that must be inspected.
func (s *UpgradeJournalStore) RecordFile(revision uint64, index int, state string) error {
	return s.update(revision, func(j *UpgradeJournal) error {
		if index < 0 || index >= len(j.FileStates) {
			return errors.New("invalid upgrade file index")
		}
		before := j.FileStates[index]
		allowed := j.Phase == "applying" && ((before == "pending" && state == "writing") || (before == "writing" && state == "applied"))
		allowed = allowed || j.Phase == "restoring" && (((before == "writing" || before == "applied") && state == "restoring") || (before == "restoring" && state == "restored"))
		if !allowed {
			return errors.New("invalid upgrade file transition")
		}
		j.FileStates[index] = state
		return nil
	})
}

// Metadata is part of the same transaction. Inapplicable metadata must be
// explicitly marked unchanged; it cannot silently disappear from validation.
func (s *UpgradeJournalStore) RecordMetadata(revision uint64, name, state string) error {
	return s.update(revision, func(j *UpgradeJournal) error {
		before, ok := j.MetadataStates[name]
		if !ok {
			return errors.New("unknown upgrade metadata")
		}
		allowed := j.Phase == "applying" && ((before == "pending" && (state == "writing" || state == "unchanged")) || (before == "writing" && state == "applied"))
		allowed = allowed || j.Phase == "restoring" && (((before == "writing" || before == "applied") && state == "restoring") || (before == "restoring" && state == "restored"))
		if !allowed {
			return errors.New("invalid metadata transition")
		}
		j.MetadataStates[name] = state
		return nil
	})
}

// BindMetadata persists the identity of the original/desired snapshots before
// writing. Resumed operations must supply the same fingerprint.
func (s *UpgradeJournalStore) BindMetadata(revision uint64, name, fingerprint string) error {
	state := s.Snapshot()
	if revision != state.Revision {
		return errors.New("upgrade journal revision changed")
	}
	if bound, ok := state.MetadataFingerprints[name]; ok {
		if bound != fingerprint {
			return errors.New("upgrade metadata snapshots changed")
		}
		return nil
	}
	return s.update(revision, func(j *UpgradeJournal) error {
		if j.Phase != "applying" || j.MetadataStates[name] != "pending" || name == "manifest" {
			return errors.New("metadata must be bound before its write intent")
		}
		if j.MetadataFingerprints == nil {
			j.MetadataFingerprints = map[string]string{}
		}
		j.MetadataFingerprints[name] = fingerprint
		return nil
	})
}

func readUpgradeJournal(path string) (UpgradeJournal, error) {
	var result UpgradeJournal
	info, err := os.Lstat(path)
	if err != nil {
		return result, err
	}
	if !info.Mode().IsRegular() || info.Size() > 8<<20 {
		return result, errors.New("invalid upgrade journal file")
	}
	file, err := os.Open(path)
	if err != nil {
		return result, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, (8<<20)+1))
	if err = errors.Join(readErr, file.Close()); err != nil {
		return result, err
	}
	if len(data) > 8<<20 {
		return result, errors.New("upgrade journal too large")
	}
	if err = configjson.DecodeObject(data, &result); err != nil {
		return result, err
	}
	if result.MetadataFingerprints == nil {
		result.MetadataFingerprints = map[string]string{}
	}
	return result, validateUpgradeJournal(result)
}

func validateUpgradeJournal(j UpgradeJournal) error {
	hexBytes := func(s string, n int) bool {
		b, e := hex.DecodeString(s)
		return e == nil && len(b) == n && s == strings.ToLower(s)
	}
	if j.SchemaVersion != 1 || !hexBytes(j.ID, 16) || len(j.Plan.Files) == 0 || len(j.Plan.Files) > 8192 || len(j.FileStates) != len(j.Plan.Files) {
		return errors.New("invalid upgrade journal header")
	}
	if len(j.MetadataStates) != 4 {
		return errors.New("upgrade metadata inventory missing")
	}
	for name, value := range j.MetadataFingerprints {
		if (name != "shortcut" && name != "registration" && name != "uninstaller") || !hexBytes(value, 32) {
			return errors.New("invalid metadata fingerprint")
		}
	}
	for _, name := range []string{"manifest", "uninstaller", "registration", "shortcut"} {
		state, ok := j.MetadataStates[name]
		if !ok || (name == "manifest" && state == "unchanged") || !validJournalState(j.Phase, state) {
			return errors.New("upgrade metadata contains unfinished or invalid work")
		}
	}
	if n, err := CompareVersions(j.Plan.PreviousVersion, j.Plan.NextVersion); err != nil || n > 0 || (n == 0) != j.Plan.Reinstall {
		return errors.New("invalid upgrade journal versions")
	}
	if !hexBytes(j.Plan.PreviousManifestSHA256, 32) || !hexBytes(j.Plan.NextManifestSHA256, 32) {
		return errors.New("invalid upgrade inventory hashes")
	}
	if j.Plan.PreviousManifestSHA256 == j.Plan.NextManifestSHA256 && !j.Plan.Reinstall {
		return errors.New("new version must have a different manifest")
	}
	seen := map[string]bool{}
	for i, file := range j.Plan.Files {
		key := strings.ToLower(file.Path)
		// A journal is a historical transaction, not the current release's
		// packaging inventory. Renamed OCR/plugin dependencies must not make a
		// completed transaction unreadable. Execution separately reconstructs
		// and compares the plan against both hash-verified archives.
		if !safeName(file.Path) || seen[key] || !allowedHistorical(file.Path) {
			return fmt.Errorf("invalid upgrade journal path: %q", file.Path)
		}
		seen[key] = true
		if file.Action == "add" {
			if file.PreviousPath != "" || file.PreviousSHA256 != "" {
				return errors.New("added file has previous content")
			}
		} else if !strings.EqualFold(file.PreviousPath, file.Path) || !hexBytes(file.PreviousSHA256, 32) {
			return errors.New("invalid previous file identity")
		}
		if file.Action == "remove" {
			if file.NextSHA256 != "" {
				return errors.New("removed file has new content")
			}
		} else if !hexBytes(file.NextSHA256, 32) {
			return errors.New("invalid next file hash")
		}
		state := j.FileStates[i]
		if file.Action == "unchanged" {
			if state != "unchanged" || file.Path != file.PreviousPath || file.PreviousSHA256 != file.NextSHA256 {
				return errors.New("unchanged file differs")
			}
			continue
		}
		if file.Action != "add" && file.Action != "replace" && file.Action != "remove" {
			return errors.New("unknown upgrade action")
		}
		if state == "unchanged" || !validJournalState(j.Phase, state) {
			return errors.New("upgrade phase contains unfinished or invalid work")
		}
	}
	switch j.Phase {
	case "prepared", "applying", "checking", "committed", "restoring", "rolled-back":
		return nil
	}
	return errors.New("unknown upgrade phase")
}

func validJournalState(phase, state string) bool {
	if state == "unchanged" {
		return true
	}
	switch phase {
	case "prepared":
		return state == "pending"
	case "applying":
		return state == "pending" || state == "writing" || state == "applied"
	case "checking", "committed":
		return state == "applied"
	case "restoring":
		return state == "pending" || state == "writing" || state == "applied" || state == "restoring" || state == "restored"
	case "rolled-back":
		return state == "pending" || state == "restored"
	}
	return false
}
