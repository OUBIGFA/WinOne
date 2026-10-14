package modulepackage

import (
	"aead.dev/minisign"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"winone/internal/distribution"
)

func signedTestManifest(t testing.TB) (Manifest, Publisher, minisign.PrivateKey) {
	t.Helper()
	public, private, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	record := func(path, text string) distribution.File {
		sum := sha256.Sum256([]byte(text))
		return distribution.File{Path: path, Size: int64(len(text)), SHA256: hex.EncodeToString(sum[:])}
	}
	return Manifest{Format: "winone.module.v1", Metadata: testMetadata(), Files: []distribution.File{record("bin/worker.exe", "inert worker"), record("LICENSE.txt", "test license")}}, Publisher{Namespace: "example", PublicKey: public.String()}, private
}
func TestSignedManifestBindsPublisherMetadataAndInventory(t *testing.T) {
	m, publisher, key := signedTestManifest(t)
	data, _ := json.Marshal(m)
	signature := minisign.Sign(key, data)
	verified, err := VerifyManifest(data, signature, publisher)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Metadata().Module.ID != "example.tool" || len(verified.Files()) != 2 || verified.Digest() == "" {
		t.Fatal("verified manifest lost signed fields")
	}
	records := verified.Files()
	records[0].Path = "changed"
	if verified.Files()[0].Path != "bin/worker.exe" {
		t.Fatal("caller changed verified inventory")
	}
	changed := append([]byte{}, data...)
	changed[strings.Index(string(changed), "1.2.0")] = '2'
	if _, err := VerifyManifest(changed, signature, publisher); !errors.Is(err, ErrInvalidSignature) {
		t.Fatal("changed metadata accepted", err)
	}
	publisher.Namespace = "another"
	if _, err := VerifyManifest(data, signature, publisher); !errors.Is(err, ErrUntrustedPublisher) {
		t.Fatal("publisher namespace was not bound", err)
	}
	_, wrong, _ := signedTestManifest(t)
	if _, err := VerifyManifest(data, signature, wrong); !errors.Is(err, ErrInvalidSignature) {
		t.Fatal("wrong key accepted", err)
	}
}
func TestSignedManifestRejectsUninstallableInventories(t *testing.T) {
	for _, change := range []func(*Manifest){
		func(m *Manifest) {
			record := m.Files[1]
			record.Path = StageReceiptName
			m.Files = append(m.Files, record)
		},
		func(m *Manifest) { m.Metadata.Module.Name.Chinese = strings.Repeat("x", MaxMetadataBytes) },
		func(m *Manifest) { m.Format = "other" }, func(m *Manifest) { m.Files = m.Files[:1] }, func(m *Manifest) { m.Files[0].Path = "../worker.exe" },
		func(m *Manifest) { m.Files[0].Path = "CON.exe" }, func(m *Manifest) { m.Files[0].SHA256 = "wrong" }, func(m *Manifest) { m.Files[0].Size = -1 },
		func(m *Manifest) {
			duplicate := m.Files[0]
			duplicate.Path = "BIN/WORKER.EXE"
			m.Files = append(m.Files, duplicate)
		},
		func(m *Manifest) { parent := m.Files[1]; parent.Path = "bin"; m.Files = append(m.Files, parent) },
		func(m *Manifest) { m.Files[0].Path = "module.json" }, func(m *Manifest) { m.Files[0].Size = MaxModuleFileBytes + 1 },
	} {
		m, publisher, key := signedTestManifest(t)
		change(&m)
		data, _ := json.Marshal(m)
		if _, err := VerifyManifest(data, minisign.Sign(key, data), publisher); err == nil {
			t.Fatal("signed but invalid inventory accepted", string(data))
		}
	}
}
func TestSignedManifestRejectsOversizedAndAmbiguousInput(t *testing.T) {
	m, publisher, key := signedTestManifest(t)
	data, _ := json.Marshal(m)
	duplicate := []byte(strings.Replace(string(data), `"format":"winone.module.v1"`, `"format":"winone.module.v1","format":"winone.module.v1"`, 1))
	if _, err := VerifyManifest(duplicate, minisign.Sign(key, duplicate), publisher); err == nil {
		t.Fatal("signed duplicate JSON accepted")
	}
	if _, err := VerifyManifest(make([]byte, MaxManifestBytes+1), nil, publisher); err == nil {
		t.Fatal("oversized manifest accepted")
	}
	if _, err := VerifyManifest(data, make([]byte, MaxSignatureBytes+1), publisher); err == nil {
		t.Fatal("oversized signature accepted")
	}
}
