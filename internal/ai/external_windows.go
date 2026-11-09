//go:build windows

package ai

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// StartExternal is called only by the native pages' explicit Read action. This
// separate process sees client homes; all WinOne settings and credentials still
// use its private XDG directories. No process-wide environment is ever changed.
func (t Translator) StartExternal(ctx context.Context, endpoint ...ExternalGatewayEndpoint) (*RoutingProcess, error) {
	if !filepath.IsAbs(t.Executable) || !filepath.IsAbs(t.DataDirectory) {
		return nil, errors.New("invalid external client runtime paths")
	}
	for _, dir := range []string{"config", "cache", "home"} {
		if err := os.MkdirAll(filepath.Join(t.DataDirectory, dir), 0700); err != nil {
			return nil, err
		}
	}
	cmd := exec.CommandContext(ctx, t.Executable)
	release, err := limitWorkerPrivilege(cmd)
	if err != nil {
		return nil, err
	}
	defer release()
	cmd.Env = externalEnvironment(t.DataDirectory, os.Environ(), t.RootDataDirectory)
	cmd.Dir = filepath.Dir(t.Executable)
	process, err := startOwnedProcess(ctx, cmd)
	if err != nil {
		return nil, err
	}
	key := rand.Text() + rand.Text()
	worker := &RoutingProcess{process: process, Key: key, stopAuxiliary: func() {}}
	started := make(chan []byte, 1)
	go func() {
		scanner := bufio.NewScanner(process.output)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		if scanner.Scan() {
			started <- append([]byte(nil), scanner.Bytes()...)
		} else {
			started <- nil
		}
		io.Copy(io.Discard, process.output)
		_ = process.finish()
	}()
	start := map[string]any{"operation": "external.serve", "key": key}
	if len(endpoint) > 0 {
		start["externalGateway"] = endpoint[0]
	}
	if err := json.NewEncoder(process.input).Encode(start); err != nil {
		worker.Close()
		return nil, err
	}
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	var raw []byte
	select {
	case raw = <-started:
	case <-ctx.Done():
		worker.Close()
		return nil, ctx.Err()
	case <-timer.C:
		worker.Close()
		return nil, errors.New("external client service start timed out")
	}
	var state struct{ URL, Error string }
	if json.Unmarshal(raw, &state) != nil || !strings.HasPrefix(state.URL, "http://127.0.0.1:") {
		worker.Close()
		return nil, errors.New("external client service returned invalid address")
	}
	worker.URL = state.URL
	return worker, nil
}

type ExternalGatewayEndpoint struct {
	URL string `json:"url"`
	Key string `json:"key"`
}

// Sync only the Host-owned endpoint. Installing/synchronizing a Library server
// remains an explicit user action; this method reads no external client files.
func (r *RoutingProcess) ExternalGateway(ctx context.Context, endpoint ExternalGatewayEndpoint) error {
	body, err := json.Marshal(endpoint)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL+"/api/winone/external/gateway", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+r.Key)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
		return errors.New(strings.TrimSpace(string(data)))
	}
	return nil
}

func externalEnvironment(directory string, identity []string, pluginRoots ...string) []string {
	env := isolatedEnvironment(directory)
	pluginRoot := directory
	if len(pluginRoots) > 0 && pluginRoots[0] != "" {
		pluginRoot = pluginRoots[0]
	}
	// Use only explicit OS path identities; inherited credentials and model
	// overrides stay stripped by isolatedEnvironment.
	for _, name := range externalClientPathVars {
		value := ""
		for _, entry := range identity {
			key, v, ok := strings.Cut(entry, "=")
			if ok && strings.EqualFold(key, name) {
				value = v
			}
		}
		filtered := env[:0]
		for _, entry := range env {
			key, _, _ := strings.Cut(entry, "=")
			if !strings.EqualFold(key, name) {
				filtered = append(filtered, entry)
			}
		}
		env = filtered
		if value != "" {
			env = append(env, name+"="+value)
		}
	}
	return append(env, "WINONE_AI_EXTERNAL_DATA="+filepath.Join(directory, "config", "magpie"), "WINONE_AI_PLUGINS_HOME="+filepath.Join(pluginRoot, "third-party"), "MAGPIE_NO_STATS=1")
}

// ExternalExport sends saved private collector credentials over the Host-only
// channel; a page capability cannot change an endpoint or start an exporter.
func (r *RoutingProcess) ExternalExport(ctx context.Context, prefs *MagpieSettings, on bool) error {
	body, err := json.Marshal(map[string]any{"magpieSettings": prefs, "on": on})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL+"/api/winone/external/settings", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+r.Key)
	request.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("external settings redirect refused") }}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
		return errors.New(strings.TrimSpace(string(body)))
	}
	return nil
}

type ExternalServiceState struct {
	Enabled, Exporting, Browser, Listening bool
	URL                                    string
}

func (r *RoutingProcess) ExternalState(ctx context.Context) (ExternalServiceState, error) {
	var state ExternalServiceState
	err := r.readAPI(ctx, "/api/winone/external/status", &state)
	return state, err
}

// ExternalEnabled reads only WinOne's own opt-in marker, never a client folder.
func (t Translator) ExternalEnabled() (bool, error) {
	data, err := os.ReadFile(filepath.Join(t.DataDirectory, "config", "magpie", "external-service.json"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var state struct{ Enabled bool }
	err = json.Unmarshal(data, &state)
	return state.Enabled, err
}

// Path/profile selectors read by the pinned upstream agentenv package. API
// credentials and cloud credential files are deliberately absent. This list is
// a client-location contract, independent of the ordinary worker's environment.
var externalClientPathVars = []string{
	"USERPROFILE", "HOME", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR",
	"CLAUDE_CONFIG_DIR", "CODEX_HOME", "COPILOT_HOME", "COPILOT_PROVIDERS_CONFIG", "CLAUDE_CODE_TMPDIR", "MAGPIE_ZED_BIN", "MAGPIE_ZED_CONFIG_DIR", "MAGPIE_ZED_PROCESS_NAMES", "CODEX_SQLITE_HOME", "GEMINI_CLI_HOME",
	"CLINE_DIR", "CLINE_DATA_DIR", "CLINE_SESSION_DATA_DIR", "CLINE_MCP_SETTINGS_PATH", "PI_CODING_AGENT_DIR", "PI_CODING_AGENT_SESSION_DIR", "PI_CONFIG_DIR", "PI_PROFILE", "OMO_CODING_AGENT_DIR", "SENPI_CODING_AGENT_DIR", "SLOCK_HOME", "PRIME_AGENT_CODING_AGENT_DIR", "STEP_CODING_AGENT_DIR", "STEPCODE_CONFIG_DIR", "MOTHX_DIR", "PENGUIN_HOME", "PENGUIN_PROJECT_ID", "OMP_PROFILE", "QODER_CONFIG_DIR", "QODERCN_CONFIG_DIR", "GROK_HOME", "GROK_BIN_DIR", "GROK_APP_HOME", "KIMI_CODE_HOME", "KIMI_SHARE_DIR", "QWEN_HOME", "ANTE_HOME", "NEWMAX_USER_DATA_DIR", "LIVEAGENT_HOME", "K_BRAIN_HOME", "OMB_DATA_DIR", "ASTRBOT_ROOT", "JCODE_HOME", "OPENCLAW_CONFIG_PATH", "OPENCLAW_STATE_DIR", "OPENCLAW_PROFILE", "OPENCLAW_HOME", "MIMOCODE_HOME", "MINIMAX_DATA_DIR", "HANA_HOME", "HERMES_HOME", "DSH_HOME", "WORKBUDDY_CONFIG_DIR", "CODEBUDDY_CONFIG_DIR", "DS_BOT_HOME", "DS_BOT_SHELL_DATA", "DEVECO_CONFIG_DIR", "ALMA_DATA_DIR", "REASONIX_HOME", "REASONIX_STATE_HOME", "PI_DESKTOP_DATA_DIR", "MISTER_MORPH_CONFIG", "T3CODE_HOME", "ATOMCODE_HOME", "SNOW_CONFIG_DIR", "QWENPAW_WORKING_DIR", "QWENPAW_SECRET_DIR", "CURSOR_CONFIG_DIR", "CURSOR_DATA_DIR", "OPENCODE_CONFIG_DIR", "OPENCODE_DB", "OPENCHAMBER_DATA_DIR", "KIROCREW_HOME", "CODEX_ACP_BIN", "CLAUDE_AGENT_ACP_BIN", "PI_ACP_BIN", "FACTORY_HOME_OVERRIDE", "NVM_DIR", "FNM_DIR", "MISE_DATA_DIR", "ASDF_DIR", "CARGO_HOME",
}
