package modulepackage

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"winone/internal/distribution"
)

// VerifiedArchive is inspection evidence, not a permanent execution permit.
// Source files can change after inspection; installation must verify its own
// private staged copy against this signed manifest before registering a module.
type VerifiedArchive struct {
	manifest   *VerifiedManifest
	definition *SettingsDefinition
}

func (v *VerifiedArchive) Manifest() *VerifiedManifest   { return v.manifest }
func (v *VerifiedArchive) Settings() *SettingsDefinition { return v.definition }

// PreviewArchive selects only a Host-trusted key from the untrusted identity
// hint, then authenticates and checks the same open archive. It never extracts
// payload or executes a validator supplied by the package.
func PreviewArchive(ctx context.Context, filename string, trust func(string) (Publisher, error)) (*VerifiedArchive, error) {
	if trust == nil {
		return nil, ErrUntrustedPublisher
	}
	inspection, err := inspectArchiveTrusted(ctx, filename, trust)
	if err != nil {
		return nil, err
	}
	defer inspection.file.Close()
	if err = inspection.copyPayload(ctx, nil); err != nil {
		return nil, err
	}
	result := &VerifiedArchive{manifest: inspection.manifest}
	if files := inspection.manifest.metadata.Settings; files != nil {
		read := func(name string, limit int64) ([]byte, error) {
			for _, record := range inspection.manifest.files {
				if !strings.EqualFold(record.Path, name) {
					continue
				}
				raw, err := readArchiveEntry(ctx, inspection.entries[record.Path], limit)
				if err != nil {
					return nil, err
				}
				sum := sha256.Sum256(raw)
				if int64(len(raw)) != record.Size || hex.EncodeToString(sum[:]) != record.SHA256 {
					return nil, ErrApprovalChanged
				}
				return raw, nil
			}
			return nil, errors.New("settings file is missing")
		}
		schema, err := read(files.Schema, MaxSettingsSchemaBytes)
		if err != nil {
			return nil, err
		}
		defaults, err := read(files.Defaults, MaxSettingsBytes)
		if err != nil {
			return nil, err
		}
		result.definition, err = compileSettings(schema, defaults)
		if err != nil {
			return nil, err
		}
		if err = result.definition.configureMigrations(inspection.manifest.metadata.Module.SettingsSchemaVersion, files.Migrations, func(name string) ([]byte, error) { return read(name, MaxMigrationPatchBytes) }); err != nil {
			return nil, err
		}
	}
	return result, ctx.Err()
}

type cancellableReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r cancellableReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func readArchiveEntry(ctx context.Context, entry *zip.File, limit int64) ([]byte, error) {
	if entry == nil || entry.UncompressedSize64 > uint64(limit) {
		return nil, errors.New("module manifest or signature is missing or too large")
	}
	reader, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(cancellableReader{ctx, reader}, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("module metadata exceeds size limit")
	}
	return data, nil
}

// VerifyArchive never extracts files or runs module code. It checks signatures,
// exact inventory membership, CRCs, sizes and SHA-256 with a reused copy buffer.
func VerifyArchive(ctx context.Context, filename string, publisher Publisher) (*VerifiedArchive, error) {
	inspection, err := inspectArchive(ctx, filename, publisher)
	if err != nil {
		return nil, err
	}
	defer inspection.file.Close()
	if err = inspection.copyPayload(ctx, nil); err != nil {
		return nil, err
	}
	return &VerifiedArchive{manifest: inspection.manifest}, nil
}

type archiveInspection struct {
	file                     *os.File
	entries                  map[string]*zip.File
	manifest                 *VerifiedManifest
	manifestBytes, signature []byte
}

func inspectArchive(ctx context.Context, filename string, publisher Publisher) (*archiveInspection, error) {
	return inspectArchiveTrusted(ctx, filename, func(string) (Publisher, error) { return publisher, nil })
}

func inspectArchiveTrusted(ctx context.Context, filename string, trust func(string) (Publisher, error)) (_ *archiveInspection, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("module archive must be a regular file")
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			file.Close()
		}
	}()
	info, err = file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("module archive changed type")
	}
	source, count, err := snapshotZIPDirectory(ctx, file, info.Size())
	if err != nil {
		return nil, err
	}
	archive, err := zip.NewReader(source, info.Size())
	if err != nil {
		return nil, err
	}
	if len(archive.File) != count {
		return nil, errors.New("module ZIP directory mismatch")
	}
	entries := map[string]*zip.File{}
	seen := map[string]bool{}
	for _, entry := range archive.File {
		key := strings.ToLower(entry.Name)
		if !distribution.ValidPayloadPath(entry.Name) || !entry.Mode().IsRegular() || seen[key] {
			return nil, errors.New("invalid, duplicate or linked module path")
		}
		entries[entry.Name] = entry
		seen[key] = true
	}
	manifestBytes, err := readArchiveEntry(ctx, entries["module.json"], MaxManifestBytes)
	if err != nil {
		return nil, err
	}
	signature, err := readArchiveEntry(ctx, entries["module.json.minisig"], MaxSignatureBytes)
	if err != nil {
		return nil, err
	}
	var identity struct {
		Metadata struct {
			Kind   string `json:"kind"`
			Module struct {
				ID string `json:"id"`
			} `json:"module"`
			Runtime struct {
				ID string `json:"id"`
			} `json:"runtime"`
		} `json:"metadata"`
	}
	if err = json.Unmarshal(manifestBytes, &identity); err != nil {
		return nil, err
	}
	packageID := identity.Metadata.Module.ID
	if identity.Metadata.Kind == "runtime" {
		packageID = identity.Metadata.Runtime.ID
	}
	namespace, _, _ := strings.Cut(packageID, ".")
	publisher, err := trust(namespace)
	if err != nil {
		return nil, err
	}
	manifest, err := VerifyManifest(manifestBytes, signature, publisher)
	if err != nil {
		return nil, err
	}
	if len(entries) != len(manifest.files)+2 {
		return nil, errors.New("module archive contains missing or unlisted files")
	}
	return &archiveInspection{file: file, entries: entries, manifest: manifest, manifestBytes: manifestBytes, signature: signature}, nil
}

func (a *archiveInspection) copyPayload(ctx context.Context, destination *os.Root) error {
	buffer := make([]byte, 64<<10)
	for _, record := range a.manifest.files {
		entry := a.entries[record.Path]
		if entry == nil || entry.UncompressedSize64 != uint64(record.Size) {
			return errors.New("module payload size or path mismatch")
		}
		if err := copyPayloadFile(ctx, entry, record, destination, buffer); err != nil {
			return err
		}
	}
	return nil
}

func copyPayloadFile(ctx context.Context, entry *zip.File, record distribution.File, destination *os.Root, buffer []byte) (err error) {
	reader, err := entry.Open()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, reader.Close()) }()
	var output *os.File
	if destination != nil {
		if err = destination.MkdirAll(filepath.Dir(record.Path), 0700); err != nil {
			return err
		}
		output, err = destination.OpenFile(record.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, output.Close()) }()
	}
	var writer io.Writer
	if output != nil {
		writer = output
	}
	if err = transferPayload(ctx, reader, record, writer, buffer); err != nil {
		return err
	}
	if output != nil {
		return output.Sync()
	}
	return nil
}

// Inspection, staging and recovery share exact size and digest checks.
func transferPayload(ctx context.Context, reader io.Reader, record distribution.File, output io.Writer, buffer []byte) error {
	digest := sha256.New()
	var writer io.Writer = digest
	if output != nil {
		writer = io.MultiWriter(output, digest)
	}
	size, err := io.CopyBuffer(writer, io.LimitReader(cancellableReader{ctx, reader}, record.Size+1), buffer)
	if err != nil {
		return err
	}
	if size != record.Size || hex.EncodeToString(digest.Sum(nil)) != record.SHA256 {
		return fmt.Errorf("module payload checksum mismatch: %s", record.Path)
	}
	return nil
}
