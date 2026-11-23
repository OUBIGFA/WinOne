package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Run against the packaged shared executable. The identity/snapshot commands
// validate dispatch and EOF cleanup without opening a network listener.
func TestPackagedRoleDispatch(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(filepath.Dir(self), "WinOne.Interconnect.Worker.exe")
	if _, err := os.Stat(executable); err != nil {
		t.Skip("run through build-interconnect-worker.ps1 -Test")
	}
	for _, role := range []string{"peer", "transfer"} {
		t.Run(role, func(t *testing.T) {
			command := exec.Command(executable, role)
			command.Stdin = bytes.NewBufferString("{\"id\":\"probe\",\"method\":\"identity\"}\n")
			output, err := command.Output()
			if err != nil {
				t.Fatal(err)
			}
			var reply struct {
				ID     string          `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if err := json.Unmarshal(output, &reply); err != nil {
				t.Fatal(err)
			}
			if reply.ID != "probe" {
				t.Fatalf("response identity changed: %s", output)
			}
			if role == "peer" && !bytes.Contains(reply.Result, []byte("psk")) {
				t.Fatalf("peer role not dispatched: %s", output)
			}
			if role == "transfer" && len(reply.Error) == 0 {
				t.Fatalf("transfer role accepted peer-only command: %s", output)
			}
		})
	}
	if err := exec.Command(executable, "invalid-role").Run(); err == nil {
		t.Fatal("invalid role accepted")
	}
}
