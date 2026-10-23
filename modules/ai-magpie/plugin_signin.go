package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/yetone/magpie/internal/plugin"
	"github.com/yetone/magpie/internal/provider"
)

func signInValue(ctx context.Context, commands <-chan signInControl) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case value, ok := <-commands:
		if !ok || value.Cancel {
			return "", context.Canceled
		}
		return value.Callback, nil
	}
}

// Plugin questions share the existing sign-in job and input pipe. They are
// transient authorization input, never part of public provider preferences.
func beginPluginSignIn(ctx context.Context, in request, commands <-chan signInControl) (provider.SignInState, bool, error) {
	fail := func(err error) (provider.SignInState, bool, error) { return provider.SignInState{}, false, err }
	connection, err := preparePlugin(ctx, in.Agent)
	if err != nil {
		return fail(err)
	}
	if len(connection.Methods) == 0 {
		return fail(errors.New("account plugin has no sign-in method"))
	}
	method := 0
	if in.Method != nil {
		method = *in.Method
	} else {
		for index, choice := range connection.Methods {
			if choice.Type == "oauth" {
				method = index
				break
			}
		}
	}
	if method < 0 || method >= len(connection.Methods) {
		return fail(errors.New("invalid sign-in method"))
	}
	emit := json.NewEncoder(os.Stdout).Encode
	inputs := map[string]string{}
	for {
		prompt, err := plugin.NextPrompt(ctx, in.Agent, method, inputs)
		if err != nil {
			return fail(errors.New("sign-in questions could not be loaded"))
		}
		if prompt == nil {
			break
		}
		if len(inputs) >= 32 {
			return fail(errors.New("too many sign-in questions"))
		}
		if err = emit(map[string]any{"signin": map[string]any{"agent": in.Agent, "state": "input", "prompt": prompt}}); err != nil {
			return fail(err)
		}
		value, err := signInValue(ctx, commands)
		if err != nil {
			return fail(err)
		}
		problem, err := plugin.Validate(ctx, in.Agent, method, prompt.Key, value)
		if err != nil {
			return fail(errors.New("sign-in input could not be checked"))
		}
		if problem != "" {
			if err = emit(map[string]string{"inputError": problem}); err != nil {
				return fail(err)
			}
			continue
		}
		inputs[prompt.Key] = value
	}
	choice := connection.Methods[method]
	if choice.Type == "api" {
		// Keyless methods obtain their own credential in authorize. Prompting
		// for a personal key would prevent these methods from completing.
		key := ""
		if !choice.Keyless {
			prompt := map[string]string{"type": "password", "key": "api-key", "message": choice.KeyTitle(connection.Name), "placeholder": choice.Placeholder}
			if err = emit(map[string]any{"signin": map[string]any{"agent": in.Agent, "state": "input", "prompt": prompt}}); err != nil {
				return fail(err)
			}
			key, err = signInValue(ctx, commands)
			if err != nil {
				return fail(err)
			}
		}
		if _, err = provider.PluginAPIKey(ctx, in.Agent, method, inputs, key); err != nil {
			return fail(errors.New("account authorization failed; check the key and try again"))
		}
		return provider.SignInState{Agent: in.Agent, State: "done"}, true, nil
	}
	if choice.Type != "oauth" {
		return fail(errors.New("unsupported sign-in method"))
	}
	state, err := provider.WinOneStartPluginSignIn(ctx, in.Agent, method, inputs)
	if err != nil {
		return fail(pluginSignInStartError(err))
	}
	return state, false, nil
}

// These exact messages are emitted by our no-implicit-install adapter. Keep
// the next action visible without echoing arbitrary plugin/network payloads.
func pluginSignInStartError(err error) error {
	for _, client := range []string{"Grok Build", "Cursor CLI", "Devin CLI"} {
		if err.Error() == client+" is required; install the official client first" {
			return err
		}
	}
	return errors.New("account sign-in could not start; check the connection and try again")
}
