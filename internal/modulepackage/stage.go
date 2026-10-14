package modulepackage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const StageReceiptName = ".winone-stage.json"

var ErrApprovalChanged = errors.New("module package differs from the approved manifest")
var ErrIncompatible = errors.New("module package is incompatible with this host")

// Admission is implemented by the platform's Host-owned stores. Keeping this
// small seam platform-neutral lets offline package validation run elsewhere.
type dependencyAdmission interface {
	checkAdmission(context.Context, *VerifiedManifest, string) error
}

type StageOptions struct {
	Directory              string
	Publisher              Publisher
	Environment            Environment
	ExpectedManifestSHA256 string
	Dependencies           dependencyAdmission
	// Set only by the registry wrapper, after approval and before payload writes.
	onCreated func(StageReceipt) error
}
type StageReceipt struct {
	SchemaVersion  int    `json:"schemaVersion"`
	StageID        string `json:"stageId"`
	ModuleID       string `json:"moduleId"`
	Version        string `json:"version"`
	ManifestSHA256 string `json:"manifestSha256"`
}
type StagedPackage struct {
	directory string
	manifest  *VerifiedManifest
	ready     bool
}

func (s *StagedPackage) Directory() string           { return s.directory }
func (s *StagedPackage) Ready() bool                 { return s.ready }
func (s *StagedPackage) Manifest() *VerifiedManifest { return s.manifest }

func validateStageOptions(options StageOptions) error {
	if !filepath.IsAbs(options.Directory) {
		return errors.New("staging requires an explicit absolute store directory")
	}
	expected, err := hex.DecodeString(options.ExpectedManifestSHA256)
	if err != nil || len(expected) != 32 {
		return ErrApprovalChanged
	}
	return nil
}
func verifyStageApproval(manifest *VerifiedManifest, options StageOptions) error {
	if manifest.Digest() != strings.ToLower(options.ExpectedManifestSHA256) {
		return ErrApprovalChanged
	}
	report, err := manifest.metadata.CheckCompatibility(options.Environment)
	if err != nil {
		return err
	}
	if !report.Compatible {
		return ErrIncompatible
	}
	return nil
}

// StageArchive writes only beneath a fresh directory in the Host-owned store.
// On a write failure it returns the unready stage with its error for recovery;
// it never deletes unknown files, registers a module, or starts an executable.
func StageArchive(ctx context.Context, filename string, options StageOptions) (*StagedPackage, error) {
	if err := validateStageOptions(options); err != nil {
		return nil, err
	}
	inspection, err := inspectArchive(ctx, filename, options.Publisher)
	if err != nil {
		return nil, err
	}
	defer inspection.file.Close()
	if err = verifyStageApproval(inspection.manifest, options); err != nil {
		return nil, err
	}
	directory, err := filepath.Abs(options.Directory)
	if err != nil {
		return nil, err
	}
	base, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer base.Close()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	name := "stage-" + hex.EncodeToString(nonce[:])
	if err = base.Mkdir(name, 0700); err != nil {
		return nil, err
	}
	result := &StagedPackage{directory: filepath.Join(directory, name), manifest: inspection.manifest}
	receipt := StageReceipt{SchemaVersion: 1, StageID: name, ModuleID: inspection.manifest.metadata.PackageID(), Version: inspection.manifest.metadata.Version, ManifestSHA256: inspection.manifest.Digest()}
	if options.onCreated != nil {
		if err = options.onCreated(receipt); err != nil {
			return result, err
		}
	}
	root, err := base.OpenRoot(name)
	if err != nil {
		return result, err
	}
	defer root.Close()
	if err = inspection.copyPayload(ctx, root); err != nil {
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if err = writeStageFile(root, "module.json", inspection.manifestBytes); err != nil {
		return result, err
	}
	if err = writeStageFile(root, "module.json.minisig", inspection.signature); err != nil {
		return result, err
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	// Receipt existence alone is not a commit: the registry must check the result
	// and reverify staged files on recovery before publishing any installed entry.
	if err = writeStageFile(root, StageReceiptName, data); err != nil {
		return result, err
	}
	result.ready = true
	return result, nil
}

func writeStageFile(root *os.Root, name string, data []byte) (err error) {
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	n, err := file.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return file.Sync()
}
