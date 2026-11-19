package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/redact"
)

// Plan sessions belong to the same private provider home as its key balance.
// All vendor detection, login, validation and persistence remain upstream's.
type planSession struct {
	Kind        string `json:"kind,omitempty"`
	SignedIn    bool   `json:"signedIn"`
	Account     string `json:"account,omitempty"`
	Ended       bool   `json:"ended,omitempty"`
	URL         string `json:"url,omitempty"`
	Bookmarklet string `json:"bookmarklet,omitempty"`
}

func planSessionOf(p provider.Provider) planSession {
	switch {
	case provider.QianwenPlan(p):
		return planSession{Kind: "qianwen", SignedIn: provider.QianwenSignedIn(p.ID), Account: provider.QianwenAccount(p.ID)}
	case provider.QianfanPlan(p):
		return planSession{Kind: "qianfan", SignedIn: provider.QianfanSignedIn(p.ID), Ended: provider.QianfanEnded(p.ID), URL: provider.QianfanSignInURL, Bookmarklet: provider.QianfanBookmarklet()}
	case provider.StepFunSite(p) != "":
		return planSession{Kind: "stepfun", SignedIn: provider.StepFunSignedIn(p.ID), URL: provider.StepFunSignInURL(provider.StepFunSite(p)), Bookmarklet: provider.StepFunBookmarklet()}
	}
	return planSession{}
}

func planOperation(ctx context.Context, in request, input io.Reader, out io.Writer) error {
	p, err := configuredProvider(in)
	if err != nil {
		return err
	}
	state := planSessionOf(p)
	encode := json.NewEncoder(out).Encode
	if in.Operation == "plans.get" {
		return encode(state)
	}
	if state.Kind == "" {
		return errors.New("this provider has no platform sign-in")
	}
	if in.Operation == "plans.signout" {
		switch state.Kind {
		case "qianwen":
			err = provider.SignOutQianwen(p.ID)
		case "qianfan":
			err = provider.SignOutQianfan(p.ID)
		case "stepfun":
			err = provider.SignOutStepFun(p.ID)
		}
		if err != nil {
			return err
		}
		return encode(planSessionOf(p))
	}
	if in.Operation != "plans.signin" {
		return errors.New("unsupported plan operation")
	}
	controls := readSignInControls(ctx, input)
	progress := func(value map[string]any) error {
		value["agent"] = state.Kind
		return encode(map[string]any{"signin": value})
	}
	if state.Kind == "qianwen" {
		address, err := provider.StartQianwenSignIn(p.Via(ctx), p.ID)
		if err != nil {
			return err
		}
		defer provider.CancelQianwenSignIn(p.ID)
		if err = progress(map[string]any{"state": "waiting", "url": address}); err != nil {
			return err
		}
		tick := time.NewTicker(200 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case control, ok := <-controls:
				if !ok || control.Cancel {
					return context.Canceled
				}
			case <-tick.C:
				next, why := provider.QianwenSignInState(p.ID)
				if next == "done" {
					return progress(map[string]any{"state": "done"})
				}
				if next == "failed" {
					return progress(map[string]any{"state": "failed", "error": redact.Scrub(why)})
				}
			}
		}
	}
	// The browser supplies the session; neither the UI configuration nor the
	// Host's response ever contains the saved cookie or access/refresh token.
	if err = progress(map[string]any{"state": "input", "url": state.URL, "prompt": map[string]string{"type": "password", "key": "plan-session", "message": "粘贴平台登录信息 / Paste platform session"}}); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case control, ok := <-controls:
			if !ok || control.Cancel {
				return context.Canceled
			}
			call, cancel := context.WithTimeout(p.Via(ctx), 20*time.Second)
			if state.Kind == "qianfan" {
				err = provider.SaveQianfanPaste(call, p.ID, control.Callback)
			} else {
				err = provider.SaveStepFunPaste(call, p.ID, provider.StepFunSite(p), control.Callback)
			}
			cancel()
			if err != nil {
				if err = encode(map[string]string{"inputError": redact.Scrub(err.Error())}); err != nil {
					return err
				}
				continue
			}
			return progress(map[string]any{"state": "done"})
		}
	}
}
