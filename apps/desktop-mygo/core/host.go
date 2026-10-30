package core

import (
	"encoding/json"
	"time"

	"winone/desktopmygo/i18n"
)

// Names is a text in both languages, as the Host's manifests carry names.
type Names map[string]string

// String returns the name in the window's language.
func (n Names) String() string {
	if text := n[i18n.Locale()]; text != "" {
		return text
	}
	if text := n[i18n.English]; text != "" {
		return text
	}
	return n[i18n.Chinese]
}

// ModuleJob is the state of an operation a module or the Host runs in the
// background, as modules.setEnabled returns it.
type ModuleJob struct {
	ID      string          `json:"id"`
	State   string          `json:"state"`
	Error   string          `json:"error,omitempty"`
	Warning string          `json:"warning,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

// FollowJob polls namespace.jobs.get until job leaves "running", as the
// React pages did every 200 ms. It runs off the UI thread.
func FollowJob(env *Env, namespace string, job ModuleJob) (ModuleJob, error) {
	for job.State == "running" {
		if job.ID == "" {
			return job, errorText(i18n.T("module.unknownResult"))
		}
		time.Sleep(200 * time.Millisecond)
		id := job.ID
		if err := env.Host.CallCompletingInto(namespace+".jobs.get", map[string]string{"id": id}, &job); err != nil {
			return job, err
		}
		if job.ID != id {
			return job, errorText(i18n.T("module.unknownResult"))
		}
	}
	return job, nil
}

type errorText string

func (e errorText) Error() string { return string(e) }

// Errorf makes an error with the given text, for messages pages compose.
func Errorf(text string) error { return errorText(text) }
