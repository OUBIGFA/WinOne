package modulepackage

import (
	"aead.dev/minisign"
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func archiveFixture(t *testing.T, change func(map[string][]byte), link bool) ([]byte, Publisher) {
	t.Helper()
	manifest, publisher, key := signedTestManifest(t)
	data, _ := json.Marshal(manifest)
	files := map[string][]byte{"module.json": data, "module.json.minisig": minisign.Sign(key, data), "bin/worker.exe": []byte("inert worker"), "LICENSE.txt": []byte("test license")}
	if change != nil {
		change(files)
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, body := range files {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0600)
		if link && name == "bin/worker.exe" {
			header.SetMode(os.ModeSymlink | 0777)
		}
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes(), publisher
}
func verifyFixture(t *testing.T, data []byte, publisher Publisher) error {
	t.Helper()
	name := filepath.Join(t.TempDir(), "tool.winone-module")
	if err := os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
	_, err := VerifyArchive(context.Background(), name, publisher)
	return err
}
func TestArchiveVerifiesActualSignedPayloadWithoutExtracting(t *testing.T) {
	data, publisher := archiveFixture(t, nil, false)
	directory := t.TempDir()
	name := filepath.Join(directory, "tool.winone-module")
	if err := os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
	result, err := VerifyArchive(context.Background(), name, publisher)
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest().Metadata().Module.ID != "example.tool" {
		t.Fatal("wrong verified module")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatal("verification extracted files", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = VerifyArchive(ctx, name, publisher); err == nil {
		t.Fatal("cancelled verification continued")
	}
}
func TestArchiveRejectsChangedMissingExtraAndLinkedFiles(t *testing.T) {
	for _, change := range []func(map[string][]byte){
		func(files map[string][]byte) { files["bin/worker.exe"] = []byte("other worker") },
		func(files map[string][]byte) { delete(files, "LICENSE.txt") },
		func(files map[string][]byte) { files["extra.exe"] = []byte("unexpected") },
		func(files map[string][]byte) { files["../worker.exe"] = []byte("outside") },
		func(files map[string][]byte) { files["BIN/WORKER.EXE"] = []byte("duplicate") },
		func(files map[string][]byte) { files["module.json.minisig"] = []byte("wrong signature") },
	} {
		data, publisher := archiveFixture(t, change, false)
		if err := verifyFixture(t, data, publisher); err == nil {
			t.Fatal("invalid archive accepted")
		}
	}
	data, publisher := archiveFixture(t, nil, true)
	if err := verifyFixture(t, data, publisher); err == nil {
		t.Fatal("symbolic link accepted")
	}
}
func TestArchiveRejectsDirectoryCountSpoofingAndTrailingData(t *testing.T) {
	data, publisher := archiveFixture(t, nil, false)
	for _, change := range []func([]byte){
		func(b []byte) { binary.LittleEndian.PutUint16(b[len(b)-22+10:], 0xffff) },
		func(b []byte) { binary.LittleEndian.PutUint16(b[len(b)-22+10:], 1) },
		func(b []byte) {
			binary.LittleEndian.PutUint16(b[len(b)-22+8:], 2)
			binary.LittleEndian.PutUint16(b[len(b)-22+10:], 2)
		},
		func(b []byte) { binary.LittleEndian.PutUint32(b[len(b)-22+12:], 0xffffffff) },
		func(b []byte) { binary.LittleEndian.PutUint16(b[len(b)-22+4:], 1) },
	} {
		copy := append([]byte{}, data...)
		change(copy)
		if err := verifyFixture(t, copy, publisher); err == nil {
			t.Fatal("unbounded or inconsistent ZIP directory accepted")
		}
	}
	if err := verifyFixture(t, append(data, []byte("trailing")...), publisher); err == nil {
		t.Fatal("trailing archive data accepted")
	}
}

func TestZIPDirectorySnapshotDoesNotObserveLaterSourceChanges(t *testing.T) {
	data, _ := archiveFixture(t, nil, false)
	source, count, err := snapshotZIPDirectory(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint16(data[len(data)-22+10:], 0xffff)
	archive, err := zip.NewReader(source, int64(len(data)))
	if err != nil || len(archive.File) != count {
		t.Fatal("parser observed mutable central directory", err)
	}
}
