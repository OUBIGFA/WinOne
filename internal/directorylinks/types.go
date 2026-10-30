// Package directorylinks creates and observes explicit, user-selected links.
// It owns only its history and entries created from a checked preview.
package directorylinks

type Request struct {
	Name      string `json:"name"`
	Source    string `json:"source"`
	Directory string `json:"directory"`
	LinkName  string `json:"linkName"`
	Kind      string `json:"kind"`
	Relative  bool   `json:"relative"`
}

// Identity uses NTFS's volume and file identifiers, never contents or path text.
type Identity struct {
	Volume   uint32 `json:"volume"`
	VolumeID string `json:"volumeId,omitempty"`
	Scope    string `json:"scope,omitempty"`
	File     string `json:"file"`
	Created  uint64 `json:"created"`
}

type Plan struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Source         string   `json:"source"`
	LinkPath       string   `json:"linkPath"`
	Kind           string   `json:"kind"`
	ObjectType     string   `json:"objectType"`
	TargetText     string   `json:"targetText"`
	Relative       bool     `json:"relative"`
	ExpiresAt      string   `json:"expiresAt"`
	SourceIdentity Identity `json:"sourceIdentity"`
	ParentIdentity Identity `json:"parentIdentity"`
}

type Record struct {
	Migration      *MigrationJournal `json:"migration,omitempty"`
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	LinkPath       string            `json:"linkPath"`
	TargetPath     string            `json:"targetPath"`
	Kind           string            `json:"kind"`
	ObjectType     string            `json:"objectType"`
	TargetText     string            `json:"targetText"`
	Relative       bool              `json:"relative"`
	CreatedAt      string            `json:"createdAt"`
	RegisteredAt   string            `json:"registeredAt,omitempty"`
	CheckedAt      string            `json:"checkedAt"`
	Status         string            `json:"status"`
	Reason         string            `json:"reason"`
	Detail         string            `json:"detail,omitempty"`
	Accessible     bool              `json:"accessible"`
	ActualType     string            `json:"actualType,omitempty"`
	ActualTarget   string            `json:"actualTarget,omitempty"`
	ActualRelative bool              `json:"actualRelative"`
	ActualTag      uint32            `json:"actualTag,omitempty"`
	ResolvedTarget string            `json:"resolvedTarget,omitempty"`
	LinkCount      uint32            `json:"linkCount,omitempty"`
	EntryIdentity  Identity          `json:"entryIdentity"`
	TargetIdentity Identity          `json:"targetIdentity"`
	ObservedEntry  Identity          `json:"observedEntry"`
	ObservedTarget Identity          `json:"observedTarget"`
}
