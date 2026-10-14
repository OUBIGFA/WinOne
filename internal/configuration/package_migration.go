package configuration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"winone/internal/catalog"
	"winone/internal/configjson"
)

const maxPackageMigrationBytes = 5 << 20

// PackageMigrationSnapshot is a Host-owned recovery record. Its digest is
// bound to the package change journal; it is never sent to the settings UI.
type PackageMigrationSnapshot struct {
	SchemaVersion       int             `json:"schemaVersion"`
	OperationID         string          `json:"operationId"`
	ModuleID            string          `json:"moduleId"`
	Revision            uint64          `json:"revision"`
	BeforeSchema        int             `json:"beforeSchema"`
	AfterSchema         int             `json:"afterSchema"`
	BeforePackageSHA256 string          `json:"beforePackageSha256"`
	AfterPackageSHA256  string          `json:"afterPackageSha256"`
	Before              json.RawMessage `json:"before"`
	After               json.RawMessage `json:"after"`
}

func migrationDigest(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == sha256.Size && value == strings.ToLower(value)
}
func migrationValue(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) > 2<<20 || configjson.ValidateObject(raw) != nil {
		return nil, errors.New("模块迁移配置无效")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return nil, err
	}
	return compact.Bytes(), nil
}
func normalizePackageMigration(value PackageMigrationSnapshot) (PackageMigrationSnapshot, error) {
	if value.SchemaVersion != 1 || !catalog.ValidID(value.OperationID) || len(value.OperationID) > 128 || !catalog.ValidOptionalID(value.ModuleID) || value.BeforeSchema < 1 || value.AfterSchema <= value.BeforeSchema || !migrationDigest(value.BeforePackageSHA256) || !migrationDigest(value.AfterPackageSHA256) || value.BeforePackageSHA256 == value.AfterPackageSHA256 {
		return value, errors.New("模块迁移恢复记录无效")
	}
	var err error
	value.Before, err = migrationValue(value.Before)
	if err != nil {
		return value, err
	}
	value.After, err = migrationValue(value.After)
	return value, err
}
func (s *HostStore) packageMigrationPath(operation string) (string, error) {
	if !catalog.ValidID(operation) || len(operation) > 128 {
		return "", errors.New("模块迁移标识无效")
	}
	return filepath.Join(filepath.Dir(s.store.path), "package-migrations", "migration-"+operation+".json"), nil
}

func (s *HostStore) SavePackageMigration(value PackageMigrationSnapshot) (string, error) {
	s.store.operation.Lock()
	defer s.store.operation.Unlock()
	value, data, err := packageMigrationBytes(value)
	if err != nil {
		return "", err
	}
	current := s.store.Snapshot()
	before, err := migrationValue(current.Values[value.ModuleID])
	if err != nil {
		return "", err
	}
	if current.Revision != value.Revision || !bytes.Equal(before, value.Before) {
		return "", ErrConflict
	}
	if s.store.NeedsRecovery() {
		return "", ErrRecoveryRequired
	}
	path, err := s.packageMigrationPath(value.OperationID)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	// A reused operation ID must not replace an earlier recovery record.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	written, writeErr := file.Write(data)
	if writeErr == nil && written != len(data) {
		writeErr = io.ErrShortWrite
	}
	syncErr := file.Sync()
	if err = errors.Join(writeErr, syncErr, file.Close()); err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// PackageMigrationDigest lets the outer preparation journal record ownership
// before backup creation. It uses the exact normalization and bytes as Save.
func PackageMigrationDigest(value PackageMigrationSnapshot) (string, error) {
	_, data, err := packageMigrationBytes(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func packageMigrationBytes(value PackageMigrationSnapshot) (PackageMigrationSnapshot, []byte, error) {
	value, err := normalizePackageMigration(value)
	if err != nil {
		return value, nil, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return value, nil, err
	}
	if len(data) > maxPackageMigrationBytes {
		return value, nil, errors.New("模块迁移备份过大")
	}
	return value, data, nil
}

func (s *HostStore) LoadPackageMigration(operation, digest string) (PackageMigrationSnapshot, error) {
	if !migrationDigest(digest) {
		return PackageMigrationSnapshot{}, errors.New("模块迁移摘要无效")
	}
	raw, err := s.readPackageMigration(operation)
	if err != nil {
		return PackageMigrationSnapshot{}, err
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != digest {
		return PackageMigrationSnapshot{}, errors.New("模块迁移备份已变化，保留原文件并先恢复")
	}
	return decodePackageMigration(operation, raw)
}

// InspectPackageMigration supplies data only to the Host recovery coordinator.
// The calculated digest is not ownership proof: package and configuration
// relationships must be checked before it can be attached to cleanup work.
func (s *HostStore) InspectPackageMigration(operation string) (PackageMigrationSnapshot, string, error) {
	raw, err := s.readPackageMigration(operation)
	if err != nil {
		return PackageMigrationSnapshot{}, "", err
	}
	value, err := decodePackageMigration(operation, raw)
	if err != nil {
		return PackageMigrationSnapshot{}, "", err
	}
	sum := sha256.Sum256(raw)
	return value, hex.EncodeToString(sum[:]), nil
}

func (s *HostStore) readPackageMigration(operation string) ([]byte, error) {
	path, err := s.packageMigrationPath(operation)
	if err != nil {
		return nil, err
	}
	directory := filepath.Dir(path)
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("模块迁移目录类型无效")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	name := filepath.Base(path)
	info, err = root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxPackageMigrationBytes {
		return nil, errors.New("模块迁移备份类型或大小无效")
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, maxPackageMigrationBytes+1))
	if err = errors.Join(readErr, file.Close()); err != nil {
		return nil, err
	}
	if len(raw) > maxPackageMigrationBytes {
		return nil, errors.New("模块迁移备份过大")
	}
	return raw, nil
}

func decodePackageMigration(operation string, raw []byte) (PackageMigrationSnapshot, error) {
	var value PackageMigrationSnapshot
	if err := configjson.DecodeObject(raw, &value); err != nil {
		return PackageMigrationSnapshot{}, err
	}
	value, err := normalizePackageMigration(value)
	if err != nil || value.OperationID != operation {
		return PackageMigrationSnapshot{}, errors.New("模块迁移备份与操作不匹配")
	}
	return value, nil
}
func sameDocumentExceptMigration(left, right Document, id string) bool {
	left, right = clone(left), clone(right)
	left.Revision = 0
	right.Revision = 0
	left.Values[id] = json.RawMessage("{}")
	right.Values[id] = json.RawMessage("{}")
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	return bytes.Equal(a, b)
}

// ApplyPackageMigration persists only one quarantined namespace. The package
// coordinator has already checked/closed the candidate and owns projection and
// resource rollback; ordinary global/lifecycle apply hooks must not run here.
// Recovery may retain later changes to other namespaces, but may not discard an
// unrelated pending configuration transaction or reinterpret externally edited data.
func (s *HostStore) ApplyPackageMigration(value PackageMigrationSnapshot, forward bool, validate func(json.RawMessage) error) error {
	s.store.operation.Lock()
	defer s.store.operation.Unlock()
	value, err := normalizePackageMigration(value)
	if err != nil {
		return err
	}
	if validate == nil {
		return errors.New("模块迁移校验器缺失")
	}
	_, registered, _ := s.moduleCatalog()
	if registered[value.ModuleID].Unavailable == nil {
		return errors.New("模块须先隔离并完成候选检查，再提交迁移配置")
	}
	current := s.store.Snapshot()
	actual, err := migrationValue(current.Values[value.ModuleID])
	if err != nil {
		return err
	}
	if !bytes.Equal(actual, value.Before) && !bytes.Equal(actual, value.After) {
		return errors.New("模块配置与迁移备份不一致，请先核对")
	}
	if forward && current.Revision != value.Revision {
		return ErrConflict
	}
	target := value.Before
	if forward {
		target = value.After
	}
	if err = validate(target); err != nil {
		return err
	}
	s.store.mu.Lock()
	var pending *Document
	if s.store.state.Pending != nil {
		copy := clone(*s.store.state.Pending)
		pending = &copy
	}
	s.store.mu.Unlock()
	if pending != nil {
		if forward || !sameDocumentExceptMigration(current, *pending, value.ModuleID) {
			return ErrRecoveryRequired
		}
		raw, err := migrationValue(pending.Values[value.ModuleID])
		if err != nil || !bytes.Equal(raw, value.Before) && !bytes.Equal(raw, value.After) {
			return ErrRecoveryRequired
		}
		if err = s.store.persist(diskState{Committed: current}); err != nil {
			return err
		}
	}
	if bytes.Equal(actual, target) {
		return nil
	}
	next := clone(current)
	next.Values[value.ModuleID] = append(json.RawMessage(nil), target...)
	// There is no live module route during this data-only commit. The outer
	// package journal retains both values until runtime/version commit finishes.
	_, err = s.store.commit(next, func(Document) error { return nil })
	return err
}
