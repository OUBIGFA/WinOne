package interconnect

import (
	"encoding/json"
	"strings"
)

// The transfer services' task records (interconnect.transfer_list). Their
// specs and states are the workers' own documents: only the fields the
// page shows are declared, and specs keep everything for reuse.

type progressBar struct {
	CurrentNum  float64 `json:"CurrentNum"`
	Max         float64 `json:"Max"`
	Description string  `json:"Description"`
	KBsPerSecond float64 `json:"KBsPerSecond"`
}

type prompt struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

// codeTransfer is a share's embedded croc code sender.
type codeTransfer struct {
	Status     string `json:"status"`
	Code       string `json:"code,omitempty"`
	QR         string `json:"qr,omitempty"`
	BrowserURL string `json:"browserUrl,omitempty"`
	Error      string `json:"error,omitempty"`
	Detail     string `json:"detail,omitempty"`
	Output     string `json:"output,omitempty"`
	Prompt     *prompt `json:"prompt,omitempty"`
	Progress   *struct {
		Progress *progressBar `json:"Progress,omitempty"`
	} `json:"progress,omitempty"`
}

type shareAddress struct {
	Name string `json:"name,omitempty"`
	IP   string `json:"ip"`
	URL  string `json:"url"`
	QR   string `json:"qr"`
}

type shareItem struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Content string  `json:"content,omitempty"`
	Size    float64 `json:"size"`
	Exists  bool    `json:"exists"`
}

type shareState struct {
	Info *struct {
		URL       string         `json:"url"`
		QR        string         `json:"qr"`
		Addresses []shareAddress `json:"addresses"`
	} `json:"info,omitempty"`
	Items     []shareItem `json:"items"`
	Downloads []struct {
		ItemID      string  `json:"itemId"`
		SpeedBps    float64 `json:"speedBps"`
		ActiveCount int     `json:"activeCount"`
	} `json:"downloads,omitempty"`
}

type taskResult struct {
	Service   string `json:"service,omitempty"`
	ID        string `json:"id,omitempty"`
	Code      string `json:"code,omitempty"`
	Link      string `json:"link,omitempty"`
	QR        string `json:"qr,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
	Downloads int    `json:"downloads,omitempty"`
	Revoked   bool   `json:"revoked,omitempty"`
	TextPath  string `json:"textPath,omitempty"`
	Manifest  *struct {
		F []struct {
			N string  `json:"n"`
			S float64 `json:"s"`
		} `json:"f"`
	} `json:"manifest,omitempty"`
}

type taskState struct {
	Status            string        `json:"status"`
	Ready             bool          `json:"ready,omitempty"`
	RelayReady        bool          `json:"relayReady,omitempty"`
	RelayError        string        `json:"relayError,omitempty"`
	Code              string        `json:"code,omitempty"`
	QR                string        `json:"qr,omitempty"`
	Address           string        `json:"address,omitempty"`
	Output            string        `json:"output,omitempty"`
	Detail            string        `json:"detail,omitempty"`
	Error             string        `json:"error,omitempty"`
	PersistenceError  string        `json:"persistenceError,omitempty"`
	CleanupError      string        `json:"cleanupError,omitempty"`
	NotificationError string        `json:"notificationError,omitempty"`
	Updating          bool          `json:"updating,omitempty"`
	Sent              float64       `json:"sent,omitempty"`
	Received          float64       `json:"received,omitempty"`
	Active            int           `json:"active,omitempty"`
	Prompt            *prompt       `json:"prompt,omitempty"`
	Transfer          *codeTransfer `json:"transfer,omitempty"`
	Share             *shareState   `json:"share,omitempty"`
	Peers             []struct {
		Direct   bool    `json:"direct"`
		Relay    string  `json:"relay,omitempty"`
		Sent     float64 `json:"sent"`
		Received float64 `json:"received"`
	} `json:"peers,omitempty"`
	Progress *struct {
		Progress *progressBar `json:"Progress,omitempty"`
	} `json:"progress,omitempty"`
	StoreProgress *struct {
		TotalBytes float64 `json:"TotalBytes"`
		TotalSize  float64 `json:"TotalSize"`
		FileName   string  `json:"FileName"`
	} `json:"storeProgress,omitempty"`
	Result *taskResult `json:"result,omitempty"`
}

// taskSpec is a task's request; raw keeps it whole for "use again".
type taskSpec struct {
	Mode        string   `json:"mode"`
	Paths       []string `json:"paths,omitempty"`
	Text        string   `json:"text,omitempty"`
	Code        string   `json:"code,omitempty"`
	OutputDir   string   `json:"outputDir,omitempty"`
	StoreOrigin string   `json:"storeOrigin,omitempty"`
	Target      string   `json:"target,omitempty"`
	raw         map[string]any
}

func (s *taskSpec) UnmarshalJSON(data []byte) error {
	type plain taskSpec
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*s = taskSpec(value)
	return json.Unmarshal(data, &s.raw)
}

type taskRecord struct {
	ID      string    `json:"id"`
	Created string    `json:"created"`
	Spec    taskSpec  `json:"spec"`
	State   taskState `json:"state"`
}

// title is a task's name: its files, or what it does.
func (r taskRecord) title(fallback string) string {
	var names []string
	for _, path := range r.Spec.Paths {
		parts := strings.FieldsFunc(path, func(c rune) bool { return c == '\\' || c == '/' })
		if len(parts) > 0 {
			names = append(names, parts[len(parts)-1])
		}
	}
	if len(names) > 0 {
		return strings.Join(names, " · ")
	}
	return fallback
}
