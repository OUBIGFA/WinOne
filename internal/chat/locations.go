package chat

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"winone/internal/settings"
)

// RelocateFiles changes only the conversation's storage, not its working
// directory. Call with the runtime closed. Existing source data is retained as
// a recovery copy; unrelated destination files are never overwritten.
func RelocateFiles(c *Conversation, parent string) error {
	root, err := existingDirectory(parent)
	if err != nil {
		return err
	}
	destination := filepath.Join(root, ".winone-chat", c.ID)
	if filepath.Clean(destination) == filepath.Clean(c.SessionDirectory) {
		return nil
	}
	source, err := existingDirectory(c.SessionDirectory)
	if err != nil {
		return err
	}
	if within(source, destination) {
		return errors.New("新保存位置不能位于当前会话目录内")
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(destination, 0700); err != nil {
		return err
	}
	realDestination, err := existingDirectory(destination)
	if err != nil {
		return err
	}
	if !within(root, realDestination) {
		return errors.New("保存位置越过了所选目录")
	}
	for _, entry := range entries {
		from := filepath.Join(source, entry.Name())
		info, err := os.Lstat(from)
		if err != nil {
			return err
		}
		if info.IsDir() && entry.Name() == "attachments" && info.Mode()&os.ModeSymlink == 0 {
			if err = copyAttachmentDirectory(from, filepath.Join(realDestination, entry.Name())); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("会话目录含有链接或子目录，原数据已保留：%s", entry.Name())
		}
		if err = copyConversationFile(from, filepath.Join(realDestination, entry.Name())); err != nil {
			return err
		}
	}
	if err = migrateDraftAttachmentPaths(source, realDestination); err != nil {
		return err
	}
	c.SessionDirectory = realDestination
	if c.SessionFile != "" {
		c.SessionFile = filepath.Join(realDestination, filepath.Base(c.SessionFile))
	}
	return nil
}

// Attachments are the only generated subtree. Reject links on both sides and
// retain the source tree; arbitrary user directories are never traversed.
func copyAttachmentDirectory(source, destination string) error {
	resolved, err := filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(resolved), filepath.Clean(source)) {
		return errors.New("附件来源经过链接，原数据已保留")
	}
	if info, err := os.Lstat(destination); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("附件目标位置不是普通文件夹")
		}
	} else if !os.IsNotExist(err) {
		return err
	} else if err = os.Mkdir(destination, 0700); err != nil {
		return err
	}
	resolved, err = filepath.EvalSymlinks(destination)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(resolved), filepath.Clean(destination)) {
		return errors.New("附件目标经过链接，原数据已保留")
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		from, to := filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())
		info, err := os.Lstat(from)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("附件目录包含链接，原数据已保留")
		}
		if info.IsDir() {
			err = copyAttachmentDirectory(from, to)
		} else if info.Mode().IsRegular() {
			err = copyConversationFile(from, to)
		} else {
			return errors.New("附件目录包含非普通文件，原数据已保留")
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Only the live draft follows the new import directory. Pi's original JSONL
// and the retained source copy are byte-preserved for history and recovery.
func migrateDraftAttachmentPaths(source, destination string) error {
	file := filepath.Join(destination, "draft.json")
	raw, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(raw) > 128*1024 {
		return errors.New("Chat 草稿过大")
	}
	var draft map[string]json.RawMessage
	if err = json.Unmarshal(raw, &draft); err != nil {
		return err
	}
	var attachments []map[string]json.RawMessage
	if len(draft["attachments"]) == 0 {
		return nil
	}
	if err = json.Unmarshal(draft["attachments"], &attachments); err != nil {
		return err
	}
	if len(attachments) == 0 {
		return nil
	}
	oldRoot := filepath.Join(source, "attachments")
	newRoot := filepath.Join(destination, "attachments")
	for _, attachment := range attachments {
		var oldPath string
		if err = json.Unmarshal(attachment["path"], &oldPath); err != nil {
			return err
		}
		if !within(oldRoot, oldPath) {
			return errors.New("草稿引用了未登记的附件，原数据已保留")
		}
		relative, err := filepath.Rel(oldRoot, oldPath)
		if err != nil {
			return err
		}
		attachment["path"], err = json.Marshal(filepath.Join(newRoot, relative))
		if err != nil {
			return err
		}
	}
	draft["attachments"], err = json.Marshal(attachments)
	if err != nil {
		return err
	}
	raw, err = json.Marshal(draft)
	if err != nil {
		return err
	}
	return settings.WriteAtomic(file, raw)
}
func fileDigest(path string) ([32]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return [32]byte{}, err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err = io.Copy(digest, file); err != nil {
		return [32]byte{}, err
	}
	var result [32]byte
	copy(result[:], digest.Sum(nil))
	return result, nil
}
func copyConversationFile(source, destination string) error {
	if info, err := os.Lstat(destination); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("目标位置已有不同类型的文件")
		}
		old, err := fileDigest(source)
		if err != nil {
			return err
		}
		existing, err := fileDigest(destination)
		if err != nil {
			return err
		}
		if old != existing {
			return fmt.Errorf("目标位置已有不同数据，未覆盖：%s", filepath.Base(destination))
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	temp, err := os.CreateTemp(filepath.Dir(destination), ".winone-transfer-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name()) // Only the private temporary copy created above.
	if _, err = io.Copy(temp, input); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = finishConversationCopy(temp.Name(), destination); err != nil {
		return fmt.Errorf("无法完成会话文件转存：%w", err)
	}
	return nil
}
