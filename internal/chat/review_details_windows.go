//go:build windows

package chat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type ReviewBranchCandidate struct {
	RefName   string `json:"refName"`
	ShortName string `json:"shortName"`
	Kind      string `json:"kind"`
	Remote    string `json:"remote,omitempty"`
	OID       string `json:"oid"`
	Default   bool   `json:"isDefaultBranch,omitempty"`
}
type ReviewOperation struct {
	Kind   string `json:"kind"`
	Marker string `json:"marker"`
	Path   string `json:"path"`
}
type ReviewMetadata struct {
	Repository bool                    `json:"repository"`
	Root       string                  `json:"root"`
	Branch     string                  `json:"branch"`
	HeadOID    string                  `json:"headOid"`
	Detached   bool                    `json:"detached"`
	Unborn     bool                    `json:"unborn"`
	Upstream   string                  `json:"upstream"`
	Ahead      int                     `json:"ahead"`
	Behind     int                     `json:"behind"`
	Candidates []ReviewBranchCandidate `json:"candidates"`
	InProgress []ReviewOperation       `json:"inProgress"`
}

// Repository identity and local tracking refs are read without fetch or checkout.
// An unset upstream is represented explicitly; zero counts are never a network
// claim about a remote that has not been fetched.
func ReadReviewMetadata(ctx context.Context, directory string) (ReviewMetadata, error) {
	state, err := ReadGitState(ctx, directory)
	if err != nil {
		return ReviewMetadata{}, err
	}
	result := ReviewMetadata{Repository: state.Repository, Branch: state.Branch, Detached: state.Detached, Unborn: state.Unborn, Candidates: []ReviewBranchCandidate{}, InProgress: []ReviewOperation{}}
	if !state.Repository {
		return result, nil
	}
	result.Root, err = reviewRoot(ctx, directory)
	if err != nil {
		return result, err
	}
	if !state.Unborn {
		result.HeadOID, err = reviewRef(ctx, result.Root, "HEAD")
		if err != nil {
			return result, err
		}
	}
	raw, err := reviewGit(ctx, result.Root, "", "for-each-ref", "--format=%(refname)%00%(refname:short)%00%(objectname)%00%(upstream:short)%00%(symref)", "refs/heads/", "refs/remotes/")
	if err != nil {
		return result, err
	}
	defaultRef := ""
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		fields := strings.Split(line, "\x00")
		if len(fields) != 5 {
			if line == "" {
				continue
			}
			return result, errors.New("Git 分支信息不完整")
		}
		if fields[4] != "" {
			if strings.HasSuffix(fields[0], "/HEAD") {
				defaultRef = strings.TrimPrefix(fields[4], "refs/remotes/")
			}
			continue
		}
		candidate := ReviewBranchCandidate{RefName: fields[1], ShortName: fields[1], OID: fields[2], Kind: "local"}
		if strings.HasPrefix(fields[0], "refs/remotes/") {
			candidate.Kind = "remote"
			candidate.Remote, _, _ = strings.Cut(strings.TrimPrefix(fields[0], "refs/remotes/"), "/")
		}
		result.Candidates = append(result.Candidates, candidate)
		if fields[0] == "refs/heads/"+state.Branch {
			result.Upstream = fields[3]
		}
	}
	if defaultRef == "" {
		for _, name := range []string{"main", "master"} {
			for _, candidate := range result.Candidates {
				if candidate.RefName == name {
					defaultRef = name
					break
				}
			}
			if defaultRef != "" {
				break
			}
		}
	}
	for i := range result.Candidates {
		candidate := &result.Candidates[i]
		candidate.Default = candidate.RefName == defaultRef
		if candidate.RefName == result.Upstream {
			candidate.Kind = "upstream"
		} else if candidate.Default && candidate.Remote != "" {
			candidate.Kind = "remote-default"
		}
	}
	if result.Upstream != "" && !state.Unborn {
		raw, err = reviewGit(ctx, result.Root, "", "rev-list", "--left-right", "--count", "HEAD..."+result.Upstream, "--")
		if err != nil {
			return result, err
		}
		if _, err = fmt.Sscanf(raw, "%d %d", &result.Ahead, &result.Behind); err != nil {
			return result, err
		}
	}
	for marker, kind := range map[string]string{"MERGE_HEAD": "merge", "rebase-merge": "rebase", "rebase-apply": "rebase", "CHERRY_PICK_HEAD": "cherry-pick", "SQUASH_MSG": "squash"} {
		raw, err = reviewGit(ctx, result.Root, "", "rev-parse", "--git-path", marker)
		if err != nil {
			return result, err
		}
		path := strings.TrimSpace(raw)
		if !filepath.IsAbs(path) {
			path = filepath.Join(result.Root, path)
		}
		if _, err = os.Stat(path); err == nil {
			result.InProgress = append(result.InProgress, ReviewOperation{kind, marker, path})
		} else if !errors.Is(err, os.ErrNotExist) {
			return result, err
		}
	}
	return result, nil
}

func reviewFileStatistics(ctx context.Context, root string, args []string, files []ReviewFile) error {
	raw, err := reviewGit(ctx, root, "", append(args, "--numstat", "-z", "--")...)
	if err != nil {
		return err
	}
	byPath := map[string]*ReviewFile{}
	for i := range files {
		byPath[files[i].Path] = &files[i]
	}
	fields := strings.Split(raw, "\x00")
	for i := 0; i < len(fields) && fields[i] != ""; i++ {
		row := strings.SplitN(fields[i], "\t", 3)
		if len(row) != 3 {
			return errors.New("Git 差异统计不完整")
		}
		name := row[2]
		if name == "" {
			if i+2 >= len(fields) {
				return errors.New("Git 重命名统计不完整")
			}
			name = fields[i+2]
			i += 2
		}
		file := byPath[name]
		if file == nil {
			continue
		}
		file.Binary = row[0] == "-" || row[1] == "-"
		if !file.Binary {
			file.Additions, err = strconv.Atoi(row[0])
			if err != nil {
				return err
			}
			file.Deletions, err = strconv.Atoi(row[1])
			if err != nil {
				return err
			}
		}
	}
	for i := range files {
		file := &files[i]
		path := filepath.Join(root, filepath.FromSlash(file.Path))
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		file.Size = info.Size()
		if !file.Untracked || !info.Mode().IsRegular() {
			continue
		}
		stream, err := os.Open(path)
		if err != nil {
			return err
		}
		counter := &reviewLineCounter{}
		_, err = io.Copy(counter, stream)
		stream.Close()
		if err != nil {
			return err
		}
		file.Binary = counter.binary
		file.Additions = counter.lines
		if counter.size > 0 && counter.last != '\n' {
			file.Additions++
		}
		if file.Binary {
			file.Additions = 0
		}
	}
	return nil
}

type reviewLineCounter struct {
	lines  int
	size   int64
	last   byte
	binary bool
}

func (c *reviewLineCounter) Write(p []byte) (int, error) {
	c.lines += bytes.Count(p, []byte{'\n'})
	c.size += int64(len(p))
	if len(p) > 0 {
		c.last = p[len(p)-1]
	}
	c.binary = c.binary || bytes.IndexByte(p, 0) >= 0
	return len(p), nil
}

func reviewFileIdentity(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("仅能将普通未跟踪文件移入回收站")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err = io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func (snapshot ReviewSnapshot) RecordedFileVersion(path, side string) ([]byte, bool, error) {
	if side != "old" && side != "new" {
		return nil, false, errors.New("请选择差异的旧版或新版")
	}
	capture := snapshot.captured[path]
	if capture == nil || !capture.Valid {
		return nil, false, errors.New("该次修改未记录完整文件快照，可查看差异或打开当前文件")
	}
	if side == "old" {
		return []byte(capture.BeforeText), capture.BeforeExists, nil
	}
	return []byte(capture.AfterText), true, nil
}

// ReadReviewVersion reads a side of a listed change. A deleted/added side is
// absent, while invalid refs, paths and missing expected objects are errors.
func ReadReviewVersion(ctx context.Context, snapshot ReviewSnapshot, path, side string) ([]byte, bool, error) {
	if side != "old" && side != "new" {
		return nil, false, errors.New("请选择差异的旧版或新版")
	}
	var chosen *ReviewFile
	for i := range snapshot.Files {
		if snapshot.Files[i].Path == path {
			chosen = &snapshot.Files[i]
			break
		}
	}
	if chosen == nil {
		return nil, false, errors.New("此文件不在当前差异中")
	}
	if side == "old" && (chosen.Untracked || strings.HasPrefix(chosen.Status, "A")) {
		return nil, false, nil
	}
	if side == "new" && strings.HasPrefix(chosen.Status, "D") {
		return nil, false, nil
	}
	if side == "old" && chosen.OldPath != "" {
		path = chosen.OldPath
	}
	path, err := reviewPath(path)
	if err != nil {
		return nil, false, err
	}
	var ref string
	switch snapshot.Scope {
	case "working":
		if side == "new" {
			absolute := filepath.Join(snapshot.Root, filepath.FromSlash(path))
			real, err := filepath.EvalSymlinks(absolute)
			if err != nil {
				return nil, false, err
			}
			if !within(snapshot.Root, real) {
				return nil, false, errors.New("文件链接超出仓库")
			}
			info, err := os.Stat(real)
			if err != nil {
				return nil, false, err
			}
			if info.Size() > 16<<20 {
				return nil, false, errors.New("文件超过 16 MB，请使用系统查看")
			}
			data, err := os.ReadFile(real)
			return data, true, err
		}
		ref = ""
	case "staged":
		if side == "old" {
			state, err := ReadGitState(ctx, snapshot.Root)
			if err != nil {
				return nil, false, err
			}
			if state.Unborn {
				return nil, false, nil
			}
			ref = "HEAD"
		}
	case "branch":
		ref = "HEAD"
		if side == "old" {
			base, err := reviewRef(ctx, snapshot.Root, snapshot.Base)
			if err != nil {
				return nil, false, err
			}
			merge, err := reviewGit(ctx, snapshot.Root, "", "merge-base", base, "HEAD")
			if err != nil {
				return nil, false, err
			}
			ref = strings.TrimSpace(merge)
		}
	case "commit":
		ref, err = reviewRef(ctx, snapshot.Root, snapshot.Base)
		if err != nil {
			return nil, false, err
		}
		if side == "old" {
			parents, err := reviewGit(ctx, snapshot.Root, "", "rev-list", "--parents", "-n", "1", ref)
			if err != nil {
				return nil, false, err
			}
			ids := strings.Fields(parents)
			if len(ids) < 2 {
				return nil, false, nil
			}
			ref = ids[1]
		}
	default:
		return nil, false, errors.New("此来源没有 Git 文件版本")
	}
	data, err := reviewGit(ctx, snapshot.Root, "", "show", "--no-ext-diff", "--no-textconv", ref+":"+path)
	return []byte(data), true, err
}
