//go:build windows

package modulepackage

import (
	"encoding/hex"
	"errors"
	"strings"
	"winone/internal/catalog"
	"winone/internal/distribution"
	"winone/internal/modulestate"
)

var ErrRecoveryRequired = errors.New("module package change requires recovery")

func validMigrationDigest(value string) bool {
	digest, err := hex.DecodeString(value)
	return err == nil && len(digest) == 32 && value == strings.ToLower(value)
}

// ReplacementStage is an already staged, approved candidate. Trust and actual
// Host compatibility are supplied again during every transition, not journaled.
type ReplacementStage struct {
	ID      string
	Options StageOptions
}
type ChangeRequest struct {
	ID               string
	RegistryRevision uint64
	StateRevision    uint64
	Current          RegisteredPackage
	Replacement      *ReplacementStage
	MigrationSHA256  string
	// Required by the Host for stores containing dependency declarations.
	// Trust and filesystem roots are supplied live, never persisted in a journal.
	Dependencies *DependencySource
}

// The inventory and journal share one atomic file. Module intent can be between
// the old and new digest after a crash; affected runtime access stays blocked.
type PackageChange struct {
	ID               string             `json:"id"`
	Phase            string             `json:"phase"`
	RegistryRevision uint64             `json:"registryRevision"`
	StateRevision    uint64             `json:"stateRevision"`
	Before           RegisteredPackage  `json:"before"`
	After            *RegisteredPackage `json:"after,omitempty"`
	PreviousState    modulestate.Record `json:"previousState"`
	MigrationSHA256  string             `json:"migrationSha256,omitempty"`
}
type ChangeReceipt struct {
	Change   PackageChange `json:"change"`
	Outcome  string        `json:"outcome"`
	Revision uint64        `json:"revision"`
}

// ChangePermit is Host-only context for checking a specific pending version.
// The monotonic registry revision prevents an old runtime from gaining access
// if a later operation happens to reuse the same human-readable operation ID.
type ChangePermit struct {
	ID               string
	RegistryRevision uint64
	// RecoveryOnly permits an unbound snapshot/shutdown probe of the current
	// prepared version. It can never publish a normal feature route.
	RecoveryOnly bool
}

func validateChange(change PackageChange) error {
	if !catalog.ValidID(change.ID) || len(change.ID) > 128 || (change.Phase != "prepared" && change.Phase != "applied" && change.Phase != "restoring") {
		return errors.New("invalid package change")
	}
	if err := validateRegistration(change.Before); err != nil {
		return err
	}
	if change.MigrationSHA256 != "" {
		if !validMigrationDigest(change.MigrationSHA256) || change.After == nil {
			return errors.New("invalid package migration reference")
		}
	}
	previous := change.PreviousState
	if previous.PendingStop || (previous.PackageSHA256 != "" && previous.PackageSHA256 != change.Before.ManifestSHA256) || (previous.Enabled && previous.PackageSHA256 == "") {
		return errors.New("invalid previous module intent")
	}
	if change.After != nil {
		if err := validateRegistration(*change.After); err != nil {
			return err
		}
		order, err := distribution.CompareVersions(change.After.Version, change.Before.Version)
		if err != nil || order <= 0 || change.After.ModuleID != change.Before.ModuleID || change.After.StageID == change.Before.StageID {
			return errors.New("replacement must be a newer staged version of the same module")
		}
	}
	return nil
}

func validateRegistryChange(snapshot RegistrySnapshot) error {
	if snapshot.SchemaVersion < 3 && (snapshot.Pending != nil && snapshot.Pending.MigrationSHA256 != "" || snapshot.LastChange != nil && snapshot.LastChange.Change.MigrationSHA256 != "") {
		return errors.New("configuration migration requires registry v3")
	}
	if snapshot.SchemaVersion == 1 && (snapshot.Pending != nil || snapshot.LastChange != nil) {
		return errors.New("legacy registry cannot contain package changes")
	}
	if change := snapshot.Pending; change != nil {
		if err := validateChange(*change); err != nil {
			return err
		}
		if change.RegistryRevision >= snapshot.Revision {
			return errors.New("invalid package change revision")
		}
		delta := snapshot.Revision - change.RegistryRevision
		if (change.Phase == "prepared" && delta != 1) || (change.Phase == "applied" && delta != 2) || (change.Phase == "restoring" && delta != 2 && delta != 3) {
			return errors.New("package phase and revision do not match")
		}
		expected := &change.Before
		if change.Phase == "applied" {
			expected = change.After
		}
		actual, exists := snapshot.Records[change.Before.ModuleID]
		if (expected == nil && exists) || (expected != nil && (!exists || actual != *expected)) {
			return errors.New("pending change does not match current inventory")
		}
		for id, record := range snapshot.Records {
			if id != change.Before.ModuleID && change.After != nil && record.StageID == change.After.StageID {
				return errors.New("replacement stage belongs to another module")
			}
		}
	}
	if receipt := snapshot.LastChange; receipt != nil {
		if err := validateChange(receipt.Change); err != nil {
			return err
		}
		phase := map[string]string{"committed": "applied", "restored": "restoring", "cancelled": "prepared"}[receipt.Outcome]
		if phase == "" || receipt.Change.Phase != phase || receipt.Revision > snapshot.Revision || receipt.Revision <= receipt.Change.RegistryRevision {
			return errors.New("invalid package change receipt")
		}
		delta := receipt.Revision - receipt.Change.RegistryRevision
		if (receipt.Outcome == "cancelled" && delta != 2) || (receipt.Outcome == "committed" && delta != 3) || (receipt.Outcome == "restored" && delta != 3 && delta != 4) {
			return errors.New("package receipt revision does not match its outcome")
		}
		if snapshot.Pending != nil && snapshot.Pending.ID == receipt.Change.ID {
			return errors.New("package change ID was reused")
		}
	}
	return nil
}
