//go:build windows

package directorylinks

import (
	"errors"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

func checked(r Record) Record {
	if r.Migration != nil {
		if r.Migration.Phase != "complete" {
			r.Accessible = false
			r.Status = "unknown"
			r.Reason = "migration-incomplete"
			if r.Migration.Phase == "failed" {
				r.Status = "failed"
				r.Reason = "migration-failed"
			}
			r.Detail = r.Migration.Message
			return r
		}
		if r.Migration.Operation == "restore" {
			return checkedRestoredMigration(r)
		}
	}
	r.CheckedAt = time.Now().UTC().Format(time.RFC3339Nano)
	r.Status, r.Reason, r.Detail = "unknown", "cannot-check", ""
	r.Accessible = false
	r.LinkCount = 0
	r.ActualTarget = ""
	r.ActualRelative = false
	r.ActualTag = 0
	r.ResolvedTarget = ""
	r.ActualType = ""
	r.ObservedEntry = Identity{}
	r.ObservedTarget = Identity{}
	entry, err := openState(r.LinkPath, false, false)
	if err != nil {
		return failedCheck(r, err, "entry")
	}
	defer entry.close()
	r.ObservedEntry = entry.identity
	r.ActualType = entry.objectType()
	r.LinkCount = entry.links
	if entry.reparse {
		tag, target, relative, e := readReparse(entry)
		if e == nil {
			r.ActualTarget = target
			r.ActualRelative = relative
			r.ActualTag = tag
			r.ResolvedTarget = target
			if relative {
				r.ResolvedTarget = filepath.Join(filepath.Dir(r.LinkPath), target)
			}
			if tag == tagJunction {
				r.ActualType = "junction-directory"
			} else if tag == tagSymlink {
				r.ActualType = "symlink-" + entry.objectType()
			}
		}
	}
	if r.EntryIdentity.File == "" || !sameObserved(entry.identity, r.EntryIdentity) {
		r.Status, r.Reason = "replaced", "entry-replaced"
		return r
	}
	if r.Kind == "hardlink" {
		if entry.reparse || entry.directory {
			r.Status, r.Reason = "replaced", "entry-type-changed"
			return r
		}
		if !sameObserved(entry.identity, r.TargetIdentity) {
			r.Status, r.Reason = "source-changed", "source-identity-changed"
			return r
		}
		accessible, e := openState(r.LinkPath, true, true)
		if e != nil {
			return failedCheck(r, e, "entry")
		}
		accessible.close()
		r.Accessible = true
		if strings.EqualFold(filepath.Clean(r.LinkPath), filepath.Clean(r.TargetPath)) {
			r.Status, r.Reason = "other-links", "source-not-recorded"
			if entry.links < 2 {
				r.Status, r.Reason = "unlinked", "no-other-links"
			}
			return r
		}
		source, e := openState(r.TargetPath, true, false)
		if e != nil {
			if !missing(e) {
				return failedCheck(r, e, "source")
			}
			if entry.links > 1 {
				r.Status, r.Reason = "other-links", "source-missing-other-links"
			} else if entry.links == 1 {
				r.Status, r.Reason = "unlinked", "source-missing-no-links"
			} else {
				r.Status, r.Reason = "unknown", "cannot-check-links"
			}
			return r
		}
		defer source.close()
		r.ObservedTarget = source.identity
		r.ResolvedTarget = source.finalPath
		if source.directory || source.reparse || !sameIdentity(entry.identity, source.identity) {
			r.Status, r.Reason = "source-changed", "source-identity-changed"
			return r
		}
		if entry.links < 2 {
			r.Status, r.Reason = "unlinked", "no-other-links"
			return r
		}
		r.Status, r.Reason = "active", "same-file"
		return r
	}
	if r.Kind == "shortcut" {
		if entry.reparse || entry.directory {
			r.Status, r.Reason = "replaced", "entry-type-changed"
			return r
		}
		target, e := shortcutTarget(r.LinkPath)
		if e != nil {
			r.Status, r.Reason, r.Detail = "unknown", "target-unverified", e.Error()
			return r
		}
		r.ActualTarget = target
		r.ResolvedTarget = target
		r.ActualType = "shortcut"
		actual, e := openState(target, true, true)
		if e != nil {
			return failedCheck(r, e, "target")
		}
		defer actual.close()
		r.Accessible = true
		r.ObservedTarget = actual.identity
		r.ResolvedTarget = actual.finalPath
		if !strings.EqualFold(filepath.Clean(target), filepath.Clean(r.TargetText)) || !sameObserved(actual.identity, r.TargetIdentity) {
			r.Status, r.Reason = "target-changed", "target-identity-changed"
			return r
		}
		r.Status, r.Reason = "active", "target-accessible"
		return r
	}
	if !entry.reparse {
		r.Status, r.Reason = "replaced", "entry-type-changed"
		return r
	}
	tag, target, relative, err := readReparse(entry)
	if err != nil {
		r.Status, r.Reason, r.Detail = "unknown", "invalid-reparse", err.Error()
		return r
	}
	wantTag := tagSymlink
	if r.Kind == "junction" {
		wantTag = tagJunction
	}
	if tag != wantTag || entry.objectType() != r.ObjectType {
		r.Status, r.Reason = "replaced", "entry-type-changed"
		return r
	}
	r.ActualType = r.Kind + "-" + entry.objectType()
	r.ActualTarget = target
	pointChanged := relative != r.Relative || !strings.EqualFold(filepath.Clean(target), filepath.Clean(r.TargetText))
	resolved := target
	if relative {
		resolved = filepath.Join(filepath.Dir(r.LinkPath), target)
	}
	r.ResolvedTarget = resolved
	actual, err := openState(resolved, true, true)
	if err != nil {
		return failedCheck(r, err, "target")
	}
	defer actual.close()
	r.ObservedTarget = actual.identity
	r.ResolvedTarget = actual.finalPath
	r.Accessible = true
	if actual.objectType() != r.ObjectType {
		r.Status, r.Reason = "broken", "target-type-changed"
		return r
	}
	linked, err := openState(r.LinkPath, true, true)
	if err != nil {
		return failedCheck(r, err, "target")
	}
	stable := sameIdentity(linked.identity, actual.identity)
	linked.close()
	lastTag, lastTarget, lastRelative, lastErr := readReparse(entry)
	if !stable || lastErr != nil || lastTag != tag || lastTarget != target || lastRelative != relative {
		r.Status, r.Reason = "unknown", "changed-during-check"
		return r
	}
	if pointChanged {
		r.Status, r.Reason = "target-changed", "target-path-changed"
		return r
	}
	if !sameObserved(actual.identity, r.TargetIdentity) {
		r.Status, r.Reason = "target-changed", "target-identity-changed"
		return r
	}
	r.Status, r.Reason = "active", "target-accessible"
	return r
}

func failedCheck(r Record, err error, side string) Record {
	r.Detail = err.Error()
	r.Reason = side + "-unverified"
	r.Status = "unknown"
	if errors.Is(err, windows.ERROR_NOT_READY) || errors.Is(err, windows.ERROR_DEVICE_NOT_CONNECTED) {
		r.Status, r.Reason = "unavailable", side+"-offline"
		return r
	}
	if missing(err) {
		path := r.LinkPath
		if side != "entry" {
			path = r.TargetPath
		}
		if side == "target" && filepath.IsAbs(r.ResolvedTarget) {
			path = r.ResolvedTarget
		}
		root := filepath.VolumeName(path) + `\`
		name, _ := windows.UTF16PtrFromString(root)
		if windows.GetDriveType(name) == windows.DRIVE_NO_ROOT_DIR {
			r.Status, r.Reason = "unavailable", side+"-offline"
			return r
		}
		r.Status = "broken"
		r.Reason = side + "-missing"
	}
	return r
}
