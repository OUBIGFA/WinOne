package worker

import (
	"strings"
	"testing"
	"winone/modules/wintray/internal/orchestrator"
)

func TestFailedStartupEntryIsNotQueueSuccess(t *testing.T) {
	results := []orchestrator.Result{{AppName: "已运行", Managed: true, Code: orchestrator.ResultAlreadyRunningSkipped}, {AppName: "次数限制", Managed: true, Code: orchestrator.ResultFrequencyLimit}}
	if err := startupResultError("zh-CN", results); err != nil {
		t.Fatal(err)
	}
	results = append(results, orchestrator.Result{AppName: "缺失程序", Code: orchestrator.ResultInvalidExePath, Message: "invalid exe path"})
	if err := startupResultError("zh-CN", results); err == nil || !strings.Contains(err.Error(), "缺失程序") {
		t.Fatalf("failure lost: %v", err)
	}
}
