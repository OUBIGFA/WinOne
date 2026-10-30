package directorylinks

// MigrationRequest either selects a new ordinary directory, or an existing
// verified migration record to restore. Paths never come from a backup import.
type MigrationRequest struct {
	RequestID string `json:"requestId,omitempty"`
	Source    string `json:"source"`
	Directory string `json:"directory"`
	Name      string `json:"name"`
	RecordID  string `json:"recordId,omitempty"`
	CheckedAt string `json:"checkedAt,omitempty"`
}
type MigrationPlan struct {
	ID                        string   `json:"id"`
	Operation                 string   `json:"operation"`
	Source                    string   `json:"source"`
	Destination               string   `json:"destination"`
	RecordID                  string   `json:"recordId,omitempty"`
	ExpiresAt                 string   `json:"expiresAt"`
	Files                     int      `json:"files"`
	Bytes                     int64    `json:"bytes"`
	Warnings                  []string `json:"warnings"`
	SourceIdentity            Identity `json:"sourceIdentity"`
	SourceParentIdentity      Identity `json:"sourceParentIdentity"`
	DestinationParentIdentity Identity `json:"destinationParentIdentity"`
	LinkIdentity              Identity `json:"linkIdentity,omitempty"`
}
type MigrationProgress struct {
	Phase      string `json:"phase"`
	Files      int    `json:"files"`
	TotalFiles int    `json:"totalFiles"`
	Bytes      int64  `json:"bytes"`
	TotalBytes int64  `json:"totalBytes"`
}
type MigrationArtifact struct {
	Path     string   `json:"path"`
	Identity Identity `json:"identity"`
	Kind     string   `json:"kind"`
}

// The journal describes recovery, not permission to delete a path. Reopening a
// history never performs filesystem recovery without explicit confirmation.
type MigrationJournal struct {
	Previous         *MigrationJournal   `json:"previous,omitempty"`
	Operation        string              `json:"operation"`
	Phase            string              `json:"phase"`
	Source           string              `json:"source"`
	Destination      string              `json:"destination"`
	Staging          string              `json:"staging"`
	Backup           string              `json:"backup"`
	SourceIdentity   Identity            `json:"sourceIdentity"`
	OriginalIdentity Identity            `json:"originalIdentity"`
	StagingIdentity  Identity            `json:"stagingIdentity"`
	Digest           string              `json:"digest,omitempty"`
	Bytes            int64               `json:"bytes"`
	Files            int                 `json:"files"`
	Message          string              `json:"message,omitempty"`
	Retained         []MigrationArtifact `json:"retained,omitempty"`
}
