package chat

import (
	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/widgets"
)

// Project is a folder conversations work in.
type Project struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Path       string           `json:"path"`
	Permission model.Permission `json:"permission,omitempty"`
}

// Conversation is one conversation of the catalog.
type Conversation struct {
	ID               string              `json:"id"`
	ProjectID        string              `json:"projectId,omitempty"`
	Title            string              `json:"title"`
	Directory        string              `json:"directory"`
	SessionDirectory string              `json:"sessionDirectory"`
	SessionFile      string              `json:"sessionFile,omitempty"`
	Model            widgets.AISelection `json:"model"`
	Permission       model.Permission    `json:"permission"`
	ThinkingLevel    string              `json:"thinkingLevel,omitempty"`
	FastMode         bool                `json:"fastMode,omitempty"`
	Archived         bool                `json:"archived,omitempty"`
	Pinned           bool                `json:"pinned,omitempty"`
	HasMessages      bool                `json:"hasMessages,omitempty"`
	CreatedAt        string              `json:"createdAt"`
	UpdatedAt        string              `json:"updatedAt"`
}

// Catalog is chat.catalog: every project and conversation.
type Catalog struct {
	Projects      []Project      `json:"projects"`
	Conversations []Conversation `json:"conversations"`
}

func (c *Catalog) conversation(id string) *Conversation {
	for i := range c.Conversations {
		if c.Conversations[i].ID == id {
			return &c.Conversations[i]
		}
	}
	return nil
}

func (c *Catalog) project(id string) *Project {
	for i := range c.Projects {
		if c.Projects[i].ID == id {
			return &c.Projects[i]
		}
	}
	return nil
}

// replace puts an updated conversation in place of its old record.
func (c *Catalog) replace(updated Conversation) {
	if old := c.conversation(updated.ID); old != nil {
		*old = updated
	}
}

// Activity is the sidebar's lightweight state of a conversation, as
// chat.activity reports it for every conversation at once.
type Activity struct {
	Phase     string `json:"phase"` // idle, running, interrupted, needs-interaction, completed, error
	Attention bool   `json:"attention"`
	Paused    bool   `json:"paused,omitempty"`
}

// statusKind is the sidebar's status of a conversation, as Cindy's
// resolveSidebarRightStatus projects it: "time" shows nothing.
func statusKind(a Activity) string {
	switch {
	case a.Phase == "error" && a.Attention:
		return "error"
	case a.Paused:
		return "paused"
	case a.Phase == "needs-interaction":
		return "awaiting"
	case a.Phase == "running":
		return "running"
	case a.Phase == "interrupted":
		return "interrupted"
	case a.Phase == "completed" && a.Attention:
		return "done"
	}
	return "time"
}

// AIConfiguration is what Chat reads of capture.ai.configuration.get.
type AIConfiguration struct {
	Providers []widgets.AIProvider            `json:"providers"`
	Tasks     map[string]*widgets.AISelection `json:"tasks"`
	// ShowReasoning nil shows thinking, as React's showReasoning!==false.
	ShowReasoning *bool `json:"showReasoning,omitempty"`
}

// reasoning reports whether thinking is shown.
func (a *AIConfiguration) reasoning() bool {
	return a == nil || a.ShowReasoning == nil || *a.ShowReasoning
}
