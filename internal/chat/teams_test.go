package chat

import (
	"crypto/rand"
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

func TestOrcaWorkerIsOrdinaryAndKeepsItsOwnPermissionWithinAProject(t *testing.T) {
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.AddProject(root, "Project")
	if err != nil {
		t.Fatal(err)
	}
	lead, err := store.NewConversation(project.ID, "", "Lead", ai.Selection{Provider: "fixture", Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	lead, err = store.UpdateConversation(lead.ID, func(row *Conversation) error {
		row.OrcaRole = "lead"
		row.OrcaTeamID = "team"
		row.OrcaWorkerPermission = "auto"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared := Conversation{ID: rand.Text(), Directory: root, Title: "Worker · reviewer", Permission: "auto", ThinkingLevel: "high", Model: lead.Model, OrcaLeadID: lead.ID, OrcaTeamID: "team", OrcaWorkerID: "worker"}
	worker, err := store.NewWorkerConversation(prepared, lead)
	if err != nil {
		t.Fatal(err)
	}
	if worker.PluginID != "" || worker.OrcaRole != "worker" || worker.Permission != "auto" || worker.ProjectID != project.ID || worker.Revision != 1 {
		t.Fatalf("Worker was not atomically admitted as an ordinary task: %+v", worker)
	}
	if _, err = store.UpdateProjectPermission(project.ID, "bypassPermissions"); err != nil {
		t.Fatal(err)
	}
	worker, err = store.Conversation(worker.ID)
	if err != nil || worker.Permission != "auto" {
		t.Fatalf("Project changed Worker permission: %+v %v", worker, err)
	}
	reopened, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	read, err := reopened.Conversation(worker.ID)
	if err != nil || read.OrcaLeadID != lead.ID || read.OrcaTeamID != "team" || read.OrcaWorkerID != "worker" || read.PluginID != "" {
		t.Fatalf("Worker identity did not survive reopen: %+v %v", read, err)
	}
	if _, err = store.NewWorkerConversation(prepared, lead); err == nil {
		t.Fatal("an existing Worker was replaced")
	}
}
