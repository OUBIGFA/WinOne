package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
	"io"
	"os"
	"strings"
	"time"
)

type agentOption struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Backend string `json:"backend,omitempty"`
	Plugin  string `json:"plugin,omitempty"`
}

var agentOptions = []agentOption{
	{ID: provider.ChatGPTAPIID, Name: "ChatGPT API"},
	{ID: "codex", Name: "ChatGPT / Codex"}, {ID: "claude", Name: "Claude Code"}, {ID: "copilot", Name: "GitHub Copilot"}, {ID: "gemini", Name: "Gemini CLI"}, {ID: "antigravity", Name: "Antigravity"},
	{ID: "cursor", Name: "Cursor"}, {ID: "grok", Name: "Grok"}, {ID: "devin", Name: "Devin"}, {ID: "kiro", Name: "Kiro"}, {ID: "zed", Name: "Zed"}, {ID: "factory", Name: "Factory"},
	{ID: "workbuddy", Name: "WorkBuddy"}, {ID: provider.WorkBuddyAIID, Name: "WorkBuddy AI"}, {ID: provider.CommandCodePlanID, Name: "Command Code"},
	{ID: "qoder", Name: "Qoder"}, {ID: provider.QoderCNID, Name: "Qoder CN"}, {ID: provider.MiMoID, Name: "Xiaomi MiMo"}, {ID: "zcode", Name: "ZCode"},
}

func knownAgent(id string) bool {
	for _, agent := range agentOptions {
		if agent.ID == id {
			return true
		}
	}
	return false
}

func pluginAccount(in request) bool {
	_, bundled := pluginForAgent(in.Agent)
	return bundled || in.AgentBackend == "plugin"
}

func accountOperation(ctx context.Context, in request, input io.Reader) error {
	encode := json.NewEncoder(os.Stdout).Encode
	provider.WinOneProviders = nil
	if in.Operation == "accounts.key-balance" {
		return readKeyBalances(ctx, in, os.Stdout)
	}
	if in.Operation == "accounts.catalog" {
		presets := provider.Presets()
		// This worker is short-lived. Wait for the upstream dynamic catalog
		// instead of letting its background fetch die when the RPC exits.
		partners := provider.PartnersNow(3 * time.Second)
		for i := range presets {
			// Prefer the bundled official color variants in light mode; the
			// shared icon renderer supplies their monochrome dark-mode form.
			if presets[i].Icon == "kimi" || presets[i].Icon == "openrouter" {
				presets[i].Icon += "-color"
			}
		}
		agents, err := accountCatalogWithExtensions(ctx)
		if err != nil {
			return err
		}
		return encode(map[string]any{"agents": agents, "presets": presets, "partners": partners})
	}
	if pluginAccount(in) {
		if settings.Load().NoAccountDiscovery {
			return errors.New("WinOne subscription accounts are disabled in privacy settings")
		}
	} else if !knownAgent(in.Agent) {
		return errors.New("unsupported agent account")
	}
	switch in.Operation {
	case "accounts.checkin":
		if !pluginAccount(in) {
			return errors.New("this account does not support daily check-in")
		}
		if _, err := findAccount(ctx, in.Agent); err != nil {
			return err
		}
		results := provider.CheckInPlugins(ctx, provider.PluginID(in.Agent))
		// Vendor messages may contain raw response bodies. Public outcomes,
		// credited amounts and timestamps are sufficient for the settings UI.
		for i := range results {
			results[i].Msg = ""
		}
		return encode(map[string]any{"checkins": results})
	case "accounts.quota":
		// The Host scopes this process to one configured WinOne account profile.
		// Use the vendor quota reader, with no discovery of outside clients.
		if _, err := findAccount(ctx, in.Agent); err != nil {
			return err
		}
		agent := in.Agent
		if pluginAccount(in) {
			agent = provider.PluginID(agent)
		}
		// Opening/refreshing this page is an explicit read, including when the
		// upstream profile is configured to fetch allowances only on request.
		readings := provider.LoginUsage(provider.Asked(ctx), agent)
		type quotaView struct {
			provider.SubscriptionQuota
			History []provider.QuotaLine `json:"history"`
		}
		quotas := make([]quotaView, 0, len(readings))
		for user, q := range readings {
			q.User = user
			// Magpie keeps a last-known reading on a vendor error, with AsOf but
			// no Error. Preserve that history while marking the failed refresh;
			// the UI must never label the retained balance as a new live reading.
			if q.Error != "" || q.AsOf != nil {
				q.Error = "Account allowance could not be read"
			}
			history := []provider.QuotaLine{}
			for _, h := range provider.QuotaHistories(time.Now().Add(-45*24*time.Hour), q.Provider, user) {
				history = append(history, h.Lines...)
			}
			quotas = append(quotas, quotaView{SubscriptionQuota: q, History: history})
		}
		return encode(map[string]any{"quotas": quotas})
	case "accounts.list":
		if pluginAccount(in) {
			connection, err := pluginAccountSnapshot(ctx, in.Agent)
			if err != nil {
				return err
			}
			accounts := provider.WinOnePluginLogins(connection)
			for i := range accounts {
				accounts[i].Agent = in.Agent
			}
			available := false
			for _, p := range provider.PluginCheckins() {
				if p.ID == provider.PluginID(in.Agent) {
					available = true
					break
				}
			}
			return encode(map[string]any{"accounts": accounts, "methods": connection.Methods, "checkinAvailable": available})
		}
		return encode(map[string]any{"accounts": provider.Logins(in.Agent)})
	case "accounts.models":
		account, err := findAccount(ctx, in.Agent)
		if err != nil || account.Account == nil {
			return errors.New("sign in to the selected agent first")
		}
		models := account.Available()
		type model struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		var result []model
		for _, value := range models {
			result = append(result, model{value.ID, value.Name})
		}
		return encode(map[string]any{"models": result})
	case "accounts.signin":
		return signIn(ctx, in, input)
	default:
		return errors.New("unsupported account operation")
	}
}

// Read the upstream vendor balance using only the selected saved provider.
// IDs replace masked key fragments, and vendor error bodies never reach IPC.
func readKeyBalances(ctx context.Context, in request, out io.Writer) error {
	p, err := configuredProvider(in)
	if err != nil {
		return err
	}
	p.ID = in.ProviderID
	if p.ID == "" || len(in.BalanceKeys) > 128 {
		return errors.New("invalid balance provider")
	}
	if len(in.BalanceKeys) == 0 {
		return json.NewEncoder(out).Encode(map[string]any{"quotas": []provider.SubscriptionQuota{}})
	}
	p.Key, p.KeyName = in.BalanceKeys[0].Key, in.BalanceKeys[0].Name
	p.Keys = in.BalanceKeys[1:]
	provider.WinOneProviders = []provider.Provider{p}
	quotas := provider.KeyBalances(provider.Asked(ctx))
	for i := range quotas {
		q := &quotas[i]
		if q.Error != "" || q.AsOf != nil {
			q.Error = "Balance could not be read"
		}
		for j := range q.BalanceParts {
			if q.BalanceParts[j].Error != "" {
				q.BalanceParts[j].Error = "Balance component could not be read"
			}
		}
		for j := range q.BalanceSplit {
			if q.BalanceSplit[j].Error != "" {
				q.BalanceSplit[j].Error = "Balance component could not be read"
			}
		}
	}
	return json.NewEncoder(out).Encode(map[string]any{"quotas": quotas})
}

// One process owns the OAuth listener until completion/cancellation. The Host
// receives only public progress and supplies a pasted callback over stdin.
type signInControl struct {
	Callback string `json:"callback"`
	Cancel   bool   `json:"cancel"`
}

func readSignInControls(ctx context.Context, input io.Reader) <-chan signInControl {
	commands := make(chan signInControl)
	go func() {
		defer close(commands)
		scanner := bufio.NewScanner(input)
		scanner.Buffer(make([]byte, 4096), 64*1024)
		for scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) == "" {
				continue
			}
			var value signInControl
			if json.Unmarshal(scanner.Bytes(), &value) != nil {
				return
			}
			select {
			case commands <- value:
			case <-ctx.Done():
				return
			}
		}
	}()
	return commands
}

func signIn(ctx context.Context, in request, input io.Reader) error {
	commands := readSignInControls(ctx, input)
	var state provider.SignInState
	var err error
	if pluginAccount(in) {
		var complete bool
		state, complete, err = beginPluginSignIn(ctx, in, commands)
		if err != nil {
			return err
		}
		if complete {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"signin": state})
		}
	} else {
		state, err = provider.StartSignIn(in.Agent)
	}
	if err != nil {
		return err
	}
	defer provider.CancelSignIn(state.ID)
	encoder := json.NewEncoder(os.Stdout)
	last := ""
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		value, ok := provider.SignInStatus(state.ID)
		if !ok {
			return errors.New("sign-in session ended")
		}
		value = normalizeSignInState(value)
		data, _ := json.Marshal(value)
		if string(data) != last {
			if err := encoder.Encode(map[string]any{"signin": value}); err != nil {
				return err
			}
			last = string(data)
		}
		if value.State == "done" || value.State == "failed" || value.State == "canceled" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case command, ok := <-commands:
			if !ok || command.Cancel {
				provider.CancelSignIn(state.ID)
				continue
			}
			if command.Callback != "" {
				if err := provider.SubmitSignInCallback(state.ID, command.Callback); err != nil {
					_ = encoder.Encode(map[string]string{"inputError": "The callback was not accepted. Check the complete address or code and try again."})
				}
			}
		case <-ticker.C:
		}
	}
}
