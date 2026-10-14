//go:build windows

package modulepackage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"winone/internal/configjson"
)

type PreparationIssue struct {
	StageID string `json:"stageId"`
	Reason  string `json:"reason"`
}
type PreparationRecovery struct {
	Recovered int                `json:"recovered"`
	Issues    []PreparationIssue `json:"issues"`
}

// UntrackedStages is a bounded metadata-only scan for the existing cleanup UI.
// A matching name is only a reason to inspect, never proof of ownership.
func (r *Registry) UntrackedStages(ctx context.Context) ([]string, error) {
	current, err := r.read(ctx)
	if err != nil {
		return nil, err
	}
	ledger, err := r.Reclamation()
	if err != nil {
		return nil, err
	}
	return r.untrackedStages(ctx, current, ledger)
}

func (r *Registry) untrackedStages(ctx context.Context, current RegistrySnapshot, ledger ReclamationSnapshot) ([]string, error) {
	known := map[string]bool{}
	for _, record := range current.Records {
		known[record.StageID] = true
	}
	if change := current.Pending; change != nil {
		known[change.Before.StageID] = true
		if change.After != nil {
			known[change.After.StageID] = true
		}
	}
	for _, item := range ledger.Items {
		known[item.Package.StageID] = true
	}
	// A completed change may not yet have reached the cleanup ledger.
	if receipt := current.LastChange; receipt != nil {
		known[receipt.Change.Before.StageID] = true
		if receipt.Change.After != nil {
			known[receipt.Change.After.StageID] = true
		}
	}
	root, err := os.OpenRoot(r.directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	directory, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	var result []string
	count := 0
	for {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		entries, readErr := directory.ReadDir(128)
		count += len(entries)
		if count > maxRegisteredModules+maxReclamationItems*2+16 {
			return nil, errors.New("package directory has too many entries to inspect")
		}
		for _, entry := range entries {
			if stageIDPattern.MatchString(entry.Name()) && !known[entry.Name()] {
				result = append(result, entry.Name())
				if len(result) > maxReclamationItems {
					return nil, errors.New("too many untracked stages; inspect the package directory")
				}
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	sort.Strings(result)
	return result, nil
}

// RecoverPreparations adopts only complete, authenticated unreferenced stages.
// It never executes or recycles files. Unsafe entries remain visible as issues;
// ordinary reclaim checks run again immediately before any later recycling.
func (r *Registry) RecoverPreparations(ctx context.Context, trust func(string) (Publisher, error)) (result PreparationRecovery, err error) {
	if trust == nil {
		return result, ErrUntrustedPublisher
	}
	_, err = r.lockedRegistry(ctx, func(current RegistrySnapshot) (RegistrySnapshot, error) {
		ledger, err := r.Reclamation()
		if err != nil {
			return current, err
		}
		ids, err := r.untrackedStages(ctx, current, ledger)
		if err != nil {
			return current, err
		}
		var candidates []ReclamationItem
		for _, id := range ids {
			if err = ctx.Err(); err != nil {
				return current, err
			}
			record, checkErr := r.inspectLegacyStage(ctx, id, trust)
			if err = ctx.Err(); err != nil {
				return current, err
			}
			if checkErr != nil {
				result.Issues = append(result.Issues, PreparationIssue{StageID: id, Reason: checkErr.Error()})
				continue
			}
			candidates = append(candidates, ReclamationItem{Kind: "preparation", Package: record, ChangeID: "legacy-" + id, Phase: "queued"})
		}
		if len(candidates) == 0 {
			return current, nil
		}
		ledger, err = nextReclamation(ledger)
		if err != nil {
			return current, err
		}
		ledger.SchemaVersion = 3
		for _, item := range candidates {
			ledger.Items[reclaimKey(item)] = item
		}
		if err = r.writeReclamation(ctx, ledger); err != nil {
			return current, err
		}
		result.Recovered = len(candidates)
		return current, nil
	})
	return result, err
}

func (r *Registry) inspectLegacyStage(ctx context.Context, id string, trust func(string) (Publisher, error)) (RegisteredPackage, error) {
	var record RegisteredPackage
	directory := filepath.Join(r.directory, id)
	if exists, err := ordinaryDirectoryExists(directory); err != nil {
		return record, err
	} else if !exists {
		return record, errors.New("untracked stage disappeared")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return record, err
	}
	raw, err := readStageFile(ctx, root, StageReceiptName, 16<<10)
	err = errors.Join(err, root.Close())
	if err != nil {
		return record, errors.New("完整暂存回执缺失或不可读，目录已保留")
	}
	var receipt StageReceipt
	if err = configjson.DecodeObject(raw, &receipt); err != nil || receipt.SchemaVersion != 1 || receipt.StageID != id {
		return record, errors.New("暂存回执无效，目录已保留")
	}
	record = RegisteredPackage{ModuleID: receipt.ModuleID, Version: receipt.Version, StageID: id, ManifestSHA256: receipt.ManifestSHA256}
	if err = validateRegistration(record); err != nil {
		return record, err
	}
	namespace, _, _ := strings.Cut(record.ModuleID, ".")
	publisher, err := trust(namespace)
	if err != nil {
		return record, err
	}
	if err = verifyReclaimDirectory(ctx, directory, record, publisher); err != nil {
		return record, err
	}
	return record, nil
}
