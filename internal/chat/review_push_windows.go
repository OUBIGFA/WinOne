//go:build windows

package chat

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type ReviewPushLease struct {
	RemoteRef   string `json:"remoteRef"`
	ExpectedOID string `json:"expectedOid"`
}
type ReviewPushState struct {
	Kind        string         `json:"kind"`
	Remote      string         `json:"remote"`
	RemoteRef   string         `json:"remoteRef"`
	RemoteOID   string         `json:"remoteOid,omitempty"`
	Ahead       int            `json:"ahead"`
	Behind      int            `json:"behind"`
	Upstream    string         `json:"upstream"`
	Stdout      string         `json:"stdout"`
	Stderr      string         `json:"stderr"`
	AheadBehind map[string]any `json:"aheadBehind"`
}

var reviewObjectID = regexp.MustCompile(`^(?:[a-fA-F0-9]{40}|[a-fA-F0-9]{64})$`)

// A normal push never becomes a force push on its own. After a rejection the
// UI can ask about the exact remote revision; confirmation uses Git's lease so
// a remote changed after the dialog cannot be overwritten.
func PushReview(ctx context.Context, directory string, lease *ReviewPushLease) (ReviewPushState, error) {
	meta, err := ReadReviewMetadata(ctx, directory)
	if err != nil {
		return ReviewPushState{}, err
	}
	if !meta.Repository || meta.Unborn || meta.Detached {
		return ReviewPushState{}, errors.New("请先选择有提交的本地分支")
	}
	raw, err := reviewGit(ctx, meta.Root, "", "remote")
	if err != nil {
		return ReviewPushState{}, err
	}
	remotes := strings.Fields(raw)
	if len(remotes) == 0 {
		return ReviewPushState{}, errors.New("仓库没有配置远端")
	}
	remote := remotes[0]
	for _, name := range remotes {
		if name == "origin" {
			remote = name
		}
		if strings.HasPrefix(meta.Upstream, name+"/") {
			remote = name
			break
		}
	}
	target := "refs/heads/" + meta.Branch
	if strings.HasPrefix(meta.Upstream, remote+"/") {
		target = "refs/heads/" + strings.TrimPrefix(meta.Upstream, remote+"/")
	}
	result := ReviewPushState{Remote: remote, RemoteRef: target, Upstream: meta.Upstream}
	args := []string{"push", "--porcelain"}
	if meta.Upstream == "" {
		args = append(args, "--set-upstream")
	}
	if lease != nil {
		if lease.RemoteRef != target || !reviewObjectID.MatchString(lease.ExpectedOID) {
			return result, errors.New("推送确认对应的远端或版本无效，请重新检查")
		}
		args = append(args, "--force-with-lease="+target+":"+lease.ExpectedOID)
	}
	result.Stdout, err = reviewGit(ctx, meta.Root, "", append(args, "--", remote, "HEAD:"+target)...)
	if err == nil {
		result.Kind = "pushed"
		meta, e := ReadReviewMetadata(ctx, meta.Root)
		if e != nil {
			return result, e
		}
		result.AheadBehind = map[string]any{"ahead": meta.Ahead, "behind": meta.Behind, "upstream": meta.Upstream, "stale": false}
		return result, nil
	}
	if lease != nil {
		return result, fmt.Errorf("远端分支已变化或拒绝推送，请重新检查；没有覆盖远端：%w", err)
	}
	detail := err.Error()
	if !strings.Contains(result.Stdout, "[rejected]") && !strings.Contains(detail, "non-fast-forward") && !strings.Contains(detail, "fetch first") {
		return result, err
	}
	result.Stderr = detail
	// Fetch only the rejected branch after this explicit Push action. It leaves
	// files and the local branch untouched and provides real divergence counts.
	if _, err = reviewGit(ctx, meta.Root, "", "fetch", "--no-tags", "--", remote, target); err != nil {
		return result, err
	}
	result.RemoteOID, err = reviewRef(ctx, meta.Root, "FETCH_HEAD")
	if err != nil {
		return result, err
	}
	counts, err := reviewGit(ctx, meta.Root, "", "rev-list", "--left-right", "--count", "HEAD..."+result.RemoteOID, "--")
	if err != nil {
		return result, err
	}
	if _, err = fmt.Sscanf(counts, "%d %d", &result.Ahead, &result.Behind); err != nil {
		return result, err
	}
	result.Kind = "needs-force"
	return result, nil
}
