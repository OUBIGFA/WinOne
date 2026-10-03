//go:build windows

package distribution

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

// ApplyUpgradeRegistration uses bound original/desired snapshots under the
// coordinator's maintenance lease. Intermediate values require recorded intent.
func ApplyUpgradeRegistration(directory, keyPath string, original, next RegistrationSnapshot, journal *UpgradeJournalStore) error {
	return executeUpgradeRegistration(directory, keyPath, original, next, journal, false)
}

// RestoreUpgradeRegistration can resume a partly completed multi-value restore.
func RestoreUpgradeRegistration(directory, keyPath string, original, next RegistrationSnapshot, journal *UpgradeJournalStore) error {
	return executeUpgradeRegistration(directory, keyPath, original, next, journal, true)
}

func executeUpgradeRegistration(directory, keyPath string, original, next RegistrationSnapshot, journal *UpgradeJournalStore, restore bool) error {
	for _, snapshot := range []RegistrationSnapshot{original, next} {
		if err := validateRegistrationRestore(directory, keyPath, snapshot); err != nil {
			return err
		}
	}
	var current RegistrationSnapshot
	inspect := func() (metadataObservation, error) {
		var err error
		// Ownership is already bound in both validated snapshots. A new key
		// interrupted before InstallLocation is written must remain recoverable.
		current, err = captureRegistration("", keyPath, false)
		if err != nil {
			return metadataObservation{}, err
		}
		return metadataObservation{before: sameRegistration(current, original), after: sameRegistration(current, next), intermediate: registrationIntermediate(current, original, next)}, nil
	}
	desired := next
	if restore {
		desired = original
	}
	return runMetadataOperation(journal, "registration", registrationOperationFingerprint(original, next), sameRegistration(original, next), restore, inspect, func() error { return replaceRegistrationValues(keyPath, desired, current) })
}

func registrationOperationFingerprint(original, next RegistrationSnapshot) string {
	canonical := func(snapshot RegistrationSnapshot) RegistrationSnapshot {
		snapshot.Values = append([]RegistrationValue(nil), snapshot.Values...)
		sort.Slice(snapshot.Values, func(i, j int) bool { return snapshot.Values[i].Name < snapshot.Values[j].Name })
		return snapshot
	}
	data, _ := json.Marshal([]RegistrationSnapshot{canonical(original), canonical(next)})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func registrationIntermediate(current, original, next RegistrationSnapshot) bool {
	if !current.Exists {
		return !original.Exists || !next.Exists
	}
	if !original.Exists && !next.Exists {
		return false
	}
	index := func(snapshot RegistrationSnapshot) map[string]RegistrationValue {
		values := map[string]RegistrationValue{}
		for _, value := range snapshot.Values {
			values[strings.ToLower(value.Name)] = value
		}
		return values
	}
	before, after, actual := index(original), index(next), index(current)
	equal := func(a, b RegistrationValue) bool { return a.Type == b.Type && bytes.Equal(a.Data, b.Data) }
	for name, value := range actual {
		old, was := before[name]
		new, is := after[name]
		if (!was || !equal(value, old)) && (!is || !equal(value, new)) {
			return false
		}
	}
	for name := range before {
		if _, both := after[name]; both {
			if _, exists := actual[name]; !exists {
				return false
			}
		}
	}
	return true
}
