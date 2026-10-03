package distribution

import "errors"

type metadataObservation struct{ before, after, intermediate bool }

// Shared ordering for bound metadata operations. Only a persisted intent may
// admit a known intermediate state; unrecognized content is never overwritten.
func runMetadataOperation(journal *UpgradeJournalStore, name, fingerprint string, equal, restore bool, inspect func() (metadataObservation, error), write func() error) error {
	state := journal.Snapshot()
	if (!restore && state.Phase != "applying") || (restore && state.Phase != "restoring") {
		return errors.New("invalid metadata operation phase")
	}
	status := state.MetadataStates[name]
	bound, exists := state.MetadataFingerprints[name]
	if (exists && bound != fingerprint) || (!exists && status != "pending") {
		return errors.New("metadata intent does not match bound snapshots")
	}
	current, err := inspect()
	if err != nil {
		return err
	}
	if status == "unchanged" || (restore && status == "pending") {
		if !current.before || (status == "unchanged" && !equal) {
			return errors.New("unmodified metadata changed externally")
		}
		return nil
	}
	wanted, prior, intent, complete := current.after, current.before, "writing", "applied"
	if restore {
		wanted, prior, intent, complete = current.before, current.after, "restoring", "restored"
	}
	if status == complete {
		if !wanted {
			return errors.New("completed metadata changed externally")
		}
		return nil
	}
	if (!restore && status != "pending" && status != "writing") || (restore && status != "writing" && status != "applied" && status != "restoring") {
		return errors.New("invalid metadata progress")
	}
	if !prior && !(status != "pending" && (wanted || current.intermediate)) {
		return errors.New("metadata differs from expected upgrade states")
	}
	if err = journal.BindMetadata(state.Revision, name, fingerprint); err != nil {
		return err
	}
	state = journal.Snapshot()
	if !restore && status == "pending" && equal && current.before {
		return journal.RecordMetadata(state.Revision, name, "unchanged")
	}
	if status != intent {
		if err = journal.RecordMetadata(state.Revision, name, intent); err != nil {
			return err
		}
	}
	if !wanted {
		if err = write(); err != nil {
			return err
		}
	}
	actual, err := inspect()
	if err != nil {
		return err
	}
	if (!restore && !actual.after) || (restore && !actual.before) {
		return errors.New("metadata operation verification failed")
	}
	return journal.RecordMetadata(journal.Snapshot().Revision, name, complete)
}
