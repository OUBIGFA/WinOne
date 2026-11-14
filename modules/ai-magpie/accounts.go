package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/yetone/magpie/internal/provider"
	"io"
	"os"
	"path/filepath"
	"strconv"
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
		agents := accountCatalog()
		return encode(map[string]any{"agents": agents, "presets": presets, "partners": partners})
	}
	if !pluginAccount(in) && !knownAgent(in.Agent) {
		return errors.New("unsupported agent account")
	}
	if in.Operation != "accounts.signin" && in.Operation != "accounts.list" && in.Operation != "accounts.import" {
		if err := prepareAccountPreferences(ctx, in); err != nil {
			return err
		}
	}
	switch in.Operation {
	case "accounts.export":
		if pluginAccount(in) || !filepath.IsAbs(in.ExportPath) {
			return errors.New("select an output path for a supported account")
		}
		exported, err := provider.ExportLogins(in.Agent, in.ExportFormat, in.ExportUsers)
		if err != nil {
			return err
		}
		var paths []string
		switch in.ExportFormat {
		case provider.ExportSub2API:
			if len(exported.Files) != 1 {
				return errors.New("account export returned an invalid file set")
			}
			// The native Save dialog obtains the user's choice, including any
			// overwrite consent. No credential contents cross the Host response.
			if err := os.WriteFile(in.ExportPath, exported.Files[0].Data, 0600); err != nil {
				return err
			}
			paths = []string{in.ExportPath}
		case provider.ExportCLIProxy:
			base := filepath.Join(in.ExportPath, "WinOne-cliproxyapi-"+in.Agent+"-"+time.Now().Format("2006-01-02"))
			directory := base
			for suffix := 2; ; suffix++ {
				err = os.Mkdir(directory, 0700)
				if err == nil {
					break
				}
				if !errors.Is(err, os.ErrExist) {
					return err
				}
				directory = base + "-" + strconv.Itoa(suffix)
			}
			for _, file := range exported.Files {
				if filepath.Base(file.Name) != file.Name || file.Name == "." || file.Name == ".." {
					return errors.New("account export returned an invalid filename")
				}
				path := filepath.Join(directory, file.Name)
				if err := os.WriteFile(path, file.Data, 0600); err != nil {
					return err
				}
				paths = append(paths, path)
			}
		default:
			return errors.New("unsupported account export format")
		}
		for i := range exported.Skipped {
			if exported.Skipped[i].Error != "" {
				exported.Skipped[i].Error = "Account could not be exported"
			}
		}
		return encode(map[string]any{"files": paths, "accounts": exported.Accounts, "skipped": exported.Skipped})
	case "accounts.import":
		// The caller supplies only the files it explicitly selected. Reuse the
		// upstream parsers and token flows inside this provider's private profile.
		if len(in.ImportFiles) == 0 || len(in.ImportFiles) > 64 {
			return errors.New("select account files to import")
		}
		total := 0
		for _, text := range in.ImportFiles {
			total += len(text)
		}
		if total > 8<<20 {
			return errors.New("account import files are too large")
		}
		if pluginAccount(in) {
			connection, err := preparePlugin(ctx, in.Agent)
			if err != nil {
				return err
			}
			if !connection.Importable {
				return errors.New("this account adapter does not import files")
			}
			results, err := provider.ImportPluginFiles(ctx, provider.PluginID(in.Agent), in.ImportFiles)
			if err != nil {
				return err
			}
			for i := range results {
				if results[i].Error != "" {
					results[i].Error = "Account import failed"
				}
			}
			return encode(map[string]any{"results": results})
		}
		var results []provider.ImportedAccount
		var err error
		switch in.Agent {
		case "codex":
			if in.AccessOnly {
				results, err = provider.ImportCodexAccessOnly(ctx, in.ImportFiles)
			} else {
				results, err = provider.ImportLogins(ctx, in.Agent, in.ImportFiles)
			}
		case "claude":
			results, err = provider.ImportLogins(ctx, in.Agent, in.ImportFiles)
		case "factory":
			results, err = provider.ImportFactoryKeys(ctx, in.ImportFiles)
		case "gemini", "antigravity":
			results, err = provider.ImportGoogleAccounts(ctx, in.Agent, in.ImportFiles)
		default:
			return errors.New("this account type does not import files")
		}
		if err != nil {
			return err
		}
		for i := range results {
			if results[i].Error != "" {
				results[i].Error = "Account import failed"
			}
		}
		return encode(map[string]any{"results": results})
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
			return encode(map[string]any{"accounts": accounts, "methods": connection.Methods, "checkinAvailable": available, "importable": connection.Importable, "importFrom": connection.ImportFrom, "importFroms": connection.ImportFroms})
		}
		importable := in.Agent == "codex" || in.Agent == "claude" || in.Agent == "factory" || in.Agent == "gemini" || in.Agent == "antigravity"
		return encode(map[string]any{"accounts": provider.Logins(in.Agent), "importable": importable})
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
	if err := configureAccounting(in); err != nil {
		return err
	}
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
	p.KeyProtocol, p.KeyWeight = in.BalanceKeys[0].Protocol, in.BalanceKeys[0].Weight
	if in.BalanceKeys[0].Plan != "" {
		p.KeyPlan = in.BalanceKeys[0].Plan
	}
	p.Keys = in.BalanceKeys[1:]
	// Host preferences use stable saved slot IDs. Upstream accounts for the
	// actual credential fingerprint, shared with the request ledger.
	balances := p.KeyOwnBalances
	p.KeyOwnBalances = map[string]provider.OwnBalance{}
	rates := p.KeyPriceRates
	p.KeyPriceRates = map[string]float64{}
	for _, key := range in.BalanceKeys {
		if balance, ok := balances[key.Name]; ok {
			p.KeyOwnBalances[provider.KeyID(key.Key)] = balance
		}
		if rate, ok := rates[key.Name]; ok {
			p.KeyPriceRates[provider.KeyID(key.Key)] = rate
		}
	}
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
		state, err = provider.StartSignInWith(in.Agent, provider.SignInOptions{User: in.User})
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
