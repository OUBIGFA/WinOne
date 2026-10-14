//go:build windows

package directorylinks

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func batchFixture(t *testing.T) (*Manager, BatchRequest) {
	t.Helper()
	root := t.TempDir()
	input, output := filepath.Join(root, "sources"), filepath.Join(root, "links")
	os.Mkdir(input, 0700)
	os.Mkdir(output, 0700)
	sources := []string{filepath.Join(input, "one %!&.txt"), filepath.Join(input, "two.txt"), filepath.Join(input, "three.txt")}
	for _, path := range sources {
		if err := os.WriteFile(path, []byte("shared content"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := New(filepath.Join(root, "history"))
	if err != nil {
		t.Fatal(err)
	}
	return m, BatchRequest{Sources: sources, Directory: output, Kind: "hardlink"}
}

func TestBatchCreatesAllEntriesAndRetainsPerItemResults(t *testing.T) {
	m, request := batchFixture(t)
	plan, err := m.PreviewBatch(request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.CreateBatchWith(context.Background(), plan.ID, func(plans []Plan) ([]ExecutionResult, error) { return ExecuteBatch(plans), nil })
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 3 || result.Completed != 3 || len(result.Records) != 3 {
		t.Fatal(result)
	}
	for _, r := range result.Records {
		if r.Status != "active" || r.LinkCount < 2 {
			t.Fatalf("unverified batch entry: %+v", r)
		}
	}
}

func TestSingleBatchUsesCustomNameAndValidatesIt(t *testing.T) {
	m, request := batchFixture(t)
	request.Sources = request.Sources[:1]
	for _, name := range []string{"alias.txt", `..\outside.txt`, "two.txt"} {
		raw, _ := json.Marshal(map[string]any{"sources": request.Sources, "directory": request.Directory, "kind": request.Kind, "linkName": name})
		var named BatchRequest
		if err := json.Unmarshal(raw, &named); err != nil {
			t.Fatal(err)
		}
		if name == "two.txt" {
			if err := os.WriteFile(filepath.Join(request.Directory, name), []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		plan, err := m.PreviewBatch(named)
		if name != "alias.txt" {
			if err == nil {
				t.Fatalf("unsafe custom name %q accepted", name)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if plan.Items[0].LinkPath != filepath.Join(request.Directory, name) {
			t.Fatalf("custom name lost: %+v", plan.Items[0])
		}
		result, err := m.CreateBatchWith(context.Background(), plan.ID, func(plans []Plan) ([]ExecutionResult, error) { return ExecuteBatch(plans), nil })
		if err != nil || result.Completed != 1 {
			t.Fatalf("custom link was not created: %+v %v", result, err)
		}
	}
}

func TestBatchStopsAfterActualFailureAndReportsUnattemptedEntries(t *testing.T) {
	m, request := batchFixture(t)
	plan, err := m.PreviewBatch(request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.CreateBatchWith(context.Background(), plan.ID, func(plans []Plan) ([]ExecutionResult, error) {
		if err := os.Rename(request.Sources[1], request.Sources[1]+".gone"); err != nil {
			t.Fatal(err)
		}
		return ExecuteBatch(plans), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Completed != 1 || result.Records[1].Status != "failed" || result.Records[2].Reason != "not-attempted" {
		t.Fatal(result)
	}
	if _, err := os.Lstat(plan.Items[2].LinkPath); !os.IsNotExist(err) {
		t.Fatal("batch continued after failure")
	}
}

func TestExportedBatchQuotesRealPathsAndCreatesWorkingLinks(t *testing.T) {
	m, request := batchFixture(t)
	plan, err := m.PreviewBatch(request)
	if err != nil {
		t.Fatal(err)
	}
	path, err := m.ExportPlan(plan.ID, m.directory)
	if err == nil {
		t.Fatal("export into missing directory accepted", path)
	}
	path, err = m.ExportPlan(plan.ID, filepath.Dir(m.directory))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(raw), "one %%!&.txt") {
		t.Fatal("batch escaping missing", err)
	}
	command := exec.Command("cmd.exe", "/d", "/c", path)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("exported commands failed: %v %s", err, output)
	}
	for _, p := range plan.Items {
		source, err := openState(p.Source, true, false)
		if err != nil {
			t.Fatal(err)
		}
		link, err := openState(p.LinkPath, true, false)
		if err != nil {
			source.close()
			t.Fatal(err)
		}
		same := sameIdentity(source.identity, link.identity)
		source.close()
		link.close()
		if !same {
			t.Fatal("script copied or altered target instead of linking")
		}
	}
}

func TestScanDoesNotTraverseJunctionTargets(t *testing.T) {
	root := t.TempDir()
	scope := filepath.Join(root, "scope")
	outside := filepath.Join(root, "outside")
	os.Mkdir(scope, 0700)
	os.Mkdir(outside, 0700)
	source := filepath.Join(outside, "file.txt")
	os.WriteFile(source, []byte("keep"), 0600)
	os.Link(source, filepath.Join(outside, "hidden-alias.txt"))
	if err := createJunction(filepath.Join(scope, "junction"), outside); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(source, filepath.Join(scope, "visible-hardlink.txt")); err != nil {
		t.Fatal(err)
	}
	m, err := New(filepath.Join(root, "history"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Scan(context.Background(), scope, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 2 || result.Scanned != 2 || len(result.Issues) != 0 {
		t.Fatal(result)
	}
	for _, r := range result.Records {
		if r.Status != "active" || !strings.HasPrefix(r.LinkPath, scope) {
			t.Fatalf("wrong scan record: %+v", r)
		}
	}
}
