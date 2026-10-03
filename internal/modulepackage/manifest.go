package modulepackage

import (
	"aead.dev/minisign"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"winone/internal/catalog"
	"winone/internal/configjson"
	"winone/internal/distribution"
)

const MaxManifestBytes = 1 << 20
const MaxSignatureBytes = 8 << 10
const MaxModuleFileBytes = int64(256 << 20)
const MaxModulePayloadBytes = int64(2 << 30)
const MaxModuleFiles = 4096

var ErrInvalidSignature = errors.New("invalid module signature")
var ErrUntrustedPublisher = errors.New("module publisher is not trusted")

type Manifest struct {
	Format   string              `json:"format"`
	Metadata Metadata            `json:"metadata"`
	Files    []distribution.File `json:"files"`
}

// Publisher comes from Host-owned trust configuration, never from an archive.
// One key authorizes only its configured publisher namespace.
type Publisher struct {
	Namespace string
	PublicKey string
}
type VerifiedManifest struct {
	metadata Metadata
	files    []distribution.File
	digest   string
}

func (v *VerifiedManifest) Metadata() Metadata {
	m := v.metadata
	m.Module = m.Module.Clone()
	return m
}
func (v *VerifiedManifest) Files() []distribution.File {
	return append([]distribution.File(nil), v.files...)
}
func (v *VerifiedManifest) Digest() string { return v.digest }

// VerifyManifest authenticates the exact small manifest before parsing it.
// A successful result still requires archive content verification, compatibility
// checks and a controlled installation transaction before any code may execute.
func VerifyManifest(data, signature []byte, publisher Publisher) (*VerifiedManifest, error) {
	if len(data) == 0 || len(data) > MaxManifestBytes || len(signature) == 0 || len(signature) > MaxSignatureBytes {
		return nil, errors.New("module manifest or signature size is invalid")
	}
	if !catalog.ValidID(publisher.Namespace) || strings.Contains(publisher.Namespace, ".") {
		return nil, ErrUntrustedPublisher
	}
	var key minisign.PublicKey
	if err := key.UnmarshalText([]byte(publisher.PublicKey)); err != nil {
		return nil, ErrUntrustedPublisher
	}
	if !minisign.Verify(key, data, signature) {
		return nil, ErrInvalidSignature
	}
	var envelope struct {
		Format   string              `json:"format"`
		Metadata json.RawMessage     `json:"metadata"`
		Files    []distribution.File `json:"files"`
	}
	if err := configjson.DecodeObject(data, &envelope); err != nil {
		return nil, err
	}
	if envelope.Format != "winone.module.v1" {
		return nil, errors.New("unsupported module package format")
	}
	// Keep the inner metadata limit even inside a larger signed file inventory.
	metadata, err := DecodeMetadata(envelope.Metadata)
	if err != nil {
		return nil, err
	}
	manifest := Manifest{Format: envelope.Format, Metadata: metadata, Files: envelope.Files}
	namespace, _, _ := strings.Cut(manifest.Metadata.Module.ID, ".")
	if namespace != publisher.Namespace {
		return nil, ErrUntrustedPublisher
	}
	if len(manifest.Files) == 0 || len(manifest.Files) > MaxModuleFiles {
		return nil, errors.New("invalid module inventory size")
	}
	files := map[string]distribution.File{}
	var total int64
	for _, file := range manifest.Files {
		normalized := strings.ToLower(file.Path)
		if !distribution.ValidPayloadPath(file.Path) || normalized == "module.json" || normalized == "module.json.minisig" || normalized == StageReceiptName {
			return nil, errors.New("invalid module payload path")
		}
		if _, exists := files[normalized]; exists {
			return nil, errors.New("duplicate module payload path")
		}
		digest, err := hex.DecodeString(file.SHA256)
		if err != nil || len(digest) != sha256.Size || file.SHA256 != strings.ToLower(file.SHA256) || file.Size < 0 || file.Size > MaxModuleFileBytes {
			return nil, errors.New("invalid module payload record")
		}
		total += file.Size
		if total > MaxModulePayloadBytes {
			return nil, errors.New("module payload exceeds size limit")
		}
		files[normalized] = file
	}
	for name := range files {
		parts := strings.Split(name, "/")
		for end := 1; end < len(parts); end++ {
			if _, exists := files[strings.Join(parts[:end], "/")]; exists {
				return nil, errors.New("module file conflicts with a parent directory")
			}
		}
	}
	worker := strings.ToLower(strings.ReplaceAll(manifest.Metadata.Module.Worker, `\`, "/"))
	if files[worker].Size == 0 || files["license.txt"].Size == 0 {
		return nil, errors.New("module worker or license is missing")
	}
	sum := sha256.Sum256(data)
	return &VerifiedManifest{metadata: manifest.Metadata, files: manifest.Files, digest: hex.EncodeToString(sum[:])}, nil
}
