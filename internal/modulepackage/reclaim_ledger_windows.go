//go:build windows

package modulepackage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"winone/internal/catalog"
	"winone/internal/configjson"
	"winone/internal/settings"
)

const reclamationFileName = ".winone-module-reclaim.json"
const maxReclamationItems = 256

var ErrReclamationPending = errors.New("package change completed but reclaim bookkeeping requires recovery")
var reclaimIDPattern = regexp.MustCompile(`^[a-f0-9]{16}-stage-[a-f0-9]{32}$`)

type ReclamationItem struct {
	Kind            string            `json:"kind,omitempty"`
	Package         RegisteredPackage `json:"package"`
	ChangeID        string            `json:"changeId"`
	ChangeRevision  uint64            `json:"changeRevision"`
	Phase           string            `json:"phase"`
	Origin          string            `json:"origin,omitempty"`
	MigrationSHA256 string            `json:"migrationSha256,omitempty"`
}
type ReclaimResult struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Path  string `json:"path,omitempty"`
}
type ReclamationSnapshot struct {
	SchemaVersion          int                        `json:"schemaVersion"`
	Revision               uint64                     `json:"revision"`
	ObservedChangeRevision uint64                     `json:"observedChangeRevision"`
	ObservedChangeID       string                     `json:"observedChangeId"`
	Items                  map[string]ReclamationItem `json:"items"`
	LastResult             *ReclaimResult             `json:"lastResult,omitempty"`
}

func reclaimKey(item ReclamationItem) string {
	return fmt.Sprintf("%016x-%s", item.ChangeRevision, item.Package.StageID)
}
func quarantineName(key string) string { return ".reclaim-" + key }

func (r *Registry) Reclamation() (ReclamationSnapshot, error) {
	root, err := os.OpenRoot(r.directory)
	if err != nil {
		return ReclamationSnapshot{}, err
	}
	defer root.Close()
	data, err := readStageFile(context.Background(), root, reclamationFileName, maxRegistryBytes)
	if errors.Is(err, os.ErrNotExist) {
		return ReclamationSnapshot{SchemaVersion: 1, Items: map[string]ReclamationItem{}}, nil
	}
	if err != nil {
		return ReclamationSnapshot{}, err
	}
	var result ReclamationSnapshot
	if err = configjson.DecodeObject(data, &result); err != nil {
		return result, err
	}
	return result, validateReclamation(result)
}

func validateReclamation(value ReclamationSnapshot) error {
	if (value.SchemaVersion != 1 && value.SchemaVersion != 2 && value.SchemaVersion != 3) || value.Items == nil || len(value.Items) > maxReclamationItems {
		return errors.New("invalid package reclaim journal")
	}
	if (value.ObservedChangeRevision == 0) != (value.ObservedChangeID == "") || (value.ObservedChangeID != "" && !catalog.ValidID(value.ObservedChangeID)) {
		return errors.New("invalid reclaim checkpoint")
	}
	for key, item := range value.Items {
		if item.Kind != "" && (item.Kind != "preparation" || value.SchemaVersion < 3) {
			return errors.New("invalid reclaim item kind")
		}
		if item.MigrationSHA256 != "" {
			if value.SchemaVersion < 2 || !validMigrationDigest(item.MigrationSHA256) {
				return errors.New("invalid migration cleanup reference")
			}
		}
		if err := validateRegistration(item.Package); err != nil {
			return err
		}
		validRevision := item.ChangeRevision > 0 && item.ChangeRevision <= value.ObservedChangeRevision
		if item.Kind == "preparation" {
			validRevision = item.ChangeRevision == 0
		}
		if key != reclaimKey(item) || !catalog.ValidID(item.ChangeID) || len(item.ChangeID) > 128 || !validRevision || (item.Phase != "queued" && item.Phase != "recycling") {
			return errors.New("invalid reclaim item")
		}
		if (item.Phase == "queued" && item.Origin != "") || (item.Phase == "recycling" && (!filepath.IsAbs(item.Origin) || filepath.Base(item.Origin) != quarantineName(key))) {
			return errors.New("invalid reclaim origin")
		}
	}
	if result := value.LastResult; result != nil {
		if !reclaimIDPattern.MatchString(result.ID) || (result.State != "recycled" && result.State != "retained-reference") || !filepath.IsAbs(result.Path) || strings.ContainsRune(result.Path, '\x00') {
			return errors.New("invalid reclaim result")
		}
	}
	return nil
}

func (r *Registry) writeReclamation(ctx context.Context, value ReclamationSnapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateReclamation(value); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > maxRegistryBytes {
		return errors.New("reclaim journal exceeds size limit")
	}
	return settings.WriteAtomic(filepath.Join(r.directory, reclamationFileName), data)
}

func nextReclamation(value ReclamationSnapshot) (ReclamationSnapshot, error) {
	if value.Revision == ^uint64(0) {
		return value, errors.New("reclaim journal revision exhausted")
	}
	value.Revision++
	value.Items = maps.Clone(value.Items)
	return value, nil
}

// Persist the retired package before the registry can replace its last receipt.
// A separate journal lets reclamation progress without altering package-operation
// revisions. Its checkpoint prevents replay from re-queuing completed work.
func (r *Registry) syncReclamation(ctx context.Context, receipt *ChangeReceipt) error {
	if receipt == nil {
		return nil
	}
	current, err := r.Reclamation()
	if err != nil {
		return err
	}
	if current.ObservedChangeRevision == receipt.Revision {
		if current.ObservedChangeID != receipt.Change.ID {
			return errors.New("reclaim checkpoint identity changed")
		}
		return nil
	}
	if current.ObservedChangeRevision > receipt.Revision {
		return errors.New("registry history is older than reclaim history")
	}
	var retired *RegisteredPackage
	if receipt.Outcome == "committed" {
		retired = &receipt.Change.Before
	} else {
		retired = receipt.Change.After
	}
	next, err := nextReclamation(current)
	if err != nil {
		return err
	}
	next.ObservedChangeID = receipt.Change.ID
	next.ObservedChangeRevision = receipt.Revision
	if retired != nil {
		item := ReclamationItem{Package: *retired, ChangeID: receipt.Change.ID, ChangeRevision: receipt.Revision, Phase: "queued"}
		item.MigrationSHA256 = receipt.Change.MigrationSHA256
		if item.MigrationSHA256 != "" && next.SchemaVersion < 2 {
			next.SchemaVersion = 2
		}
		next.Items[reclaimKey(item)] = item
	}
	return r.writeReclamation(ctx, next)
}

// SyncReclamation is an explicit recovery action, not a settings-page read.
func (r *Registry) SyncReclamation(ctx context.Context) (ReclamationSnapshot, error) {
	_, err := r.withRegistryLease(ctx, func(current RegistrySnapshot) (RegistrySnapshot, error) { return current, nil })
	if err != nil {
		return ReclamationSnapshot{}, err
	}
	return r.Reclamation()
}
