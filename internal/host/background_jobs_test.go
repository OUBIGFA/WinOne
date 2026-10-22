package host

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

type configurationHandlerFunc func(Request) Response

func (f configurationHandlerFunc) Handle(r Request) Response { return f(r) }

func configurationTicket(t *testing.T, response Response) string {
	t.Helper()
	var ticket struct {
		ID string `json:"configurationJob"`
	}
	if err := json.Unmarshal(response.Result, &ticket); response.Error != nil || err != nil || ticket.ID == "" {
		t.Fatalf("expected an admitted job, got %+v (%v)", response, err)
	}
	return ticket.ID
}

func waitConfigurationJob(t *testing.T, queue *ConfigurationQueue, response Response) ConfigurationJob {
	t.Helper()
	id := configurationTicket(t, response)
	params, _ := json.Marshal(map[string]string{"id": id})
	deadline := time.Now().Add(3 * time.Second)
	for {
		response := queue.Handle(Request{Version: 1, ID: "poll", Method: "configuration.jobs.get", Params: params})
		var job ConfigurationJob
		if err := json.Unmarshal(response.Result, &job); response.Error != nil || err != nil || job.ID != id {
			t.Fatalf("invalid job result: %+v (%v)", response, err)
		}
		if job.State != "running" {
			return job
		}
		if time.Now().After(deadline) {
			t.Fatal("job did not complete")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestDirectoryJobsCoexistWithAIAndKeepMaintenanceExclusive(t *testing.T) {
	// Exercise every filesystem entry: even reads may block on an unavailable
	// volume or behind this tool's history lock, but not on the global AI lease.
	methods := []string{"browse", "list", "preview", "previewBatch", "create", "create.elevated", "createBatch", "createBatch.elevated", "rename", "inspect", "inspectBatch", "scan", "remove", "forget", "forgetInvalid", "exportPlan", "reveal", "menu.set"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			entered, finish := make(chan Request, 1), make(chan struct{})
			var once sync.Once
			releaseJob := func() { once.Do(func() { close(finish) }) }
			defer releaseJob()
			queue := NewConfigurationQueue(configurationHandlerFunc(func(r Request) Response {
				entered <- r
				<-finish
				return Response{ID: r.ID, Result: json.RawMessage(`{"done":true}`)}
			}))
			ai, ok := queue.AcquireAction()
			if !ok {
				t.Fatal("AI lease was not admitted")
			}
			defer ai()
			response := queue.Handle(Request{Version: 1, ID: "directory", Method: "directory-links." + method, PeerPID: 123, Params: json.RawMessage(`{"token":"preview"}`)})
			configurationTicket(t, response)
			select {
			case request := <-entered:
				if request.PeerPID != 123 || string(request.Params) != `{"token":"preview"}` {
					t.Fatal("the async job lost its caller identity or payload", request)
				}
			case <-time.After(time.Second):
				t.Fatal("filesystem job waited for the AI stream")
			}
			if queue.Busy() {
				t.Fatal("ordinary filesystem work announced global maintenance")
			}
			if action, allowed := queue.AcquireAction(); !allowed {
				t.Fatal("Chat/tool action rejected while browsing files")
			} else {
				action()
			}
			if release, allowed := queue.AcquireMaintenance(); allowed {
				release()
				t.Fatal("maintenance overtook admitted work")
			}
			releaseJob()
			job := waitConfigurationJob(t, queue, response)
			if job.State != "completed" || job.Response == nil || job.Response.ID != "directory" || string(job.Response.Result) != `{"done":true}` {
				t.Fatal("original result was not delivered", job)
			}
		})
	}
}

func TestDirectoryJobLimitDoesNotBlockCancellationOrOtherModules(t *testing.T) {
	finish := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(finish) }) }
	defer release()
	queue := NewConfigurationQueue(configurationHandlerFunc(func(r Request) Response {
		if r.Method != "directory-links.scan.cancel" {
			<-finish
			return Response{ID: r.ID, Error: &RPCError{Code: "directory_links", Message: "scan cancelled"}}
		}
		return Response{ID: r.ID, Result: json.RawMessage(`{"requested":true}`)}
	}))
	var admitted []Response
	for i := 0; i < maxBackgroundJobs; i++ {
		response := queue.Handle(Request{Version: 1, ID: fmt.Sprint(i), Method: "directory-links.scan"})
		configurationTicket(t, response)
		admitted = append(admitted, response)
	}
	if response := queue.Handle(Request{Version: 1, ID: "excess", Method: "directory-links.browse"}); response.Error == nil || response.Error.Code != "action_busy" {
		t.Fatal("unbounded filesystem jobs admitted", response)
	}
	if response := queue.Handle(Request{Version: 1, ID: "cancel", Method: "directory-links.scan.cancel"}); string(response.Result) != `{"requested":true}` {
		t.Fatal("cancellation was queued behind slow I/O", response)
	}
	if action, allowed := queue.AcquireAction(); !allowed {
		t.Fatal("directory queue saturation disabled another module")
	} else {
		action()
	}
	release()
	for _, response := range admitted {
		job := waitConfigurationJob(t, queue, response)
		if job.State != "failed" || job.Response == nil || job.Response.Error == nil || job.Response.Error.Code != "directory_links" {
			t.Fatal("filesystem failure was not preserved", job)
		}
	}
	maintenance, allowed := queue.AcquireMaintenance()
	if !allowed {
		t.Fatal("failed jobs retained their leases")
	}
	defer maintenance()
	if response := queue.Handle(Request{Version: 1, ID: "blocked", Method: "directory-links.browse"}); response.Error == nil || response.Error.Code != "configuration_busy" {
		t.Fatal("filesystem work bypassed actual maintenance", response)
	}
}
