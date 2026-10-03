package distribution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"golang.org/x/mod/semver"
)

// UpgradePlan describes payload changes only. It does not establish publisher
// trust, validate the live installation, or authorize changing its files.
type UpgradePlan struct {
	PreviousVersion        string        `json:"previousVersion"`
	NextVersion            string        `json:"nextVersion"`
	PreviousManifestSHA256 string        `json:"previousManifestSha256"`
	NextManifestSHA256     string        `json:"nextManifestSha256"`
	Files                  []UpgradeFile `json:"files"`
}

type UpgradeFile struct {
	Path           string `json:"path"`
	Action         string `json:"action"`
	PreviousPath   string `json:"previousPath,omitempty"`
	PreviousSHA256 string `json:"previousSha256,omitempty"`
	NextSHA256     string `json:"nextSha256,omitempty"`
}

// CompareVersions uses Go's maintained SemVer implementation and restricts
// accepted spellings to the existing WinOne package-name contract.
func CompareVersions(previous, next string) (int, error) {
	for _, version := range []string{previous, next} {
		if !versionPattern.MatchString(version) || !semver.IsValid("v"+version) {
			return 0, errors.New("invalid upgrade version")
		}
	}
	return semver.Compare("v"+previous, "v"+next), nil
}

// PlanUpgrade verifies both archives before comparing their inventories. The
// executor must separately validate live files and recheck candidate contents.
func PlanUpgrade(previousPackage, nextPackage string) (UpgradePlan, error) {
	previous, err := Verify(previousPackage)
	if err != nil {
		return UpgradePlan{}, err
	}
	next, err := Verify(nextPackage)
	if err != nil {
		return UpgradePlan{}, err
	}
	order, err := CompareVersions(previous.Version, next.Version)
	if err != nil {
		return UpgradePlan{}, err
	}
	if order >= 0 {
		return UpgradePlan{}, errors.New("upgrade must target a newer version; same-version repair is a separate operation")
	}
	plan := UpgradePlan{PreviousVersion: previous.Version, NextVersion: next.Version, Files: []UpgradeFile{}}
	// Canonical manifest digests bind the plan to the verified inventory, not
	// to incidental JSON formatting or the surrounding ZIP compression.
	plan.PreviousManifestSHA256 = manifestFingerprint(previous)
	plan.NextManifestSHA256 = manifestFingerprint(next)
	old := map[string]File{}
	for _, file := range previous.Files {
		old[strings.ToLower(file.Path)] = file
	}
	for _, file := range next.Files {
		key := strings.ToLower(file.Path)
		change := UpgradeFile{Path: file.Path, Action: "add", NextSHA256: file.SHA256}
		if prior, ok := old[key]; ok {
			change.PreviousPath, change.PreviousSHA256 = prior.Path, prior.SHA256
			change.Action = "replace"
			if prior.Path == file.Path && prior.SHA256 == file.SHA256 && prior.Size == file.Size {
				change.Action = "unchanged"
			}
			delete(old, key)
		}
		plan.Files = append(plan.Files, change)
	}
	for _, file := range old {
		plan.Files = append(plan.Files, UpgradeFile{Path: file.Path, PreviousPath: file.Path, Action: "remove", PreviousSHA256: file.SHA256})
	}
	sort.Slice(plan.Files, func(i, j int) bool { return plan.Files[i].Path < plan.Files[j].Path })
	return plan, nil
}

func manifestFingerprint(manifest Manifest) string {
	manifest.Files = append([]File(nil), manifest.Files...)
	sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
	data, _ := json.Marshal(manifest)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
